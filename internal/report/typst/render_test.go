package typst

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
