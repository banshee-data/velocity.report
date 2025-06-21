package db

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
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
			frames_per_mps    DOUBLE,
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
	radarObject RadarObject,
) error {
	var startTime float64
	var endTime float64
	var deltaTimeMs int64
	var maxSpeed float64
	var minSpeed float64
	var speedChange float64
	var maxMagnitude int64
	var avgMagnitude int64
	var totalFrames int64
	var framesPerMps float64
	var length float64

	var err error

	if startTime, err = strconv.ParseFloat(radarObject.StartTime, 64); err != nil {
		return fmt.Errorf("failed to parse start_time: %v", err)
	}
	if endTime, err = strconv.ParseFloat(radarObject.EndTime, 64); err != nil {
		return fmt.Errorf("failed to parse end_time: %v", err)
	}
	if deltaTimeMs, err = strconv.ParseInt(radarObject.DeltaTimeMs, 10, 64); err != nil {
		return fmt.Errorf("failed to parse delta_time_msec: %v", err)
	}
	if maxSpeed, err = strconv.ParseFloat(radarObject.MaxSpeed, 64); err != nil {
		return fmt.Errorf("failed to parse max_mps: %v", err)
	}
	if minSpeed, err = strconv.ParseFloat(radarObject.MinSpeed, 64); err != nil {
		return fmt.Errorf("failed to parse min_mps: %v", err)
	}
	if speedChange, err = strconv.ParseFloat(radarObject.SpeedChange, 64); err != nil {
		return fmt.Errorf("failed to parse speed_change: %v", err)
	}
	if maxMagnitude, err = strconv.ParseInt(radarObject.MaxMagnitude, 10, 64); err != nil {
		return fmt.Errorf("failed to parse max_mag: %v", err)
	}
	if avgMagnitude, err = strconv.ParseInt(radarObject.AvgMagnitude, 10, 64); err != nil {
		return fmt.Errorf("failed to parse avg_mag: %v", err)
	}
	if totalFrames, err = strconv.ParseInt(radarObject.TotalFrames, 10, 64); err != nil {
		return fmt.Errorf("failed to parse total_frames: %v", err)
	}
	if framesPerMps, err = strconv.ParseFloat(radarObject.FramesPerMps, 64); err != nil {
		return fmt.Errorf("failed to parse frames_per_mps: %v", err)
	}
	if length, err = strconv.ParseFloat(radarObject.Length, 64); err != nil {
		return fmt.Errorf("failed to parse length_m: %v", err)
	}

	_, err = db.Exec(
		`INSERT INTO radar_objects (
			classifier, start_time, end_time, delta_time_ms, max_speed, min_speed,
			speed_change, max_magnitude, avg_magnitude, total_frames,
			frames_per_mps, length
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		radarObject.Classifier, startTime, endTime, deltaTimeMs, maxSpeed, minSpeed,
		speedChange, maxMagnitude, avgMagnitude, totalFrames,
		framesPerMps, length,
	)
	if err != nil {
		return err
	}
	return nil
}

type RadarObject struct {
	Classifier   string `json:"dir"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	DeltaTimeMs  string `json:"delta_time_msec"`
	MaxSpeed     string `json:"max_mps"`
	MinSpeed     string `json:"min_mps"`
	SpeedChange  string `json:"speed_change"`
	MaxMagnitude string `json:"max_mag"`
	AvgMagnitude string `json:"avg_mag"`
	TotalFrames  string `json:"total_frames"`
	FramesPerMps string `json:"frames_per_mps"`
	Length       string `json:"length_m"`
}

func (e *RadarObject) String() string {
	return fmt.Sprintf(
		"Classifier: %s, StartTime: %s, EndTime: %s, DeltaTimeMs: %s, MaxSpeed: %s, MinSpeed: %s, SpeedChange: %s, MaxMagnitude: %s, AvgMagnitude: %s, TotalFrames: %s, FramesPerMps: %s, Length: %s",
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
			frames_per_mps, length FROM radar_objects ORDER BY timestamp DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var radar_objects []RadarObject
	for rows.Next() {

		var (
			classifier     string
			start_time     float64
			end_time       float64
			max_speed      float64
			min_speed      float64
			speed_change   float64
			frames_per_mps float64
			length         float64
			delta_time_ms  int64
			max_magnitude  int64
			avg_magnitude  int64
			total_frames   int64
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
			&frames_per_mps,
			&length,
		); err != nil {
			return nil, err
		}
		radar_objects = append(radar_objects, RadarObject{
			Classifier:   classifier,
			StartTime:    fmt.Sprintf("%f", start_time),
			EndTime:      fmt.Sprintf("%f", end_time),
			DeltaTimeMs:  fmt.Sprintf("%d", delta_time_ms),
			MaxSpeed:     fmt.Sprintf("%f", max_speed),
			MinSpeed:     fmt.Sprintf("%f", min_speed),
			SpeedChange:  fmt.Sprintf("%f", speed_change),
			MaxMagnitude: fmt.Sprintf("%d", max_magnitude),
			AvgMagnitude: fmt.Sprintf("%d", avg_magnitude),
			TotalFrames:  fmt.Sprintf("%d", total_frames),
			FramesPerMps: fmt.Sprintf("%f", frames_per_mps),
			Length:       fmt.Sprintf("%f", length),
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
