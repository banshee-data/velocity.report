package typst

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; install via Phase 7 packaging or the make install-typst target")
	}

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
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; install via Phase 7 packaging or the make install-typst target")
	}

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
