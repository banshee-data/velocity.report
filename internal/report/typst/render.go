// Package typst renders velocity reports through the Typst typesetter via the
// github.com/Dadido3/go-typst wrapper.
//
// The package embeds the .typ templates, materialises them along with the
// caller's data and chart SVGs into a temporary working directory, and shells
// out to `typst compile`. The Atkinson Hyperlegible fonts are materialised
// from the chart asset package so generation works from a deployed binary
// with no source tree present. The typst executable itself is resolved via the
// typstbin subpackage (embedded binary → PATH → dev download).
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
	"sort"
	"strings"
	"time"

	gotypst "github.com/Dadido3/go-typst"
	"github.com/banshee-data/velocity.report/internal/report/chart/assets"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

//go:embed templates
var templatesFS embed.FS

type templateSourceFS interface {
	fs.FS
	fs.ReadFileFS
}

var (
	renderTemplatesFS  = templateSourceFS(templatesFS)
	renderResolveTypst = typstbin.Resolve
	renderMkdirTemp    = os.MkdirTemp
	renderRemoveAll    = os.RemoveAll
	renderMkdirAll     = os.MkdirAll
	renderWriteFile    = os.WriteFile
	renderAllFonts     = assets.AllFonts
)

// EntryReport is the radar speed report's entry template, and the default.
const EntryReport = "report.typ"

// templateSets names, for each entry template a document may compile from,
// every embedded template that entry needs, itself included. A render
// materialises only its entry's set, and a source archive ships only that
// set, so the archive recompiles exactly what was rendered: an import missing
// from a set fails the render rather than surfacing later as a source ZIP
// that no longer compiles. The sets are stated rather than derived from the
// #import lines so a reviewer sees them; a test holds them to those lines.
var templateSets = map[string][]string{
	EntryReport: {"report.typ", "preamble.typ", "sections.typ"},
}

// templateSet returns the files an entry needs, or an error for an entry
// that is not registered.
func templateSet(entry string) ([]string, error) {
	files, ok := templateSets[entry]
	if !ok {
		return nil, fmt.Errorf("unknown template entry %q", entry)
	}
	return files, nil
}

// Asset is a binary blob (chart SVG, map SVG, etc.) that the report embeds
// via #image(). The Name is used as the relative path inside the working
// directory; the template references it as e.g. `data.charts.timeseries`.
type Asset struct {
	Name string
	Data []byte
}

// Options controls a single Render call.
type Options struct {
	// Entry is the template the document compiles from, one of the
	// registered entries; empty means EntryReport.
	Entry string

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

	// PDFMetadata overrides selected PDF Info/XMP fields after Typst compiles
	// the document. Typst 0.13.x exposes document metadata, but not the PDF
	// creator tool, so we patch the finished PDF incrementally.
	PDFMetadata PDFMetadata
}

// Render compiles the embedded templates against opts.Data and writes the
// resulting PDF to out. The working directory used for compilation is removed
// before Render returns.
func Render(out io.Writer, opts Options) error {
	entry := opts.Entry
	if entry == "" {
		entry = EntryReport
	}
	if _, err := templateSet(entry); err != nil {
		return err
	}
	workDir, err := renderMkdirTemp("", "velocity-report-typst-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer renderRemoveAll(workDir)

	if err := materialiseTemplates(workDir, entry); err != nil {
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
		// Enforce the documented relative-path contract: an asset name must stay
		// within workDir. filepath.Join cleans the path, so an absolute name or
		// `..` traversal would resolve outside workDir — reject it so misuse
		// fails fast rather than writing to an arbitrary location.
		if dest != workDir && !strings.HasPrefix(dest, workDir+string(os.PathSeparator)) {
			return fmt.Errorf("asset name %q escapes work dir", asset.Name)
		}
		if err := renderMkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
		}
		if err := renderWriteFile(dest, asset.Data, 0o644); err != nil {
			return fmt.Errorf("write asset %s: %w", asset.Name, err)
		}
	}

	execPath, cleanup, rerr := renderResolveTypst()
	if rerr != nil {
		return fmt.Errorf("resolve typst binary: %w", rerr)
	}
	defer cleanup()
	caller := gotypst.CLI{ExecutablePath: execPath}

	// Bootstrap: typst reads the document from stdin, which has no implicit
	// path, so relative imports inside the entry file fail. We address this
	// by feeding a one-line bootstrap on stdin that includes the real entry
	// via an absolute path rooted at workDir (so `/report.typ` resolves to
	// workDir/report.typ). The entry is a registered set key, never caller
	// text, so it cannot inject Typst.
	bootstrap := []byte(`#include "/` + entry + `"`)

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

	compileOut := out
	var compiled bytes.Buffer
	if !opts.PDFMetadata.empty() {
		compileOut = &compiled
	}

	if err := caller.Compile(bytes.NewReader(bootstrap), compileOut, compileOpts); err != nil {
		return fmt.Errorf("typst compile: %w", err)
	}
	if opts.PDFMetadata.empty() {
		return nil
	}

	updatedPDF, err := applyPDFMetadata(compiled.Bytes(), opts.PDFMetadata)
	if err != nil {
		return fmt.Errorf("apply pdf metadata: %w", err)
	}
	if _, err := out.Write(updatedPDF); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}

// Sources returns the radar report's .typ template files keyed by their base
// name (report.typ, preamble.typ, sections.typ). It is used to assemble the
// recompilable source ZIP that ships alongside each generated PDF.
func Sources() (map[string][]byte, error) {
	return SourcesFor(EntryReport)
}

// SourcesFor returns the template files one entry compiles from, keyed by
// base name: exactly the set Render materialises for it.
func SourcesFor(entry string) (map[string][]byte, error) {
	files, err := templateSet(entry)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(files))
	for _, name := range files {
		body, rerr := renderTemplatesFS.ReadFile("templates/" + name)
		if rerr != nil {
			return nil, fmt.Errorf("read templates: %w", rerr)
		}
		out[name] = body
	}
	return out, nil
}

// MarshalData renders opts.Data the same way Render writes data.json, so the
// source ZIP and the compiled document agree byte-for-byte.
func MarshalData(data any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// materialiseTemplates copies one entry's template set into workDir at the
// top level (so report.typ ends up at workDir/report.typ).
func materialiseTemplates(workDir, entry string) error {
	sources, err := SourcesFor(entry)
	if err != nil {
		return err
	}
	if err := renderMkdirAll(workDir, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := renderWriteFile(filepath.Join(workDir, name), sources[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// materialiseFonts writes the embedded Atkinson Hyperlegible fonts into
// workDir/fonts and returns that directory for use as a typst --font-path.
func materialiseFonts(workDir string) (string, error) {
	fontDir := filepath.Join(workDir, "fonts")
	if err := renderMkdirAll(fontDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir fonts: %w", err)
	}
	for name, data := range renderAllFonts() {
		if err := renderWriteFile(filepath.Join(fontDir, name), data, 0o644); err != nil {
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
	return renderWriteFile(filepath.Join(workDir, "data.json"), body, 0o644)
}
