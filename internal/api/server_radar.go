package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/units"
)

// convertEventAPISpeed applies unit conversion to the Speed field of an EventAPI
func convertEventAPISpeed(event db.EventAPI, targetUnits string) db.EventAPI {
	if event.Speed != nil {
		convertedSpeed := units.ConvertSpeed(*event.Speed, targetUnits)
		event.Speed = &convertedSpeed
	}
	return event
}

// supportedGroups is a mapping of allowed group tokens to seconds for radar stats grouping
var supportedGroups = map[string]int64{
	"15m": 15 * 60,
	"30m": 30 * 60,
	"1h":  60 * 60,
	"2h":  2 * 60 * 60,
	"3h":  3 * 60 * 60,
	"4h":  4 * 60 * 60,
	"6h":  6 * 60 * 60,
	"8h":  8 * 60 * 60,
	"12h": 12 * 60 * 60,
	"24h": 24 * 60 * 60,
	// special grouping that aggregates all values into a single bucket
	// the server will pass 0 to the DB which treats it as 'all'
	"all": 0,
	"2d":  2 * 24 * 60 * 60,
	"3d":  3 * 24 * 60 * 60,
	"7d":  7 * 24 * 60 * 60,
	"14d": 14 * 24 * 60 * 60,
	"28d": 28 * 24 * 60 * 60,
}

func (s *Server) showRadarObjectStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check for units override in query parameter
	displayUnits := s.units // default to CLI-set units
	if u := r.URL.Query().Get("units"); u != "" {
		if units.IsValid(u) {
			displayUnits = u
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'units' parameter. Must be one of: %s", units.GetValidUnitsString()))
			return
		}
	}

	// Check for timezone override in query parameter
	displayTimezone := s.timezone // default to CLI-set timezone
	if tz := r.URL.Query().Get("timezone"); tz != "" {
		if units.IsTimezoneValid(tz) {
			displayTimezone = tz
		} else {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'timezone' parameter. Must be one of: %s", units.GetValidTimezonesString()))
			return
		}
	}

	// Check for optional start/end/group parameters for time range + grouping
	// start and end are expected as unix timestamps (seconds). group is a
	// human-friendly code that maps to seconds (see supportedGroups below).
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	groupStr := r.URL.Query().Get("group")

	// All three params are required for range-grouped query
	if startStr == "" || endStr == "" {
		s.writeJSONError(w, http.StatusBadRequest, "'start' and 'end' must be provided for radar stats queries")
		return
	}
	startUnix, err1 := strconv.ParseInt(startStr, 10, 64)
	endUnix, err2 := strconv.ParseInt(endStr, 10, 64)
	if err1 != nil || err2 != nil || startUnix <= 0 || endUnix <= 0 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'start' or 'end' parameter; must be unix timestamps in seconds")
		return
	}

	// If group is not provided, default to the smallest group (15m)
	groupSeconds := supportedGroups["15m"]
	if groupStr != "" {
		var ok bool
		groupSeconds, ok = supportedGroups[groupStr]
		if !ok {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'group' parameter. Supported values: %v", keysOfMap(supportedGroups)))
			return
		}
	}

	// parse optional min_speed query parameter (in display units)
	// if provided, convert to mps before passing to DB
	minSpeedMPS := 0.0 // default: let DB use its internal default when 0
	if minSpeedStr := r.URL.Query().Get("min_speed"); minSpeedStr != "" {
		// parse as float in displayUnits
		if minSpeedValue, err := strconv.ParseFloat(minSpeedStr, 64); err == nil {
			minSpeedMPS = units.ConvertToMPS(minSpeedValue, displayUnits)
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'min_speed' parameter; must be a number")
			return
		}
	}

	// parse optional data source parameter (radar_objects or radar_data_transits)
	// default to radar_objects when empty
	dataSource := r.URL.Query().Get("source")
	if dataSource == "" {
		dataSource = "radar_objects"
	} else if dataSource != "radar_objects" && dataSource != "radar_data_transits" {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'source' parameter; must be 'radar_objects' or 'radar_data_transits'")
		return
	}

	modelVersion := ""
	if dataSource == "radar_data_transits" {
		modelVersion = r.URL.Query().Get("model_version")
		if modelVersion == "" {
			modelVersion = "hourly-cron"
		}
	}

	// Optional histogram computation parameters
	computeHist := false
	if ch := r.URL.Query().Get("compute_histogram"); ch != "" {
		computeHist = (ch == "1" || strings.ToLower(ch) == "true")
	}
	histBucketSize := 0.0
	if hbs := r.URL.Query().Get("hist_bucket_size"); hbs != "" {
		if v, err := strconv.ParseFloat(hbs, 64); err == nil {
			histBucketSize = v
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'hist_bucket_size' parameter; must be a number")
			return
		}
	}
	histMax := 0.0
	if hm := r.URL.Query().Get("hist_max"); hm != "" {
		if v, err := strconv.ParseFloat(hm, 64); err == nil {
			histMax = v
		} else {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'hist_max' parameter; must be a number")
			return
		}
	}

	// Convert histogram params from display units to mps for DB call
	bucketSizeMPS := 0.0
	maxMPS := 0.0
	if computeHist && histBucketSize > 0 {
		bucketSizeMPS = units.ConvertToMPS(histBucketSize, displayUnits)
	}
	if computeHist && histMax > 0 {
		maxMPS = units.ConvertToMPS(histMax, displayUnits)
	}

	siteID := 0
	if siteIDStr := r.URL.Query().Get("site_id"); siteIDStr != "" {
		parsedSiteID, err := strconv.Atoi(siteIDStr)
		if err != nil || parsedSiteID <= 0 {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'site_id' parameter; must be a positive integer")
			return
		}
		siteID = parsedSiteID
	}

	// Optional boundary hour filtering threshold
	// If set, filters out first/last hours of each day with fewer than this many data points
	boundaryThreshold := 0
	if btStr := r.URL.Query().Get("boundary_threshold"); btStr != "" {
		parsedBT, err := strconv.Atoi(btStr)
		if err != nil || parsedBT < 0 {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'boundary_threshold' parameter; must be a non-negative integer")
			return
		}
		boundaryThreshold = parsedBT
	}

	result, dbErr := s.db.RadarObjectRollupRange(startUnix, endUnix, groupSeconds, minSpeedMPS, dataSource, modelVersion, bucketSizeMPS, maxMPS, siteID, boundaryThreshold)
	if dbErr != nil {
		s.writeJSONError(w, http.StatusInternalServerError,
			fmt.Sprintf("Failed to retrieve radar stats: %v", dbErr))
		return
	}

	// Convert StartTime to requested timezone (Display timezone) and apply unit conversions.
	// RadarObjectRollupRow.StartTime is stored in UTC by the DB layer.
	for i := range result.Metrics {
		// convert timestamp to display timezone; if conversion fails, keep UTC value
		if displayTimezone != "" {
			if t, err := units.ConvertTime(result.Metrics[i].StartTime, displayTimezone); err == nil {
				result.Metrics[i].StartTime = t
			} else {
				// log and continue with UTC value
				log.Printf("failed to convert start time to timezone %s: %v", displayTimezone, err)
			}
		}

		result.Metrics[i].MaxSpeed = units.ConvertSpeed(result.Metrics[i].MaxSpeed, displayUnits)
		result.Metrics[i].P50Speed = units.ConvertSpeed(result.Metrics[i].P50Speed, displayUnits)
		result.Metrics[i].P85Speed = units.ConvertSpeed(result.Metrics[i].P85Speed, displayUnits)
		result.Metrics[i].P98Speed = units.ConvertSpeed(result.Metrics[i].P98Speed, displayUnits)
	}

	// Build response
	respObj := map[string]interface{}{
		"metrics": result.Metrics,
	}

	// Include the actual min_speed used (converted to display units)
	if result.MinSpeedUsed > 0 {
		respObj["min_speed_used"] = units.ConvertSpeed(result.MinSpeedUsed, displayUnits)
	}

	if len(result.Histogram) > 0 {
		// convert histogram keys to strings for JSON stability
		histOut := make(map[string]int64, len(result.Histogram))
		for k, v := range result.Histogram {
			// convert bucket start (which is in mps) back to the requested display units
			conv := units.ConvertSpeed(k, displayUnits)
			key := fmt.Sprintf("%.2f", conv)
			// accumulate in case multiple mps bins map to the same display-unit label
			histOut[key] += v
		}
		respObj["histogram"] = histOut
	}

	if siteID > 0 {
		if periods, err := s.db.ListSiteConfigPeriods(&siteID); err == nil {
			angles := uniqueAnglesForRange(periods, float64(startUnix), float64(endUnix))
			if len(angles) > 0 {
				respObj["cosine_correction"] = map[string]interface{}{
					"angles":  angles,
					"applied": true,
				}
			}
		}
	}

	if err := json.NewEncoder(w).Encode(respObj); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to write radar stats")
		return
	}
}

// keysOfMap returns the keys of a string->int64 map as a sorted slice for error messages.
func keysOfMap(m map[string]int64) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
