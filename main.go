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
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	// "regexp"

	"strings"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/radar/api"
	"github.com/banshee-data/radar/db"
	"github.com/banshee-data/radar/serialmux"
)

var (
	//go:embed static/*
	staticFiles embed.FS
	devMode     = flag.Bool("dev", false, "Run in dev mode")
	listen      = flag.String("listen", ":8080", "Listen address")
)

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

// Main
func main() {
	flag.Parse()

	if *listen == "" {
		log.Fatal("Listen address is required")
	}

	// var r radar.RadarPortInterface
	var m serialmux.SerialMuxInterface
	if *devMode {
		data, err := os.ReadFile("fixtures.txt")
		if err != nil {
			log.Fatalf("failed to open fixtures file: %v", err)
		}
		m = serialmux.NewMockSerialMux(data)
	} else {
		var err error
		m, err = serialmux.NewRealSerialMux("/dev/ttySC1")
		if err != nil {
			log.Fatalf("failed to create radar port: %v", err)
		}
	}
	defer m.Close()

	db, err := db.NewDB("sensor_data.db")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create a wait group for the HTTP server, serial monitor, and event handler routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// run the monitor routine to manage IO on the serial port
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := m.Monitor(ctx); err != nil && err != context.Canceled {
			log.Printf("failed to monitor serial port: %v", err)
		}
		log.Print("monitor routine terminated")
	}()

	// subscribe to the serial port messages
	// and pass them to event handler
	wg.Add(1)
	go func() {
		defer wg.Done()
		id, c := m.Subscribe()
		defer m.Unsubscribe(id)
		for {
			select {
			case payload := <-c:
				if err := handleEvent(db, payload); err != nil {
					log.Printf("error handling event: %v", err)
				}
			case <-ctx.Done():
				log.Printf("subscribe routine terminated")
				return
			}
		}
	}()

	// HTTP server goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		mux := http.NewServeMux()

		// mount the admin debugging routes (accessible only in dev mode or over Tailscale)
		db.AttachAdminRoutes(mux)
		m.AttachAdminRoutes(mux)

		// create a new API server instance using the radar port and database
		// and mount the API handlers
		apiMux := api.NewServer(m, db).ServeMux()
		mux.Handle("/api/", http.StripPrefix("/api", apiMux))

		// read static files from the embedded filesystem in production or from
		// the local ./static in dev for easier iteration without restarting the
		// server
		var staticHandler http.Handler
		if *devMode {
			staticHandler = http.FileServer(http.Dir("./static"))
		} else {
			staticHandler = http.FileServer(http.FS(staticFiles))
		}
		mux.Handle("/", staticHandler)

		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("got request %q", r.URL.Path)
			mux.ServeHTTP(w, r)
		})

		server := &http.Server{
			Addr:    *listen,
			Handler: h,
		}

		// Start server in a goroutine so it doesn't block
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("failed to start server: %v", err)
			}
		}()

		// Wait for context cancellation to shut down server
		<-ctx.Done()
		log.Println("shutting down HTTP server...")

		// Create a shutdown context with a timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}

		log.Printf("HTTP server routine stopped")
	}()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Printf("Graceful shutdown complete")
}
