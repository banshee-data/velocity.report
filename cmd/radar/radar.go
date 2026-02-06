package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
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

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
	"github.com/banshee-data/velocity.report/internal/units"

	// optional lidar integration
	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/version"
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
	versionFlag  = flag.Bool("version", false, "Print version information and exit")
	versionShort = flag.Bool("v", false, "Print version information and exit (shorthand)")
	configFile   = flag.String("config", "", "Path to JSON tuning configuration file (overrides individual tuning flags)")
)

// Lidar options (when enabling lidar via -enable-lidar)
var (
	enableLidar    = flag.Bool("enable-lidar", false, "Enable lidar components inside this radar binary")
	lidarListen    = flag.String("lidar-listen", ":8081", "HTTP listen address for lidar monitor (when enabled)")
	lidarUDPPort   = flag.Int("lidar-udp-port", 2369, "UDP port to listen for lidar packets")
	lidarNoParse   = flag.Bool("lidar-no-parse", false, "Disable lidar packet parsing when lidar is enabled")
	lidarSensor    = flag.String("lidar-sensor", "hesai-pandar40p", "Sensor name identifier for lidar background manager")
	lidarForward   = flag.Bool("lidar-forward", false, "Forward lidar UDP packets to another port")
	lidarFwdPort   = flag.Int("lidar-forward-port", 2368, "Port to forward lidar UDP packets to")
	lidarFwdAddr   = flag.String("lidar-forward-addr", "localhost", "Address to forward lidar UDP packets to")
	lidarFGForward = flag.Bool("lidar-foreground-forward", false, "Forward foreground-only LiDAR packets to a separate port (e.g., 2370)")
	lidarFGFwdPort = flag.Int("lidar-foreground-forward-port", 2370, "Port to forward foreground LiDAR packets to")
	lidarFGFwdAddr = flag.String("lidar-foreground-forward-addr", "localhost", "Address to forward foreground LiDAR packets to")
	lidarPCAPDir   = flag.String("lidar-pcap-dir", "../sensor_data/lidar", "Safe directory for PCAP files (only files within this directory can be replayed)")
	// Visualiser gRPC streaming (M2)
	lidarForwardMode = flag.String("lidar-forward-mode", "lidarview", "Forward mode: lidarview (UDP only), grpc (gRPC only), or both (UDP + gRPC)")
	lidarGRPCListen  = flag.String("lidar-grpc-listen", "localhost:50051", "gRPC server listen address for visualiser streaming")
)

// Transit worker options (compute radar_data -> radar_data_transits)
var (
	enableTransitWorker    = flag.Bool("enable-transit-worker", true, "Enable transit worker to periodically compute transits from radar_data")
	transitWorkerInterval  = flag.Duration("transit-worker-interval", 1*time.Hour, "Interval for transit worker (e.g., 1h)")
	transitWorkerWindow    = flag.Duration("transit-worker-window", 65*time.Minute, "Lookback window for transit worker (should be slightly larger than interval)")
	transitWorkerThreshold = flag.Int("transit-worker-threshold", 1, "Gap threshold in seconds for sessionizing transits")
	transitWorkerModel     = flag.String("transit-worker-model", "hourly-cron", "Model version string for computed transits")
)

// Constants
const SCHEMA_VERSION = "0.0.2"

// Main
func main() {
	flag.Parse()

	// Configure logging: default to stdout; optionally tee to a debug log file via env.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	var debugLogFile *os.File
	if debugPath := os.Getenv("VELOCITY_DEBUG_LOG"); debugPath != "" {
		if err := os.MkdirAll(filepath.Dir(debugPath), 0o755); err == nil {
			if f, err := os.OpenFile(debugPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				debugLogFile = f
				lidar.SetDebugLogger(f)
			} else {
				log.Printf("warning: failed to open debug log %s: %v", debugPath, err)
			}
		} else {
			log.Printf("warning: failed to create debug log directory for %s", debugPath)
		}
	}
	if debugLogFile != nil {
		defer debugLogFile.Close()
	}

	// Handle version flags (-v, --version)
	if *versionFlag || *versionShort {
		fmt.Printf("velocity-report v%s (git SHA: %s)\n", version.Version, version.GitSHA)
		os.Exit(0)
	}

	// Check if first argument is a subcommand
	if flag.NArg() > 0 {
		subcommand := flag.Arg(0)
		if subcommand == "version" {
			fmt.Printf("velocity-report v%s\n", version.Version)
			fmt.Printf("git SHA: %s\n", version.GitSHA)
			os.Exit(0)
		}
		if subcommand == "migrate" {
			// Re-parse flags after "migrate" subcommand to allow:
			//   velocity-report migrate up --db-path /custom.db
			// or:
			//   velocity-report --db-path /custom.db migrate up
			//
			// flag.Parse() stops at first non-flag arg, so flags after "migrate"
			// weren't parsed. Create new FlagSet to parse remaining args.
			migrateFlags := flag.NewFlagSet("migrate", flag.ExitOnError)
			migrateDBPath := migrateFlags.String("db-path", *dbPathFlag, "path to sqlite DB file")

			// Parse flags from args after "migrate"
			remainingArgs := flag.Args()[1:] // Everything after "migrate"
			if err := migrateFlags.Parse(remainingArgs); err != nil {
				log.Fatalf("Failed to parse migrate flags: %v", err)
			}

			// Pass positional args (non-flag args after parsing) to migrate command
			db.RunMigrateCommand(migrateFlags.Args(), *migrateDBPath)
			return
		}
		if subcommand == "transits" {
			runTransitsCommand(flag.Args()[1:])
			return
		}
		log.Fatalf("Unknown subcommand: %s", subcommand)
	}

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

	// Load tuning configuration from file if specified.
	// Deferred until after subcommand dispatch so commands like migrate/transits
	// don't require a valid tuning config.
	var tuningCfg *config.TuningConfig
	if *configFile != "" {
		var err error
		tuningCfg, err = config.LoadTuningConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load tuning config from %s: %v", *configFile, err)
		}
		log.Printf("Loaded tuning configuration from %s", *configFile)
	} else {
		tuningCfg = config.DefaultTuningConfig()
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

	if err := radarSerial.Initialise(); err != nil {
		log.Fatalf("failed to initialise device: %v", err)
	} else {
		log.Printf("initialised device %s", radarSerial)
	}

	// Log version and git SHA on startup
	log.Printf("velocity-report v%s (git SHA: %s)", version.Version, version.GitSHA)

	// Use the CLI flag value (defaults to ./sensor_data.db). We intentionally
	// avoid relying on environment variables for configuration unless needed.
	database, err := db.NewDB(*dbPathFlag)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Create a wait group for the HTTP server, serial monitor, and event handler routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Lidar webserver instance (if enabled)
	var lidarWebServer *monitor.WebServer
	var foregroundForwarder *network.ForegroundForwarder
	var bgFlusher *lidar.BackgroundFlusher

	// Optionally initialize lidar components inside this binary
	if *enableLidar {
		// Use the main DB instance for lidar data (no separate lidar DB file)
		lidarDB := database

		// Always use tuning config (either from --config file or built-in defaults)
		bgNoiseRelative := tuningCfg.GetNoiseRelative()
		bgFlushInterval := tuningCfg.GetFlushInterval()
		bgFlushDisable := tuningCfg.GetFlushDisable()
		seedFromFirst := tuningCfg.GetSeedFromFirst()
		frameBufferTimeout := tuningCfg.GetBufferTimeout()
		minFramePoints := tuningCfg.GetMinFramePoints()

		// Create BackgroundManager using BackgroundConfig for cleaner configuration
		bgConfig := lidar.DefaultBackgroundConfig().
			WithNoiseRelativeFraction(float32(bgNoiseRelative)).
			WithSeedFromFirstObservation(seedFromFirst)

		backgroundManager := lidar.NewBackgroundManager(*lidarSensor, 40, 1800, bgConfig.ToBackgroundParams(), lidarDB)
		if backgroundManager != nil {
			log.Printf("BackgroundManager created and registered for sensor %s", *lidarSensor)
		}

		// Start periodic background grid flushing using BackgroundFlusher
		// Skip if explicitly disabled (--lidar-bg-flush-disable) or interval is zero
		if backgroundManager != nil && bgFlushInterval > 0 && !bgFlushDisable {
			bgFlusher = lidar.NewBackgroundFlusher(lidar.BackgroundFlusherConfig{
				Manager:  backgroundManager,
				Store:    lidarDB,
				Interval: bgFlushInterval,
				Reason:   "periodic_flush",
			})
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := bgFlusher.Run(ctx); err != nil {
					log.Printf("Background flusher error: %v", err)
				}
			}()
		}

		// Lidar parser and frame builder (optional)
		var parser *parse.Pandar40PParser
		var frameBuilder *lidar.FrameBuilder
		var tracker *lidar.Tracker
		var classifier *lidar.TrackClassifier
		var visualiserServer *visualiser.Server // Hoisted so WebServerConfig callbacks can reference it

		// Optional foreground-only forwarder (Pandar40-compatible) for live mode
		if *lidarFGForward {
			fg, err := network.NewForegroundForwarder(*lidarFGFwdAddr, *lidarFGFwdPort, nil)
			if err != nil {
				log.Printf("failed to create foreground forwarder: %v", err)
			} else {
				foregroundForwarder = fg
				foregroundForwarder.Start(ctx)
				defer foregroundForwarder.Close()
				log.Printf("Foreground forwarder enabled to %s:%d", *lidarFGFwdAddr, *lidarFGFwdPort)
			}
		}

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

			// Initialise tracking components
			tracker = lidar.NewTracker(lidar.DefaultTrackerConfig())
			classifier = lidar.NewTrackClassifier()
			log.Printf("Tracker and classifier initialized for sensor %s", *lidarSensor)

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

			// Initialise visualiser components if gRPC mode is enabled
			var visualiserPublisher *visualiser.Publisher
			var frameAdapter *visualiser.FrameAdapter
			var lidarViewAdapter *visualiser.LidarViewAdapter

			// Validate forward mode
			forwardMode := *lidarForwardMode
			validModes := map[string]bool{"lidarview": true, "grpc": true, "both": true}
			if !validModes[forwardMode] {
				log.Fatalf("Invalid --lidar-forward-mode: %s (must be: lidarview, grpc, or both)", forwardMode)
			}

			// Initialise gRPC publisher if needed
			if forwardMode == "grpc" || forwardMode == "both" {
				vizConfig := visualiser.DefaultConfig()
				vizConfig.ListenAddr = *lidarGRPCListen
				vizConfig.SensorID = *lidarSensor
				vizConfig.EnableDebug = *debugMode
				vizConfig.MaxClients = 5
				visualiserPublisher = visualiser.NewPublisher(vizConfig)
				visualiserServer = visualiser.NewServer(visualiserPublisher)

				if err := visualiserPublisher.Start(); err != nil {
					log.Fatalf("Failed to start visualiser publisher: %v", err)
				}
				defer visualiserPublisher.Stop()

				// Register gRPC service (must happen after Start() to ensure GRPCServer is initialised)
				visualiser.RegisterService(visualiserPublisher.GRPCServer(), visualiserServer)

				frameAdapter = visualiser.NewFrameAdapter(*lidarSensor)

				// Wire M3.5 split streaming: connect background manager to publisher
				// so that background snapshots are sent periodically instead of
				// embedding the full point cloud in every frame (~96% bandwidth reduction).
				if backgroundManager != nil {
					visualiserPublisher.SetBackgroundManager(
						&backgroundManagerBridge{mgr: backgroundManager},
					)
					frameAdapter.SplitStreaming = true
					log.Printf("Visualiser background split streaming enabled (interval=%s)", vizConfig.BackgroundInterval)
				}

				log.Printf("Visualiser gRPC server started on %s", *lidarGRPCListen)
			}

			// Initialise LidarView adapter for UDP forwarding if needed
			if forwardMode == "lidarview" || forwardMode == "both" {
				if foregroundForwarder != nil {
					lidarViewAdapter = visualiser.NewLidarViewAdapter(foregroundForwarder)
					log.Printf("LidarView adapter enabled (forwarding to %s:%d)", *lidarFGFwdAddr, *lidarFGFwdPort)
				}
			}

			// Create tracking pipeline callback with all necessary dependencies
			pipelineConfig := &lidar.TrackingPipelineConfig{
				BackgroundManager:   backgroundManager,
				FgForwarder:         foregroundForwarder,
				Tracker:             tracker,
				Classifier:          classifier,
				DB:                  lidarDB.DB, // Pass underlying sql.DB to avoid import cycle
				SensorID:            *lidarSensor,
				DebugMode:           *debugMode,
				VisualiserPublisher: visualiserPublisher,
				VisualiserAdapter:   frameAdapter,
				LidarViewAdapter:    lidarViewAdapter,
				MaxFrameRate:        12, // Prevent PCAP catch-up bursts from flooding the pipeline
			}
			callback := pipelineConfig.NewFrameCallback()

			frameBuilder = lidar.NewFrameBuilder(lidar.FrameBuilderConfig{
				SensorID:      *lidarSensor,
				FrameCallback: callback,
				// Use CLI-configurable MinFramePoints and BufferTimeout so devs can tune
				MinFramePoints:  minFramePoints,
				FrameBufferSize: 100,
				BufferTimeout:   frameBufferTimeout,
				CleanupInterval: 250 * time.Millisecond,
			})
			// Enable lightweight frame-completion logging only when --debug is set.
			// PCAP mode no longer forces debug logging so operators can choose verbosity.
			if frameBuilder != nil {
				frameBuilder.SetDebug(*debugMode)
			}
		}

		// Packet forwarding (optional)
		var packetForwarder *network.PacketForwarder
		// Create a PacketStats instance and wire it into the forwarder, listener and webserver
		packetStats := monitor.NewPacketStats()
		if *lidarForward {
			createdForwarder, err := network.NewPacketForwarder(*lidarFwdAddr, *lidarFwdPort, packetStats, time.Minute)
			if err != nil {
				log.Printf("failed to create lidar forwarder: %v", err)
			} else {
				packetForwarder = createdForwarder
				defer packetForwarder.Close()
			}
		}

		udpAddr := fmt.Sprintf(":%d", *lidarUDPPort)
		udpListenerConfig := network.UDPListenerConfig{
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
		}

		// Start lidar webserver for monitoring (moved into internal/api)
		// Provide a PacketStats instance if parsing/forwarding is enabled
		// Pass the same PacketStats instance to the webserver so it shows live stats
		lidarWebServer = monitor.NewWebServer(monitor.WebServerConfig{
			Address:           *lidarListen,
			Stats:             packetStats,
			ForwardingEnabled: *lidarForward,
			ForwardAddr:       *lidarFwdAddr,
			ForwardPort:       *lidarFwdPort,
			ParsingEnabled:    !*lidarNoParse,
			UDPPort:           *lidarUDPPort,
			DB:                lidarDB,
			SensorID:          *lidarSensor,
			Parser:            parser,
			FrameBuilder:      frameBuilder,
			PCAPSafeDir:       *lidarPCAPDir,
			PacketForwarder:   packetForwarder,
			UDPListenerConfig: udpListenerConfig,
			PlotsBaseDir:      filepath.Join(*lidarPCAPDir, "plots"),
			OnPCAPStarted: func() {
				if visualiserServer != nil {
					visualiserServer.SetReplayMode(true)
					log.Printf("[Visualiser] PCAP started — switched to replay mode")
				}
			},
			OnPCAPStopped: func() {
				if visualiserServer != nil {
					visualiserServer.SetReplayMode(false)
					log.Printf("[Visualiser] PCAP stopped — switched to live mode")
				}
			},
			OnPCAPProgress: func(current, total uint64) {
				if visualiserServer != nil {
					visualiserServer.SetPCAPProgress(current, total)
				}
			},
		})
		// Wire tracker for in-memory config access via /api/lidar/params
		if tracker != nil {
			lidarWebServer.SetTracker(tracker)
		}
		// Create and wire sweep runner for web-triggered parameter sweeps
		httpClient := &http.Client{Timeout: 30 * time.Second}
		// Construct base URL for loopback communication
		// Normalize wildcard hosts (0.0.0.0, ::, "") to localhost for client connectivity
		baseURL := "http://localhost" + *lidarListen
		if !strings.HasPrefix(*lidarListen, ":") {
			host, port, err := net.SplitHostPort(*lidarListen)
			if err == nil {
				// Normalize wildcard/unspecified hosts to localhost
				if host == "" || host == "0.0.0.0" || host == "::" {
					baseURL = "http://localhost:" + port
				} else {
					baseURL = "http://" + *lidarListen
				}
			} else {
				// If SplitHostPort fails, use the address as-is (backward compatibility)
				baseURL = "http://" + *lidarListen
			}
		}
		sweepClient := monitor.NewClient(httpClient, baseURL, *lidarSensor)
		sweepRunner := sweep.NewRunner(sweepClient)
		lidarWebServer.SetSweepRunner(sweepRunner)
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
				if err := serialmux.HandleEvent(database, payload); err != nil {
					log.Printf("error handling event: %v", err)
				}
			case <-ctx.Done():
				log.Printf("subscribe routine terminated")
				return
			}
		}
	}()

	// Create transit worker controller before HTTP server so we can pass it to the API
	// Always create the controller so the API can provide UI controls
	transitWorker := db.NewTransitWorker(database, *transitWorkerThreshold, *transitWorkerModel)
	transitWorker.Interval = *transitWorkerInterval
	transitWorker.Window = *transitWorkerWindow
	transitController := db.NewTransitController(transitWorker)

	// Only start the worker goroutine if enabled via CLI flag
	if *enableTransitWorker {
		log.Printf("Starting transit worker: interval=%v, window=%v, threshold=%ds, model=%s",
			transitWorker.Interval, transitWorker.Window, *transitWorkerThreshold, *transitWorkerModel)

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := transitController.Run(ctx); err != nil && err != context.Canceled {
				log.Printf("Transit worker error: %v", err)
			}
		}()
	} else {
		log.Printf("Transit worker not started (use --enable-transit-worker to enable)")
	}

	// HTTP server goroutine: construct an api.Server and delegate run/shutdown to it
	wg.Add(1)
	go func() {
		defer wg.Done()
		apiServer := api.NewServer(radarSerial, database, *unitsFlag, *timezoneFlag)
		// Set the transit controller so API can provide UI controls
		apiServer.SetTransitController(transitController)

		// Attach admin routes that belong to other components
		// (these modify the mux returned by apiServer.ServeMux internally)
		mux := apiServer.ServeMux()
		radarSerial.AttachAdminRoutes(mux)
		database.AttachAdminRoutes(mux)

		// Attach Lidar routes if enabled
		if lidarWebServer != nil {
			lidarWebServer.RegisterRoutes(mux)
		}

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

// runTransitsCommand handles transit-related subcommands:
//   - transits analyse: Show transit statistics and overlaps
//   - transits delete <model-version>: Delete all transits for a model version
//   - transits migrate <from-version> <to-version>: Migrate transits from one model version to another
//   - transits rebuild: Delete all transits and rebuild from full history
func runTransitsCommand(args []string) {
	transitFlags := flag.NewFlagSet("transits", flag.ExitOnError)
	transitDBPath := transitFlags.String("db-path", *dbPathFlag, "path to sqlite DB file")
	transitModel := transitFlags.String("model", "hourly-cron", "model version for transit worker")
	transitThreshold := transitFlags.Int("threshold", 1, "gap threshold in seconds for sessionizing transits")

	if err := transitFlags.Parse(args); err != nil {
		log.Fatalf("Failed to parse transits flags: %v", err)
	}

	// Open database without migration check for CLI commands
	database, err := db.OpenDB(*transitDBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create CLI handler
	cli := db.NewTransitCLI(database, *transitModel, *transitThreshold, os.Stdout)

	if transitFlags.NArg() < 1 {
		cli.PrintUsage()
		fmt.Println("Options:")
		transitFlags.PrintDefaults()
		os.Exit(1)
	}

	ctx := context.Background()
	subCmd := transitFlags.Arg(0)

	switch subCmd {
	case "analyse", "analyze":
		if _, err := cli.Analyse(ctx); err != nil {
			log.Fatalf("Failed to analyse transits: %v", err)
		}

	case "delete":
		if transitFlags.NArg() < 2 {
			log.Fatal("Usage: velocity-report transits delete <model-version>")
		}
		modelVersion := transitFlags.Arg(1)

		// Confirm deletion
		fmt.Printf("This will delete all transits with model_version = %q\n", modelVersion)
		fmt.Print("Are you sure? [y/N]: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}

		if _, err := cli.Delete(ctx, modelVersion); err != nil {
			log.Fatalf("Failed to delete transits: %v", err)
		}

	case "migrate":
		if transitFlags.NArg() < 3 {
			log.Fatal("Usage: velocity-report transits migrate <from-version> <to-version>")
		}
		fromVersion := transitFlags.Arg(1)
		toVersion := transitFlags.Arg(2)

		fmt.Printf("This will:\n")
		fmt.Printf("  1. Delete all transits with model_version = %q\n", fromVersion)
		fmt.Printf("  2. Re-process full radar_data history with model_version = %q\n", toVersion)
		fmt.Print("Are you sure? [y/N]: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}

		if err := cli.Migrate(ctx, fromVersion, toVersion); err != nil {
			log.Fatalf("Failed to migrate transits: %v", err)
		}

	case "rebuild":
		fmt.Printf("This will:\n")
		fmt.Printf("  1. Delete all existing transits with model_version = %q\n", *transitModel)
		fmt.Printf("  2. Re-process full radar_data history\n")
		fmt.Print("Are you sure? [y/N]: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}

		if err := cli.Rebuild(ctx); err != nil {
			log.Fatalf("Failed to rebuild transits: %v", err)
		}

	default:
		log.Fatalf("Unknown transits subcommand: %s", subCmd)
	}
}

// backgroundManagerBridge adapts *lidar.BackgroundManager to satisfy
// visualiser.BackgroundManagerInterface, converting between the two
// package-specific snapshot types. This avoids a circular import between
// the lidar and visualiser packages.
type backgroundManagerBridge struct {
	mgr *lidar.BackgroundManager
}

func (b *backgroundManagerBridge) GenerateBackgroundSnapshot() (interface{}, error) {
	data, err := b.mgr.GenerateBackgroundSnapshot()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	// Convert *lidar.BackgroundSnapshotData → *visualiser.BackgroundSnapshot
	return &visualiser.BackgroundSnapshot{
		SequenceNumber: data.SequenceNumber,
		TimestampNanos: data.TimestampNanos,
		X:              data.X,
		Y:              data.Y,
		Z:              data.Z,
		Confidence:     data.Confidence,
		GridMetadata: visualiser.GridMetadata{
			Rings:            data.Rings,
			AzimuthBins:      data.AzimuthBins,
			RingElevations:   data.RingElevations,
			SettlingComplete: data.SettlingComplete,
		},
	}, nil
}

func (b *backgroundManagerBridge) GetBackgroundSequenceNumber() uint64 {
	return b.mgr.GetBackgroundSequenceNumber()
}
