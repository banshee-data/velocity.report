package api

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/report"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/units"
)

// handleChartTimeSeries renders a time-series SVG chart.
//
//	GET /api/charts/timeseries?site_id=N&start=YYYY-MM-DD&end=YYYY-MM-DD&tz=US/Pacific&units=mph&group=1h
func (s *Server) handleChartTimeSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	q := r.URL.Query()

	siteID, startUnix, endUnix, displayUnits, loc, ok := s.parseChartParams(w, q)
	if !ok {
		return
	}

	groupStr := q.Get("group")
	if groupStr == "" {
		groupStr = "1h"
	}
	groupSeconds, validGroup := supportedGroups[groupStr]
	if !validGroup {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'group' parameter. Supported values: %v", keysOfMap(supportedGroups)))
		return
	}

	minSpeedMPS := parseMinSpeed(q, displayUnits)
	paper := parsePaperSize(q)

	source := parseSource(q)
	if !isValidReportSource(source) {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'source'. Must be one of: radar_objects, radar_data, radar_data_transits")
		return
	}

	result, err := s.db.RadarObjectRollupRange(
		startUnix, endUnix, groupSeconds, minSpeedMPS,
		source, parseModelVersion(q),
		0, 0, // no histogram
		siteID, parseBoundaryThreshold(q),
	)
	if err != nil {
		log.Printf("Chart timeseries DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query data")
		return
	}

	pts := report.ConvertToTimeSeriesPoints(result.Metrics, displayUnits, loc)
	if parseExpandedChart(q) {
		pts = chart.ExpandTimeSeriesGapsInRange(
			pts,
			groupSeconds,
			time.Unix(startUnix, 0).In(loc),
			time.Unix(endUnix, 0).In(loc),
		)
	}
	p98Ref, err := s.resolveTimeSeriesP98Reference(q, siteID, startUnix, endUnix, minSpeedMPS, displayUnits)
	if err != nil {
		log.Printf("Chart timeseries summary DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query summary data")
		return
	}
	data := chart.TimeSeriesData{
		Points:       pts,
		Units:        displayUnits,
		P98Reference: p98Ref,
	}

	svg, err := chart.RenderTimeSeries(data, chart.DefaultTimeSeriesStyle(paper))
	if err != nil {
		log.Printf("Chart timeseries render error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to render chart")
		return
	}

	writeSVG(w, svg)
}

// handleChartHistogram renders a histogram SVG chart.
//
//	GET /api/charts/histogram?site_id=N&start=YYYY-MM-DD&end=YYYY-MM-DD&units=mph&bucket_size=5&max=70
func (s *Server) handleChartHistogram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	q := r.URL.Query()

	siteID, startUnix, endUnix, displayUnits, _, ok := s.parseChartParams(w, q)
	if !ok {
		return
	}

	bucketSize, histMax := parseHistogramParams(q, displayUnits)
	paper := parsePaperSize(q)

	bucketSizeMPS := units.ConvertToMPS(bucketSize, displayUnits)
	histMaxMPS := units.ConvertToMPS(histMax, displayUnits)
	minSpeedMPS := parseMinSpeed(q, displayUnits)

	source := parseSource(q)
	if !isValidReportSource(source) {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'source'. Must be one of: radar_objects, radar_data, radar_data_transits")
		return
	}

	result, err := s.db.RadarObjectRollupRange(
		startUnix, endUnix, 0, minSpeedMPS,
		source, parseModelVersion(q),
		bucketSizeMPS, histMaxMPS,
		siteID, parseBoundaryThreshold(q),
	)
	if err != nil {
		log.Printf("Chart histogram DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query data")
		return
	}

	displayHist := report.ConvertHistogramKeys(result.Histogram, displayUnits)
	data := chart.HistogramData{
		Buckets:   displayHist,
		Units:     displayUnits,
		BucketSz:  bucketSize,
		MaxBucket: histMax,
	}

	svg, err := chart.RenderHistogram(data, chart.DefaultHistogramStyle(paper))
	if err != nil {
		log.Printf("Chart histogram render error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to render chart")
		return
	}

	writeSVG(w, svg)
}

// handleChartComparison renders a comparison histogram SVG chart.
//
//	GET /api/charts/comparison?site_id=N&start=YYYY-MM-DD&end=YYYY-MM-DD&compare_start=...&compare_end=...&units=mph&bucket_size=5&max=70
func (s *Server) handleChartComparison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	q := r.URL.Query()

	siteID, startUnix, endUnix, displayUnits, _, ok := s.parseChartParams(w, q)
	if !ok {
		return
	}
	paper := parsePaperSize(q)

	// Parse comparison window (ISO 8601 instants, like start/end).
	compareStart := q.Get("compare_start")
	compareEnd := q.Get("compare_end")
	if compareStart == "" || compareEnd == "" {
		s.writeJSONError(w, http.StatusBadRequest, "'compare_start' and 'compare_end' are required")
		return
	}
	compareStartUnix, err := parseInstant("compare_start", compareStart)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	compareEndUnix, err := parseInstant("compare_end", compareEnd)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	bucketSize, histMax := parseHistogramParams(q, displayUnits)
	bucketSizeMPS := units.ConvertToMPS(bucketSize, displayUnits)
	histMaxMPS := units.ConvertToMPS(histMax, displayUnits)
	minSpeedMPS := parseMinSpeed(q, displayUnits)

	source := parseSource(q)
	if !isValidReportSource(source) {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'source'. Must be one of: radar_objects, radar_data, radar_data_transits")
		return
	}
	modelVersion := parseModelVersion(q)
	boundaryThreshold := parseBoundaryThreshold(q)

	// Primary period.
	primaryResult, err := s.db.RadarObjectRollupRange(
		startUnix, endUnix, 0, minSpeedMPS,
		source, modelVersion,
		bucketSizeMPS, histMaxMPS,
		siteID, boundaryThreshold,
	)
	if err != nil {
		log.Printf("Chart comparison primary DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query primary data")
		return
	}

	// Comparison period.
	compareSource := q.Get("compare_source")
	if compareSource == "" {
		compareSource = source
	} else if !isValidReportSource(compareSource) {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'compare_source'. Must be one of: radar_objects, radar_data, radar_data_transits")
		return
	}
	compareResult, err := s.db.RadarObjectRollupRange(
		compareStartUnix, compareEndUnix, 0, minSpeedMPS,
		compareSource, modelVersion,
		bucketSizeMPS, histMaxMPS,
		siteID, boundaryThreshold,
	)
	if err != nil {
		log.Printf("Chart comparison compare DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query comparison data")
		return
	}

	primaryHist := report.ConvertHistogramKeys(primaryResult.Histogram, displayUnits)
	compareHist := report.ConvertHistogramKeys(compareResult.Histogram, displayUnits)

	svg, err := chart.RenderComparison(
		chart.HistogramData{Buckets: primaryHist, Units: displayUnits, BucketSz: bucketSize, MaxBucket: histMax},
		chart.HistogramData{Buckets: compareHist, Units: displayUnits, BucketSz: bucketSize, MaxBucket: histMax},
		fmt.Sprintf("%s–%s", q.Get("start"), q.Get("end")),
		fmt.Sprintf("%s–%s", compareStart, compareEnd),
		chart.DefaultHistogramStyle(paper),
	)
	if err != nil {
		log.Printf("Chart comparison render error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to render chart")
		return
	}

	writeSVG(w, svg)
}

// writeSVG writes SVG bytes to the response with appropriate headers.
func writeSVG(w http.ResponseWriter, svg []byte) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	if _, err := w.Write(svg); err != nil {
		log.Printf("Failed to write SVG response: %v", err)
	}
}

func parsePaperSize(q url.Values) chart.PaperSize {
	return chart.NormalisePaperSize(q.Get("paper_size"))
}

func parseExpandedChart(q url.Values) bool {
	raw := q.Get("expanded_chart")
	if raw == "" {
		raw = q.Get("expanded")
	}
	expanded, err := strconv.ParseBool(raw)
	return err == nil && expanded
}

func (s *Server) resolveTimeSeriesP98Reference(q url.Values, siteID int, startUnix, endUnix int64, minSpeedMPS float64, displayUnits string) (float64, error) {
	if v := q.Get("p98_ref"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f, nil
		}
	}

	summaryResult, err := s.db.RadarObjectRollupRange(
		startUnix, endUnix, 0, minSpeedMPS,
		parseSource(q), parseModelVersion(q),
		0, 0,
		siteID, parseBoundaryThreshold(q),
	)
	if err != nil {
		return math.NaN(), err
	}
	if len(summaryResult.Metrics) == 0 {
		return math.NaN(), nil
	}

	return units.ConvertSpeed(summaryResult.Metrics[0].P98Speed, displayUnits), nil
}

// parseChartParams extracts and validates common chart query parameters.
// Returns false if validation failed and an error was already written.
func (s *Server) parseChartParams(w http.ResponseWriter, q url.Values) (siteID int, startUnix, endUnix int64, displayUnits string, loc *time.Location, ok bool) {
	// site_id
	siteIDStr := q.Get("site_id")
	if siteIDStr == "" {
		s.writeJSONError(w, http.StatusBadRequest, "'site_id' is required")
		return
	}
	parsed, err := strconv.Atoi(siteIDStr)
	if err != nil || parsed <= 0 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'site_id'; must be a positive integer")
		return
	}
	siteID = parsed

	// start / end / tz
	startUnix, endUnix, loc, err = parseDateRange(q, s.timezone)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// units
	displayUnits = q.Get("units")
	if displayUnits == "" {
		displayUnits = s.units
	}
	if !units.IsValid(displayUnits) {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'units' parameter. Must be one of: %s", units.GetValidUnitsString()))
		return
	}

	ok = true
	return
}

// parseDateRange reads start/end/tz from query params and returns the unix
// range plus the resolved display-timezone location.
//
// `start` and `end` are ISO 8601 / RFC3339 instants (e.g.
// 2026-06-09T00:00:00-07:00) that fully specify the query window — the server
// performs no calendar-day interpretation, so callers control the exact
// boundaries. `tz` is a display-only hint used to label chart axes and format
// response timestamps; it does not affect which rows are queried.
func parseDateRange(q url.Values, defaultTz string) (startUnix, endUnix int64, loc *time.Location, err error) {
	startStr := q.Get("start")
	endStr := q.Get("end")
	if startStr == "" || endStr == "" {
		return 0, 0, nil, errors.New("'start' and 'end' parameters are required (ISO 8601, e.g. 2026-06-09T00:00:00Z)")
	}
	startUnix, err = parseInstant("start", startStr)
	if err != nil {
		return 0, 0, nil, err
	}
	endUnix, err = parseInstant("end", endStr)
	if err != nil {
		return 0, 0, nil, err
	}
	loc, err = resolveTimezone(q.Get("tz"), defaultTz)
	if err != nil {
		return 0, 0, nil, err
	}
	return startUnix, endUnix, loc, nil
}

// parseInstant parses an ISO 8601 / RFC3339 datetime string (with offset) into
// unix seconds. The field name is used only for the error message.
func parseInstant(field, value string) (int64, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, fmt.Errorf("invalid '%s'; expected an ISO 8601 datetime with offset (e.g. 2026-06-09T00:00:00Z): %v", field, err)
	}
	return t.Unix(), nil
}

// resolveTimezone loads the IANA timezone `tz`, falling back to `fallback` and
// then UTC when empty. Used to resolve the display timezone for chart axes and
// response formatting.
func resolveTimezone(tz, fallback string) (*time.Location, error) {
	if tz == "" {
		tz = fallback
	}
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid 'tz' parameter: %v", err)
	}
	return loc, nil
}

// parseLocalDateRange parses two YYYY-MM-DD calendar dates in loc and returns the
// inclusive unix-second range [start 00:00:00, end 23:59:59] in that zone. The
// report-generation path still takes calendar dates (stored, shown in the PDF,
// and embedded in filenames), so it interprets them here.
func parseLocalDateRange(startStr, endStr string, loc *time.Location) (startUnix, endUnix int64, err error) {
	startTime, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid 'start' date: %v", err)
	}
	endTime, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid 'end' date: %v", err)
	}
	return startTime.Unix(), inclusiveLocalDateEnd(endTime).Unix(), nil
}

func inclusiveLocalDateEnd(day time.Time) time.Time {
	return day.AddDate(0, 0, 1).Add(-time.Second)
}

// parseMinSpeed parses the optional min_speed query param (in display units)
// and converts to mps.
func parseMinSpeed(q url.Values, displayUnits string) float64 {
	if s := q.Get("min_speed"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return units.ConvertToMPS(v, displayUnits)
		}
	}
	return 0
}

// parseSource returns the data source from query params, defaulting to radar_objects.
func parseSource(q url.Values) string {
	s := q.Get("source")
	if s == "" {
		return "radar_objects"
	}
	return s
}

// parseModelVersion returns the model version from query params.
func parseModelVersion(q url.Values) string {
	s := q.Get("model_version")
	if s == "" {
		return ""
	}
	return s
}

// parseBoundaryThreshold returns the boundary threshold from query params.
func parseBoundaryThreshold(q url.Values) int {
	if s := q.Get("boundary_threshold"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v >= 0 {
			return v
		}
	}
	return 0
}

// parseHistogramParams parses bucket_size and max query params, returning
// values in display units.
func parseHistogramParams(q url.Values, displayUnits string) (bucketSize, histMax float64) {
	bucketSize = 5.0 // default
	if s := q.Get("bucket_size"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			bucketSize = v
		}
	}
	histMax = 70.0 // default
	if displayUnits == "kph" {
		histMax = 110.0
	}
	if s := q.Get("max"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			histMax = v
		}
	}
	return
}
