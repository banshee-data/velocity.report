package tex

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/version"
)

//go:embed templates/*.tex
var templateFS embed.FS

// TemplateData holds all data needed to render the report .tex file.
// String fields are assumed to be pre-escaped by the caller using EscapeTeX
// where appropriate. The templates output values as-is.
type TemplateData struct {
	// Site information
	Location    string
	Surveyor    string
	Contact     string
	SpeedLimit  int
	Description string

	// Survey period
	StartDate        string // formatted display date
	EndDate          string
	StartTimeDisplay string
	EndTimeDisplay   string
	Timezone         string
	Units            string

	// Statistics (formatted strings)
	P50        string
	P85        string
	P98        string
	MaxSpeed   string
	TotalCount int
	HoursCount int

	// Chart file paths (relative to .tex directory)
	TimeSeriesChart string // "timeseries.pdf"
	HistogramChart  string // "histogram.pdf"
	CompareChart    string // "" if no comparison
	MapChart        string // "" if no map

	// Font directory (absolute path to fonts for \setmainfont)
	FontDir string

	// Histogram table (pre-rendered LaTeX tabular, empty if no histogram)
	HistogramTableTeX string

	// Statistics table rows
	StatRows []StatRow

	// Comparison metadata (empty if no comparison)
	CompareStartDate        string
	CompareEndDate          string
	CompareStartTimeDisplay string
	CompareEndTimeDisplay   string
	CompareP50              string
	CompareP85              string
	CompareP98              string
	CompareMax              string
	CompareCount            int

	// Comparison deltas: absolute (primary - compare) with sign.
	DeltaP50 string
	DeltaP85 string
	DeltaP98 string
	DeltaMax string

	// Comparison deltas: percentage change from t1 to t2 ((compare-primary)/primary*100).
	DeltaP50Pct string
	DeltaP85Pct string
	DeltaP98Pct string
	DeltaMaxPct string

	// Comparison per-period cosine correction.
	CompareCosineAngle           float64
	CompareCosineFactor          float64
	CompareCosineCorrectionLabel string

	// Compare timeseries chart path (comparison mode, "" if absent).
	CompareTimeSeriesChart string

	// Daily summary rows (comparison mode: both periods merged, sorted by time).
	DailyStatRows []StatRow

	// Pre-rendered LaTeX stat tables (styled supertabular with caption).
	StatTableTeX         string
	DailyStatTableTeX    string
	StatsTablesOneColumn bool

	// Dual-period histogram table (pre-rendered LaTeX; comparison mode only).
	DualHistogramTableTeX string

	// Key metrics table (pre-rendered LaTeX; always set; content differs by mode).
	KeyMetricsTableTeX string

	// Formatted count strings (with comma separators).
	TotalCountFormatted        string
	CompareTotalCountFormatted string
	CombinedCountFormatted     string

	// Radar/survey parameters
	Source                string
	CompareSource         string // t2 data source (comparison mode only)
	Group                 string
	MinSpeed              float64
	CosineAngle           float64
	CosineFactor          float64
	CosineCorrectionLabel string
	ModelVersion          string
	FirmwareVersion       string // optional; omitted from hardware table when empty

	// Speed limit note (e.g. "Posted speed limit: 25 mph")
	SpeedLimitNote string

	// LaTeX paper option (e.g. "a4paper" or "letterpaper"). Required: the
	// preamble template renders `\documentclass[10pt,<PaperOption>]{article}`
	// with no fallback, so an empty value produces an invalid documentclass
	// line. Production callers set this via report.paperTexOption().
	PaperOption string
}

// StatRow represents one row of the detailed statistics table.
type StatRow struct {
	StartTime string
	Count     int
	P50       string
	P85       string
	P98       string
	MaxSpeed  string
}

// BuildStatRows converts chart TimeSeriesPoints to StatRows for the table.
func BuildStatRows(pts []chart.TimeSeriesPoint, loc *time.Location) []StatRow {
	rows := make([]StatRow, len(pts))
	for i, pt := range pts {
		rows[i] = StatRow{
			StartTime: FormatTime(pt.StartTime, loc),
			Count:     pt.Count,
			P50:       FormatNumber(pt.P50Speed),
			P85:       FormatNumber(pt.P85Speed),
			P98:       FormatNumber(pt.P98Speed),
			MaxSpeed:  FormatNumber(pt.MaxSpeed),
		}
	}
	return rows
}

// RenderTeX renders the complete .tex file from the template data.
// Uses <<>> delimiters to avoid clashing with LaTeX braces.
// The output is prefixed with a metadata comment block identifying the
// pipeline version, git SHA, and generation timestamp.
func RenderTeX(data TemplateData) ([]byte, error) {
	tmpl, err := template.New("report.tex").
		Delims("<<", ">>").
		ParseFS(templateFS, "templates/*.tex")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	header := fmt.Sprintf(
		"%% velocity.report tex output\n%% Pipeline: go | Version: %s | SHA: %s\n%% Generated: %s\n%%\n",
		version.Version, version.GitSHA, time.Now().UTC().Format(time.RFC3339),
	)
	return append([]byte(header), buf.Bytes()...), nil
}
