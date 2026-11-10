package docsite

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// SiteDirEnv names a built offline docs site (docs_html/_site). When it is
// set, TestEmbeddedDocsRoutes serves every generated page through the
// embedded handler the server mounts at /docs/ and checks each page's
// internal links. Unset, the test skips, so `make test-go` does not need a
// docs build. `make test-docs-offline-routes` sets it after
// `make build-docs-offline`.
const SiteDirEnv = "VELOCITY_DOCS_SITE_DIR"

// renderedPageMarker and shellMarker follow
// scripts/verify-embedded-docs-server.py, which applies the same rules to a
// release binary. Keep the two in step.
var (
	renderedPageMarker = []byte(`name="velocity-docs-page" content="rendered"`)
	shellMarker        = []byte(`class="shell"`)
)

// routeCheckHost stands in for the server's address. Links resolving to any
// other host are external and not checked.
const routeCheckHost = "docs-route-check.invalid"

type anchor struct {
	href       string
	appSurface bool
}

type routeReport struct {
	pages    int
	links    int
	failures []string
}

// renderedPages returns every generated HTML page under siteRoot, sorted.
// Pass-through HTML files (no rendered marker) are not docs pages.
func renderedPages(siteRoot string) ([]string, error) {
	var pages []string
	err := filepath.WalkDir(siteRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(body, renderedPageMarker) {
			pages = append(pages, path)
		}
		return nil
	})
	sort.Strings(pages)
	return pages, err
}

// pageURLPath maps a generated file to the URL the server serves it at:
// index.html files at their directory, everything else at its own path.
func pageURLPath(siteRoot, page string) (string, error) {
	rel, err := filepath.Rel(siteRoot, page)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "index.html":
		return DefaultMount, nil
	case strings.HasSuffix(rel, "/index.html"):
		return DefaultMount + strings.TrimSuffix(rel, "index.html"), nil
	default:
		return DefaultMount + rel, nil
	}
}

// anchorHrefs returns the href of every <a> in body, with whether the anchor
// is marked data-docs-app-surface (a deliberate link out of /docs/ to an
// application route the docs handler does not serve).
func anchorHrefs(body []byte) []anchor {
	var out []anchor
	tokens := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokens.Token()
			if token.Data != "a" {
				continue
			}
			var href string
			appSurface := false
			for _, attr := range token.Attr {
				switch attr.Key {
				case "href":
					href = attr.Val
				case "data-docs-app-surface":
					appSurface = true
				}
			}
			if href != "" {
				out = append(out, anchor{href: href, appSurface: appSurface})
			}
		}
	}
}

// get issues one request without following redirects. A redirect is a
// failure: a docs link should name the URL that serves the page.
func get(handler http.Handler, target *url.URL) (int, []byte) {
	req := httptest.NewRequest(http.MethodGet, "http://"+routeCheckHost+"/", nil)
	req.URL = &url.URL{Path: target.Path, RawPath: target.RawPath, RawQuery: target.RawQuery}
	req.RequestURI = target.RequestURI()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// verifyDocsRoutes serves each rendered page under siteRoot through handler
// (a mux with the docs mounted at DefaultMount) and checks it returns 200
// with the docs shell. It then checks each distinct internal link on those
// pages returns 200, skipping external links, links outside /docs/, and
// links marked data-docs-app-surface.
func verifyDocsRoutes(siteRoot string, handler http.Handler) (routeReport, error) {
	var report routeReport
	pages, err := renderedPages(siteRoot)
	if err != nil {
		return report, err
	}
	if len(pages) == 0 {
		return report, fmt.Errorf("offline docs site has no rendered HTML pages: %s", siteRoot)
	}
	report.pages = len(pages)

	checked := map[string]bool{}
	for _, page := range pages {
		rel, _ := filepath.Rel(siteRoot, page)
		urlPath, err := pageURLPath(siteRoot, page)
		if err != nil {
			return report, err
		}
		pageURL := &url.URL{Scheme: "http", Host: routeCheckHost, Path: urlPath}
		status, body := get(handler, pageURL)
		if status != http.StatusOK || !bytes.Contains(body, shellMarker) {
			report.failures = append(report.failures,
				fmt.Sprintf("%s: GET %s status=%d, shell present=%t", rel, urlPath, status, bytes.Contains(body, shellMarker)))
			continue
		}
		for _, link := range anchorHrefs(body) {
			if link.appSurface {
				continue
			}
			ref, err := url.Parse(link.href)
			if err != nil {
				report.failures = append(report.failures, fmt.Sprintf("%s: unparseable docs link %q: %v", rel, link.href, err))
				continue
			}
			target := pageURL.ResolveReference(ref)
			if target.Scheme != "http" && target.Scheme != "https" {
				continue
			}
			if target.Host != routeCheckHost || !strings.HasPrefix(target.Path, DefaultMount) {
				continue
			}
			target.Fragment = ""
			target.RawFragment = ""
			key := target.String()
			if checked[key] {
				continue
			}
			checked[key] = true
			if status, _ := get(handler, target); status != http.StatusOK {
				report.failures = append(report.failures,
					fmt.Sprintf("%s: broken docs link %q (GET %s status=%d)", rel, link.href, target.RequestURI(), status))
			}
		}
	}
	report.links = len(checked)
	return report, nil
}

func mountedDocs(t *testing.T, handler http.Handler) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if err := Mount(mux, DefaultMount, handler); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return mux
}

// TestEmbeddedDocsRoutes is the PR-time stand-in for the release binary's
// route check (scripts/verify-embedded-docs-server.py, run by
// static-build-ci on main). It uses the handler `velocity serve` mounts by
// default, so the pages come from this test binary's go:embed of
// docs_html/_site: compiling it also proves the tree is embeddable.
func TestEmbeddedDocsRoutes(t *testing.T) {
	siteRoot := os.Getenv(SiteDirEnv)
	if siteRoot == "" {
		t.Skipf("%s is unset; run make test-docs-offline-routes after make build-docs-offline", SiteDirEnv)
	}
	siteRoot, err := filepath.Abs(siteRoot)
	if err != nil {
		t.Fatalf("resolve %s: %v", SiteDirEnv, err)
	}

	handler, err := EmbeddedHandler()
	if err != nil {
		t.Fatalf("EmbeddedHandler: %v", err)
	}
	mux := mountedDocs(t, handler)

	// The embed is compiled from the checkout's docs_html/_site. If the env
	// var names a different or newer build, the check would read one tree
	// and serve another. Compare the index bytes so that mismatch fails
	// loudly rather than passing against the stub.
	diskIndex, err := os.ReadFile(filepath.Join(siteRoot, "index.html"))
	if err != nil {
		t.Fatalf("read %s/index.html: %v", siteRoot, err)
	}
	if status, served := get(mux, &url.URL{Path: DefaultMount}); status != http.StatusOK || !bytes.Equal(served, diskIndex) {
		t.Fatalf("embedded /docs/ (status %d) differs from %s/index.html: the test binary embeds a different docs build (stub or stale); rebuild with make build-docs-offline", status, siteRoot)
	}

	report, err := verifyDocsRoutes(siteRoot, mux)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.failures) > 0 {
		t.Fatalf("embedded docs verification failed: %d failure(s) across %d pages and %d internal links:\n  - %s",
			len(report.failures), report.pages, report.links, strings.Join(report.failures, "\n  - "))
	}
	t.Logf("embedded docs verified: %d pages, %d internal links", report.pages, report.links)
}

// TestVerifyDocsRoutesRules exercises the checker itself on a small site, so
// its rules are tested on every `make test-go` run, not only with a build.
func TestVerifyDocsRoutesRules(t *testing.T) {
	const shell = `<meta name="velocity-docs-page" content="rendered"><div class="shell">`
	files := map[string]string{
		"index.html": shell +
			`<a href="guide/">ok dir</a>` +
			`<a href="guide/#part">same page, fragment</a>` +
			`<a href="/docs/asset.css?v=1">ok asset</a>` +
			`<a href="missing/">broken</a>` +
			`<a href="guide">redirects to guide/</a>` +
			`<a href="guide/index.html">redirects to ./</a>` +
			`<a href="/app/settings" >outside docs</a>` +
			`<a href="/public_html/" data-docs-app-surface>app surface</a>` +
			`<a href="https://example.com/docs/x">external</a>` +
			`<a href="mailto:a@example.com">mail</a>` +
			`</div>`,
		"guide/index.html": shell + `<a href="../">up</a><a href="../missing/">broken again</a></div>`,
		"asset.css":        "body{}",
		// Rendered marker but no shell: the page itself fails.
		"bare/index.html": `<meta name="velocity-docs-page" content="rendered">no shell`,
		// No rendered marker: not a docs page, links not followed.
		"passthrough.html": `<a href="nowhere/">ignored</a>`,
	}
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := DiskHandler(root)
	if err != nil {
		t.Fatal(err)
	}

	report, err := verifyDocsRoutes(root, mountedDocs(t, handler))
	if err != nil {
		t.Fatal(err)
	}
	if report.pages != 3 {
		t.Errorf("pages = %d, want 3 (index, guide, bare)", report.pages)
	}
	// /docs/ and /docs/missing/ from guide/; /docs/guide/ (twice, once with
	// a fragment), /docs/asset.css?v=1, /docs/missing/ again, /docs/guide
	// and /docs/guide/index.html from the index: six distinct targets.
	if report.links != 6 {
		t.Errorf("links = %d, want 6", report.links)
	}
	got := strings.Join(report.failures, "\n")
	for _, want := range []string{
		`bare/index.html: GET /docs/bare/ status=200, shell present=false`,
		// Pages are walked in path order, so guide/ reports the shared
		// broken target first and index.html's "missing/" is deduplicated.
		`guide/index.html: broken docs link "../missing/" (GET /docs/missing/ status=404)`,
		`index.html: broken docs link "guide" (GET /docs/guide status=301)`,
		`index.html: broken docs link "guide/index.html" (GET /docs/guide/index.html status=301)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("failures missing %q; got:\n%s", want, got)
		}
	}
	if len(report.failures) != 4 {
		t.Errorf("failures = %d, want 4 (missing/ shares a target with ../missing/):\n%s", len(report.failures), got)
	}
	for _, unwanted := range []string{"public_html", "example.com", "mailto", "app/settings", "nowhere"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("failures should not mention %q:\n%s", unwanted, got)
		}
	}

	empty := t.TempDir()
	if _, err := verifyDocsRoutes(empty, http.NotFoundHandler()); err == nil {
		t.Error("expected a site with no rendered pages to fail")
	}
}

func TestPageURLPath(t *testing.T) {
	root := filepath.FromSlash("/site")
	for page, want := range map[string]string{
		"/site/index.html":      "/docs/",
		"/site/docs/index.html": "/docs/docs/",
		"/site/asset.html":      "/docs/asset.html",
	} {
		got, err := pageURLPath(root, filepath.FromSlash(page))
		if err != nil || got != want {
			t.Errorf("pageURLPath(%s) = %q, %v; want %q", page, got, err, want)
		}
	}
}
