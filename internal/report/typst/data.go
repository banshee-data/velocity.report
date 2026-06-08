package typst

// ReportData is the structured payload consumed by the .typ templates. It is
// marshalled to data.json in the compilation working directory and read back
// via `json("/data.json")` inside report.typ.
//
// The JSON shape here is the contract between the Go report orchestrator and
// the Typst templates (report.typ / preamble.typ / sections.typ). Field names
// and nesting must match what the templates reference. In particular:
//
//   - Percentile values are *float64 so that zero-sample rows marshal to JSON
//     null, which Typst's json() decodes to `none`; the templates render that
//     as "--" via fmt-speed/fmt-speed-bare.
//   - Compare is a pointer: nil marshals to JSON null, which the templates
//     test with `data.compare != none` to switch between single and
//     comparison layouts. The key is always present.
type ReportData struct {
	// Paper is the Typst paper name ("a4" or "us-letter"), derived from the
	// report config's paper size so the page matches the requested format.
	Paper                string       `json:"paper"`
	Site                 SiteData     `json:"site"`
	Period               PeriodData   `json:"period"`
	Compare              *CompareData `json:"compare"`
	Overall              SummaryData  `json:"overall"`
	Radar                RadarData    `json:"radar"`
	Histogram            Histogram    `json:"histogram"`
	Daily                []RollupRow  `json:"daily"`
	Granular             []RollupRow  `json:"granular"`
	CosineCorrectionNote string       `json:"cosine_correction_note"`
	Charts               ChartRefs    `json:"charts"`
}

// SiteData carries the human-facing site identity used in the title block,
// overview, and site-information sections.
type SiteData struct {
	Location        string `json:"location"`
	Surveyor        string `json:"surveyor"`
	Contact         string `json:"contact"`
	SpeedLimit      int    `json:"speed_limit"`
	SiteDescription string `json:"site_description"`
	SpeedLimitNote  string `json:"speed_limit_note"`
}

// PeriodData describes the primary survey window and query parameters.
type PeriodData struct {
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	StartISO    string `json:"start_iso"`
	EndISO      string `json:"end_iso"`
	Timezone    string `json:"timezone"`
	Group       string `json:"group"`
	Units       string `json:"units"`
	MinSpeedStr string `json:"min_speed_str"`
}

// SummaryData is the aggregate over a survey period (the "overall" block, and
// each period's overall inside a comparison).
type SummaryData struct {
	TotalCount int      `json:"total_count"`
	P50        *float64 `json:"p50"`
	P85        *float64 `json:"p85"`
	P98        *float64 `json:"p98"`
	MaxSpeed   *float64 `json:"max_speed"`
}

// RollupRow is one row of the daily or granular percentile tables. Daily rows
// populate Date; granular rows populate Bucket. The unused label is omitted so
// the JSON stays clean.
type RollupRow struct {
	Date     string   `json:"date,omitempty"`
	Bucket   string   `json:"bucket,omitempty"`
	Count    int      `json:"count"`
	P50      *float64 `json:"p50"`
	P85      *float64 `json:"p85"`
	P98      *float64 `json:"p98"`
	MaxSpeed *float64 `json:"max_speed"`
}

// HistogramBucket is one labelled speed bucket with its count and share.
type HistogramBucket struct {
	Label   string   `json:"label"`
	Count   int      `json:"count"`
	Percent *float64 `json:"percent"`
}

// Histogram is the velocity-distribution table data for one period.
type Histogram struct {
	Buckets []HistogramBucket `json:"buckets"`
	Units   string            `json:"units"`
}

// RadarData carries the cosine-correction figures (pre-formatted strings, so
// the templates' str() wrapper is a no-op) and the static hardware specs.
type RadarData struct {
	CosineErrorAngle         string `json:"cosine_error_angle"`
	CosineErrorFactor        string `json:"cosine_error_factor"`
	CompareCosineErrorAngle  string `json:"compare_cosine_error_angle"`
	CompareCosineErrorFactor string `json:"compare_cosine_error_factor"`
	SensorModel              string `json:"sensor_model"`
	FirmwareVersion          string `json:"firmware_version"`
	TransmitFrequency        string `json:"transmit_frequency"`
	SampleRate               string `json:"sample_rate"`
	VelocityResolution       string `json:"velocity_resolution"`
	AzimuthFOV               string `json:"azimuth_fov"`
	ElevationFOV             string `json:"elevation_fov"`
}

// CompareData is the comparison ("t2") period. Present only in comparison
// reports; a nil *CompareData marshals to JSON null → Typst none.
type CompareData struct {
	StartDate string      `json:"start_date"`
	EndDate   string      `json:"end_date"`
	StartISO  string      `json:"start_iso"`
	EndISO    string      `json:"end_iso"`
	Overall   SummaryData `json:"overall"`
	Daily     []RollupRow `json:"daily"`
	Granular  []RollupRow `json:"granular"`
	Histogram Histogram   `json:"histogram"`
}

// ChartRefs holds the root-relative paths of the SVG charts materialised next
// to the templates. Empty strings mean "no such chart"; the templates skip
// figures whose path is "".
type ChartRefs struct {
	TimeSeries        string `json:"timeseries"`
	TimeSeriesCompare string `json:"timeseries_compare"`
	Histogram         string `json:"histogram"`
	Map               string `json:"map"`
}
