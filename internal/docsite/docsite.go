package docsite

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	radar "github.com/banshee-data/velocity.report"
)

const (
	SourceEmbed = "embed"
	SourceDisk  = "disk"

	DefaultDiskDir = "docs_html/_site"
	DefaultMount   = "/docs/"
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

func Mount(mux *http.ServeMux, mountPath string, handler http.Handler) error {
	if mux == nil {
		return errors.New("docs mux is nil")
	}
	if handler == nil {
		return errors.New("docs handler is nil")
	}

	trimmed := strings.Trim(mountPath, "/")
	if trimmed == "" {
		return fmt.Errorf("docs mount path %q must not be root", mountPath)
	}
	prefix := "/" + trimmed
	prefixWithSlash := prefix + "/"

	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		target := prefixWithSlash
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	mux.Handle(prefixWithSlash, http.StripPrefix(prefix, handler))
	return nil
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
