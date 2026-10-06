package docsite

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSite creates a minimal on-disk docs site and returns its directory.
func writeSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>docs</h1>"), 0o644); err != nil {
		t.Fatalf("writing index.html: %v", err)
	}
	return dir
}

func TestHandlerDispatchesOnSource(t *testing.T) {
	dir := writeSite(t)

	t.Run("disk source uses the disk handler", func(t *testing.T) {
		h, err := Handler(SourceDisk, dir)
		if err != nil {
			t.Fatalf("Handler(disk): %v", err)
		}
		// http.FileServer redirects /index.html to /, so ask for the
		// directory root and let it serve the index.
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "<h1>docs</h1>") {
			t.Errorf("body = %q, want the on-disk index", rec.Body)
		}
	})

	t.Run("embed source uses the embedded handler", func(t *testing.T) {
		h, err := Handler(SourceEmbed, dir)
		if err != nil {
			t.Fatalf("Handler(embed): %v", err)
		}
		// The embedded site is a stub in test builds, but it must still
		// answer the index rather than falling through to the disk dir.
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("disk source with a bad dir reports the error", func(t *testing.T) {
		if _, err := Handler(SourceDisk, filepath.Join(dir, "nope")); err == nil {
			t.Fatal("Handler(disk, missing) succeeded, want an error")
		}
	})
}

func TestMountRejectsBadArguments(t *testing.T) {
	handler := http.NotFoundHandler()

	tests := []struct {
		name    string
		mux     *http.ServeMux
		mount   string
		handler http.Handler
		wantErr string
	}{
		{"nil mux", nil, DefaultMount, handler, "docs mux is nil"},
		{"nil handler", http.NewServeMux(), DefaultMount, nil, "docs handler is nil"},
		// Mounting at root would swallow every route on the shared mux.
		{"root mount", http.NewServeMux(), "/", handler, "must not be root"},
		{"empty mount", http.NewServeMux(), "", handler, "must not be root"},
		{"slashes only", http.NewServeMux(), "///", handler, "must not be root"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Mount(tc.mux, tc.mount, tc.handler)
			if err == nil {
				t.Fatalf("Mount(%q) succeeded, want an error", tc.mount)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestMountNormalisesMountPath(t *testing.T) {
	// A path given without surrounding slashes must mount at the same place
	// as the canonical "/docs/" form.
	mux := http.NewServeMux()
	if err := Mount(mux, "docs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("mounted:" + r.URL.Path))
	})); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/guide.html", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// StripPrefix removes "/docs", so the handler sees the sub-path.
	if got := rec.Body.String(); got != "mounted:/guide.html" {
		t.Errorf("body = %q, want %q", got, "mounted:/guide.html")
	}
}

func TestMountRedirectPreservesQueryString(t *testing.T) {
	mux := http.NewServeMux()
	if err := Mount(mux, DefaultMount, http.NotFoundHandler()); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs?q=speed&page=2", nil))

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	// Dropping the query on the redirect would lose a docs search.
	if loc := rec.Header().Get("Location"); loc != "/docs/?q=speed&page=2" {
		t.Errorf("Location = %q, want %q", loc, "/docs/?q=speed&page=2")
	}
}

func TestMountRedirectWithoutQueryString(t *testing.T) {
	mux := http.NewServeMux()
	if err := Mount(mux, DefaultMount, http.NotFoundHandler()); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/docs/" {
		t.Errorf("Location = %q, want %q", loc, "/docs/")
	}
}

func TestDiskHandlerRejectsNonDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	_, err := DiskHandler(file)
	if err == nil {
		t.Fatal("DiskHandler on a regular file succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("error = %v, want it to report a non-directory", err)
	}
}

func TestDiskHandlerRejectsDirWithoutIndex(t *testing.T) {
	// An empty directory means the Eleventy build has not run; serving it
	// would give visitors a directory listing instead of the docs.
	_, err := DiskHandler(t.TempDir())
	if err == nil {
		t.Fatal("DiskHandler on a dir without index.html succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "missing index.html") {
		t.Errorf("error = %v, want it to report the missing index", err)
	}
}

func TestDiskHandlerDefaultsDirWhenEmpty(t *testing.T) {
	// An empty diskDir falls back to DefaultDiskDir, resolved relative to the
	// working directory. From a temp dir that path does not exist, so the
	// error must name the default rather than the empty string.
	t.Chdir(t.TempDir())

	_, err := DiskHandler("")
	if err == nil {
		t.Fatal("DiskHandler(\"\") succeeded, want an error for the missing default dir")
	}
	if !strings.Contains(err.Error(), filepath.FromSlash(DefaultDiskDir)) {
		t.Errorf("error = %v, want it to name %q", err, DefaultDiskDir)
	}
}

func TestEmbeddedStubHandlerServesIndexOnly(t *testing.T) {
	stub := []byte("<html>stub</html>")
	h := embeddedStubHandler(stub)

	t.Run("root serves the stub", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Body.String(); got != string(stub) {
			t.Errorf("body = %q, want %q", got, stub)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
	})

	t.Run("index.html serves the stub", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/index.html", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("any other path is 404", func(t *testing.T) {
		// The stub is a single page: deep links must not all resolve to it,
		// or a missing docs build looks like a working site.
		for _, path := range []string{"/guide.html", "/assets/style.css", "/sub/dir/"} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s status = %d, want 404", path, rec.Code)
			}
		}
	})
}
