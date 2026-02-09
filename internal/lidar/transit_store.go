package lidar

import (
	"database/sql"
	"fmt"
	"sort"
)

// LidarTransit represents a polished transit record in the lidar_transits table.
// Analogous to radar_data_transits but for LiDAR-tracked objects.
type LidarTransit struct {
	TransitID                int64   `json:"transit_id"`
	TrackID                  string  `json:"track_id"`
	SensorID                 string  `json:"sensor_id"`
	TransitStartUnix         float64 `json:"transit_start_unix"`
	TransitEndUnix           float64 `json:"transit_end_unix"`
	MaxSpeedMps              float32 `json:"max_speed_mps"`
	MinSpeedMps              float32 `json:"min_speed_mps"`
	AvgSpeedMps              float32 `json:"avg_speed_mps"`
	P50SpeedMps              float32 `json:"p50_speed_mps"`
	P85SpeedMps              float32 `json:"p85_speed_mps"`
	P95SpeedMps              float32 `json:"p95_speed_mps"`
	TrackLengthM             float32 `json:"track_length_m"`
	ObservationCount         int     `json:"observation_count"`
	ObjectClass              string  `json:"object_class,omitempty"`
	ClassificationConfidence float32 `json:"classification_confidence,omitempty"`
	QualityScore             float32 `json:"quality_score"`
	BboxLengthAvg            float32 `json:"bbox_length_avg"`
	BboxWidthAvg             float32 `json:"bbox_width_avg"`
	BboxHeightAvg            float32 `json:"bbox_height_avg"`
	CreatedAt                float64 `json:"created_at,omitempty"`
}

// TransitSummary holds aggregate statistics for a set of transits.
type TransitSummary struct {
	TotalCount   int            `json:"total_count"`
	AvgSpeedMps  float32        `json:"avg_speed_mps"`
	P50SpeedMps  float32        `json:"p50_speed_mps"`
	P85SpeedMps  float32        `json:"p85_speed_mps"`
	P95SpeedMps  float32        `json:"p95_speed_mps"`
	MaxSpeedMps  float32        `json:"max_speed_mps"`
	ByClass      map[string]int `json:"by_class"`
	SpeedBuckets map[string]int `json:"speed_buckets"` // "0-20", "20-30", "30-40", "40-50", "50+"
}

// TransitStore handles database operations for lidar_transits.
type TransitStore struct {
	db *sql.DB
}

// NewTransitStore creates a new TransitStore.
func NewTransitStore(db *sql.DB) *TransitStore {
	return &TransitStore{db: db}
}

// InsertTransit inserts a new transit record into the database.
func (ts *TransitStore) InsertTransit(t *LidarTransit) error {
	query := `
		INSERT INTO lidar_transits (
			track_id, sensor_id, transit_start_unix, transit_end_unix,
			max_speed_mps, min_speed_mps, avg_speed_mps,
			p50_speed_mps, p85_speed_mps, p95_speed_mps,
			track_length_m, observation_count,
			object_class, classification_confidence,
			quality_score, bbox_length_avg, bbox_width_avg, bbox_height_avg
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := ts.db.Exec(query,
		t.TrackID, t.SensorID, t.TransitStartUnix, t.TransitEndUnix,
		t.MaxSpeedMps, t.MinSpeedMps, t.AvgSpeedMps,
		t.P50SpeedMps, t.P85SpeedMps, t.P95SpeedMps,
		t.TrackLengthM, t.ObservationCount,
		t.ObjectClass, t.ClassificationConfidence,
		t.QualityScore, t.BboxLengthAvg, t.BboxWidthAvg, t.BboxHeightAvg,
	)
	if err != nil {
		return fmt.Errorf("failed to insert transit: %w", err)
	}

	id, err := result.LastInsertId()
	if err == nil {
		t.TransitID = id
	}

	return nil
}

// ListTransits retrieves transits with optional filters.
func (ts *TransitStore) ListTransits(sensorID string, startUnix, endUnix float64, minSpeed, maxSpeed float32, limit int) ([]*LidarTransit, error) {
	query := `
		SELECT
			transit_id, track_id, sensor_id, transit_start_unix, transit_end_unix,
			max_speed_mps, min_speed_mps, avg_speed_mps,
			p50_speed_mps, p85_speed_mps, p95_speed_mps,
			track_length_m, observation_count,
			object_class, classification_confidence,
			quality_score, bbox_length_avg, bbox_width_avg, bbox_height_avg,
			created_at
		FROM lidar_transits
		WHERE 1=1
	`

	args := []interface{}{}

	// Apply filters
	if sensorID != "" {
		query += " AND sensor_id = ?"
		args = append(args, sensorID)
	}

	if startUnix > 0 {
		query += " AND transit_end_unix >= ?"
		args = append(args, startUnix)
	}

	if endUnix > 0 {
		query += " AND transit_start_unix <= ?"
		args = append(args, endUnix)
	}

	if minSpeed > 0 {
		query += " AND p85_speed_mps >= ?"
		args = append(args, minSpeed)
	}

	if maxSpeed > 0 {
		query += " AND p85_speed_mps <= ?"
		args = append(args, maxSpeed)
	}

	query += " ORDER BY transit_start_unix DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := ts.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query transits: %w", err)
	}
	defer rows.Close()

	transits := []*LidarTransit{}
	for rows.Next() {
		t := &LidarTransit{}
		var objectClass sql.NullString
		var classConf sql.NullFloat64

		err := rows.Scan(
			&t.TransitID, &t.TrackID, &t.SensorID, &t.TransitStartUnix, &t.TransitEndUnix,
			&t.MaxSpeedMps, &t.MinSpeedMps, &t.AvgSpeedMps,
			&t.P50SpeedMps, &t.P85SpeedMps, &t.P95SpeedMps,
			&t.TrackLengthM, &t.ObservationCount,
			&objectClass, &classConf,
			&t.QualityScore, &t.BboxLengthAvg, &t.BboxWidthAvg, &t.BboxHeightAvg,
			&t.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transit row: %w", err)
		}

		if objectClass.Valid {
			t.ObjectClass = objectClass.String
		}
		if classConf.Valid {
			t.ClassificationConfidence = float32(classConf.Float64)
		}

		transits = append(transits, t)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transit rows: %w", err)
	}

	return transits, nil
}

// GetTransitSummary computes aggregate statistics for transits in a time range.
func (ts *TransitStore) GetTransitSummary(sensorID string, startUnix, endUnix float64) (*TransitSummary, error) {
	// Build query to fetch relevant transits
	query := `
		SELECT
			p50_speed_mps, p85_speed_mps, p95_speed_mps, avg_speed_mps, max_speed_mps,
			object_class
		FROM lidar_transits
		WHERE 1=1
	`

	args := []interface{}{}

	if sensorID != "" {
		query += " AND sensor_id = ?"
		args = append(args, sensorID)
	}

	if startUnix > 0 {
		query += " AND transit_end_unix >= ?"
		args = append(args, startUnix)
	}

	if endUnix > 0 {
		query += " AND transit_start_unix <= ?"
		args = append(args, endUnix)
	}

	rows, err := ts.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query transits for summary: %w", err)
	}
	defer rows.Close()

	summary := &TransitSummary{
		ByClass:      make(map[string]int),
		SpeedBuckets: make(map[string]int),
	}

	var p50Speeds []float32
	var p85Speeds []float32
	var p95Speeds []float32
	var avgSpeedSum float32
	var maxSpeedOverall float32

	for rows.Next() {
		var p50, p85, p95, avg, max float32
		var objectClass sql.NullString

		err := rows.Scan(&p50, &p85, &p95, &avg, &max, &objectClass)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transit summary row: %w", err)
		}

		summary.TotalCount++
		p50Speeds = append(p50Speeds, p50)
		p85Speeds = append(p85Speeds, p85)
		p95Speeds = append(p95Speeds, p95)
		avgSpeedSum += avg

		if max > maxSpeedOverall {
			maxSpeedOverall = max
		}

		// Classify by object class
		class := "unknown"
		if objectClass.Valid && objectClass.String != "" {
			class = objectClass.String
		}
		summary.ByClass[class]++

		// Speed buckets (m/s)
		speedKmh := p85 * 3.6 // Convert m/s to km/h for bucketing
		bucket := ""
		switch {
		case speedKmh < 20:
			bucket = "0-20"
		case speedKmh < 30:
			bucket = "20-30"
		case speedKmh < 40:
			bucket = "30-40"
		case speedKmh < 50:
			bucket = "40-50"
		default:
			bucket = "50+"
		}
		summary.SpeedBuckets[bucket]++
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transit summary rows: %w", err)
	}

	if summary.TotalCount == 0 {
		return summary, nil
	}

	// Compute aggregate percentiles (median-of-percentiles approximation)
	// NOTE: This is an approximation - taking the median of p85 values from individual
	// transits does not give the true p85 across all observations. However, for dashboard
	// display this provides a reasonable summary metric. For precise percentiles across
	// all observations, track-level data would need to be queried.
	sort.Slice(p50Speeds, func(i, j int) bool { return p50Speeds[i] < p50Speeds[j] })
	sort.Slice(p85Speeds, func(i, j int) bool { return p85Speeds[i] < p85Speeds[j] })
	sort.Slice(p95Speeds, func(i, j int) bool { return p95Speeds[i] < p95Speeds[j] })

	summary.P50SpeedMps = p50Speeds[len(p50Speeds)/2]
	summary.P85SpeedMps = p85Speeds[len(p85Speeds)/2]
	summary.P95SpeedMps = p95Speeds[len(p95Speeds)/2]
	summary.AvgSpeedMps = avgSpeedSum / float32(summary.TotalCount)
	summary.MaxSpeedMps = maxSpeedOverall

	return summary, nil
}

// ShouldPromoteToTransit checks if a track meets quality thresholds for transit promotion.
// Label-aware: if UserLabel is set, only promote good_* tracks with good quality.
// Unlabelled: fall back to TrainingDataFilter thresholds.
func ShouldPromoteToTransit(track *RunTrack) bool {
	// If labelled, check labels
	if track.UserLabel != "" {
		// Only promote good_vehicle, good_pedestrian, good_other
		switch track.UserLabel {
		case "good_vehicle", "good_pedestrian", "good_other":
			// OK - these are valid detections
		default:
			// noise, split, merge, missed - don't promote
			return false
		}

		// Check quality label if present
		if track.QualityLabel != "" {
			switch track.QualityLabel {
			case "perfect", "good":
				return true
			default:
				// truncated, noisy_velocity, stopped_recovered - don't promote
				return false
			}
		}

		// Labelled as good but no quality label - promote
		return true
	}

	// Unlabelled: use training data filter thresholds
	durationSecs := float64(track.EndUnixNanos-track.StartUnixNanos) / 1e9
	if durationSecs < 2.0 {
		return false
	}
	if track.ObservationCount < 20 {
		return false
	}

	// Compute quality score if we have the track object
	// For now, just use basic thresholds
	return true
}

// TransitFromRunTrack creates a LidarTransit from a RunTrack.
func TransitFromRunTrack(track *RunTrack) *LidarTransit {
	// Compute track length and duration
	durationSecs := float64(track.EndUnixNanos-track.StartUnixNanos) / 1e9
	trackLengthM := track.AvgSpeedMps * float32(durationSecs)

	// Compute quality score - simple version
	qualityScore := float32(0.7) // Default for unlabelled tracks

	if track.QualityLabel != "" {
		switch track.QualityLabel {
		case "perfect":
			qualityScore = 1.0
		case "good":
			qualityScore = 0.8
		case "truncated":
			qualityScore = 0.5
		case "noisy_velocity":
			qualityScore = 0.4
		case "stopped_recovered":
			qualityScore = 0.6
		}
	}

	// Estimate min/max speeds from available data
	// These are approximations when detailed speed history is not available
	const minSpeedFactor = 0.5 // Assume min speed is ~50% of average
	const maxSpeedFactor = 1.5 // Fallback: assume max is ~150% of average

	minSpeed := track.AvgSpeedMps * minSpeedFactor
	maxSpeed := track.PeakSpeedMps
	if maxSpeed == 0 {
		maxSpeed = track.AvgSpeedMps * maxSpeedFactor
	}

	return &LidarTransit{
		TrackID:                  track.TrackID,
		SensorID:                 track.SensorID,
		TransitStartUnix:         float64(track.StartUnixNanos) / 1e9,
		TransitEndUnix:           float64(track.EndUnixNanos) / 1e9,
		MaxSpeedMps:              maxSpeed,
		MinSpeedMps:              minSpeed,
		AvgSpeedMps:              track.AvgSpeedMps,
		P50SpeedMps:              track.P50SpeedMps,
		P85SpeedMps:              track.P85SpeedMps,
		P95SpeedMps:              track.P95SpeedMps,
		TrackLengthM:             trackLengthM,
		ObservationCount:         track.ObservationCount,
		ObjectClass:              track.ObjectClass,
		ClassificationConfidence: track.ObjectConfidence,
		QualityScore:             qualityScore,
		BboxLengthAvg:            track.BoundingBoxLengthAvg,
		BboxWidthAvg:             track.BoundingBoxWidthAvg,
		BboxHeightAvg:            track.BoundingBoxHeightAvg,
	}
}
