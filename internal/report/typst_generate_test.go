package report

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report/typst"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
	"github.com/banshee-data/velocity.report/internal/version"
)

func mockTypstPDFFixture() []byte {
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

func setBuildMetadataForTest(t *testing.T, appVersion, gitSHA string) {
	t.Helper()
	oldVersion := version.Version
	oldGitSHA := version.GitSHA
	version.Version = appVersion
	version.GitSHA = gitSHA
	t.Cleanup(func() {
		version.Version = oldVersion
		version.GitSHA = oldGitSHA
	})
}

// mockTypstBinary writes a fake `typst` executable that drains stdin and emits
// a minimal PDF to stdout — exactly the stdio contract go-typst uses. It lets
// the end-to-end Typst generate tests run without a real typst install.
func mockTypstBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "typst")
	pdf := string(mockTypstPDFFixture())
	body := "#!/bin/sh\n" +
		"# Mock typst: discard the document on stdin, write a minimal PDF.\n" +
		"cat > /dev/null\n" +
		"cat <<'EOF'\n" + pdf + "EOF\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write mock typst: %v", err)
	}
	return script
}

// readReportDataFromZip extracts and decodes data.json from a generated source
// ZIP so tests can assert on the structured payload the templates receive.
func readReportDataFromZip(t *testing.T, zipPath string) (typst.ReportData, map[string]struct{}) {
	t.Helper()
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := map[string]struct{}{}
	var dataJSON []byte
	for _, f := range zr.File {
		names[f.Name] = struct{}{}
		if f.Name == "data.json" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open data.json: %v", err)
			}
			dataJSON, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("read data.json: %v", err)
			}
		}
	}
	if dataJSON == nil {
		t.Fatal("data.json not found in source zip")
	}
	var rd typst.ReportData
	if err := json.Unmarshal(dataJSON, &rd); err != nil {
		t.Fatalf("decode data.json: %v", err)
	}
	return rd, names
}

func readZipEntry(t *testing.T, zipPath, entryName string) []byte {
	t.Helper()
	raw, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", entryName, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", entryName, err)
		}
		return data
	}
	t.Fatalf("%s not found in source zip", entryName)
	return nil
}

func baseTypstConfig(outDir string) Config {
	return Config{
		SiteID:         1,
		Location:       "Test Location",
		Surveyor:       "Test Surveyor",
		Contact:        "test@example.com",
		SpeedLimit:     25,
		StartDate:      "2025-06-02",
		EndDate:        "2025-06-04",
		Timezone:       "UTC",
		Units:          "mph",
		Group:          "1h",
		Source:         "radar_data_transits",
		Histogram:      true,
		HistBucketSize: 5,
		CosineAngle:    21.0,
		OutputDir:      outDir,
	}
}

func TestGenerateTypst_Single(t *testing.T) {
	setBuildMetadataForTest(t, "1.2.3-test", "deadbeefcafebabe")
	t.Setenv(typstbin.EnvPath, mockTypstBinary(t))
	outDir := t.TempDir()
	m := &mockDB{}

	cfg := baseTypstConfig(outDir)
	savedMapSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>saved map</text></svg>`)
	cfg.IncludeMap = true
	cfg.MapSVG = savedMapSVG
	res, err := GenerateTypst(context.Background(), m, cfg)
	if err != nil {
		t.Fatalf("GenerateTypst: %v", err)
	}

	pdfBytes, err := os.ReadFile(res.PDFPath)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF: %q", pdfBytes[:min(8, len(pdfBytes))])
	}
	for _, want := range []string{
		"/Creator (velocity.report v1.2.3-test)",
		"/Keywords (git-sha:deadbeefcafebabe)",
		"<xmp:CreatorTool>velocity.report v1.2.3-test</xmp:CreatorTool>",
		"<pdf:Keywords>git-sha:deadbeefcafebabe</pdf:Keywords>",
	} {
		if !bytes.Contains(pdfBytes, []byte(want)) {
			t.Fatalf("generated pdf missing metadata %q", want)
		}
	}

	rd, names := readReportDataFromZip(t, res.ZIPPath)
	if rd.Compare != nil {
		t.Errorf("single report should have nil compare, got %+v", rd.Compare)
	}
	if rd.Overall.TotalCount == 0 {
		t.Error("expected non-zero overall total count")
	}
	// Single-period reports omit the daily table; only comparison mode merges a
	// daily table.
	if len(rd.Daily) != 0 {
		t.Errorf("single report should not populate a daily table, got %d rows", len(rd.Daily))
	}
	if len(rd.Granular) == 0 {
		t.Error("single report should populate the granular table")
	}
	if len(rd.Histogram.Buckets) == 0 {
		t.Error("single report should populate histogram buckets")
	}
	if rd.Radar.CosineErrorAngle != "21.0" {
		t.Errorf("cosine angle = %q, want 21.0", rd.Radar.CosineErrorAngle)
	}
	if rd.CosineCorrectionNote == "" {
		t.Error("cosine correction note should be set when an angle applies")
	}

	// The source archive must be recompilable: Typst sources + data + fonts.
	for _, want := range []string{"report.typ", "preamble.typ", "sections.typ", "data.json", "README.md"} {
		if _, ok := names[want]; !ok {
			t.Errorf("source zip missing %s", want)
		}
	}
	if _, ok := names["charts/map.svg"]; !ok {
		t.Error("source zip missing charts/map.svg")
	}
	if got := readZipEntry(t, res.ZIPPath, "charts/map.svg"); !bytes.Equal(got, savedMapSVG) {
		t.Errorf("charts/map.svg = %q, want exact saved map SVG", got)
	}
	if _, ok := names["report.tex"]; ok {
		t.Error("typst source zip should not contain report.tex")
	}
	hasFont := false
	for n := range names {
		if filepath.Dir(n) == "fonts" {
			hasFont = true
			break
		}
	}
	if !hasFont {
		t.Error("source zip should bundle fonts/")
	}
}

func TestGenerateTypst_Comparison(t *testing.T) {
	t.Setenv(typstbin.EnvPath, mockTypstBinary(t))
	outDir := t.TempDir()
	m := &mockDB{}

	cfg := baseTypstConfig(outDir)
	cfg.CompareStart = "2025-01-15"
	cfg.CompareEnd = "2025-01-19"
	cfg.CompareCosineAngle = 21.0

	res, err := GenerateTypst(context.Background(), m, cfg)
	if err != nil {
		t.Fatalf("GenerateTypst: %v", err)
	}

	rd, _ := readReportDataFromZip(t, res.ZIPPath)
	if rd.Compare == nil {
		t.Fatal("comparison report must populate compare block")
	}
	if rd.Compare.Overall.TotalCount == 0 {
		t.Error("compare overall total count should be populated")
	}
	if len(rd.Compare.Daily) == 0 {
		t.Error("compare daily rows should be populated")
	}
	if len(rd.Compare.Granular) == 0 {
		t.Error("compare granular rows should be populated")
	}
	if len(rd.Compare.Histogram.Buckets) == 0 {
		t.Error("compare histogram buckets should be populated")
	}
	if rd.Compare.StartDate != "2025-01-15" || rd.Compare.EndDate != "2025-01-19" {
		t.Errorf("compare dates = %s..%s", rd.Compare.StartDate, rd.Compare.EndDate)
	}
	if rd.Radar.CompareCosineErrorAngle != "21.0" {
		t.Errorf("compare cosine angle = %q, want 21.0", rd.Radar.CompareCosineErrorAngle)
	}
}

// TestGeneratePDF_DefaultsToTypst verifies the public entry point uses Typst.
func TestGeneratePDF_DefaultsToTypst(t *testing.T) {
	t.Setenv(typstbin.EnvPath, mockTypstBinary(t))
	outDir := t.TempDir()
	m := &mockDB{}

	res, err := GeneratePDF(context.Background(), m, baseTypstConfig(outDir))
	if err != nil {
		t.Fatalf("GeneratePDF: %v", err)
	}
	_, names := readReportDataFromZip(t, res.ZIPPath)
	if _, ok := names["report.typ"]; !ok {
		t.Error("default engine should produce a Typst (report.typ) source zip")
	}
}

func TestBuildHistogramBuckets(t *testing.T) {
	// cutoff 5, bucket size 5, max 50. Keys below 5 fold into "<5"; keys >= 50
	// fold into "50+"; the rest are explicit ranges.
	hist := map[float64]int64{
		0:  10, // below cutoff
		5:  20,
		10: 60,
		50: 10, // at/above max
	}
	buckets := buildHistogramBuckets(hist, 5, 5, 50)

	type want struct {
		label   string
		count   int
		percent float64
	}
	wants := []want{
		{"<5", 10, 10.0},
		{"5-10", 20, 20.0},
		{"10-15", 60, 60.0},
		{"50+", 10, 10.0},
	}
	if len(buckets) != len(wants) {
		t.Fatalf("got %d buckets, want %d: %+v", len(buckets), len(wants), buckets)
	}
	for i, w := range wants {
		b := buckets[i]
		if b.Label != w.label || b.Count != w.count {
			t.Errorf("bucket %d = {%s, %d}, want {%s, %d}", i, b.Label, b.Count, w.label, w.count)
		}
		if b.Percent == nil || *b.Percent != w.percent {
			t.Errorf("bucket %d percent = %v, want %v", i, b.Percent, w.percent)
		}
	}
}

func TestToTypstRows_ZeroCountNilPercentiles(t *testing.T) {
	base := time.Date(2025, 6, 2, 8, 0, 0, 0, time.UTC)
	rows := []db.RadarObjectsRollupRow{
		{StartTime: base, Count: 0}, // no samples → percentiles must be null
		{StartTime: base.Add(time.Hour), Count: 120, P50Speed: 11.0, P85Speed: 15.0, P98Speed: 19.0, MaxSpeed: 24.0},
	}
	out := toTypstRows(rows, "mph", time.UTC, false)
	if len(out) != len(rows) {
		t.Fatalf("got %d rows, want %d", len(out), len(rows))
	}
	// Row with zero count must leave percentile pointers nil (→ JSON null → "--").
	if out[0].P50 != nil {
		t.Errorf("zero-count row should have nil P50, got %v", *out[0].P50)
	}
	if out[0].Bucket == "" {
		t.Error("granular rows should set Bucket label")
	}
	if out[0].Date != "" {
		t.Error("granular rows should not set Date label")
	}
	// Row with samples must carry converted percentile values.
	if out[1].P50 == nil {
		t.Error("non-zero row should have a P50 value")
	}
}
