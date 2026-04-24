package report

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

	// Should have 6 DB calls: primary (summary, time-series, daily) + comparison (summary, time-series, daily).
	if m.callCount != 6 {
		t.Errorf("expected 6 DB calls, got %d", m.callCount)
	}
}

func TestGenerate_EscapesTemplateFields(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	outDir := t.TempDir()
	m := &mockDB{}

	cfg := Config{
		SiteID:          1,
		Location:        "Clarendon Avenue, San Francisco",
		Surveyor:        "J. Engineer",
		Contact:         "test@example.com",
		SpeedLimit:      25,
		SiteDescription: "Escaping regression test",
		StartDate:       "2025-06-01",
		EndDate:         "2025-06-02",
		Timezone:        "UTC",
		Units:           "mph",
		Group:           "1h",
		Source:          "radar_data_transits",
		ModelVersion:    "hourly-cron",
		MinSpeed:        5.0,
		Histogram:       true,
		HistBucketSize:  5.0,
		HistMax:         70.0,
		OutputDir:       outDir,
	}

	result, err := Generate(context.Background(), m, cfg)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	zipData, err := os.ReadFile(result.ZIPPath)
	if err != nil {
		t.Fatalf("read ZIP: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open ZIP: %v", err)
	}

	var reportTex string
	for _, f := range r.File {
		if f.Name != "report.tex" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open report.tex: %v", err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read report.tex: %v", err)
		}
		reportTex = string(data)
		break
	}
	if reportTex == "" {
		t.Fatal("report.tex not found in ZIP")
	}
	if !strings.Contains(reportTex, `radar\_data\_transits`) {
		t.Fatalf("expected escaped source field in report.tex, got:\n%s", reportTex)
	}
}

func TestGenerate_TimeSeriesSVGIncludesAggregateP98Reference(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	outDir := t.TempDir()
	m := &mockDB{}

	cfg := Config{
		SiteID:    1,
		Location:  "Test Street",
		Surveyor:  "J. Engineer",
		Contact:   "test@example.com",
		StartDate: "2025-06-01",
		EndDate:   "2025-06-02",
		Timezone:  "UTC",
		Units:     "mph",
		Group:     "1h",
		Source:    "radar_objects",
		Histogram: true,
		OutputDir: outDir,
	}

	result, err := Generate(context.Background(), m, cfg)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	timeseriesSVG := readZipEntry(t, result.ZIPPath, "timeseries.svg")
	if !strings.Contains(timeseriesSVG, `class="p98-reference"`) {
		t.Fatalf("expected aggregate p98 reference line in timeseries.svg, got:\n%s", timeseriesSVG)
	}
	if !strings.Contains(timeseriesSVG, "p98 overall") {
		t.Fatalf("expected p98 overall legend label in timeseries.svg, got:\n%s", timeseriesSVG)
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
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected err to wrap ErrInvalidConfig, got: %v", err)
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
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected err to wrap ErrInvalidConfig, got: %v", err)
	}
}

func TestGenerate_InvalidStartDate(t *testing.T) {
	binDir := createMockBinaries(t)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	m := &mockDB{}
	cfg := Config{
		Group:     "1h",
		Timezone:  "UTC",
		StartDate: "not-a-date",
		EndDate:   "2025-01-31",
	}
	_, err := Generate(context.Background(), m, cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected err to wrap ErrInvalidConfig, got: %v", err)
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
	texRoot := t.TempDir()
	binDir := filepath.Join(texRoot, "bin")
	texmfDist := filepath.Join(texRoot, "texmf-dist")
	texmfHome := filepath.Join(texRoot, "texmf")
	texmfVar := filepath.Join(texRoot, "texmf-var")
	fmtDir := filepath.Join(texmfDist, "web2c", "xelatex")

	for _, dir := range []string{binDir, texmfHome, texmfVar, fmtDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(fmtDir, "xelatex.fmt"), []byte("fmt"), 0644); err != nil {
		t.Fatalf("write xelatex fmt: %v", err)
	}

	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("TEXFORMATS", "/existing/formats")

	env := envSliceToMap(buildTexEnv(texRoot))

	sep := string(os.PathListSeparator)
	if got := env["TEXMFHOME"]; got != texmfHome {
		t.Fatalf("TEXMFHOME = %q, want %q", got, texmfHome)
	}
	if got := env["TEXMFDIST"]; got != texmfDist {
		t.Fatalf("TEXMFDIST = %q, want %q", got, texmfDist)
	}
	if got := env["TEXMFVAR"]; got != texmfVar {
		t.Fatalf("TEXMFVAR = %q, want %q", got, texmfVar)
	}
	if got := env["TEXMFCNF"]; got != filepath.Join(texmfDist, "web2c")+sep {
		t.Fatalf("TEXMFCNF = %q", got)
	}
	if got := env["OPENTYPEFONTS"]; got != filepath.Join(texmfDist, "fonts", "opentype")+"//"+sep {
		t.Fatalf("OPENTYPEFONTS = %q", got)
	}
	if got := env["PATH"]; !strings.HasPrefix(got, binDir+sep) {
		t.Fatalf("PATH = %q, want prefix %q", got, binDir+sep)
	}
	if got := env["TEXFORMATS"]; !strings.HasPrefix(got, fmtDir+sep) || !strings.Contains(got, "/existing/formats") {
		t.Fatalf("TEXFORMATS = %q", got)
	}
}

func TestRunXeLatex_VendoredSetsTexFormats(t *testing.T) {
	texRoot := t.TempDir()
	binDir := filepath.Join(texRoot, "bin")
	fmtDir := filepath.Join(texRoot, "texmf-dist", "web2c", "xelatex")
	for _, dir := range []string{binDir, filepath.Join(texRoot, "texmf"), filepath.Join(texRoot, "texmf-var"), fmtDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(fmtDir, "xelatex.fmt"), []byte("fmt"), 0644); err != nil {
		t.Fatalf("write xelatex fmt: %v", err)
	}

	xelatex := filepath.Join(binDir, "xelatex")
	script := fmt.Sprintf(`#!/bin/sh
case "$TEXFORMATS" in
  %q:*) ;;
  %q) ;;
  *)
    echo "missing TEXFORMATS: $TEXFORMATS"
    exit 1
    ;;
esac
texfile=""
for arg in "$@"; do
  case "$arg" in
    *.tex) texfile="$arg" ;;
  esac
done
base=$(echo "$texfile" | sed 's/\.tex$//')
echo "%%PDF-1.4 mock" > "${base}.pdf"
echo "mock log" > "${base}.log"
`, fmtDir, fmtDir)
	if err := os.WriteFile(xelatex, []byte(script), 0755); err != nil {
		t.Fatalf("write mock xelatex: %v", err)
	}

	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "report.tex"), []byte("\\relax\n"), 0644); err != nil {
		t.Fatalf("write report.tex: %v", err)
	}

	t.Setenv("VELOCITY_TEX_ROOT", texRoot)
	t.Setenv("PATH", "/usr/bin:/bin")

	if err := runXeLatex(context.Background(), workDir, "report.tex"); err != nil {
		t.Fatalf("runXeLatex error: %v", err)
	}
}

func TestResolveTexEnvironment_UsesVelocityFmtWhenOptedIn(t *testing.T) {
	texRoot := t.TempDir()
	fmtDir := filepath.Join(texRoot, "texmf-dist", "web2c", "xelatex")
	for _, dir := range []string{filepath.Join(texRoot, "bin"), filepath.Join(texRoot, "texmf"), filepath.Join(texRoot, "texmf-var"), fmtDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(fmtDir, "velocity-report.fmt"), []byte("fmt"), 0644); err != nil {
		t.Fatalf("write velocity-report fmt: %v", err)
	}

	t.Setenv("VELOCITY_TEX_ROOT", texRoot)
	t.Setenv("VELOCITY_USE_VELOCITY_FMT", "1")
	t.Setenv("PATH", "/usr/bin:/bin")

	latexEnv := resolveTexEnvironment()
	if latexEnv.fmtName != "velocity-report" {
		t.Fatalf("fmtName = %q, want velocity-report", latexEnv.fmtName)
	}

	args := buildXeLatexArgs("report.tex", latexEnv.fmtName)
	if !containsArg(args, "-fmt=velocity-report") {
		t.Fatalf("expected -fmt=velocity-report in args, got %v", args)
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

func envSliceToMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		result[key] = value
	}
	return result
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func readZipEntry(t *testing.T, zipPath, entryName string) string {
	t.Helper()

	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read ZIP: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open ZIP: %v", err)
	}
	for _, f := range r.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", entryName, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", entryName, err)
		}
		return string(data)
	}
	t.Fatalf("%s not found in ZIP", entryName)
	return ""
}
