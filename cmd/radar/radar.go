package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	// "regexp"

	_ "modernc.org/sqlite"

	radar "github.com/banshee-data/velocity.report"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

var (
	fixtureMode = flag.Bool("fixture", false, "Load fixture to local database")
	devMode     = flag.Bool("dev", false, "Run in dev mode")
	listen      = flag.String("listen", ":8080", "Listen address")
	port        = flag.String("port", "/dev/ttySC1", "Serial port to use (ignored in dev mode)")
	units       = flag.String("units", "mph", "Speed units for display (mps, mph, kmph)")
)

// Constants
const DB_FILE = "sensor_data.db"
const SCHEMA_VERSION = "0.0.2"

func handleRadarObject(d *db.DB, payload string) error {
	log.Printf("Raw RadarObject Line: %+v", payload)

	// log to the database and return error if present
	return d.RecordRadarObject(payload)
}

func handleRawData(d *db.DB, payload string) error {
	log.Printf("Raw Data Line: %+v", payload)

	// TODO: disable via flag/config
	return d.RecordRawData(payload)
}

var CurrentState map[string]any

func handleConfigResponse(payload string) error {
	var configValues map[string]any

	if err := json.Unmarshal([]byte(payload), &configValues); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// update the current state with the new config values
	for k, v := range configValues {
		if CurrentState == nil {
			CurrentState = make(map[string]any)
		}
		CurrentState[k] = v
	}

	// log the current line
	log.Printf("Config Line: %+v", payload)

	return nil
}

func handleEvent(db *db.DB, payload string) error {
	if strings.Contains(payload, "end_time") || strings.Contains(payload, "classifier") {
		// This is a rollup event
		if err := handleRadarObject(db, payload); err != nil {
			return fmt.Errorf("failed to handle RadarObject event: %v", err)
		}
	} else if strings.Contains(payload, `magnitude`) || strings.Contains(payload, `speed`) {
		// This is a raw data event
		handleRawData(db, payload)
	} else if strings.HasPrefix(payload, `{`) {
		// This is a config response
		if err := handleConfigResponse(payload); err != nil {
			return fmt.Errorf("failed to handle config response: %v", err)
		}
	} else {
		// Unknown event type
		log.Printf("unknown event type: %s", payload)
	}
	return nil
}

// Main
func main() {
	flag.Parse()

	if *listen == "" {
		log.Fatal("Listen address is required")
	}
	if *port == "" {
		log.Fatal("Serial port is required")
	}
	if *units != "mps" && *units != "mph" && *units != "kmph" && *units != "kph" {
		log.Fatal("Units must be one of: mps, mph, kmph, kph")
	}

	// var r radar.RadarPortInterface
	var radarSerial serialmux.SerialMuxInterface
	if *devMode {
		radarSerial = serialmux.NewMockSerialMux([]byte(""))
	} else if *fixtureMode {
		data, err := os.ReadFile("fixtures.txt")
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		firstLine := lines[0]
		if err != nil {
			log.Fatalf("failed to open fixtures file: %v", err)
		}
		radarSerial = serialmux.NewMockSerialMux([]byte(firstLine + "\n"))
	} else {
		var err error
		radarSerial, err = serialmux.NewRealSerialMux(*port)
		if err != nil {
			log.Fatalf("failed to create radar port: %v", err)
		}
	}
	defer radarSerial.Close()

	if err := radarSerial.Initialize(); err != nil {
		log.Fatalf("failed to initialise device: %v", err)
	} else {
		log.Printf("initialised device %s", radarSerial)
	}

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
		if err := radarSerial.Monitor(ctx); err != nil && err != context.Canceled {
			log.Printf("failed to monitor serial port: %v", err)
		}
		log.Print("monitor routine terminated")
	}()

	// subscribe to the serial port messages
	// and pass them to event handler
	wg.Add(1)
	go func() {
		defer wg.Done()
		id, c := radarSerial.Subscribe()
		defer radarSerial.Unsubscribe(id)
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

		// create a new API server instance using the radar port and database
		// and mount the API handlers
		apiServer := api.NewServer(radarSerial, db, *units)
		mux := apiServer.ServeMux()

		radarSerial.AttachAdminRoutes(mux)
		db.AttachAdminRoutes(mux)

		// read static files from the embedded filesystem in production or from
		// the local ./static in dev for easier iteration without restarting the
		// server
		var staticHandler http.Handler
		if *devMode {
			staticHandler = http.FileServer(http.Dir("./static"))
		} else {
			staticHandler = http.FileServer(http.FS(radar.StaticFiles))
		}

		mux.Handle("/favicon.ico", staticHandler)

		// serve frontend app from /app route
		// In dev mode, check build directory exists
		if *devMode {
			buildDir := "./web/build"
			if _, err := os.Stat(buildDir); os.IsNotExist(err) {
				log.Fatalf("Build directory %s does not exist. Run 'cd web && pnpm run build' first.", buildDir)
			}
		}

		// Unified frontend handler that works for both dev and production
		mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
			// Strip /app prefix and normalize path
			path := strings.TrimPrefix(r.URL.Path, "/app")
			if path == "" || path == "/" {
				path = "/index.html"
			}

			// Redirect trailing slash URLs to non-trailing slash for consistent relative path resolution
			if len(path) > 1 && strings.HasSuffix(path, "/") {
				redirectURL := strings.TrimSuffix(r.URL.Path, "/")
				if r.URL.RawQuery != "" {
					redirectURL += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}

			// Helper function to serve file content
			serveContent := func(content []byte, filename string) {
				http.ServeContent(w, r, filename, time.Time{}, strings.NewReader(string(content)))
			}

			// Helper function to try serving a file from filesystem or embedded FS
			tryServeFile := func(requestedPath string) bool {
				if *devMode {
					// Dev mode: serve from filesystem
					fullPath := filepath.Join("./web/build", requestedPath)
					if _, err := os.Stat(fullPath); err == nil {
						http.ServeFile(w, r, fullPath)
						return true
					}
				} else {
					// Production mode: serve from embedded filesystem
					embedPath := filepath.Join("web/build", strings.TrimPrefix(requestedPath, "/"))
					if content, err := radar.WebBuildFiles.ReadFile(embedPath); err == nil {
						serveContent(content, requestedPath)
						return true
					}
				}
				return false
			}

			// Try to serve the requested file directly first
			if tryServeFile(path) {
				return
			}

			// File doesn't exist, try with .html extension for prerendered routes
			if !strings.HasSuffix(path, ".html") {
				htmlPath := path + ".html"
				if tryServeFile(htmlPath) {
					return
				}
			}

			// Fall back to index.html for SPA routing
			if !tryServeFile("/index.html") {
				http.NotFound(w, r)
			}
		})
		// redirect root to /app
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/app/", http.StatusFound)
				return
			}
			http.NotFound(w, r)
		})

		server := &http.Server{
			Addr:    *listen,
			Handler: api.LoggingMiddleware(mux),
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

		// Create a shutdown context with a shorter timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
			// Force close the server if graceful shutdown fails
			if err := server.Close(); err != nil {
				log.Printf("HTTP server force close error: %v", err)
			}
		}

		log.Printf("HTTP server routine stopped")
	}()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Printf("Graceful shutdown complete")
}
