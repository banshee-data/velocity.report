package api

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
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

	result, err := s.db.RadarObjectRollupRange(
		startUnix, endUnix, groupSeconds, minSpeedMPS,
		parseSource(q), parseModelVersion(q),
		0, 0, // no histogram
		siteID, parseBoundaryThreshold(q),
	)
	if err != nil {
		log.Printf("Chart timeseries DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query data")
		return
	}

	pts := convertToTimeSeriesPoints(result.Metrics, displayUnits, loc)
	data := chart.TimeSeriesData{
		Points: pts,
		Units:  displayUnits,
		Title:  fmt.Sprintf("Vehicle speeds — %s–%s", q.Get("start"), q.Get("end")),
	}

	svg, err := chart.RenderTimeSeries(data, chart.DefaultWebTimeSeriesStyle())
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

	bucketSizeMPS := units.ConvertToMPS(bucketSize, displayUnits)
	histMaxMPS := units.ConvertToMPS(histMax, displayUnits)
	minSpeedMPS := parseMinSpeed(q, displayUnits)

	result, err := s.db.RadarObjectRollupRange(
		startUnix, endUnix, 0, minSpeedMPS,
		parseSource(q), parseModelVersion(q),
		bucketSizeMPS, histMaxMPS,
		siteID, parseBoundaryThreshold(q),
	)
	if err != nil {
		log.Printf("Chart histogram DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query data")
		return
	}

	displayHist := convertHistogramKeys(result.Histogram, displayUnits)
	data := chart.HistogramData{
		Buckets:   displayHist,
		Units:     displayUnits,
		BucketSz:  bucketSize,
		MaxBucket: histMax,
	}

	svg, err := chart.RenderHistogram(data, chart.DefaultWebHistogramStyle())
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

	siteID, startUnix, endUnix, displayUnits, loc, ok := s.parseChartParams(w, q)
	if !ok {
		return
	}

	// Parse comparison dates.
	compareStart := q.Get("compare_start")
	compareEnd := q.Get("compare_end")
	if compareStart == "" || compareEnd == "" {
		s.writeJSONError(w, http.StatusBadRequest, "'compare_start' and 'compare_end' are required")
		return
	}
	cStartTime, err := time.ParseInLocation("2006-01-02", compareStart, loc)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'compare_start': %v", err))
		return
	}
	cEndTime, err := time.ParseInLocation("2006-01-02", compareEnd, loc)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'compare_end': %v", err))
		return
	}
	cEndTime = cEndTime.Add(24*time.Hour - time.Second)

	bucketSize, histMax := parseHistogramParams(q, displayUnits)
	bucketSizeMPS := units.ConvertToMPS(bucketSize, displayUnits)
	histMaxMPS := units.ConvertToMPS(histMax, displayUnits)
	minSpeedMPS := parseMinSpeed(q, displayUnits)

	source := parseSource(q)
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
	}
	compareResult, err := s.db.RadarObjectRollupRange(
		cStartTime.Unix(), cEndTime.Unix(), 0, minSpeedMPS,
		compareSource, modelVersion,
		bucketSizeMPS, histMaxMPS,
		siteID, boundaryThreshold,
	)
	if err != nil {
		log.Printf("Chart comparison compare DB error: %v", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to query comparison data")
		return
	}

	primaryHist := convertHistogramKeys(primaryResult.Histogram, displayUnits)
	compareHist := convertHistogramKeys(compareResult.Histogram, displayUnits)

	svg, err := chart.RenderComparison(
		chart.HistogramData{Buckets: primaryHist, Units: displayUnits, BucketSz: bucketSize, MaxBucket: histMax},
		chart.HistogramData{Buckets: compareHist, Units: displayUnits, BucketSz: bucketSize, MaxBucket: histMax},
		fmt.Sprintf("%s–%s", q.Get("start"), q.Get("end")),
		fmt.Sprintf("%s–%s", compareStart, compareEnd),
		chart.DefaultWebHistogramStyle(),
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
	w.Header().Set("Cache-Control", "max-age=300")
	if _, err := w.Write(svg); err != nil {
		log.Printf("Failed to write SVG response: %v", err)
	}
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

	// start / end dates
	startStr := q.Get("start")
	endStr := q.Get("end")
	if startStr == "" || endStr == "" {
		s.writeJSONError(w, http.StatusBadRequest, "'start' and 'end' date parameters are required (YYYY-MM-DD)")
		return
	}

	// timezone
	tz := q.Get("tz")
	if tz == "" {
		tz = s.timezone
	}
	loc, err = time.LoadLocation(tz)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'tz' parameter: %v", err))
		return
	}

	startTime, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'start' date: %v", err))
		return
	}
	endTime, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'end' date: %v", err))
		return
	}
	// End date is inclusive: advance to end-of-day.
	endTime = endTime.Add(24*time.Hour - time.Second)

	startUnix = startTime.Unix()
	endUnix = endTime.Unix()

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

// convertToTimeSeriesPoints converts DB rows to chart data points.
func convertToTimeSeriesPoints(rows []db.RadarObjectsRollupRow, displayUnits string, loc *time.Location) []chart.TimeSeriesPoint {
	pts := make([]chart.TimeSeriesPoint, len(rows))
	for i, r := range rows {
		pts[i] = chart.TimeSeriesPoint{
			StartTime: r.StartTime.In(loc),
			P50Speed:  units.ConvertSpeed(r.P50Speed, displayUnits),
			P85Speed:  units.ConvertSpeed(r.P85Speed, displayUnits),
			P98Speed:  units.ConvertSpeed(r.P98Speed, displayUnits),
			MaxSpeed:  units.ConvertSpeed(r.MaxSpeed, displayUnits),
			Count:     int(r.Count),
		}
	}
	return pts
}

// convertHistogramKeys returns a new histogram map with keys converted
// from mps to display units.
func convertHistogramKeys(hist map[float64]int64, displayUnits string) map[float64]int64 {
	if hist == nil {
		return nil
	}
	out := make(map[float64]int64, len(hist))
	for k, v := range hist {
		out[units.ConvertSpeed(k, displayUnits)] = v
	}
	return out
}
