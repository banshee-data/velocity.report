package docsite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	radar "github.com/banshee-data/velocity.report"
)

const (
	SourceEmbed = "embed"
	SourceDisk  = "disk"

	DefaultDiskDir = "docs_html/_site"
)

func ValidateSource(source string) error {
	switch source {
	case SourceEmbed, SourceDisk:
		return nil
	default:
		return fmt.Errorf("invalid docs source %q: valid values are embed or disk", source)
	}
}

func Handler(source, diskDir string) (http.Handler, error) {
	if err := ValidateSource(source); err != nil {
		return nil, err
	}
	if source == SourceDisk {
		return DiskHandler(diskDir)
	}
	return EmbeddedHandler()
}

func EmbeddedHandler() (http.Handler, error) {
	siteFS, err := fs.Sub(radar.DocsSiteFiles, "docs_html/_site")
	if err != nil {
		return nil, fmt.Errorf("open embedded docs site: %w", err)
	}
	if _, err := fs.Stat(siteFS, "index.html"); err == nil {
		return http.FileServer(http.FS(siteFS)), nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read embedded docs index: %w", err)
	}

	if len(radar.DocsSiteStub) == 0 {
		return nil, fmt.Errorf("embedded docs site is missing index.html and stub page is empty")
	}
	log.Printf("Embedded docs site is not built; serving stub page")
	return embeddedStubHandler(radar.DocsSiteStub), nil
}

func DiskHandler(diskDir string) (http.Handler, error) {
	if diskDir == "" {
		diskDir = DefaultDiskDir
	}
	absDir, err := filepath.Abs(diskDir)
	if err != nil {
		return nil, fmt.Errorf("resolve docs disk dir %q: %w", diskDir, err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("open docs disk dir %q: %w", absDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("docs disk path %q is not a directory", absDir)
	}
	if _, err := os.Stat(filepath.Join(absDir, "index.html")); err != nil {
		return nil, fmt.Errorf("docs disk dir %q missing index.html: %w", absDir, err)
	}
	return http.FileServer(http.Dir(absDir)), nil
}

// Start binds a TCP listener at `listen` and serves the offline docs until
// `ctx` is cancelled. Returned errors include any bind failure as well as
// server runtime errors. For tests that need to know the bound port without
// racing on it, use Run with a pre-bound listener instead.
func Start(ctx context.Context, listen, source, diskDir string) error {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listen, err)
	}
	return Run(ctx, listener, source, diskDir)
}

// Run serves the offline docs on the provided listener until `ctx` is
// cancelled. It takes ownership of the listener: on shutdown the listener is
// closed by the underlying http.Server. This is the test-friendly entry
// point — callers that have already bound a port avoid the close-then-rebind
// race that affected an earlier version of TestStartShutdown.
func Run(ctx context.Context, listener net.Listener, source, diskDir string) error {
	handler, err := Handler(source, diskDir)
	if err != nil {
		_ = listener.Close()
		return err
	}

	server := &http.Server{Handler: handler}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Offline docs server listening on %s (source=%s)", listener.Addr(), source)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown docs server: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func embeddedStubHandler(stub []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(stub))
	})
}
