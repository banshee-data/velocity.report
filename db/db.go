package db

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tailscale/tailsql/server/tailsql"
	_ "modernc.org/sqlite"
	"tailscale.com/tsweb"
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
			uptime            DOUBLE,
			magnitude         DOUBLE,
			speed             DOUBLE,
			timestamp         TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS radar_objects (
			classifier        TEXT,
			start_time        DOUBLE,
			end_time          DOUBLE,
			delta_time_ms     BIGINT,
			max_speed         DOUBLE,
			min_speed         DOUBLE,
			speed_change      DOUBLE,
			max_magnitude     BIGINT,
			avg_magnitude     BIGINT,
			total_frames      BIGINT,
			frames_per_unit_speed  DOUBLE,
			length            DOUBLE,
			timestamp         TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS commands (
			command_id        BIGINT PRIMARY KEY,
			command           TEXT,
			timestamp         TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS log (
			log_id            BIGINT PRIMARY KEY,
			command_id        BIGINT,
			log_data          TEXT,
			timestamp         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(command_id) REFERENCES commands(command_id)
		);
	`)
	if err != nil {
		return nil, err
	}

	return &DB{db}, nil
}
func (db *DB) RecordRadarObject(
	classifier string,
	start_time, end_time float64,
	delta_time_ms int64,
	max_speed, min_speed, speed_change float64,
	max_magnitude, avg_magnitude, total_frames int64,
	frames_per_unit_speed, length float64,
) error {
	_, err := db.Exec(
		`INSERT INTO radar_objects (
			classifier, start_time, end_time, delta_time_ms, max_speed, min_speed,
			speed_change, max_magnitude, avg_magnitude, total_frames,
			frames_per_unit_speed, length
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		classifier, start_time, end_time, delta_time_ms, max_speed, min_speed,
		speed_change, max_magnitude, avg_magnitude, total_frames,
		frames_per_unit_speed, length,
	)
	if err != nil {
		return err
	}
	return nil
}

type RadarObject struct {
	Classifier         string
	StartTime          float64
	EndTime            float64
	DeltaTimeMs        int64
	MaxSpeed           float64
	MinSpeed           float64
	SpeedChange        float64
	MaxMagnitude       int64
	AvgMagnitude       int64
	TotalFrames        int64
	FramesPerUnitSpeed float64
	Length             float64
}

func (e *RadarObject) String() string {
	return fmt.Sprintf(
		"Classifier: %s, StartTime: %f, EndTime: %f, DeltaTimeMs: %d, MaxSpeed: %f, MinSpeed: %f, SpeedChange: %f, MaxMagnitude: %d, AvgMagnitude: %d, TotalFrames: %d, FramesPerUnitSpeed: %f, Length: %f",
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
		e.FramesPerUnitSpeed,
		e.Length,
	)
}

func (db *DB) RadarObjects() ([]RadarObject, error) {
	rows, err := db.Query(`SELECT classifier, start_time, end_time, delta_time_ms, max_speed, min_speed,
			speed_change, max_magnitude, avg_magnitude, total_frames,
			frames_per_unit_speed, length FROM radar_objects ORDER BY timestamp DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var radar_objects []RadarObject
	for rows.Next() {

		var (
			classifier            string
			start_time            float64
			end_time              float64
			max_speed             float64
			min_speed             float64
			speed_change          float64
			frames_per_unit_speed float64
			length                float64
			delta_time_ms         int64
			max_magnitude         int64
			avg_magnitude         int64
			total_frames          int64
		)

		if err := rows.Scan(
			&classifier,
			&start_time,
			&end_time,
			&delta_time_ms,
			&max_speed,
			&min_speed,
			&speed_change,
			&max_magnitude,
			&avg_magnitude,
			&total_frames,
			&frames_per_unit_speed,
			&length,
		); err != nil {
			return nil, err
		}
		radar_objects = append(radar_objects, RadarObject{
			Classifier:         classifier,
			StartTime:          start_time,
			EndTime:            end_time,
			DeltaTimeMs:        delta_time_ms,
			MaxSpeed:           max_speed,
			MinSpeed:           min_speed,
			SpeedChange:        speed_change,
			MaxMagnitude:       max_magnitude,
			AvgMagnitude:       avg_magnitude,
			TotalFrames:        total_frames,
			FramesPerUnitSpeed: frames_per_unit_speed,
			Length:             length,
		},
		)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return radar_objects, nil
}

func (db *DB) RecordObservation(uptime, magnitude, speed float64) error {
	_, err := db.Exec("INSERT INTO data (uptime, magnitude, speed) VALUES (?, ?, ?)", uptime, magnitude, speed)
	if err != nil {
		return err
	}
	return nil
}

type Event struct {
	Magnitude float64
	Uptime    float64
	Speed     float64
}

func (e *Event) String() string {
	return fmt.Sprintf("Uptime: %f, Magnitude: %f, Speed: %f", e.Uptime, e.Magnitude, e.Speed)
}

func (db *DB) Events() ([]Event, error) {
	rows, err := db.Query("SELECT uptime, magnitude, speed FROM data ORDER BY uptime DESC LIMIT 500")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var uptime, magnitude, speed float64
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
