package db

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	_ "modernc.org/sqlite"

	"gonum.org/v1/gonum/stat"
)

func (db *DB) RecordRadarObject(rawRadarJSON string) error {
	var err error
	if rawRadarJSON == "" {
		return fmt.Errorf("rawRadarJSON cannot be empty")
	}

	_, err = db.Exec(
		`INSERT INTO radar_objects (raw_event) VALUES (?)`, rawRadarJSON,
	)
	if err != nil {
		return err
	}
	return nil
}

type RadarObject struct {
	Classifier   string
	StartTime    time.Time
	EndTime      time.Time
	DeltaTimeMs  int64
	MaxSpeed     float64
	MinSpeed     float64
	SpeedChange  float64
	MaxMagnitude int64
	AvgMagnitude int64
	TotalFrames  int64
	FramesPerMps float64
	Length       float64
}

func (e *RadarObject) String() string {
	return fmt.Sprintf(
		"Classifier: %s, StartTime: %s, EndTime: %s, DeltaTimeMs: %d, MaxSpeed: %f, MinSpeed: %f, SpeedChange: %f, MaxMagnitude: %d, AvgMagnitude: %d, TotalFrames: %d, FramesPerMps: %f, Length: %f",
		e.Classifier,
		e.StartTime,
		e.EndTime,
		e.DeltaTimeMs,
		e.MaxSpeed,
		e.MinSpeed,
		e.SpeedChange,
		e.MaxMagnitude,
		e.AvgMagnitude,
		e.TotalFrames,
		e.FramesPerMps,
		e.Length,
	)
}

func (db *DB) RadarObjects() ([]RadarObject, error) {
	rows, err := db.Query(`SELECT classifier, start_time, end_time, delta_time_ms, max_speed, min_speed,
			speed_change, max_magnitude, avg_magnitude, total_frames,
			frames_per_mps, length_m FROM radar_objects ORDER BY write_timestamp DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var radar_objects []RadarObject
	for rows.Next() {
		var r RadarObject

		var startTimeFloat, endTimeFloat float64

		if err := rows.Scan(
			&r.Classifier,
			&startTimeFloat,
			&endTimeFloat,
			&r.DeltaTimeMs,
			&r.MaxSpeed,
			&r.MinSpeed,
			&r.SpeedChange,
			&r.MaxMagnitude,
			&r.AvgMagnitude,
			&r.TotalFrames,
			&r.FramesPerMps,
			&r.Length,
		); err != nil {
			return nil, err
		}

		// Convert float values to seconds and nanoseconds
		startTimeSeconds := int64(startTimeFloat)
		startTimeNanos := int64(math.Round((startTimeFloat-float64(startTimeSeconds))*1e6) * 1e3) // Round to microseconds, then convert to nanoseconds
		endTimeSeconds := int64(endTimeFloat)
		endTimeNanos := int64(math.Round((endTimeFloat-float64(endTimeSeconds))*1e6) * 1e3) // Round to microseconds, then convert to nanoseconds

		// Assign the converted times to the RadarObject
		r.StartTime = time.Unix(startTimeSeconds, startTimeNanos).UTC()
		r.EndTime = time.Unix(endTimeSeconds, endTimeNanos).UTC()

		radar_objects = append(radar_objects, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return radar_objects, nil
}

// RadarObjectsRollupRow represents an aggregate row for radar object rollup.
type RadarObjectsRollupRow struct {
	Classifier string    `json:"classifier"`
	StartTime  time.Time `json:"start_time"`
	Count      int64     `json:"count"`
	P50Speed   float64   `json:"p50_speed"`
	P85Speed   float64   `json:"p85_speed"`
	P98Speed   float64   `json:"p98_speed"`
	MaxSpeed   float64   `json:"max_speed"`
}

func (e *RadarObjectsRollupRow) String() string {
	return fmt.Sprintf(
		"Classifier: %s, StartTime: %s, Count: %d, P50Speed: %f, P85Speed: %f, P98Speed: %f, MaxSpeed: %f",
		e.Classifier,
		e.StartTime,
		e.Count,
		e.P50Speed,
		e.P85Speed,
		e.P98Speed,
		e.MaxSpeed,
	)
}

// RadarStatsResult combines time-aggregated metrics with an optional histogram.
type RadarStatsResult struct {
	Metrics      []RadarObjectsRollupRow
	Histogram    map[float64]int64 // bucket start (mps) -> count; nil if histogram not requested
	MinSpeedUsed float64           // actual minimum speed filter applied (mps)
}

// buildCosineSpeedExpr generates the SQL expression for applying cosine error correction.
// The expression checks for invalid sensor angles (near 90° = perpendicular to traffic) and
// returns NULL for such readings instead of producing infinite speeds.
// speedColumn is the column name containing the raw speed (e.g., "ro.max_speed" or "rd.speed").
func buildCosineSpeedExpr(speedColumn string) string {
	return fmt.Sprintf(
		"CASE "+
			"WHEN ABS(COS(COALESCE(scp.cosine_error_angle, 0) * %.10f)) < %.10f "+
			"THEN NULL "+
			"ELSE %s / COS(COALESCE(scp.cosine_error_angle, 0) * %.10f) "+
			"END",
		radiansPerDegree,
		nearPerpendicularThreshold,
		speedColumn,
		radiansPerDegree,
	)
}

// RadarObjectRollupRange aggregates radar speed sources into time buckets and optionally computes a histogram.
// dataSource may be either "radar_objects" (default), "radar_data", or "radar_data_transits".
// If histBucketSize > 0, a histogram is computed; histMax (if > 0) clips histogram values above that threshold.
// Both histBucketSize and histMax are in meters-per-second (mps).
// boundaryThreshold: if > 0, filters out boundary hours (first/last hour of each day) with fewer than this many data points.
// This helps exclude incomplete survey periods. Set to 0 to disable boundary filtering.
// NOTE: Boundary filtering is always applied at 1-hour granularity, regardless of the requested groupSeconds.
// This ensures consistent filtering whether requesting hourly, daily (24h), or overall (all) aggregation.
func (db *DB) RadarObjectRollupRange(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string, modelVersion string, histBucketSize, histMax float64, siteID int, boundaryThreshold int) (*RadarStatsResult, error) {
	if endUnix <= startUnix {
		return nil, fmt.Errorf("end must be greater than start")
	}
	// groupSeconds == 0 is allowed and treated as the 'all' aggregation (single bucket).
	if groupSeconds < 0 {
		return nil, fmt.Errorf("groupSeconds must be non-negative")
	}

	// default minimum speed (meters per second) if caller passes 0
	if minSpeed <= 0 {
		minSpeed = 2.2352 // 2.2352 mps ≈ 5 mph
	}

	// default data source
	if dataSource == "" {
		dataSource = "radar_objects"
	}

	// Store the actual min_speed being used for return in result
	actualMinSpeed := minSpeed

	var rows *sql.Rows
	var err error
	useConfigPeriods := siteID > 0
	switch dataSource {
	case "radar_objects":
		if useConfigPeriods {
			speedExpr := buildCosineSpeedExpr("ro.max_speed")
			query := fmt.Sprintf(
				`SELECT ro.write_timestamp, %s
				 FROM radar_objects ro
				 LEFT JOIN site_config_periods scp
				   ON scp.site_id = ?
				  AND ro.write_timestamp >= scp.effective_start_unix
				  AND (scp.effective_end_unix IS NULL OR ro.write_timestamp < scp.effective_end_unix)
				 WHERE %s > ?
				   AND ro.write_timestamp BETWEEN ? AND ?`,
				speedExpr,
				speedExpr,
			)
			rows, err = db.Query(query, siteID, minSpeed, startUnix, endUnix)
		} else {
			rows, err = db.Query(`SELECT write_timestamp, max_speed FROM radar_objects WHERE max_speed > ? AND write_timestamp BETWEEN ? AND ?`, minSpeed, startUnix, endUnix)
		}
	case "radar_data":
		if useConfigPeriods {
			speedExpr := buildCosineSpeedExpr("rd.speed")
			query := fmt.Sprintf(
				`SELECT rd.write_timestamp, %s
				 FROM radar_data rd
				 LEFT JOIN site_config_periods scp
				   ON scp.site_id = ?
				  AND rd.write_timestamp >= scp.effective_start_unix
				  AND (scp.effective_end_unix IS NULL OR rd.write_timestamp < scp.effective_end_unix)
				 WHERE %s > ?
				   AND rd.write_timestamp BETWEEN ? AND ?`,
				speedExpr,
				speedExpr,
			)
			rows, err = db.Query(query, siteID, minSpeed, startUnix, endUnix)
		} else {
			rows, err = db.Query(`SELECT write_timestamp, speed FROM radar_data WHERE speed > ? AND write_timestamp BETWEEN ? AND ?`, minSpeed, startUnix, endUnix)
		}
	case "radar_data_transits":
		// radar_data_transits stores transit_start_unix and transit_max_speed
		if modelVersion == "" {
			modelVersion = "hourly-cron"
		}
		if useConfigPeriods {
			speedExpr := buildCosineSpeedExpr("rt.transit_max_speed")
			query := fmt.Sprintf(
				`SELECT rt.transit_start_unix, %s
				 FROM radar_data_transits rt
				 LEFT JOIN site_config_periods scp
				   ON scp.site_id = ?
				  AND rt.transit_start_unix >= scp.effective_start_unix
				  AND (scp.effective_end_unix IS NULL OR rt.transit_start_unix < scp.effective_end_unix)
				 WHERE rt.model_version = ?
				   AND %s > ?
				   AND rt.transit_start_unix BETWEEN ? AND ?`,
				speedExpr,
				speedExpr,
			)
			rows, err = db.Query(query, siteID, modelVersion, minSpeed, startUnix, endUnix)
		} else {
			rows, err = db.Query(`SELECT transit_start_unix, transit_max_speed FROM radar_data_transits WHERE model_version = ? AND transit_max_speed > ? AND transit_start_unix BETWEEN ? AND ?`, modelVersion, minSpeed, startUnix, endUnix)
		}
	default:
		return nil, fmt.Errorf("unsupported dataSource: %s", dataSource)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Read all raw data points first
	type dataPoint struct {
		ts  int64
		spd float64
	}
	var rawData []dataPoint
	for rows.Next() {
		var tsFloat float64
		var spd float64
		if err := rows.Scan(&tsFloat, &spd); err != nil {
			return nil, err
		}
		ts := int64(math.Round(tsFloat))
		rawData = append(rawData, dataPoint{ts: ts, spd: spd})
	}

	// Apply boundary hour filtering at 1-hour granularity if enabled
	// This creates a set of excluded timestamps, regardless of the requested groupSeconds
	excludedHours := make(map[int64]bool) // map of hour bucket starts to exclude
	if boundaryThreshold > 0 && len(rawData) > 0 {
		// First pass: bucket at 1-hour granularity to count samples per hour
		hourBuckets := make(map[int64]int) // hourStart -> count
		for _, dp := range rawData {
			hourStart := (dp.ts / 3600) * 3600 // truncate to hour
			hourBuckets[hourStart]++
		}

		// Group hours by day
		dayHours := make(map[string][]int64) // YYYY-MM-DD -> []hourStarts
		for hourStart := range hourBuckets {
			t := time.Unix(hourStart, 0).UTC()
			dayKey := t.Format("2006-01-02")
			dayHours[dayKey] = append(dayHours[dayKey], hourStart)
		}

		// Only apply boundary filtering if we have multiple days
		if len(dayHours) > 1 {
			for _, hours := range dayHours {
				if len(hours) == 0 {
					continue
				}
				// Sort hours for this day
				sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })

				firstHour := hours[0]
				lastHour := hours[len(hours)-1]

				// Check if first hour has too few samples
				if hourBuckets[firstHour] < boundaryThreshold {
					excludedHours[firstHour] = true
				}
				// Check if last hour has too few samples (and is different from first)
				if lastHour != firstHour && hourBuckets[lastHour] < boundaryThreshold {
					excludedHours[lastHour] = true
				}
			}
		}
	}

	// Filter raw data to exclude points in excluded boundary hours
	var filteredData []dataPoint
	if len(excludedHours) > 0 {
		for _, dp := range rawData {
			hourStart := (dp.ts / 3600) * 3600
			if !excludedHours[hourStart] {
				filteredData = append(filteredData, dp)
			}
		}
	} else {
		filteredData = rawData
	}

	// Now bucket the filtered data according to the requested groupSeconds
	buckets := make(map[int64][]float64)
	bucketMax := make(map[int64]float64)
	var allSpeedsForHist []float64
	if histBucketSize > 0 {
		allSpeedsForHist = make([]float64, 0, len(filteredData))
	}

	// Special-case: groupSeconds == 0 means 'all' -- aggregate all rows into a single bucket
	if groupSeconds == 0 {
		var allSpeeds []float64
		var allMax float64
		var minTs int64 = 0
		for _, dp := range filteredData {
			allSpeeds = append(allSpeeds, dp.spd)
			if histBucketSize > 0 {
				allSpeedsForHist = append(allSpeedsForHist, dp.spd)
			}
			if allMax == 0 || dp.spd > allMax {
				allMax = dp.spd
			}
			if minTs == 0 || dp.ts < minTs {
				minTs = dp.ts
			}
		}

		// Determine bucket start: midnight (00:00:00) UTC of minTs (or startUnix if no rows)
		var bucketStart int64
		if minTs == 0 {
			bucketStart = time.Unix(startUnix, 0).UTC().Truncate(24 * time.Hour).Unix()
		} else {
			bucketStart = time.Unix(minTs, 0).UTC().Truncate(24 * time.Hour).Unix()
		}

		if len(allSpeeds) > 0 {
			buckets[bucketStart] = allSpeeds
			bucketMax[bucketStart] = allMax
		}
	} else {
		for _, dp := range filteredData {
			// compute bucket start aligned to startUnix
			offset := dp.ts - startUnix
			if offset < 0 {
				offset = 0
			}
			bucketOffset := (offset / groupSeconds) * groupSeconds
			bucketStart := startUnix + bucketOffset

			buckets[bucketStart] = append(buckets[bucketStart], dp.spd)
			if histBucketSize > 0 {
				allSpeedsForHist = append(allSpeedsForHist, dp.spd)
			}
			if curr, ok := bucketMax[bucketStart]; !ok || dp.spd > curr {
				bucketMax[bucketStart] = dp.spd
			}
		}
	}

	aggregated := []RadarObjectsRollupRow{}

	// collect and sort bucket starts
	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	// Note: Boundary hour filtering has already been applied at 1-hour granularity
	// above (see excludedHours), so we just use the filtered data directly here.

	// Build aggregated results from all buckets
	for _, bucketStart := range keys {
		speeds := buckets[bucketStart]

		agg := RadarObjectsRollupRow{
			Classifier: "all",
			StartTime:  time.Unix(bucketStart, 0).UTC(),
		}

		if len(speeds) > 0 {
			agg.MaxSpeed = bucketMax[bucketStart]
			agg.Count = int64(len(speeds))

			sorted := make([]float64, len(speeds))
			copy(sorted, speeds)
			sort.Float64s(sorted)

			agg.P50Speed = stat.Quantile(0.5, stat.Empirical, sorted, nil)
			agg.P85Speed = stat.Quantile(0.85, stat.Empirical, sorted, nil)
			agg.P98Speed = stat.Quantile(0.98, stat.Empirical, sorted, nil)
		} else {
			agg.MaxSpeed = 0
			agg.Count = 0
			agg.P50Speed = 0
			agg.P85Speed = 0
			agg.P98Speed = 0
		}

		aggregated = append(aggregated, agg)
	}

	// Note: Histogram data comes from allSpeedsForHist which was built from
	// filteredData, so it already excludes boundary hours. No rebuild needed.

	// Compute histogram if requested
	var histogram map[float64]int64
	if histBucketSize > 0 && len(allSpeedsForHist) > 0 {
		histogram = make(map[float64]int64)
		for _, spd := range allSpeedsForHist {
			var binStart float64
			if histMax > 0 && spd >= histMax {
				// aggregate all values >= histMax into a single bucket at histMax
				binStart = histMax
			} else {
				// compute bin start aligned to histBucketSize
				binIdx := math.Floor(spd / histBucketSize)
				binStart = binIdx * histBucketSize
			}
			histogram[binStart] = histogram[binStart] + 1
		}
	}

	return &RadarStatsResult{
		Metrics:      aggregated,
		Histogram:    histogram,
		MinSpeedUsed: actualMinSpeed,
	}, nil
}

func (db *DB) RecordRawData(rawDataJSON string) error {
	var err error
	if rawDataJSON == "" {
		return fmt.Errorf("rawDataJSON cannot be empty")
	}

	_, err = db.Exec(`INSERT INTO radar_data (raw_event) VALUES (?)`, rawDataJSON)
	if err != nil {
		return err
	}
	return nil
}

type Event struct {
	Magnitude sql.NullFloat64
	Uptime    sql.NullFloat64
	Speed     sql.NullFloat64
}

func (e *Event) String() string {
	return fmt.Sprintf("Uptime: %f, Magnitude: %f, Speed: %f", e.Uptime.Float64, e.Magnitude.Float64, e.Speed.Float64)
}

type EventAPI struct {
	Magnitude *float64 `json:"magnitude,omitempty"`
	Uptime    *float64 `json:"uptime,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
}

func EventToAPI(e Event) EventAPI {
	var mag, up, spd *float64
	if e.Magnitude.Valid {
		mag = &e.Magnitude.Float64
	}
	if e.Uptime.Valid {
		up = &e.Uptime.Float64
	}
	if e.Speed.Valid {
		spd = &e.Speed.Float64
	}
	return EventAPI{
		Magnitude: mag,
		Uptime:    up,
		Speed:     spd,
	}
}

func (db *DB) Events() ([]Event, error) {
	rows, err := db.Query("SELECT uptime, magnitude, speed FROM radar_data ORDER BY uptime DESC LIMIT 500")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var uptime, magnitude, speed sql.NullFloat64
		if err := rows.Scan(&uptime, &magnitude, &speed); err != nil {
			return nil, err
		}
		events = append(events, Event{
			Uptime:    uptime,
			Magnitude: magnitude,
			Speed:     speed,
		},
		)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// DataRange represents a data coverage window in unix seconds.
type DataRange struct {
	StartUnix float64 `json:"start_unix"`
	EndUnix   float64 `json:"end_unix"`
}

// RadarDataRange returns the earliest and latest radar object timestamps.
func (db *DB) RadarDataRange() (*DataRange, error) {
	var start sql.NullFloat64
	var end sql.NullFloat64
	if err := db.DB.QueryRow("SELECT MIN(write_timestamp), MAX(write_timestamp) FROM radar_objects").Scan(&start, &end); err != nil {
		return nil, err
	}
	if !start.Valid || !end.Valid {
		return &DataRange{}, nil
	}
	return &DataRange{
		StartUnix: start.Float64,
		EndUnix:   end.Float64,
	}, nil
}
