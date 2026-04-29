package docsite

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSource(t *testing.T) {
	for _, source := range []string{SourceEmbed, SourceDisk} {
		if err := ValidateSource(source); err != nil {
			t.Fatalf("ValidateSource(%q) returned error: %v", source, err)
		}
	}
	if err := ValidateSource("other"); err == nil {
		t.Fatal("expected invalid source to fail")
	}
}

func TestEmbeddedHandlerServesIndex(t *testing.T) {
	handler, err := EmbeddedHandler()
	if err != nil {
		t.Fatalf("EmbeddedHandler returned error: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
}

func TestDiskHandlerServesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("offline docs"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	handler, err := DiskHandler(dir)
	if err != nil {
		t.Fatalf("DiskHandler returned error: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
}

func TestDiskHandlerMissingSite(t *testing.T) {
	if _, err := DiskHandler(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing disk site to fail")
	}
}

func TestHandlerInvalidSource(t *testing.T) {
	if _, err := Handler("bad", ""); err == nil {
		t.Fatal("expected invalid source to fail")
	}
}

func TestMountServesDocsUnderPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("offline docs"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assetDir := filepath.Join(dir, "assets")
	if err := os.Mkdir(assetDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "site.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	handler, err := DiskHandler(dir)
	if err != nil {
		t.Fatalf("DiskHandler returned error: %v", err)
	}
	mux := http.NewServeMux()
	if err := Mount(mux, DefaultMount, handler); err != nil {
		t.Fatalf("Mount returned error: %v", err)
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/docs/", want: "offline docs"},
		{path: "/docs/assets/site.css", want: "body{}"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", tc.path, rec.Code)
		}
		body, err := io.ReadAll(rec.Body)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		if string(body) != tc.want {
			t.Fatalf("GET %s body = %q, want %q", tc.path, string(body), tc.want)
		}
	}
}

func TestMountRedirectsBarePrefix(t *testing.T) {
	mux := http.NewServeMux()
	if err := Mount(mux, "/docs/", http.NotFoundHandler()); err != nil {
		t.Fatalf("Mount returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/docs?from=settings", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /docs status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	if got, want := rec.Header().Get("Location"), "/docs/?from=settings"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}
