// Package typst renders velocity reports through the Typst typesetter via the
// github.com/Dadido3/go-typst wrapper.
//
// The package embeds the .typ templates, materialises them along with the
// caller's data and chart SVGs into a temporary working directory, and shells
// out to `typst compile`. The Atkinson Hyperlegible fonts are materialised
// from the chart asset package so generation works from a deployed binary
// with no source tree present. The typst executable itself is resolved via the
// typstbin subpackage (embedded binary → env override → PATH).
package typst

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	gotypst "github.com/Dadido3/go-typst"
	"github.com/banshee-data/velocity.report/internal/report/chart/assets"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

//go:embed templates
var templatesFS embed.FS

// Asset is a binary blob (chart SVG, map SVG, etc.) that the report embeds
// via #image(). The Name is used as the relative path inside the working
// directory; the template references it as e.g. `data.charts.timeseries`.
type Asset struct {
	Name string
	Data []byte
}

// Options controls a single Render call.
type Options struct {
	// Data is the structured payload exposed to the template as `data` after
	// being marshalled to data.json in the working directory. Normally a
	// ReportData value.
	Data any

	// Assets are extra files (SVG charts, etc.) to materialise alongside the
	// templates so that #image() calls resolve. The file path inside the
	// working directory is Asset.Name.
	Assets []Asset

	// FontDir is an additional directory of .ttf/.otf files passed to typst via
	// --font-path. The embedded Atkinson Hyperlegible fonts are always made
	// available regardless of this value; FontDir is only needed for extra
	// faces during development.
	FontDir string

	// IgnoreSystemFonts, when true, instructs typst to ignore the host's
	// system fonts and use only the embedded fonts (+ FontDir). Recommended
	// for reproducible builds.
	IgnoreSystemFonts bool

	// CreationTime, when non-zero, is passed to typst as --creation-timestamp
	// for reproducible PDF metadata.
	CreationTime time.Time

	// TypstPath overrides the typst executable. When empty, the binary is
	// resolved via typstbin (embedded → VELOCITY_TYPST_PATH → PATH).
	TypstPath string
}

// Render compiles the embedded templates against opts.Data and writes the
// resulting PDF to out. The working directory used for compilation is removed
// before Render returns.
func Render(out io.Writer, opts Options) error {
	workDir, err := os.MkdirTemp("", "velocity-report-typst-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	if err := materialiseTemplates(workDir); err != nil {
		return err
	}
	if err := writeData(workDir, opts.Data); err != nil {
		return err
	}
	fontDir, err := materialiseFonts(workDir)
	if err != nil {
		return err
	}
	for _, asset := range opts.Assets {
		dest := filepath.Join(workDir, asset.Name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, asset.Data, 0o644); err != nil {
			return fmt.Errorf("write asset %s: %w", asset.Name, err)
		}
	}

	execPath := opts.TypstPath
	if execPath == "" {
		resolved, cleanup, rerr := typstbin.Resolve()
		if rerr != nil {
			return fmt.Errorf("resolve typst binary: %w", rerr)
		}
		defer cleanup()
		execPath = resolved
	}
	caller := gotypst.CLI{ExecutablePath: execPath}

	// Bootstrap: typst reads the document from stdin, which has no implicit
	// path, so relative imports inside the entry file fail. We address this
	// by feeding a one-line bootstrap on stdin that includes the real entry
	// via an absolute path rooted at workDir (so `/report.typ` resolves to
	// workDir/report.typ).
	bootstrap := []byte(`#include "/report.typ"`)

	fontPaths := []string{fontDir}
	if opts.FontDir != "" {
		fontPaths = append(fontPaths, opts.FontDir)
	}
	compileOpts := &gotypst.OptionsCompile{
		Root:              workDir,
		Format:            gotypst.OutputFormatPDF,
		IgnoreSystemFonts: opts.IgnoreSystemFonts,
		FontPaths:         fontPaths,
	}
	if !opts.CreationTime.IsZero() {
		compileOpts.CreationTime = opts.CreationTime
	}

	if err := caller.Compile(bytes.NewReader(bootstrap), out, compileOpts); err != nil {
		return fmt.Errorf("typst compile: %w", err)
	}
	return nil
}

// Sources returns the embedded .typ template files keyed by their base name
// (report.typ, preamble.typ, sections.typ). It is used to assemble the
// recompilable source ZIP that ships alongside each generated PDF.
func Sources() (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, rerr := templatesFS.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[filepath.Base(path)] = body
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read templates: %w", err)
	}
	return out, nil
}

// MarshalData renders opts.Data the same way Render writes data.json, so the
// source ZIP and the compiled document agree byte-for-byte.
func MarshalData(data any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// materialiseTemplates copies the embedded templates/ tree into workDir at
// the top level (so report.typ ends up at workDir/report.typ).
func materialiseTemplates(workDir string) error {
	return fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("templates", path)
		if err != nil {
			return err
		}
		dest := filepath.Join(workDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		body, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, body, 0o644)
	})
}

// materialiseFonts writes the embedded Atkinson Hyperlegible fonts into
// workDir/fonts and returns that directory for use as a typst --font-path.
func materialiseFonts(workDir string) (string, error) {
	fontDir := filepath.Join(workDir, "fonts")
	if err := os.MkdirAll(fontDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir fonts: %w", err)
	}
	for name, data := range assets.AllFonts() {
		if err := os.WriteFile(filepath.Join(fontDir, name), data, 0o644); err != nil {
			return "", fmt.Errorf("write font %s: %w", name, err)
		}
	}
	return fontDir, nil
}

func writeData(workDir string, data any) error {
	body, err := MarshalData(data)
	if err != nil {
		return fmt.Errorf("marshal report data: %w", err)
	}
	return os.WriteFile(filepath.Join(workDir, "data.json"), body, 0o644)
}
