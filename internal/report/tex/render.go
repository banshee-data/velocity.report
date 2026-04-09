package tex

import (
	"bytes"
	"embed"
	"text/template"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/chart"
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
	StartDate string // formatted display date
	EndDate   string
	Timezone  string
	Units     string

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
	CompareStartDate string
	CompareEndDate   string
	CompareP50       string
	CompareP85       string
	CompareP98       string
	CompareMax       string
	CompareCount     int

	// Radar/survey parameters
	Source       string
	Group        string
	MinSpeed     float64
	CosineAngle  float64
	CosineFactor float64
	ModelVersion string

	// Speed limit note (e.g. "Posted speed limit: 25 mph")
	SpeedLimitNote string
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
	return buf.Bytes(), nil
}
