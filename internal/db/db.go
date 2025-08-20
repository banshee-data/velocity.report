package db

import (
	"compress/gzip"
	"database/sql"
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

	"gonum.org/v1/gonum/stat"
)

type DB struct {
	*sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS data (
			write_timestamp   DOUBLE DEFAULT (unixepoch('subsec')),
			raw_event         JSON NOT NULL,
			uptime            DOUBLE AS (json_extract(raw_event, '$.uptime'))                                        STORED,
			magnitude         DOUBLE AS (json_extract(raw_event, '$.magnitude'))                                     STORED,
			speed             DOUBLE AS (json_extract(raw_event, '$.speed'))                                         STORED
		);
		CREATE TABLE IF NOT EXISTS radar_objects (
			write_timestamp   DOUBLE DEFAULT (unixepoch('subsec')),
      raw_event         JSON NOT NULL,
			classifier        TEXT NOT NULL   AS (json_extract(raw_event, '$.classifier'))                           STORED,
			start_time        DOUBLE NOT NULL AS (json_extract(raw_event, '$.start_time'))                           STORED,
			end_time          DOUBLE NOT NULL AS (json_extract(raw_event, '$.end_time'))                             STORED,
			delta_time_ms     BIGINT NOT NULL AS (json_extract(raw_event, '$.delta_time_msec'))                      STORED,
			max_speed         DOUBLE NOT NULL AS (json_extract(raw_event, '$.max_speed_mps'))                        STORED,
			min_speed         DOUBLE NOT NULL AS (json_extract(raw_event, '$.min_speed_mps'))                        STORED,
			speed_change      DOUBLE NOT NULL AS (json_extract(raw_event, '$.speed_change'))                         STORED,
			max_magnitude     BIGINT NOT NULL AS (json_extract(raw_event, '$.max_magnitude'))                        STORED,
			avg_magnitude     BIGINT NOT NULL AS (json_extract(raw_event, '$.avg_magnitude'))                        STORED,
			total_frames      BIGINT NOT NULL AS (json_extract(raw_event, '$.total_frames'))                         STORED,
			frames_per_mps    DOUBLE NOT NULL AS (json_extract(raw_event, '$.frames_per_mps'))                       STORED,
			length            DOUBLE NOT NULL AS (json_extract(raw_event, '$.length_m'))                             STORED
		);
		CREATE TABLE IF NOT EXISTS commands (
			command_id        BIGINT PRIMARY KEY,
			command           TEXT,
			write_timestamp   DOUBLE DEFAULT (unixepoch('subsec'))
		);
		CREATE TABLE IF NOT EXISTS log (
			log_id            BIGINT PRIMARY KEY,
			command_id        BIGINT,
			log_data          TEXT,
			write_timestamp   DOUBLE DEFAULT (unixepoch('subsec')),
			FOREIGN KEY(command_id) REFERENCES commands(command_id)
		);
	`)
	if err != nil {
		return nil, err
	}

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
			frames_per_mps, length FROM radar_objects ORDER BY write_timestamp DESC LIMIT 100`)
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
	Count    int64
	P50Speed float64
	P85Speed float64
	P98Speed float64
	MaxSpeed float64
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

func classifier(mag int64) string {
	switch {
	case mag < 40:
		return "ped"
	case mag < 150:
		return "car"
	default:
		return "truck"
	}
}

// RadarObjectRollup presents an aggregate view of the radar objects, to feed a percentile and/or volume graph
func (db *DB) RadarObjectRollup(days ...int) ([]RadarObjectsRollupRow, error) {
	// Set default days to 1 if not provided
	numDays := 1
	if len(days) > 0 && days[0] > 0 {
		numDays = days[0]
	}

	timeframeEnd := time.Now().Unix()
	timeframeStart := timeframeEnd - int64(24*60*60*numDays)

	rows, err := db.Query(`SELECT max_magnitude, max_speed FROM radar_objects WHERE write_timestamp BETWEEN ? AND ?`, timeframeStart, timeframeEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type statResult struct {
		MaxMagnitude int64
		MaxSpeed     float64
	}

	results := make(map[string][]statResult)

	for rows.Next() {
		var r statResult
		if err := rows.Scan(&r.MaxMagnitude, &r.MaxSpeed); err != nil {
			return nil, err
		}
		classifier := classifier(r.MaxMagnitude)

		l, ok := results[classifier]
		if !ok {
			l = []statResult{}
		}

		l = append(l, r)
		results[classifier] = l
	}

	aggregated := []RadarObjectsRollupRow{}

	for classifier, stats := range results {
		// Compute aggregate statistics for each classifier
		agg := RadarObjectsRollupRow{
			Classifier: classifier,
			StartTime:  time.Unix(timeframeStart, 0).UTC(),
		}

		for _, s := range stats {
			agg.MaxSpeed = math.Max(agg.MaxSpeed, s.MaxSpeed)
		}

		// count stat values for each classifier
		agg.Count = int64(len(stats))

		// collect speeds for percentile calculation
		speeds := make([]float64, 0, len(stats))
		for _, s := range stats {
			speeds = append(speeds, s.MaxSpeed)
		}

		// sort the speeds slice
		sorted := make([]float64, len(speeds))
		copy(sorted, speeds)
		sort.Float64s(sorted)

		// calculate percentiles
		agg.P50Speed = stat.Quantile(0.5, stat.Empirical, sorted, nil)
		agg.P85Speed = stat.Quantile(0.85, stat.Empirical, sorted, nil)
		agg.P98Speed = stat.Quantile(0.98, stat.Empirical, sorted, nil)

		// Store the aggregate row
		aggregated = append(aggregated, agg)
	}

	return aggregated, nil
}

func (db *DB) RecordRawData(rawDataJSON string) error {
	var err error
	if rawDataJSON == "" {
		return fmt.Errorf("rawDataJSON cannot be empty")
	}

	_, err = db.Exec(`INSERT INTO data (raw_event) VALUES (?)`, rawDataJSON)
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
	rows, err := db.Query("SELECT uptime, magnitude, speed FROM data ORDER BY uptime DESC LIMIT 500")
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
