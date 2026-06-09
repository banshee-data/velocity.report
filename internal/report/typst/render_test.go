package typst

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type failingReadFS struct {
	fs.FS
}

func (f failingReadFS) ReadFile(string) ([]byte, error) {
	return nil, errors.New("read failed")
}

func restoreRenderDeps(t *testing.T) {
	t.Helper()
	oldTemplatesFS := renderTemplatesFS
	oldResolve := renderResolveTypst
	oldMkdirTemp := renderMkdirTemp
	oldRemoveAll := renderRemoveAll
	oldMkdirAll := renderMkdirAll
	oldWriteFile := renderWriteFile
	oldAllFonts := renderAllFonts
	t.Cleanup(func() {
		renderTemplatesFS = oldTemplatesFS
		renderResolveTypst = oldResolve
		renderMkdirTemp = oldMkdirTemp
		renderRemoveAll = oldRemoveAll
		renderMkdirAll = oldMkdirAll
		renderWriteFile = oldWriteFile
		renderAllFonts = oldAllFonts
	})
}

func mockTypstCLI(t *testing.T, pdf []byte, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "typst")
	body := "#!/bin/sh\n"
	if exitCode != 0 {
		body += fmt.Sprintf("exit %d\n", exitCode)
	} else {
		body += "printf '%s' '" + string(pdf) + "'\n"
	}
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write mock typst: %v", err)
	}
	return script
}

func withMockTypstOnPath(t *testing.T, pdf []byte, exitCode int) string {
	t.Helper()
	script := mockTypstCLI(t, pdf, exitCode)
	t.Setenv(typstbin.EnvNoDownload, "1")
	t.Setenv("PATH", filepath.Dir(script)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return script
}

func requireTypstForSmokeTest(t *testing.T) {
	t.Helper()
	t.Setenv(typstbin.EnvNoDownload, "1")
	if typstbin.Embedded() {
		return
	}
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not embedded or on PATH; run make install-typst and add bin/ to PATH")
	}
}

func testMetadataFixturePDF() []byte {
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="xmp-writer"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmlns:pdf="http://ns.adobe.com/pdf/1.3/"><xmp:CreatorTool>Typst 0.13.1</xmp:CreatorTool></rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="r"?>`
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
		fmt.Sprintf("4 0 obj\n<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(xmp), xmp),
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for index, obj := range objects {
		offsets[index+1] = pdf.Len()
		pdf.WriteString(obj)
	}
	xrefStart := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	fmt.Fprintf(&pdf, "%010d %05d f \n", 0, 65535)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&pdf, "%010d %05d n \n", offsets[index], 0)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R /Info 3 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefStart)
	return pdf.Bytes()
}

func TestApplyPDFMetadata(t *testing.T) {
	pdf, err := applyPDFMetadata(testMetadataFixturePDF(), PDFMetadata{
		Creator:  "velocity.report v1.2.3",
		Keywords: []string{"git-sha:abc123"},
	})
	if err != nil {
		t.Fatalf("applyPDFMetadata: %v", err)
	}

	for _, want := range []string{
		"/Creator (velocity.report v1.2.3)",
		"/Keywords (git-sha:abc123)",
		"<xmp:CreatorTool>velocity.report v1.2.3</xmp:CreatorTool>",
		"<pdf:Keywords>git-sha:abc123</pdf:Keywords>",
		"/Prev ",
	} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Fatalf("updated pdf missing %q", want)
		}
	}
}

func TestRenderWithMockTypstAndMetadata(t *testing.T) {
	withMockTypstOnPath(t, testMetadataFixturePDF(), 0)
	var out bytes.Buffer
	err := Render(&out, Options{
		Data:              map[string]any{"ok": true},
		Assets:            []Asset{{Name: "charts/test.svg", Data: []byte("<svg/>")}},
		FontDir:           t.TempDir(),
		IgnoreSystemFonts: true,
		CreationTime:      time.Unix(123, 0),
		PDFMetadata:       PDFMetadata{Creator: "velocity.report v1.2.3", Keywords: []string{"git-sha:abc123"}},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"/Creator (velocity.report v1.2.3)",
		"/Keywords (git-sha:abc123)",
		"<xmp:CreatorTool>velocity.report v1.2.3</xmp:CreatorTool>",
	} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("rendered PDF missing %q", want)
		}
	}
}

func TestRenderWithoutMetadataUsesRawTypstOutput(t *testing.T) {
	rawPDF := testMetadataFixturePDF()
	withMockTypstOnPath(t, rawPDF, 0)
	var out bytes.Buffer
	err := Render(&out, Options{
		Data: map[string]any{"ok": true},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(out.Bytes(), rawPDF) {
		t.Fatal("Render should pass through Typst output unchanged when no PDF metadata override is requested")
	}
}

func TestRenderRejectsEscapingAssetPathsAndPropagatesErrors(t *testing.T) {
	if err := Render(&bytes.Buffer{}, Options{
		Data:   map[string]any{"ok": true},
		Assets: []Asset{{Name: "../escape.svg", Data: []byte("<svg/>")}},
	}); err == nil || !bytes.Contains([]byte(err.Error()), []byte("escapes work dir")) {
		t.Fatalf("Render asset escape error = %v, want escapes work dir", err)
	}

	withMockTypstOnPath(t, nil, 2)
	err := Render(&bytes.Buffer{}, Options{
		Data: map[string]any{"ok": true},
	})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("typst compile")) {
		t.Fatalf("Render compile error = %v, want typst compile", err)
	}

	withMockTypstOnPath(t, testMetadataFixturePDF(), 0)
	err = Render(failingWriter{}, Options{
		Data:        map[string]any{"ok": true},
		PDFMetadata: PDFMetadata{Creator: "velocity.report v1.2.3"},
	})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("write pdf")) {
		t.Fatalf("Render write error = %v, want write pdf", err)
	}
}

func TestSourcesAndMarshalData(t *testing.T) {
	sources, err := Sources()
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	for _, want := range []string{"report.typ", "preamble.typ", "sections.typ"} {
		if _, ok := sources[want]; !ok {
			t.Fatalf("Sources missing %s", want)
		}
	}
	jsonBody, err := MarshalData(map[string]any{"creator": "velocity.report"})
	if err != nil {
		t.Fatalf("MarshalData: %v", err)
	}
	if !bytes.Contains(jsonBody, []byte(`"creator": "velocity.report"`)) {
		t.Fatalf("MarshalData = %s", jsonBody)
	}
}

func TestRenderHelperFilesystemErrors(t *testing.T) {
	templatesDir := t.TempDir()
	if err := materialiseTemplates(templatesDir); err != nil {
		t.Fatalf("materialiseTemplates: %v", err)
	}
	if _, err := os.Stat(filepath.Join(templatesDir, "report.typ")); err != nil {
		t.Fatalf("materialised report.typ missing: %v", err)
	}

	blockedTemplates := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedTemplates, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocked template path: %v", err)
	}
	if err := materialiseTemplates(blockedTemplates); err == nil {
		t.Fatal("materialiseTemplates should fail when the target root is a file")
	}

	fontsDir := t.TempDir()
	fontRoot, err := materialiseFonts(fontsDir)
	if err != nil {
		t.Fatalf("materialiseFonts: %v", err)
	}
	entries, err := os.ReadDir(fontRoot)
	if err != nil {
		t.Fatalf("read materialised fonts: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("materialiseFonts should write bundled fonts")
	}

	blockedFonts := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedFonts, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocked font path: %v", err)
	}
	if _, err := materialiseFonts(blockedFonts); err == nil {
		t.Fatal("materialiseFonts should fail when the target root is a file")
	}

	dataDir := t.TempDir()
	if err := writeData(dataDir, map[string]any{"ok": true}); err != nil {
		t.Fatalf("writeData: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(dataDir, "data.json")); err != nil || !bytes.Contains(body, []byte(`"ok": true`)) {
		t.Fatalf("writeData body = %q, err=%v", body, err)
	}

	if err := writeData(t.TempDir(), map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("writeData should fail on unmarshalable data")
	}

	blockedData := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedData, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocked data path: %v", err)
	}
	if err := writeData(blockedData, map[string]any{"ok": true}); err == nil {
		t.Fatal("writeData should fail when the target root is a file")
	}
}

func TestRenderResolveAndMetadataErrors(t *testing.T) {
	if typstbin.Embedded() {
		t.Skip("embedded typst build cannot exercise the missing-binary path")
	}
	t.Setenv(typstbin.EnvNoDownload, "1")
	t.Setenv("PATH", t.TempDir())
	if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err == nil || !bytes.Contains([]byte(err.Error()), []byte("resolve typst binary")) {
		t.Fatalf("Render resolve error = %v, want resolve typst binary", err)
	}

	withMockTypstOnPath(t, testMetadataFixturePDF(), 0)
	if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err != nil {
		t.Fatalf("Render with typst resolved from PATH: %v", err)
	}

	withMockTypstOnPath(t, []byte("%PDF-1.7\nnot-a-real-pdf\n"), 0)
	err := Render(&bytes.Buffer{}, Options{
		Data:        map[string]any{"ok": true},
		PDFMetadata: PDFMetadata{Creator: "velocity.report v1.2.3"},
	})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("apply pdf metadata")) {
		t.Fatalf("Render metadata error = %v, want apply pdf metadata", err)
	}

	withMockTypstOnPath(t, testMetadataFixturePDF(), 0)
	err = Render(&bytes.Buffer{}, Options{
		Data:   map[string]any{"ok": true},
		Assets: []Asset{{Name: ".", Data: []byte("x")}},
	})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("write asset")) {
		t.Fatalf("Render asset write error = %v, want write asset", err)
	}
}

func TestSourcesAndTemplateErrors(t *testing.T) {
	restoreRenderDeps(t)
	oldTemplates := templatesFS
	templatesFS = embed.FS{}
	t.Cleanup(func() { templatesFS = oldTemplates })
	renderTemplatesFS = templatesFS

	if _, err := Sources(); err == nil {
		t.Fatal("Sources should fail when the embedded template tree is unavailable")
	}
	if err := materialiseTemplates(t.TempDir()); err == nil {
		t.Fatal("materialiseTemplates should fail when the embedded template tree is unavailable")
	}

	restoreRenderDeps(t)
	renderTemplatesFS = failingReadFS{FS: fstest.MapFS{"templates/report.typ": {Data: []byte("x")}}}
	if _, err := Sources(); err == nil {
		t.Fatal("Sources should fail when template reads fail")
	}
	if err := materialiseTemplates(t.TempDir()); err == nil {
		t.Fatal("materialiseTemplates should fail when template reads fail")
	}
}

func TestRenderSeammedSetupErrors(t *testing.T) {
	t.Run("mkdir temp", func(t *testing.T) {
		restoreRenderDeps(t)
		renderMkdirTemp = func(string, string) (string, error) { return "", errors.New("mkdir temp failed") }
		if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err == nil || !strings.Contains(err.Error(), "create temp dir") {
			t.Fatalf("Render mkdir temp error = %v, want create temp dir", err)
		}
	})

	t.Run("write data", func(t *testing.T) {
		restoreRenderDeps(t)
		renderResolveTypst = func() (string, func(), error) {
			return mockTypstCLI(t, testMetadataFixturePDF(), 0), func() {}, nil
		}
		renderWriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(name, "data.json") {
				return errors.New("write data failed")
			}
			return os.WriteFile(name, data, perm)
		}
		if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err == nil || !strings.Contains(err.Error(), "write data failed") {
			t.Fatalf("Render writeData error = %v, want write data failed", err)
		}
	})

	t.Run("materialise templates", func(t *testing.T) {
		restoreRenderDeps(t)
		renderTemplatesFS = embed.FS{}
		if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err == nil {
			t.Fatal("Render should fail when template materialisation fails")
		}
	})

	t.Run("write font", func(t *testing.T) {
		restoreRenderDeps(t)
		renderResolveTypst = func() (string, func(), error) {
			return mockTypstCLI(t, testMetadataFixturePDF(), 0), func() {}, nil
		}
		renderAllFonts = func() map[string][]byte { return map[string][]byte{"bad-font.ttf": []byte("x")} }
		renderWriteFile = func(name string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(name, "bad-font.ttf") {
				return errors.New("write font failed")
			}
			return os.WriteFile(name, data, perm)
		}
		if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err == nil || !strings.Contains(err.Error(), "write font bad-font.ttf") {
			t.Fatalf("Render font write error = %v, want write font bad-font.ttf", err)
		}
	})

	t.Run("asset mkdir", func(t *testing.T) {
		restoreRenderDeps(t)
		renderResolveTypst = func() (string, func(), error) {
			return mockTypstCLI(t, testMetadataFixturePDF(), 0), func() {}, nil
		}
		renderMkdirAll = func(path string, perm os.FileMode) error {
			if strings.Contains(path, string(filepath.Separator)+"charts") {
				return errors.New("mkdir assets failed")
			}
			return os.MkdirAll(path, perm)
		}
		if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}, Assets: []Asset{{Name: "charts/test.svg", Data: []byte("<svg/>")}}}); err == nil || !strings.Contains(err.Error(), "mkdir ") {
			t.Fatalf("Render asset mkdir error = %v, want mkdir failure", err)
		}
	})
}

func TestParsePDFTrailerReferencesAndID(t *testing.T) {
	trailer, err := parsePDFTrailer([]byte("<< /Size 5 /Root 1 0 R /Info 3 0 R /ID [<abc> <def>] >>"))
	if err != nil {
		t.Fatalf("parsePDFTrailer: %v", err)
	}
	if trailer.root != (pdfRef{number: 1, generation: 0}) {
		t.Fatalf("root = %+v, want 1 0 R", trailer.root)
	}
	if trailer.info == nil || *trailer.info != (pdfRef{number: 3, generation: 0}) {
		t.Fatalf("info = %+v, want 3 0 R", trailer.info)
	}
	if trailer.id != "[<abc> <def>]" {
		t.Fatalf("id = %q, want [<abc> <def>]", trailer.id)
	}
}

// TestRenderSampleFixture is the Phase 0 smoke test: it asserts that the
// embedded templates compile against the bundled sample fixture and yield a
// valid multi-page PDF. The test is skipped when the typst binary is not
// available, since CI on the Pi image will not always have it.
func TestRenderSampleFixture(t *testing.T) {
	requireTypstForSmokeTest(t)

	body, err := os.ReadFile(filepath.Join("testdata", "sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	fontDir, err := filepath.Abs(filepath.Join("..", "chart", "assets"))
	if err != nil {
		t.Fatalf("resolve font dir: %v", err)
	}

	var buf bytes.Buffer
	if err := Render(&buf, Options{
		Data:              data,
		FontDir:           fontDir,
		IgnoreSystemFonts: true,
	}); err != nil {
		t.Fatalf("render: %v", err)
	}

	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("output is not a PDF (first 8 bytes = %q)", buf.Bytes()[:min(8, buf.Len())])
	}
	if buf.Len() < 10_000 {
		t.Fatalf("output PDF is suspiciously small: %d bytes", buf.Len())
	}
}

func TestRenderSampleFixtureWithoutHistogramBuckets(t *testing.T) {
	requireTypstForSmokeTest(t)

	body, err := os.ReadFile(filepath.Join("testdata", "sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	data["histogram"] = map[string]any{"units": "mph"}
	if charts, ok := data["charts"].(map[string]any); ok {
		charts["histogram"] = ""
	}

	fontDir, err := filepath.Abs(filepath.Join("..", "chart", "assets"))
	if err != nil {
		t.Fatalf("resolve font dir: %v", err)
	}

	var buf bytes.Buffer
	if err := Render(&buf, Options{
		Data:              data,
		FontDir:           fontDir,
		IgnoreSystemFonts: true,
	}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("output is not a PDF (first 8 bytes = %q)", buf.Bytes()[:min(8, buf.Len())])
	}
}
