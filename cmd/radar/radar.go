package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
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
	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/monitor"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
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
	pdfLatexFlow = flag.String("pdf-latex-flow", "inherit", "PDF LaTeX flow: inherit, precompiled, or full")
	pdfTexRoot   = flag.String("pdf-tex-root", "", "TeX root for precompiled PDF flow (sets VELOCITY_TEX_ROOT)")
	versionFlag  = flag.Bool("version", false, "Print version information and exit")
	versionShort = flag.Bool("v", false, "Print version information and exit (shorthand)")
	configFile   = flag.String("config", config.DefaultConfigPath, "Path to JSON tuning configuration file")
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

const (
	pdfLaTeXFlowInherit     = "inherit"
	pdfLaTeXFlowPrecompiled = "precompiled"
	pdfLaTeXFlowFull        = "full"
)

func resolvePrecompiledTeXRoot(rawRoot string) (string, error) {
	texRoot := strings.TrimSpace(rawRoot)
	if texRoot == "" {
		return "", fmt.Errorf("empty TeX root")
	}

	absRoot, err := filepath.Abs(texRoot)
	if err != nil {
		return "", fmt.Errorf("resolve TeX root %q: %w", texRoot, err)
	}

	rootInfo, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("stat TeX root %q: %w", absRoot, err)
	}
	if !rootInfo.IsDir() {
		return "", fmt.Errorf("TeX root %q is not a directory", absRoot)
	}

	compilerPath := filepath.Join(absRoot, "bin", "xelatex")
	compilerInfo, err := os.Stat(compilerPath)
	if err != nil {
		return "", fmt.Errorf("missing compiler at %q: %w", compilerPath, err)
	}
	if compilerInfo.IsDir() {
		return "", fmt.Errorf("compiler path %q is a directory", compilerPath)
	}
	if compilerInfo.Mode()&0o111 == 0 {
		return "", fmt.Errorf("compiler not executable at %q", compilerPath)
	}

	return absRoot, nil
}

func configurePDFLaTeXFlow(flow, texRootFlag string) error {
	normalizedFlow := strings.ToLower(strings.TrimSpace(flow))
	if normalizedFlow == "" {
		normalizedFlow = pdfLaTeXFlowInherit
	}

	switch normalizedFlow {
	case pdfLaTeXFlowInherit:
		if strings.TrimSpace(texRootFlag) == "" {
			return nil
		}
		resolvedRoot, err := resolvePrecompiledTeXRoot(texRootFlag)
		if err != nil {
			return err
		}
		if err := os.Setenv("VELOCITY_TEX_ROOT", resolvedRoot); err != nil {
			return fmt.Errorf("set VELOCITY_TEX_ROOT: %w", err)
		}
		log.Printf("PDF LaTeX flow: inherit (explicit VELOCITY_TEX_ROOT=%s)", resolvedRoot)
		return nil
	case pdfLaTeXFlowPrecompiled:
		texRoot := strings.TrimSpace(texRootFlag)
		if texRoot == "" {
			texRoot = strings.TrimSpace(os.Getenv("VELOCITY_TEX_ROOT"))
		}
		if texRoot == "" {
			return fmt.Errorf("--pdf-latex-flow=precompiled requires --pdf-tex-root or VELOCITY_TEX_ROOT")
		}
		resolvedRoot, err := resolvePrecompiledTeXRoot(texRoot)
		if err != nil {
			return err
		}
		if err := os.Setenv("VELOCITY_TEX_ROOT", resolvedRoot); err != nil {
			return fmt.Errorf("set VELOCITY_TEX_ROOT: %w", err)
		}
		log.Printf("PDF LaTeX flow: precompiled (VELOCITY_TEX_ROOT=%s)", resolvedRoot)
		return nil
	case pdfLaTeXFlowFull:
		if err := os.Unsetenv("VELOCITY_TEX_ROOT"); err != nil {
			return fmt.Errorf("unset VELOCITY_TEX_ROOT: %w", err)
		}
		log.Printf("PDF LaTeX flow: full (system TeX)")
		return nil
	default:
		return fmt.Errorf(
			"invalid --pdf-latex-flow=%q (valid: %s, %s, %s)",
			flow,
			pdfLaTeXFlowInherit,
			pdfLaTeXFlowPrecompiled,
			pdfLaTeXFlowFull,
		)
	}
}

// Main
func main() {
	flag.Parse()

	// Configure logging: default to stdout; optionally tee to a debug log file via env.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	// Three-stream LiDAR logging: VELOCITY_LIDAR_{OPS,DEBUG,TRACE}_LOG env vars.
	// Falls back to legacy VELOCITY_DEBUG_LOG (all streams to one file) when
	// the new vars are not set.
	var logFiles []*os.File
	opsPath := os.Getenv("VELOCITY_LIDAR_OPS_LOG")
	lidarDebugPath := os.Getenv("VELOCITY_LIDAR_DEBUG_LOG")
	tracePath := os.Getenv("VELOCITY_LIDAR_TRACE_LOG")

	if opsPath != "" || lidarDebugPath != "" || tracePath != "" {
		writers := lidar.LogWriters{}
		// Determine a fallback writer: the first explicitly set path, so
		// unspecified streams still produce output (design: avoid silent log loss).
		fallbackPath := firstNonEmpty(opsPath, lidarDebugPath, tracePath)
		openLog := func(path string) (io.Writer, error) {
			if path == "" {
				path = fallbackPath
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("create directory for %s: %w", path, err)
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", path, err)
			}
			logFiles = append(logFiles, f)
			return f, nil
		}
		if w, err := openLog(opsPath); err == nil {
			writers.Ops = w
		} else {
			log.Printf("warning: %v", err)
		}
		if w, err := openLog(lidarDebugPath); err == nil {
			writers.Debug = w
		} else {
			log.Printf("warning: %v", err)
		}
		if w, err := openLog(tracePath); err == nil {
			writers.Trace = w
		} else {
			log.Printf("warning: %v", err)
		}
		lidar.SetLogWriters(writers)
		// Wire sub-package loggers to the same streams.
		l2frames.SetLogWriters(writers.Ops, writers.Debug, writers.Trace)
		l3grid.SetLogWriters(writers.Ops, writers.Debug, writers.Trace)
		pipeline.SetLogWriters(writers.Ops, writers.Debug, writers.Trace)
	} else if debugPath := os.Getenv("VELOCITY_DEBUG_LOG"); debugPath != "" {
		// Legacy: route all three streams to a single file.
		if err := os.MkdirAll(filepath.Dir(debugPath), 0o755); err == nil {
			if f, err := os.OpenFile(debugPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				logFiles = append(logFiles, f)
				lidar.SetDebugLogger(f)
				// Wire sub-package loggers to the same single writer.
				l2frames.SetDebugLogger(f)
				l3grid.SetDebugLogger(f)
				pipeline.SetDebugLogger(f)
			} else {
				log.Printf("warning: failed to open debug log %s: %v", debugPath, err)
			}
		} else {
			log.Printf("warning: failed to create debug log directory for %s", debugPath)
		}
	}
	defer func() {
		for _, f := range logFiles {
			if err := f.Close(); err != nil {
				log.Printf("warning: failed to close log file: %v", err)
			}
		}
	}()

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

	if err := configurePDFLaTeXFlow(*pdfLatexFlow, *pdfTexRoot); err != nil {
		log.Fatalf("Failed to configure PDF LaTeX flow: %v", err)
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

	// Load tuning configuration from file.
	// Deferred until after subcommand dispatch so commands like migrate/transits
	// don't require a valid tuning config.
	tuningCfg, err := config.LoadTuningConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load tuning config from %s: %v", *configFile, err)
	}
	log.Printf("Loaded tuning configuration from %s", *configFile)

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
	var bgFlusher *l3grid.BackgroundFlusher

	// Optionally initialize lidar components inside this binary
	if *enableLidar {
		// Use the main DB instance for lidar data (no separate lidar DB file)
		lidarDB := database

		// Always use tuning config (loaded from --config file; mandatory)
		bgFlushInterval := tuningCfg.GetFlushInterval()
		bgFlushEnable := tuningCfg.GetBackgroundFlush()
		frameBufferTimeout := tuningCfg.GetBufferTimeout()
		minFramePoints := tuningCfg.GetMinFramePoints()

		// Create BackgroundManager from TuningConfig. All tunable parameters
		// come exclusively from the config file (single source of truth).
		bgConfig := l3grid.BackgroundConfigFromTuning(tuningCfg)

		backgroundManager := l3grid.NewBackgroundManager(*lidarSensor, 40, 1800, bgConfig.ToBackgroundParams(), lidarDB)
		if backgroundManager != nil {
			log.Printf("BackgroundManager created and registered for sensor %s", *lidarSensor)
		}

		// Start periodic background grid flushing using BackgroundFlusher
		// Skip if explicitly disabled (background_flush = false) or interval is zero
		if backgroundManager != nil && bgFlushInterval > 0 && bgFlushEnable {
			bgFlusher = l3grid.NewBackgroundFlusher(l3grid.BackgroundFlusherConfig{
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
		var frameBuilder *l2frames.FrameBuilder
		var tracker *l5tracks.Tracker
		var classifier *l6objects.TrackClassifier
		var visualiserServer *visualiser.Server       // Hoisted so WebServerConfig callbacks can reference it
		var visualiserPublisher *visualiser.Publisher // Hoisted so OnVRLogLoad callback can reference it
		var vrlogRecorderMu sync.Mutex
		var vrlogRecorder *recorder.Recorder
		var vrlogRecorderPath string

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

			// Initialise tracking components from tuning config
			trackerCfg := l5tracks.TrackerConfigFromTuning(tuningCfg)
			tracker = l5tracks.NewTracker(trackerCfg)
			classifier = l6objects.NewTrackClassifierWithMinObservations(
				tuningCfg.GetMinObservationsForClassification(),
			)
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
			pipelineConfig := &pipeline.TrackingPipelineConfig{
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
				HeightBandFloor:     tuningCfg.GetHeightBandFloor(),
				HeightBandCeiling:   tuningCfg.GetHeightBandCeiling(),
				RemoveGround:        tuningCfg.GetRemoveGround(),
			}
			callback := pipelineConfig.NewFrameCallback()

			frameBuilder = l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
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
			VRLogSafeDir: func() string {
				baseDir, err := filepath.Abs(filepath.Join(*lidarPCAPDir, "vrlog"))
				if err != nil {
					log.Printf("Warning: failed to resolve VRLOG safe dir: %v", err)
					return filepath.Join(*lidarPCAPDir, "vrlog")
				}
				return baseDir
			}(),
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
			OnPCAPTimestamps: func(startNs, endNs int64) {
				if visualiserServer != nil {
					visualiserServer.SetPCAPTimestamps(startNs, endNs)
				}
			},
			OnRecordingStart: func(runID string) {
				if visualiserPublisher == nil {
					log.Printf("[Visualiser] VRLOG recording skipped (publisher not initialised)")
					return
				}
				vrlogRecorderMu.Lock()
				defer vrlogRecorderMu.Unlock()

				if vrlogRecorder != nil {
					visualiserPublisher.ClearRecorder()
					_ = vrlogRecorder.Close()
					vrlogRecorder = nil
					vrlogRecorderPath = ""
				}

				baseDir, err := filepath.Abs(filepath.Join(*lidarPCAPDir, "vrlog"))
				if err != nil {
					log.Printf("[Visualiser] VRLOG recording failed: %v", err)
					return
				}
				if err := os.MkdirAll(baseDir, 0755); err != nil {
					log.Printf("[Visualiser] VRLOG recording failed: %v", err)
					return
				}
				recordPath := filepath.Join(baseDir, runID)
				rec, err := recorder.NewRecorder(recordPath, *lidarSensor)
				if err != nil {
					log.Printf("[Visualiser] VRLOG recording failed: %v", err)
					return
				}
				vrlogRecorder = rec
				vrlogRecorderPath = rec.Path()
				visualiserPublisher.SetRecorder(rec)
				log.Printf("[Visualiser] VRLOG recording started: %s", vrlogRecorderPath)
			},
			OnRecordingStop: func(runID string) string {
				if visualiserPublisher == nil {
					return ""
				}
				vrlogRecorderMu.Lock()
				defer vrlogRecorderMu.Unlock()

				if vrlogRecorder == nil {
					return ""
				}
				visualiserPublisher.ClearRecorder()
				_ = vrlogRecorder.Close()
				path := vrlogRecorderPath
				vrlogRecorder = nil
				vrlogRecorderPath = ""
				log.Printf("[Visualiser] VRLOG recording stopped: %s", path)
				return path
			},
			OnVRLogLoad: func(vrlogPath string) error {
				if visualiserPublisher == nil {
					return fmt.Errorf("visualiser publisher not initialised")
				}
				if visualiserServer != nil {
					visualiserServer.SetVRLogMode(true)
				}
				// Stop any existing replay first
				visualiserPublisher.StopVRLogReplay()
				// Open the VRLOG directory as a replayer
				replayer, err := recorder.NewReplayer(vrlogPath)
				if err != nil {
					return fmt.Errorf("failed to open vrlog: %w", err)
				}
				// Start replay through the publisher
				if err := visualiserPublisher.StartVRLogReplay(replayer); err != nil {
					replayer.Close()
					return fmt.Errorf("failed to start vrlog replay: %w", err)
				}
				if err := visualiserPublisher.SendBackgroundSnapshot(); err != nil {
					log.Printf("[Visualiser] Failed to send background snapshot: %v", err)
				}
				log.Printf("[Visualiser] VRLOG replay started: %s", vrlogPath)
				return nil
			},
			OnVRLogStop: func() {
				if visualiserPublisher != nil {
					visualiserPublisher.StopVRLogReplay()
					log.Printf("[Visualiser] VRLOG replay stopped")
				}
				if visualiserServer != nil {
					visualiserServer.SetVRLogMode(false)
					visualiserServer.SetReplayMode(false)
				}
			},
		})
		// Wire tracker for in-memory config access via /api/lidar/params
		if tracker != nil {
			lidarWebServer.SetTracker(tracker)
		}
		if classifier != nil {
			lidarWebServer.SetClassifier(classifier)
		}
		// Create and wire sweep runner using direct in-process backend.
		// This eliminates all HTTP overhead for sweep runner ↔ webserver communication.
		sweepBackend := monitor.NewDirectBackend(*lidarSensor, lidarWebServer)
		sweepRunner := sweep.NewRunner(sweepBackend)
		lidarWebServer.SetSweepRunner(sweepRunner)

		// Set up auto-tuner
		autoTuner := sweep.NewAutoTuner(sweepRunner)
		lidarWebServer.SetAutoTuneRunner(autoTuner)

		// Set up sweep persistence
		sweepStore := sqlite.NewSweepStore(lidarDB.DB)
		lidarWebServer.SetSweepStore(sweepStore)
		sweepRunner.SetPersister(sweepStore)
		autoTuner.SetPersister(sweepStore)

		// Wire ground truth scorer and scene store for label-aware auto-tuning.
		// The scene store enables persisting optimal params after ground truth sweeps.
		// The scorer closure resolves the scene's reference_run_id at evaluation time.
		sceneStore := sqlite.NewSceneStore(lidarDB.DB)
		analysisRunStore := sqlite.NewAnalysisRunStore(lidarDB.DB)
		autoTuner.SetSceneStore(sceneStore)
		groundTruthScorer := func(sceneID, candidateRunID string, weights sweep.GroundTruthWeights) (float64, error) {
			scene, err := sceneStore.GetScene(sceneID)
			if err != nil {
				return 0, fmt.Errorf("loading scene %s: %w", sceneID, err)
			}
			if scene.ReferenceRunID == "" {
				return 0, fmt.Errorf("scene %s has no reference_run_id set", sceneID)
			}
			// Convert sweep weights to lidar evaluator weights
			lidarWeights := adapters.GroundTruthWeights{
				DetectionRate:     weights.DetectionRate,
				Fragmentation:     weights.Fragmentation,
				FalsePositives:    weights.FalsePositives,
				VelocityCoverage:  weights.VelocityCoverage,
				QualityPremium:    weights.QualityPremium,
				TruncationRate:    weights.TruncationRate,
				VelocityNoiseRate: weights.VelocityNoiseRate,
				StoppedRecovery:   weights.StoppedRecovery,
			}
			evaluator := adapters.NewGroundTruthEvaluator(analysisRunStore, lidarWeights)
			result, err := evaluator.Evaluate(scene.ReferenceRunID, candidateRunID)
			if err != nil {
				return 0, err
			}
			return result.CompositeScore, nil
		}
		autoTuner.SetGroundTruthScorer(groundTruthScorer)

		// Set up HINT tuner for human-in-the-loop parameter optimisation
		hintTuner := sweep.NewHINTTuner(autoTuner)
		hintTuner.SetPersister(sweepStore)
		hintTuner.SetGroundTruthScorer(groundTruthScorer)
		hintTuner.SetSceneStore(sceneStore)
		hintTuner.SetSceneGetter(&hintSceneAdapter{store: sceneStore})
		hintTuner.SetLabelQuerier(&hintLabelAdapter{store: analysisRunStore})
		hintTuner.SetRunCreator(&hintRunCreator{runner: sweepRunner})
		lidarWebServer.SetHINTRunner(hintTuner)

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
		if _, err := fmt.Scanln(&confirm); err != nil {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
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
		if _, err := fmt.Scanln(&confirm); err != nil {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
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
		if _, err := fmt.Scanln(&confirm); err != nil {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
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

// backgroundManagerBridge adapts *l3grid.BackgroundManager to satisfy
// visualiser.BackgroundManagerInterface, converting between the two
// package-specific snapshot types. This avoids a circular import between
// the lidar and visualiser packages.
type backgroundManagerBridge struct {
	mgr *l3grid.BackgroundManager
}

func (b *backgroundManagerBridge) GenerateBackgroundSnapshot() (interface{}, error) {
	data, err := b.mgr.GenerateBackgroundSnapshot()
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	// Convert *l3grid.BackgroundSnapshotData → *visualiser.BackgroundSnapshot
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

// --- HINT adapters ---
// These bridge the lidar package types to the sweep package interfaces
// to avoid circular imports.

// hintSceneAdapter bridges sqlite.SceneStore to sweep.SceneGetter.
type hintSceneAdapter struct {
	store *sqlite.SceneStore
}

func (a *hintSceneAdapter) GetScene(sceneID string) (*sweep.HINTScene, error) {
	scene, err := a.store.GetScene(sceneID)
	if err != nil {
		return nil, err
	}
	return &sweep.HINTScene{
		SceneID:           scene.SceneID,
		SensorID:          scene.SensorID,
		PCAPFile:          scene.PCAPFile,
		PCAPStartSecs:     scene.PCAPStartSecs,
		PCAPDurationSecs:  scene.PCAPDurationSecs,
		ReferenceRunID:    scene.ReferenceRunID,
		OptimalParamsJSON: scene.OptimalParamsJSON,
	}, nil
}

func (a *hintSceneAdapter) SetReferenceRun(sceneID, runID string) error {
	return a.store.SetReferenceRun(sceneID, runID)
}

// hintLabelAdapter bridges sqlite.AnalysisRunStore to sweep.LabelProgressQuerier.
type hintLabelAdapter struct {
	store *sqlite.AnalysisRunStore
}

func (a *hintLabelAdapter) GetLabelingProgress(runID string) (int, int, map[string]int, error) {
	return a.store.GetLabelingProgress(runID)
}

func (a *hintLabelAdapter) GetRunTracks(runID string) ([]sweep.HINTRunTrack, error) {
	tracks, err := a.store.GetRunTracks(runID)
	if err != nil {
		return nil, err
	}
	result := make([]sweep.HINTRunTrack, len(tracks))
	for i, t := range tracks {
		result[i] = sweep.HINTRunTrack{
			TrackID:        t.TrackID,
			StartUnixNanos: t.StartUnixNanos,
			EndUnixNanos:   t.EndUnixNanos,
			UserLabel:      t.UserLabel,
			QualityLabel:   t.QualityLabel,
		}
	}
	return result, nil
}

func (a *hintLabelAdapter) UpdateTrackLabel(runID, trackID, userLabel, qualityLabel string, confidence float32, labelerID, labelSource string) error {
	return a.store.UpdateTrackLabel(runID, trackID, userLabel, qualityLabel, confidence, labelerID, labelSource)
}

// hintRunCreator bridges the sweep.Runner to sweep.ReferenceRunCreator.
// It creates a single-combo sweep run to generate a reference run with given params.
type hintRunCreator struct {
	runner *sweep.Runner
}

func (a *hintRunCreator) CreateSweepRun(sensorID, pcapFile string, paramsJSON json.RawMessage, pcapStartSecs, pcapDurationSecs float64) (string, error) {
	// For HINT reference runs, we start a single-combo sweep with the given params.
	// Parse paramsJSON into a single-combination sweep: one SweepParam per key with a single fixed value.
	var sweepParams []sweep.SweepParam
	if len(paramsJSON) > 0 && string(paramsJSON) != "null" {
		var rawParams map[string]interface{}
		if err := json.Unmarshal(paramsJSON, &rawParams); err != nil {
			return "", fmt.Errorf("parsing paramsJSON for reference run: %w", err)
		}
		for name, value := range rawParams {
			// Infer type from the Go value (JSON numbers are always float64,
			// booleans are bool, strings are string).
			typ := "float64"
			switch value.(type) {
			case bool:
				typ = "bool"
			case string:
				typ = "string"
			}
			sweepParams = append(sweepParams, sweep.SweepParam{
				Name:   name,
				Type:   typ,
				Values: []interface{}{value},
			})
		}
	}

	req := sweep.SweepRequest{
		Mode:             "params",
		DataSource:       "pcap",
		PCAPFile:         pcapFile,
		PCAPStartSecs:    pcapStartSecs,
		PCAPDurationSecs: pcapDurationSecs,
		Params:           sweepParams,
		EnableRecording:  true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := a.runner.StartWithRequest(ctx, req); err != nil {
		return "", fmt.Errorf("creating reference run: %w", err)
	}

	// Poll for completion using a ticker
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("reference run timed out")
		case <-ticker.C:
			state := a.runner.GetSweepState()
			if state.Status == sweep.SweepStatusComplete || state.Status == sweep.SweepStatusError {
				if len(state.Results) > 0 && state.Results[0].RunID != "" {
					return state.Results[0].RunID, nil
				}
				return "", fmt.Errorf("reference run completed without run ID")
			}
		}
	}
}

// firstNonEmpty returns the first non-empty string from its arguments.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
