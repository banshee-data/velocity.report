package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	// "regexp"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
	"github.com/banshee-data/velocity.report/internal/units"

	// optional lidar integration
	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
)

var (
	fixtureMode  = flag.Bool("fixture", false, "Load fixture to local database")
	debugMode    = flag.Bool("debug", false, "Run in debug mode (enables debug output in reports)")
	listen       = flag.String("listen", ":8080", "Listen address")
	port         = flag.String("port", "/dev/ttySC1", "Serial port to use")
	unitsFlag    = flag.String("units", "mph", "Speed units for display (mps, mph, kmph)")
	timezoneFlag = flag.String("timezone", "UTC", "Timezone for display (UTC, US/Eastern, US/Pacific, etc.)")
	disableRadar = flag.Bool("disable-radar", false, "Disable radar serial port (serve DB only)")
	dbPathFlag   = flag.String("db-path", "sensor_data.db", "path to sqlite DB file (defaults to sensor_data.db)")
)

// Lidar options (when enabling lidar via -enable-lidar)
var (
	enableLidar   = flag.Bool("enable-lidar", false, "Enable lidar components inside this radar binary")
	lidarListen   = flag.String("lidar-listen", ":8081", "HTTP listen address for lidar monitor (when enabled)")
	lidarUDPPort  = flag.Int("lidar-udp-port", 2369, "UDP port to listen for lidar packets")
	lidarNoParse  = flag.Bool("lidar-no-parse", false, "Disable lidar packet parsing when lidar is enabled")
	lidarSensor   = flag.String("lidar-sensor", "hesai-pandar40p", "Sensor name identifier for lidar background manager")
	lidarForward  = flag.Bool("lidar-forward", false, "Forward lidar UDP packets to another port")
	lidarFwdPort  = flag.Int("lidar-forward-port", 2368, "Port to forward lidar UDP packets to")
	lidarFwdAddr  = flag.String("lidar-forward-addr", "localhost", "Address to forward lidar UDP packets to")
	lidarPCAPMode = flag.Bool("lidar-pcap-mode", false, "Enable PCAP mode: disable UDP network listening, accept PCAP files via API for replay")
	// Background tuning knobs
	lidarBgFlushInterval = flag.Duration("lidar-bg-flush-interval", 60*time.Second, "Interval to flush background grid to database when reading PCAP")
	lidarBgNoiseRelative = flag.Float64("lidar-bg-noise-relative", 0.315, "Background NoiseRelativeFraction: fraction of range treated as expected measurement noise (e.g., 0.01 = 1%)")
	// FrameBuilder tuning knobs
	lidarFrameBufferTimeout = flag.Duration("lidar-frame-buffer-timeout", 500*time.Millisecond, "FrameBuilder buffer timeout: finalize idle frames after this duration")
	lidarMinFramePoints     = flag.Int("lidar-min-frame-points", 1000, "FrameBuilder MinFramePoints: minimum points required for a valid frame before finalizing")
	// Seed background from first observation (useful for PCAP replay and dev runs)
	// Default: true in this branch to re-enable the dev-friendly behavior; can be
	// disabled via CLI when running in production if desired.
	lidarSeedFromFirst = flag.Bool("lidar-seed-from-first", true, "Seed background cells from first observation (dev/pcap helper)")
)

// Constants
const SCHEMA_VERSION = "0.0.2"

// Main
func main() {
	flag.Parse()

	if *listen == "" {
		log.Fatal("Listen address is required")
	}
	if *port == "" {
		log.Fatal("Serial port is required")
	}
	if !units.IsValid(*unitsFlag) {
		log.Printf("Error: Invalid units '%s'. Valid options are: %s", *unitsFlag, units.GetValidUnitsString())
		os.Exit(1)
	}
	if !units.IsTimezoneValid(*timezoneFlag) {
		log.Printf("Error: Invalid timezone '%s'. Valid options are: %s", *timezoneFlag, units.GetValidTimezonesString())
		os.Exit(1)
	}

	// var r radar.RadarPortInterface
	var radarSerial serialmux.SerialMuxInterface

	// If disableRadar is set, provide a no-op serial mux implementation so
	// the HTTP admin routes and DB remain available while the device is
	// absent.
	if *disableRadar {
		radarSerial = serialmux.NewDisabledSerialMux()
	} else if *debugMode {
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

	// Use the CLI flag value (defaults to ./sensor_data.db). We intentionally
	// avoid relying on environment variables for configuration unless needed.
	db, err := db.NewDB(*dbPathFlag)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create a wait group for the HTTP server, serial monitor, and event handler routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Optionally initialize lidar components inside this binary
	if *enableLidar {
		// Use the main DB instance for lidar data (no separate lidar DB file)
		lidarDB := db

		// Create BackgroundManager and register persistence
		backgroundParams := lidar.BackgroundParams{
			BackgroundUpdateFraction:       0.02,
			ClosenessSensitivityMultiplier: 3.0,
			SafetyMarginMeters:             0.5,
			FreezeDurationNanos:            int64(5 * time.Second),
			NeighborConfirmationCount:      3,
			SettlingPeriodNanos:            int64(5 * time.Minute),
			SnapshotIntervalNanos:          int64(2 * time.Hour),
			ChangeThresholdForSnapshot:     100,
			NoiseRelativeFraction:          float32(*lidarBgNoiseRelative),
			// When running in PCAP mode / dev runs seed the background grid from first observations
			// so replayed captures can build an initial background without live warmup.
			SeedFromFirstObservation: *lidarSeedFromFirst,
		}

		backgroundManager := lidar.NewBackgroundManager(*lidarSensor, 40, 1800, backgroundParams, lidarDB)
		if backgroundManager != nil {
			log.Printf("BackgroundManager created and registered for sensor %s", *lidarSensor)
		}

		// Start periodic background grid flushing when a positive flush interval is configured.
		// Previously this only ran in PCAP mode; enable it in dev runs too so periodic
		// persisted snapshot logs appear when developers set the flag.
		if backgroundManager != nil && *lidarBgFlushInterval > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(*lidarBgFlushInterval)
				defer ticker.Stop()

				mode := "dev"
				if *lidarPCAPMode {
					mode = "pcap"
				}
				log.Printf("Background grid flush timer started (mode=%s): interval=%v", mode, *lidarBgFlushInterval)

				for {
					select {
					case <-ctx.Done():
						log.Printf("Background flush timer terminated")
						// Final flush before exit
						if err := backgroundManager.Persist(lidarDB, "final_flush"); err != nil {
							log.Printf("Error during final background flush: %v", err)
						} else {
							log.Printf("Final background grid flushed to database")
						}
						return
					case <-ticker.C:
						// Use a descriptive reason depending on mode to make logs clearer
						reason := "periodic_flush"
						if *lidarPCAPMode {
							reason = "periodic_pcap_flush"
						}
						if err := backgroundManager.Persist(lidarDB, reason); err != nil {
							log.Printf("Error flushing background grid: %v", err)
						} else {
							log.Printf("Background grid flushed to database")
						}
					}
				}
			}()
		}

		// Lidar parser and frame builder (optional)
		var parser *parse.Pandar40PParser
		var frameBuilder *lidar.FrameBuilder
		if !*lidarNoParse {
			config, err := parse.LoadEmbeddedPandar40PConfig()
			if err != nil {
				log.Fatalf("Failed to load embedded lidar configuration: %v", err)
			}
			if err := config.Validate(); err != nil {
				log.Fatalf("Invalid embedded lidar configuration: %v", err)
			}
			parser = parse.NewPandar40PParser(*config)
			parser.SetDebug(*debugMode)
			parse.ConfigureTimestampMode(parser)

			// Wire per-ring elevation corrections from parser config into BackgroundManager
			// This ensures background ASC exports use the same per-channel elevations as frames.
			if backgroundManager != nil {
				elev := parse.ElevationsFromConfig(config)
				if elev != nil {
					if err := backgroundManager.SetRingElevations(elev); err != nil {
						log.Printf("Failed to set ring elevations for background manager %s: %v", *lidarSensor, err)
					} else {
						log.Printf("BackgroundManager ring elevations set for sensor %s", *lidarSensor)
					}
				} else {
					log.Printf("No elevation corrections available for sensor %s; background export will use z=0 projection", *lidarSensor)
				}
			}

			// FrameBuilder callback: feed completed frames into BackgroundManager
			callback := func(frame *lidar.LiDARFrame) {
				if frame == nil || len(frame.Points) == 0 {
					return
				}
				if *debugMode {
					log.Printf("[FrameBuilder] Completed frame: %s, Points: %d, Azimuth: %.1f°-%.1f°", frame.FrameID, len(frame.Points), frame.MinAzimuth, frame.MaxAzimuth)
				}
				polar := make([]lidar.PointPolar, 0, len(frame.Points))
				for _, p := range frame.Points {
					polar = append(polar, lidar.PointPolar{
						Channel:     p.Channel,
						Azimuth:     p.Azimuth,
						Elevation:   p.Elevation,
						Distance:    p.Distance,
						Intensity:   p.Intensity,
						Timestamp:   p.Timestamp.UnixNano(),
						BlockID:     p.BlockID,
						UDPSequence: p.UDPSequence,
					})
				}
				if backgroundManager != nil {
					if *debugMode {
						// Provide extra context at the exact handoff so we can trace delivery
						var firstAz, lastAz float64
						var firstTS, lastTS int64
						if len(polar) > 0 {
							firstAz = polar[0].Azimuth
							lastAz = polar[len(polar)-1].Azimuth
							firstTS = polar[0].Timestamp
							lastTS = polar[len(polar)-1].Timestamp
						}
						log.Printf("[FrameBuilder->Background] Delivering frame %s -> %d points to BackgroundManager (azimuth: %.1f°->%.1f°, ts: %d->%d)", frame.FrameID, len(polar), firstAz, lastAz, firstTS, lastTS)
					}
					backgroundManager.ProcessFramePolar(polar)
				}
			}

			frameBuilder = lidar.NewFrameBuilder(lidar.FrameBuilderConfig{
				SensorID:      *lidarSensor,
				FrameCallback: callback,
				// Use CLI-configurable MinFramePoints and BufferTimeout so devs can tune
				MinFramePoints:  *lidarMinFramePoints,
				FrameBufferSize: 100,
				BufferTimeout:   *lidarFrameBufferTimeout,
				CleanupInterval: 250 * time.Millisecond,
			})
			// Enable lightweight frame-completion logging when in debug or PCAP mode
			if frameBuilder != nil {
				frameBuilder.SetDebug(*debugMode || *lidarPCAPMode)
			}
		}

		// Packet forwarding (optional)
		var packetForwarder *network.PacketForwarder
		// Create a PacketStats instance and wire it into the forwarder, listener and webserver
		packetStats := monitor.NewPacketStats()
		// Disable forwarding when in PCAP mode (no network UDP)
		if *lidarForward && !*lidarPCAPMode {
			createdForwarder, err := network.NewPacketForwarder(*lidarFwdAddr, *lidarFwdPort, packetStats, time.Minute)
			if err != nil {
				log.Printf("failed to create lidar forwarder: %v", err)
			} else {
				packetForwarder = createdForwarder
				defer packetForwarder.Close()
			}
		} else if *lidarForward && *lidarPCAPMode {
			log.Printf("Warning: Forwarding is disabled in PCAP mode (no network UDP)")
		}

		// Start UDP listener for lidar (or skip if in PCAP mode)
		var lidarUDPListener *network.UDPListener
		if !*lidarPCAPMode {
			udpAddr := fmt.Sprintf(":%d", *lidarUDPPort)
			lidarUDPListener = network.NewUDPListener(network.UDPListenerConfig{
				Address:        udpAddr,
				RcvBuf:         4 << 20,
				LogInterval:    time.Minute,
				Stats:          packetStats,
				Forwarder:      packetForwarder,
				Parser:         parser,
				FrameBuilder:   frameBuilder,
				DB:             lidarDB,
				DisableParsing: *lidarNoParse,
				UDPPort:        *lidarUDPPort,
			})

			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := lidarUDPListener.Start(ctx); err != nil && err != context.Canceled {
					log.Printf("Lidar UDP listener error: %v", err)
				}
				log.Print("lidar UDP listener terminated")
			}()
		} else {
			log.Printf("PCAP mode enabled: UDP network listening disabled, use API to trigger PCAP replay")
		}

		// Start lidar webserver for monitoring (moved into internal/api)
		// Provide a PacketStats instance if parsing/forwarding is enabled
		// Pass the same PacketStats instance to the webserver so it shows live stats
		lidarWebServer := monitor.NewWebServer(monitor.WebServerConfig{
			Address:           *lidarListen,
			Stats:             packetStats,
			ForwardingEnabled: *lidarForward && !*lidarPCAPMode,
			ForwardAddr:       *lidarFwdAddr,
			ForwardPort:       *lidarFwdPort,
			ParsingEnabled:    !*lidarNoParse,
			UDPPort:           *lidarUDPPort,
			DB:                lidarDB,
			SensorID:          *lidarSensor,
			Parser:            parser,
			FrameBuilder:      frameBuilder,
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lidarWebServer.Start(ctx); err != nil {
				log.Printf("Lidar webserver error: %v", err)
			}
		}()
	}

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
				if err := serialmux.HandleEvent(db, payload); err != nil {
					log.Printf("error handling event: %v", err)
				}
			case <-ctx.Done():
				log.Printf("subscribe routine terminated")
				return
			}
		}
	}()

	// HTTP server goroutine: construct an api.Server and delegate run/shutdown to it
	wg.Add(1)
	go func() {
		defer wg.Done()
		apiServer := api.NewServer(radarSerial, db, *unitsFlag, *timezoneFlag)

		// Attach admin routes that belong to other components
		// (these modify the mux returned by apiServer.ServeMux internally)
		mux := apiServer.ServeMux()
		radarSerial.AttachAdminRoutes(mux)
		db.AttachAdminRoutes(mux)

		if err := apiServer.Start(ctx, *listen, *debugMode); err != nil {
			// If ctx was canceled we expect nil or context.Canceled; log other errors
			if err != context.Canceled {
				log.Printf("HTTP server error: %v", err)
			}
		}
	}()

	// Wait for all goroutines to finish
	wg.Wait()
	log.Printf("Graceful shutdown complete")
}
