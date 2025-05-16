package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	// "regexp"

	"strings"

	"github.com/banshee-data/radar/api"
	"github.com/banshee-data/radar/db"
	"github.com/banshee-data/radar/radar"
	_ "modernc.org/sqlite"
)

//go:embed static/*
var staticFiles embed.FS

// Constants
const DB_FILE = "sensor_data.db"
const SCHEMA_VERSION = "0.0.2"

type SerialEvent struct {
	Clock float64 `json:"clock"`
}

func handleEvent(db *db.DB, payload string) error {
	if strings.HasPrefix(payload, "{") {
		var e SerialEvent
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			return fmt.Errorf("failed to unmarshal JSON: %v", err)
		}
		log.Printf("Parsed event: %+v", e)
	} else {
		segments := strings.Split(payload, ",")

		var uptime, magnitude, speed float64
		var err error

		uptime, err = strconv.ParseFloat(segments[0], 64)
		if err != nil {
			return fmt.Errorf("failed to parse uptime: %v", err)
		}

		magnitude, err = strconv.ParseFloat(segments[1], 64)
		if err != nil {
			return fmt.Errorf("failed to parse magnitude: %v", err)
		}
		speed, err = strconv.ParseFloat(segments[2], 64)
		if err != nil {
			return fmt.Errorf("failed to parse speed: %v", err)
		}

		if err := db.RecordObservation(uptime, magnitude, speed); err != nil {
			log.Printf("failed to record observation: %v", err)
		} else {
			log.Printf("Recorded observation: uptime=%.2f, magnitude=%.2f, speed=%.2f", uptime, magnitude, speed)
		}
	}
	return nil
}

var dev_mode = flag.Bool("dev", false, "Run in dev mode")

// Main
func main() {
	flag.Parse()

	var r radar.RadarPortInterface
	if *dev_mode {
		fixtures, err := os.Open("fixtures.txt")
		if err != nil {
			log.Fatalf("failed to open fixtures file: %v", err)
		}
		r = &radar.MockRadarPort{
			Data:       fixtures,
			EventsChan: make(chan string),
		}
	} else {
		var err error
		r, err = radar.NewRadarPort("/dev/ttySC1")
		if err != nil {
			log.Fatalf("failed to create radar port: %v", err)
		}
	}

	db, err := db.NewDB("sensor_data.db")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	defer db.Close()
	defer r.Close()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		log.Printf("starting monitor")
		for payload := range r.Events() {
			if err := handleEvent(db, payload); err != nil {
				log.Printf("error handling event: %v", err)
			}
		}
		wg.Done()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		if err := r.Monitor(ctx); err != nil {
			log.Printf("monitor loop error: %v", err)
		} else {
			log.Printf("monitor loop finished")
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		server := api.NewServer(r, db)
		mux := http.NewServeMux()

		apiMux := server.ServeMux()

		// Serve API traffic
		if *dev_mode {
			mux.Handle("/", http.FileServer(http.Dir("./static")))
		} else {
			mux.Handle("/", http.FileServer(http.FS(staticFiles)))
		}

		mux.Handle("/api/", http.StripPrefix("/api", apiMux))

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("got request %q", r.URL.Path)
			mux.ServeHTTP(w, r)
		})

		if err := http.ListenAndServe(":8080", h); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
		wg.Done()
	}()

	wg.Wait()
	log.Printf("done")

}
