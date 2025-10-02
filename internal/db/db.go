package db

import (
	"compress/gzip"
	"database/sql"
	_ "embed"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/tailscale/tailsql/server/tailsql"
	_ "modernc.org/sqlite"
	"tailscale.com/tsweb"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"gonum.org/v1/gonum/stat"
)

// compile-time assertion: ensure DB implements lidar.BgStore (InsertBgSnapshot)
var _ lidar.BgStore = (*DB)(nil)

type DB struct {
	*sql.DB
}

// ListRecentBgSnapshots returns the last N BgSnapshots for a sensor_id, ordered by most recent.
func (db *DB) ListRecentBgSnapshots(sensorID string, limit int) ([]*lidar.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT ?`
	rows, err := db.Query(q, sensorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []*lidar.BgSnapshot
	for rows.Next() {
		var snapID int64
		var sensor string
		var takenUnix int64
		var rings int
		var azBins int
		var paramsJSON sql.NullString
		var ringElevations sql.NullString
		var blob []byte
		var changed int
		var reason sql.NullString
		if err := rows.Scan(&snapID, &sensor, &takenUnix, &rings, &azBins, &paramsJSON, &ringElevations, &blob, &changed, &reason); err != nil {
			return nil, err
		}
		snap := &lidar.BgSnapshot{
			SnapshotID:         &snapID,
			SensorID:           sensor,
			TakenUnixNanos:     takenUnix,
			Rings:              rings,
			AzimuthBins:        azBins,
			ParamsJSON:         paramsJSON.String,
			RingElevationsJSON: ringElevations.String,
			GridBlob:           blob,
			ChangedCellsCount:  changed,
			SnapshotReason:     reason.String,
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// schema.sql contains the SQL statements for creating the database schema.
// It defines tables such as radar_data, radar_objects, radar_commands, and radar_command_log which store radar event and command information.
// The schema is embedded directly into the binary and executed when a new database is created
// via the NewDB function, ensuring consistent schema across all deployments.

//go:embed schema.sql
var schemaSQL string

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(schemaSQL)
	if err != nil {
		return nil, err
	}

	log.Println("ran database initialisation script")

	return &DB{db}, nil
}

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

// InsertBgSnapshot persists a Background snapshot into the lidar_bg_snapshot table
// and returns the new snapshot_id.
func (db *DB) InsertBgSnapshot(s *lidar.BgSnapshot) (int64, error) {
	if s == nil {
		return 0, nil
	}
	stmt := `INSERT INTO lidar_bg_snapshot (sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(stmt, s.SensorID, s.TakenUnixNanos, s.Rings, s.AzimuthBins, s.ParamsJSON, s.RingElevationsJSON, s.GridBlob, s.ChangedCellsCount, s.SnapshotReason)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestBgSnapshot returns the most recent BgSnapshot for the given sensor_id, or nil if none.
func (db *DB) GetLatestBgSnapshot(sensorID string) (*lidar.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT 1` // nolint:lll

	row := db.QueryRow(q, sensorID)
	var snapID int64
	var sensor string
	var takenUnix int64
	var rings int
	var azBins int
	var paramsJSON sql.NullString
	var ringElevations sql.NullString
	var blob []byte
	var changed int
	var reason sql.NullString

	if err := row.Scan(&snapID, &sensor, &takenUnix, &rings, &azBins, &paramsJSON, &ringElevations, &blob, &changed, &reason); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	snap := &lidar.BgSnapshot{
		SnapshotID:         &snapID,
		SensorID:           sensor,
		TakenUnixNanos:     takenUnix,
		Rings:              rings,
		AzimuthBins:        azBins,
		ParamsJSON:         paramsJSON.String,
		RingElevationsJSON: ringElevations.String,
		GridBlob:           blob,
		ChangedCellsCount:  changed,
		SnapshotReason:     reason.String,
	}
	return snap, nil
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
	Classifier string
	StartTime  time.Time
	Count      int64
	P50Speed   float64
	P85Speed   float64
	P98Speed   float64
	MaxSpeed   float64
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

// RadarObjectRollupRange now aggregates all radar_objects into buckets by time only (no classifier grouping).
// dataSource may be either "radar_objects" (default) or "radar_data_transits".
// When empty, the function defaults to "radar_objects".
func (db *DB) RadarObjectRollupRange(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string) ([]RadarObjectsRollupRow, error) {
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

	var rows *sql.Rows
	var err error
	switch dataSource {
	case "radar_objects":
		rows, err = db.Query(`SELECT write_timestamp, max_speed FROM radar_objects WHERE max_speed > ? AND write_timestamp BETWEEN ? AND ?`, minSpeed, startUnix, endUnix)
	case "radar_data_transits":
		// radar_data_transits stores transit_start_unix and transit_max_speed
		rows, err = db.Query(`SELECT transit_start_unix, transit_max_speed FROM radar_data_transits WHERE model_version='rebuild-full' AND transit_max_speed > ? AND transit_start_unix BETWEEN ? AND ?`, minSpeed, startUnix, endUnix)
	default:
		return nil, fmt.Errorf("unsupported dataSource: %s", dataSource)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// map: bucketStart -> []speeds
	buckets := make(map[int64][]float64)
	// track max speed per bucket
	bucketMax := make(map[int64]float64)

	// Special-case: groupSeconds == 0 means 'all' -- aggregate all rows into a single bucket.
	if groupSeconds == 0 {
		var allSpeeds []float64
		var allMax float64
		var minTs int64 = 0
		for rows.Next() {
			var tsFloat float64
			var spd float64
			if err := rows.Scan(&tsFloat, &spd); err != nil {
				return nil, err
			}
			ts := int64(math.Round(tsFloat))
			allSpeeds = append(allSpeeds, spd)
			if allMax == 0 || spd > allMax {
				allMax = spd
			}
			if minTs == 0 || ts < minTs {
				minTs = ts
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
		for rows.Next() {
			var tsFloat float64
			var spd float64
			if err := rows.Scan(&tsFloat, &spd); err != nil {
				return nil, err
			}
			ts := int64(math.Round(tsFloat))

			// compute bucket start aligned to startUnix
			offset := ts - startUnix
			if offset < 0 {
				offset = 0
			}
			bucketOffset := (offset / groupSeconds) * groupSeconds
			bucketStart := startUnix + bucketOffset

			buckets[bucketStart] = append(buckets[bucketStart], spd)
			if curr, ok := bucketMax[bucketStart]; !ok || spd > curr {
				bucketMax[bucketStart] = spd
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

	return aggregated, nil
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
	Magnitude *float64 `json:"Magnitude,omitempty"`
	Uptime    *float64 `json:"Uptime,omitempty"`
	Speed     *float64 `json:"Speed,omitempty"`
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

func (db *DB) AttachAdminRoutes(mux *http.ServeMux) {
	debug := tsweb.Debugger(mux)
	// create a tailSQL instance and point it to our DB
	tsql, err := tailsql.NewServer(tailsql.Options{
		RoutePrefix: "/debug/tailsql/",
	})
	if err != nil {
		log.Fatalf("failed to create tailsql server: %v", err)
	}
	tsql.SetDB("sqlite://radar.db", db.DB, &tailsql.DBOptions{
		Label: "Radar DB",
	})

	// mount the tailSQL server on the debug /tailsql path
	debug.Handle("tailsql/", "SQL live debugging", tsql.NewMux())

	debug.Handle("backup", "Create and download a backup of the database now", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unixTime := time.Now().Unix()
		backupPath := fmt.Sprintf("backup-%d.db", unixTime)
		if _, err := db.DB.Exec("VACUUM INTO ?", backupPath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create backup: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", backupPath))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")

		// Send the backup file to the client
		backupFile, err := os.Open(backupPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to open backup file: %v", err), http.StatusInternalServerError)
			return
		}

		// close the backup file after sending it
		// and remove it from the filesystem
		defer func() {
			backupFile.Close()
			if err := os.Remove(backupPath); err != nil {
				log.Printf("Failed to remove backup file: %v", err)
			}
		}()

		gzipWriter := gzip.NewWriter(w)
		defer gzipWriter.Close()
		if _, err := gzipWriter.Write([]byte{}); err != nil {
			// Need to write something to initialize the gzip header
			http.Error(w, fmt.Sprintf("Failed to initialize gzip writer: %v", err), http.StatusInternalServerError)
			return
		}

		// Copy the backup file content to the gzip writer
		if _, err := io.Copy(gzipWriter, backupFile); err != nil {
			http.Error(w, fmt.Sprintf("Failed to write backup file: %v", err), http.StatusInternalServerError)
			return
		}
	}))
}
