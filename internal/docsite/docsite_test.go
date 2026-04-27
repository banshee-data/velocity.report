package docsite

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestStartShutdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("offline docs"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Start(ctx, addr, SourceDisk, dir)
	}()

	client := http.Client{Timeout: time.Second}
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("docs server did not start: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("docs server did not shut down")
	}
}
