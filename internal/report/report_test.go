package report

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

// mockDB implements the DB interface with fixture data.
type mockDB struct {
	callCount int
}

func (m *mockDB) RadarObjectRollupRange(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string, modelVersion string, histBucketSize, histMax float64, siteID int, boundaryThreshold int) (*db.RadarStatsResult, error) {
	m.callCount++
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	if groupSeconds == 0 {
		// Summary query — single aggregate row + histogram.
		result := &db.RadarStatsResult{
			Metrics: []db.RadarObjectsRollupRow{
				{
					Classifier: "vehicle",
					StartTime:  base,
					Count:      1200,
					P50Speed:   11.176, // ~25 mph
					P85Speed:   15.646, // ~35 mph
					P98Speed:   20.117, // ~45 mph
					MaxSpeed:   24.587, // ~55 mph
				},
			},
			MinSpeedUsed: minSpeed,
		}
		if histBucketSize > 0 {
			result.Histogram = map[float64]int64{
				4.47:  50,
				6.71:  200,
				8.94:  400,
				11.18: 300,
				13.41: 150,
				15.65: 80,
				17.88: 20,
			}
		}
		return result, nil
	}

	// Time-series query — multiple rows.
	rows := make([]db.RadarObjectsRollupRow, 4)
	for i := range rows {
		rows[i] = db.RadarObjectsRollupRow{
			Classifier: "vehicle",
			StartTime:  base.Add(time.Duration(i) * time.Hour),
			Count:      int64(100 + i*50),
			P50Speed:   10.0 + float64(i)*0.5,
			P85Speed:   14.0 + float64(i)*0.5,
			P98Speed:   18.0 + float64(i)*0.5,
			MaxSpeed:   22.0 + float64(i)*0.5,
		}
	}
	return &db.RadarStatsResult{
		Metrics:      rows,
		MinSpeedUsed: minSpeed,
	}, nil
}

func (m *mockDB) GetSite(ctx context.Context, id int) (*db.Site, error) {
	return &db.Site{
		ID:       id,
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Test Surveyor",
		Contact:  "test@example.com",
	}, nil
}

// createMockBinaries writes shell scripts that fake rsvg-convert and xelatex.
func createMockBinaries(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()

	// Mock rsvg-convert: copy input to output (or create a placeholder).
	rsvg := filepath.Join(binDir, "rsvg-convert")
	rsvgScript := `#!/bin/sh
# Mock rsvg-convert: create a small placeholder PDF.
output=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) output="$2"; shift 2 ;;
        *) shift ;;
    esac
done
if [ -n "$output" ]; then
    echo "%PDF-1.4 mock" > "$output"
fi
`
	if err := os.WriteFile(rsvg, []byte(rsvgScript), 0755); err != nil {
		t.Fatalf("write mock rsvg-convert: %v", err)
	}

	// Mock xelatex: create report.pdf in the working directory.
	xelatex := filepath.Join(binDir, "xelatex")
	xelatexScript := `#!/bin/sh
# Mock xelatex: create a placeholder PDF from the .tex filename.
texfile=""
for arg in "$@"; do
    case "$arg" in
        *.tex) texfile="$arg" ;;
    esac
done
if [ -n "$texfile" ]; then
    base=$(echo "$texfile" | sed 's/\.tex$//')
    echo "%PDF-1.4 mock xelatex output" > "${base}.pdf"
    echo "mock log" > "${base}.log"
    echo "mock aux" > "${base}.aux"
fi
`
	if err := os.WriteFile(xelatex, []byte(xelatexScript), 0755); err != nil {
		t.Fatalf("write mock xelatex: %v", err)
	}

	return binDir
}

func TestGenerate_EndToEnd(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	outDir := t.TempDir()
	m := &mockDB{}

	cfg := Config{
		SiteID:          1,
		Location:        "Test Street",
		Surveyor:        "J. Engineer",
		Contact:         "test@example.com",
		SpeedLimit:      25,
		SiteDescription: "Test site for unit tests",
		SpeedLimitNote:  "Posted: 25 mph",

		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
		Timezone:  "UTC",

		Units:             "mph",
		Group:             "1h",
		Source:            "radar_objects",
		ModelVersion:      "hourly-cron",
		MinSpeed:          5.0,
		BoundaryThreshold: 0,

		Histogram:      true,
		HistBucketSize: 5.0,
		HistMax:        70.0,

		CosineAngle: 10.0,
		OutputDir:   outDir,
	}

	result, err := Generate(context.Background(), m, cfg)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Check result paths are populated.
	if result.PDFPath == "" {
		t.Error("PDFPath is empty")
	}
	if result.ZIPPath == "" {
		t.Error("ZIPPath is empty")
	}
	if result.RunID == "" {
		t.Error("RunID is empty")
	}

	// Check PDF exists.
	if _, err := os.Stat(result.PDFPath); err != nil {
		t.Errorf("PDF not found: %v", err)
	}

	// Check ZIP exists and contains expected files.
	zipData, err := os.ReadFile(result.ZIPPath)
	if err != nil {
		t.Fatalf("read ZIP: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open ZIP: %v", err)
	}

	zipNames := make(map[string]bool)
	for _, f := range r.File {
		zipNames[f.Name] = true
	}
	for _, want := range []string{"report.tex", "timeseries.svg", "histogram.svg"} {
		if !zipNames[want] {
			t.Errorf("ZIP missing %s; has: %v", want, zipNames)
		}
	}

	// Verify the mock DB was called at least twice (summary + time-series).
	if m.callCount < 2 {
		t.Errorf("expected ≥2 DB calls, got %d", m.callCount)
	}
}

func TestGenerate_WithComparison(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	outDir := t.TempDir()
	m := &mockDB{}

	cfg := Config{
		SiteID:   1,
		Location: "Compare Street",
		Surveyor: "J. Engineer",
		Contact:  "test@example.com",

		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
		Timezone:  "UTC",

		Units:        "mph",
		Group:        "1h",
		Source:       "radar_objects",
		ModelVersion: "hourly-cron",
		MinSpeed:     5.0,

		Histogram:      true,
		HistBucketSize: 5.0,
		HistMax:        70.0,

		CompareStart: "2025-05-01",
		CompareEnd:   "2025-05-02",

		OutputDir: outDir,
	}

	result, err := Generate(context.Background(), m, cfg)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// ZIP should include comparison.svg.
	zipData, err := os.ReadFile(result.ZIPPath)
	if err != nil {
		t.Fatalf("read ZIP: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open ZIP: %v", err)
	}
	zipNames := make(map[string]bool)
	for _, f := range r.File {
		zipNames[f.Name] = true
	}
	if !zipNames["comparison.svg"] {
		t.Errorf("ZIP missing comparison.svg; has: %v", zipNames)
	}

	// Should have 3 DB calls: summary, time-series, comparison.
	if m.callCount != 3 {
		t.Errorf("expected 3 DB calls, got %d", m.callCount)
	}
}

func TestGenerate_InvalidGroup(t *testing.T) {
	m := &mockDB{}
	cfg := Config{
		Group:    "invalid",
		Timezone: "UTC",
	}
	_, err := Generate(context.Background(), m, cfg)
	if err == nil || !strings.Contains(err.Error(), "unsupported group") {
		t.Errorf("expected unsupported group error, got: %v", err)
	}
}

func TestGenerate_InvalidTimezone(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	m := &mockDB{}
	cfg := Config{
		Group:    "1h",
		Timezone: "Not/A/Zone",
	}
	_, err := Generate(context.Background(), m, cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid timezone") {
		t.Errorf("expected invalid timezone error, got: %v", err)
	}
}

func TestGenerate_OutputDirMustBeAbsolute(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	m := &mockDB{}
	cfg := Config{
		SiteID:          1,
		Location:        "Test Street",
		Surveyor:        "J. Engineer",
		Contact:         "test@example.com",
		SpeedLimit:      25,
		SiteDescription: "Test site for unit tests",
		SpeedLimitNote:  "Posted: 25 mph",
		StartDate:       "2025-06-01",
		EndDate:         "2025-06-02",
		Timezone:        "UTC",
		Units:           "mph",
		Group:           "1h",
		Source:          "radar_objects",
		ModelVersion:    "hourly-cron",
		MinSpeed:        5.0,
		OutputDir:       "relative/output/path",
	}

	_, err := Generate(context.Background(), m, cfg)
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("expected absolute output dir error, got: %v", err)
	}
}

func TestConvertSVGToPDF_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := convertSVGToPDF(context.Background(), "/nonexistent.svg", "/out.pdf")
	if err == nil {
		t.Error("expected error for missing rsvg-convert")
	}
}

func TestCheckRsvgConvert(t *testing.T) {
	// Just verify it doesn't panic. May pass or fail depending on host.
	_ = checkRsvgConvert()
}

func TestCheckXeLatex(t *testing.T) {
	// Just verify it doesn't panic.
	_ = checkXeLatex()
}

func TestCheckXeLatex_VendoredMissing(t *testing.T) {
	t.Setenv("VELOCITY_TEX_ROOT", "/nonexistent/tex/root")
	err := checkXeLatex()
	if err == nil || !strings.Contains(err.Error(), "vendored xelatex not found") {
		t.Errorf("expected vendored xelatex error, got: %v", err)
	}
}

func TestSanitiseFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Main Street", "main_street"},
		{"Elm St & 5th Ave!", "elm_st__5th_ave"},
		{"café", "caf"},
		{"test-site_01", "test-site_01"},
	}
	for _, tt := range tests {
		got := sanitiseFilename(tt.in)
		if got != tt.want {
			t.Errorf("sanitiseFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSupportedGroups(t *testing.T) {
	// Verify a few key entries.
	checks := map[string]int64{
		"1h":  3600,
		"all": 0,
		"24h": 86400,
		"7d":  604800,
	}
	for k, want := range checks {
		got, ok := supportedGroups[k]
		if !ok {
			t.Errorf("supportedGroups missing %q", k)
			continue
		}
		if got != want {
			t.Errorf("supportedGroups[%q] = %d, want %d", k, got, want)
		}
	}
}

func TestBuildTexEnv(t *testing.T) {
	env := buildTexEnv("/opt/texlive")
	if len(env) == 0 {
		t.Fatal("expected non-empty env")
	}
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "TEXMFDIST=") {
			found = true
			if !strings.Contains(e, "/opt/texlive/texmf-dist") {
				t.Errorf("unexpected TEXMFDIST: %s", e)
			}
		}
	}
	if !found {
		t.Error("TEXMFDIST not found in env")
	}
}

func TestReadLogExcerpt_Missing(t *testing.T) {
	got := readLogExcerpt(t.TempDir(), "nonexistent.tex")
	if got != "(log file not found)" {
		t.Errorf("expected not-found message, got: %q", got)
	}
}

func TestReadLogExcerpt_Truncation(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "line")
	}
	os.WriteFile(filepath.Join(dir, "test.log"), []byte(strings.Join(lines, "\n")), 0644)
	got := readLogExcerpt(dir, "test.tex")
	// Should have at most 50 lines.
	resultLines := strings.Split(got, "\n")
	if len(resultLines) > 51 { // 50 lines + possible trailing newline split
		t.Errorf("expected ≤51 lines, got %d", len(resultLines))
	}
}
