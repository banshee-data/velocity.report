package report

// Config holds all parameters needed to generate a report.
// This is self-contained — no import of internal/api.
type Config struct {
	// Site identification
	SiteID int

	// Site metadata (pre-resolved by API handler)
	Location        string
	Surveyor        string
	Contact         string
	SpeedLimit      int
	SiteDescription string
	SpeedLimitNote  string

	// Survey period
	StartDate string // YYYY-MM-DD
	EndDate   string // YYYY-MM-DD
	Timezone  string // e.g. "US/Pacific"

	// Query parameters
	Units             string  // "mph" or "kph"
	Group             string  // e.g. "1h", "4h"
	Source            string  // "radar_objects" or "radar_data_transits"
	ModelVersion      string  // e.g. "hourly-cron"
	FirmwareVersion   string  // optional; displayed in hardware table when non-empty
	MinSpeed          float64 // display units
	BoundaryThreshold int

	// Histogram parameters
	Histogram      bool
	HistBucketSize float64 // display units
	HistMax        float64 // display units

	// Comparison period (empty = no comparison)
	CompareStart  string
	CompareEnd    string
	CompareSource string

	// Radar calibration
	CosineAngle        float64 // degrees; for primary period
	CompareCosineAngle float64 // degrees; for comparison period (0 = same as primary)

	// Site map (embedded SVG bytes, rendered as figure if IncludeMap is true)
	IncludeMap bool
	MapSVG     []byte

	// PaperSize selects physical chart dimensions and LaTeX paper
	// ("a4" or "letter"). Empty defaults to "a4".
	PaperSize string

	// Output directory (absolute path)
	OutputDir string
}

// Result contains paths to generated report files.
type Result struct {
	PDFPath string
	ZIPPath string
	RunID   string
}

// supportedGroups maps group codes to seconds.
var supportedGroups = map[string]int64{
	"15m": 15 * 60, "30m": 30 * 60,
	"1h": 3600, "2h": 7200, "3h": 10800, "4h": 14400,
	"6h": 21600, "8h": 28800, "12h": 43200, "24h": 86400,
	"all": 0,
	"2d":  172800, "3d": 259200, "7d": 604800, "14d": 1209600, "28d": 2419200,
}
