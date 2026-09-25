package server

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

func TestRunHeadwayRefusesWithoutOracle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runHeadway(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--oracle is required") || !strings.Contains(stderr.String(), "0.5.2.4") {
		t.Errorf("stderr = %q, want the reason only the oracle exists", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

func TestRunHeadwayRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--oracle", "--paper", "a3"},
		{"--oracle", "extra"},
		{"--bogus"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runHeadway(args, &stdout, &stderr); code != 2 {
			t.Errorf("runHeadway(%v) exit %d, want 2 (stderr %q)", args, code, stderr.String())
		}
	}
}

// headwayMockPDF is a structurally valid PDF with an Info dictionary, enough
// for the report's metadata stamp.
func headwayMockPDF() string {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
	}
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = pdf.Len()
		pdf.WriteString(obj)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n%010d %05d f \n", len(objects)+1, 0, 65535)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&pdf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R /Info 3 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.String()
}

// TestRunHeadwayOracleWritesTheReport runs the CLI end to end with a
// stand-in typst first on PATH.
func TestRunHeadwayOracleWritesTheReport(t *testing.T) {
	if typstbin.Embedded() {
		t.Skip("an embedded typst takes precedence over PATH")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncat > /dev/null\ncat <<'EOF'\n" + headwayMockPDF() + "EOF\n"
	if err := os.WriteFile(filepath.Join(bin, "typst"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(typstbin.EnvNoDownload, "1")

	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := runHeadway([]string{"--oracle", "--output", out, "--paper", "a4"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "Status: SYNTHETIC ORACLE\n") {
		t.Errorf("stdout = %q, want the status first", stdout.String())
	}
	for _, name := range []string{"headway_synthetic_oracle_report.pdf", "headway_synthetic_oracle_report_sources.zip"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
		if !strings.Contains(stdout.String(), filepath.Join(out, name)) {
			t.Errorf("stdout does not name %s", name)
		}
	}
}
