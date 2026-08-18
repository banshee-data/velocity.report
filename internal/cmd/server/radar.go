package server

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	// "regexp"

	_ "modernc.org/sqlite"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/docsite"
	"github.com/banshee-data/velocity.report/internal/serialmux"
	"github.com/banshee-data/velocity.report/internal/tailscale"
	"github.com/banshee-data/velocity.report/internal/units"

	// optional lidar integration
	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	"github.com/banshee-data/velocity.report/internal/lidar/server"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
	"github.com/banshee-data/velocity.report/internal/version"
)

// serveFlags is the per-applet flag set for the radar/server applet. It is
// bound to a dedicated FlagSet (not the global flag.CommandLine) so the server
// can co-exist with the other applets (device, tune) inside the single
// multi-call velocity binary without flag-registration collisions at startup.
var serveFlags = flag.NewFlagSet("velocity-serve", flag.ExitOnError)

var (
	fixtureMode   = serveFlags.Bool("fixture", false, "Load fixture to local database")
	debugMode     = serveFlags.Bool("debug", false, "Run in debug mode (enables debug output in reports)")
	listen        = serveFlags.String("listen", "127.0.0.1:8080", "Listen address (use 0.0.0.0:8080 for all IPv4 interfaces, or [::]:8080 for IPv4+IPv6)")
	docsSource    = serveFlags.String("docs-source", docsite.SourceEmbed, "Offline docs source for /docs/: embed or disk")
	port          = serveFlags.String("port", "/dev/ttySC1", "Serial port to use")
	unitsFlag     = serveFlags.String("units", "mph", "Speed units for display (mps, mph, kmph)")
	timezoneFlag  = serveFlags.String("timezone", "UTC", "Timezone for display (UTC, US/Eastern, US/Pacific, etc.)")
	disableRadar  = serveFlags.Bool("disable-radar", false, "Disable radar serial port (serve DB only)")
	dbPathFlag    = serveFlags.String("db-path", defaultRuntimeDBPath, "path to sqlite DB file (defaults to sensor_data.db)")
	versionFlag   = serveFlags.Bool("version", false, "Print version information and exit")
	versionShort  = serveFlags.Bool("v", false, "Print version information and exit (shorthand)")
	configFile    = serveFlags.String("config", config.DefaultConfigPath, "Path to JSON tuning configuration file")
	logLevel      = serveFlags.String("log-level", "ops", "LiDAR log verbosity: ops, diag, or trace")
	selfCheck     = serveFlags.Bool("self-check", false, "Run static-build self-check (DNS, UDP, libpcap) and exit non-zero on any failure")
	selfCheckLive = serveFlags.String("self-check-live-capture", "", "Also capture a generated UDP packet on this interface (for release validation)")
)

const (
	defaultRuntimeDBPath       = "sensor_data.db"
	deployedRuntimeDBPath      = "/var/lib/velocity-report/sensor_data.db"
	deployedVelocityBinaryPath = "/opt/velocity-report/current/velocity"
)

func parseMigrateCommandArgs(args []string, defaultDBPath string) ([]string, string, bool, error) {
	dbPath := defaultDBPath
	explicitDBPath := false
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positionals = append(positionals, args[i+1:]...)
			return positionals, dbPath, explicitDBPath, nil
		case arg == "--db-path":
			if i+1 >= len(args) {
				return nil, "", false, fmt.Errorf("flag needs an argument: --db-path")
			}
			dbPath = args[i+1]
			explicitDBPath = true
			i++
		case strings.HasPrefix(arg, "--db-path="):
			dbPath = strings.TrimPrefix(arg, "--db-path=")
			explicitDBPath = true
			if dbPath == "" {
				return nil, "", false, fmt.Errorf("flag needs an argument: --db-path")
			}
		case arg == "--help" || arg == "-h":
			return []string{"help"}, dbPath, explicitDBPath, nil
		case strings.HasPrefix(arg, "-"):
			return nil, "", false, fmt.Errorf("unknown migrate flag: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}

	return positionals, dbPath, explicitDBPath, nil
}

func resolveDataCommandDBPath(configured string) string {
	return resolveDataCommandDBPathWith(configured, installedApplianceLayoutPresent())
}

func resolveDataCommandDBPathWith(configured string, installedAppliance bool) string {
	trimmed := strings.TrimSpace(configured)
	if trimmed == "" || trimmed != defaultRuntimeDBPath || !installedAppliance {
		return trimmed
	}
	return deployedRuntimeDBPath
}

func installedApplianceLayoutPresent() bool {
	if _, err := os.Stat(deployedVelocityBinaryPath); err == nil {
		return true
	}

	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	cleanExe := filepath.Clean(exe)
	return strings.HasPrefix(cleanExe, "/opt/velocity-report/versions/") && strings.HasSuffix(cleanExe, "/velocity")
}

// Lidar options (when enabling lidar via -enable-lidar)
var (
	enableLidar    = serveFlags.Bool("enable-lidar", false, "Enable lidar components inside this radar binary")
	lidarListen    = serveFlags.String("lidar-listen", "127.0.0.1:8081", "HTTP listen address for lidar monitor (use 0.0.0.0:8081 for all IPv4 interfaces, or [::]:8081 for IPv4+IPv6)")
	lidarUDPPort   = serveFlags.Int("lidar-udp-port", 2369, "UDP port to listen for lidar packets")
	lidarUDPRcvBuf = serveFlags.Int("lidar-udp-rcv-buf", 4<<20, "UDP receive buffer size in bytes for LiDAR listener")
	lidarNoParse   = serveFlags.Bool("lidar-no-parse", false, "Disable lidar packet parsing when lidar is enabled")
	lidarForward   = serveFlags.Bool("lidar-forward", false, "Forward lidar UDP packets to another port")
	lidarFwdPort   = serveFlags.Int("lidar-forward-port", 2368, "Port to forward lidar UDP packets to")
	lidarFwdAddr   = serveFlags.String("lidar-forward-addr", "localhost", "Address to forward lidar UDP packets to")
	lidarFGForward = serveFlags.Bool("lidar-foreground-forward", false, "Forward foreground-only LiDAR packets to a separate port (e.g., 2370)")
	lidarFGFwdPort = serveFlags.Int("lidar-foreground-forward-port", 2370, "Port to forward foreground LiDAR packets to")
	lidarFGFwdAddr = serveFlags.String("lidar-foreground-forward-addr", "localhost", "Address to forward foreground LiDAR packets to")
	lidarPCAPDir   = serveFlags.String("lidar-pcap-dir", "../sensor_data/lidar", "Safe directory for PCAP files (only files within this directory can be replayed)")
	// Visualiser gRPC streaming (M2)
	lidarForwardMode = serveFlags.String("lidar-forward-mode", "lidarview", "Forward mode: lidarview (UDP only), grpc (gRPC only), or both (UDP + gRPC)")
	lidarGRPCListen  = serveFlags.String("lidar-grpc-listen", "localhost:50051", "gRPC server listen address for visualiser streaming")
)

// Transit worker options (compute radar_data -> radar_data_transits)
var (
	enableTransitWorker    = serveFlags.Bool("enable-transit-worker", true, "Enable transit worker to periodically compute transits from radar_data")
	transitWorkerInterval  = serveFlags.Duration("transit-worker-interval", 1*time.Hour, "Interval for transit worker (e.g., 1h)")
	transitWorkerWindow    = serveFlags.Duration("transit-worker-window", 65*time.Minute, "Lookback window for transit worker (should be slightly larger than interval)")
	transitWorkerThreshold = serveFlags.Int("transit-worker-threshold", 1, "Gap threshold in seconds for sessionizing transits")
	transitWorkerModel     = serveFlags.String("transit-worker-model", "hourly-cron", "Model version string for computed transits")
)

// Constants
const SCHEMA_VERSION = "0.0.2"

func visitedFlags() map[string]bool {
	visited := make(map[string]bool)
	serveFlags.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func defaultRuntimeSerialOptions() serialmux.PortOptions {
	return serialmux.PortOptions{
		BaudRate: 19200,
		DataBits: 8,
		StopBits: 1,
		Parity:   "N",
	}
}

func runtimeSerialSnapshot(database *db.DB, portPath string, serialActive bool, useDatabase bool) (api.SerialConfigSnapshot, error) {
	if !serialActive {
		return api.SerialConfigSnapshot{}, nil
	}

	if useDatabase {
		if database == nil {
			return api.SerialConfigSnapshot{}, fmt.Errorf("database serial configuration requested without a database handle")
		}

		configs, err := database.GetEnabledSerialConfigs()
		if err != nil {
			return api.SerialConfigSnapshot{}, fmt.Errorf("failed to load enabled serial configurations: %w", err)
		}
		if len(configs) > 0 {
			cfg := configs[0]
			opts, err := serialmux.PortOptions{
				BaudRate: cfg.BaudRate,
				DataBits: cfg.DataBits,
				StopBits: cfg.StopBits,
				Parity:   cfg.Parity,
			}.Normalise()
			if err != nil {
				return api.SerialConfigSnapshot{}, fmt.Errorf("enabled serial configuration %d is invalid: %w", cfg.ID, err)
			}

			return api.SerialConfigSnapshot{
				ConfigID: cfg.ID,
				PortPath: cfg.PortPath,
				Source:   "database",
				Options:  opts,
			}, nil
		}
	}

	trimmedPath := strings.TrimSpace(portPath)
	if trimmedPath == "" {
		return api.SerialConfigSnapshot{}, nil
	}

	return api.SerialConfigSnapshot{
		PortPath: trimmedPath,
		Source:   "cli",
		Options:  defaultRuntimeSerialOptions(),
	}, nil
}

func runtimeSerialFactory(reloadEnabled bool) api.SerialMuxFactory {
	if !reloadEnabled {
		return nil
	}

	return func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return serialmux.NewRealSerialMuxWithOptions(path, opts)
	}
}

func newRuntimeSerialManager(database *db.DB, current serialmux.SerialMuxInterface, snapshot api.SerialConfigSnapshot, reloadEnabled bool) *api.SerialPortManager {
	return api.NewSerialPortManager(database, current, snapshot, runtimeSerialFactory(reloadEnabled))
}

// Main
// tailscaleServeTarget derives the loopback HTTP URL that `tailscale serve`
// should proxy to from the server's --listen address, so the published HTTPS
// endpoint always points at the port we actually serve on (:80 on the image,
// :8080 in dev) instead of a hard-coded guess.  Falls back to the package
// default when the address has no parseable port.
func tailscaleServeTarget(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return tailscale.LocalServeHTTPTarget
	}
	return "http://127.0.0.1:" + port
}

// Main is the entry point for the server (serve) applet of the multi-call
// velocity binary. args is the argument slice after the program name (i.e.
// os.Args[1:] for the bare `velocity-report` / `velocity serve` forms). It
// returns the process exit code.
func Main(args []string) int {
	// Subcommand dispatch — check before flag parsing so subcommand flags
	// don't collide with the server's flags.
	if len(args) > 0 && args[0] == "pdf" {
		return runPDF(args[1:], os.Stdout, os.Stderr)
	}
	if len(args) > 0 && args[0] == "sql" {
		return runSQL(args[1:], os.Stdout, os.Stderr)
	}

	if err := serveFlags.Parse(args); err != nil {
		// flag.ExitOnError already prints usage and exits on parse errors;
		// this guard is belt-and-suspenders for future error modes.
		log.Fatalf("parsing flags: %v", err)
	}

	// Configure logging: default to stdout; optionally tee to a log file via env.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	// Three-stream LiDAR logging.
	//
	// Verbosity is controlled by --log-level (ops|diag|trace, default: ops).
	// ops is always routed to stdout. When VELOCITY_DEBUG_LOG is set, diag
	// and trace (if enabled by --log-level) are routed to that file;
	// otherwise they also go to stdout.
	writers := lidar.LogWriters{Ops: os.Stdout}
	var debugLogFile *os.File
	if p := os.Getenv("VELOCITY_DEBUG_LOG"); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			log.Fatalf("create directory for %s: %v. Check parent path exists and permissions allow writing", p, err)
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatalf("open debug log %s: %v. Check directory exists and file permissions", p, err)
		}
		debugLogFile = f
	}
	defer func() {
		if debugLogFile != nil {
			debugLogFile.Close()
		}
	}()

	// debugDest is the file for diag/trace streams: the debug log file if
	// set, otherwise stdout (so nothing is silently lost).
	debugDest := io.Writer(os.Stdout)
	if debugLogFile != nil {
		debugDest = debugLogFile
	}
	switch *logLevel {
	case "trace":
		writers.Trace = debugDest
		fallthrough
	case "diag":
		writers.Diag = debugDest
	case "ops":
		// ops-only: diag and trace stay nil (disabled)
	default:
		log.Fatalf("Unrecognised --log-level=%q: valid values are ops, diag, trace (e.g. --log-level=diag)", *logLevel)
	}
	lidar.SetLogWriters(writers)
	network.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	parse.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	l2frames.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	l3grid.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	l4perception.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	l5tracks.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	l6objects.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	server.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	pipeline.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	sqlite.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	sweep.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	l9endpoints.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)
	recorder.SetLogWriters(writers.Ops, writers.Diag, writers.Trace)

	// Handle version flags (-v, --version)
	if *versionFlag || *versionShort {
		version.Print("velocity-report")
		return 0
	}

	// Static-build self-check — verify DNS, UDP, and libpcap work in the
	// current runtime. Used by the static-build smoke test to catch
	// musl/libc-static failure modes before shipping.
	if *selfCheck {
		return runSelfCheck(os.Stdout, *selfCheckLive)
	}

	// Check if first argument is a subcommand
	if serveFlags.NArg() > 0 {
		subcommand := serveFlags.Arg(0)
		if subcommand == "version" {
			version.Print("velocity-report")
			return 0
		}
		if subcommand == "migrate" {
			remainingArgs := serveFlags.Args()[1:]
			// explicitDBPath is parsed and unit-tested for completeness, but the
			// migrate command itself now echoes the resolved absolute DB target
			// and refuses to create a stray DB for non-bootstrap actions (see
			// db.RunMigrateCommand), so main() no longer needs it here.
			migrateArgs, migrateDBPath, _, err := parseMigrateCommandArgs(remainingArgs, resolveDataCommandDBPath(*dbPathFlag))
			if err != nil {
				log.Fatalf("Could not parse migrate flags: %v. Run 'velocity-report migrate help' for usage", err)
			}

			db.RunMigrateCommand(migrateArgs, migrateDBPath)
			return 0
		}
		if subcommand == "transits" {
			runTransitsCommand(serveFlags.Args()[1:])
			return 0
		}
		log.Fatalf("Unknown subcommand: %s: try 'velocity-report --help' for available commands", subcommand)
	}

	if *listen == "" {
		log.Fatal("Listen address is required: use --listen, e.g. --listen 0.0.0.0:8080")
	}
	if err := docsite.ValidateSource(*docsSource); err != nil {
		log.Fatal(err)
	}
	if !units.IsValid(*unitsFlag) {
		log.Printf("Invalid units %q: valid options are: %s", *unitsFlag, units.GetValidUnitsString())
		return 1
	}
	if !units.IsTimezoneValid(*timezoneFlag) {
		log.Printf("Invalid timezone %q: valid options are: %s", *timezoneFlag, units.GetValidTimezonesString())
		return 1
	}

	// Load tuning configuration from file, falling back to the binary-embedded
	// defaults when no file is present at the configured path (the shipped image
	// carries no on-disk tuning.defaults.json). Deferred until after subcommand
	// dispatch so commands like migrate/transits don't require a valid config.
	tuningCfg, err := config.LoadTuningConfigOrEmbedded(*configFile, radarassets.TuningDefaults)
	if err != nil {
		log.Fatalf("Failed to load tuning config from %s: %v. Check the file exists and is valid JSON", *configFile, err)
	}
	log.Printf("Loaded tuning configuration (config=%s)", *configFile)
	ensureSupportedTuning(tuningCfg, log.Fatalf)
	if *enableLidar {
		ensureValidLidarNetworkingFlags(
			*lidarUDPPort,
			*lidarUDPRcvBuf,
			*lidarFwdPort,
			*lidarFGFwdPort,
			log.Fatalf,
		)
	}

	// Compute tuning config hash for VRLOG provenance.
	tuningHash := tuningHashOrWarn(tuningCfg, log.Printf)

	// Use the CLI flag value (defaults to ./sensor_data.db). We intentionally
	// avoid relying on environment variables for configuration unless needed.
	database, err := db.NewDB(*dbPathFlag)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v. Check file path is correct and directory is writable", err)
	}
	defer database.Close()

	serialActive := !*disableRadar
	reloadEnabled := !*disableRadar && !*debugMode && !*fixtureMode
	serialSnapshot, err := runtimeSerialSnapshot(database, *port, serialActive, reloadEnabled)
	if err != nil {
		log.Fatalf("Failed to load serial runtime configuration: %v", err)
	}
	if serialActive && serialSnapshot.PortPath == "" {
		log.Fatal("Serial port is required: save and enable a serial configuration or use --port, e.g. --port /dev/ttySC1")
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
		if err != nil {
			log.Fatalf("Could not open fixtures file: %v. Ensure fixtures.txt exists in the working directory", err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || lines[0] == "" {
			log.Fatal("Fixtures file is empty: add at least one line of sample data to fixtures.txt")
		}
		radarSerial = serialmux.NewMockSerialMux([]byte(lines[0] + "\n"))
	} else {
		var err error
		radarSerial, err = serialmux.NewRealSerialMuxWithOptions(serialSnapshot.PortPath, serialSnapshot.Options)
		if err != nil {
			log.Fatalf("failed to create radar port %s from %s configuration: %v. Check device is connected and port path is correct (default /dev/ttySC1)", serialSnapshot.PortPath, serialSnapshot.Source, err)
		}
	}
	if err := radarSerial.Initialise(); err != nil {
		log.Fatalf("failed to initialise device: %v. Check device is powered on and responding", err)
	} else {
		log.Printf("initialised device %s", radarSerial)
	}

	// Log version and git SHA on startup
	log.Printf("velocity-report v%s (git SHA: %s)", version.Version, version.GitSHA)

	serialManager := newRuntimeSerialManager(database, radarSerial, serialSnapshot, reloadEnabled)
	defer serialManager.Close()

	// Create a wait group for the HTTP server, serial monitor, and event handler routines
	var wg sync.WaitGroup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Lidar webserver instance (if enabled)
	var lidarServer *server.Server
	var foregroundForwarder *network.ForegroundForwarder
	var bgFlusher *l3grid.BackgroundFlusher

	// Optionally initialize lidar components inside this binary
	if *enableLidar {
		lidarSensorID := tuningCfg.GetSensor()
		lidarUDPListenPort := *lidarUDPPort
		lidarUDPRcvBuf := *lidarUDPRcvBuf
		lidarForwardPortCfg := *lidarFwdPort
		lidarFGForwardPortCfg := *lidarFGFwdPort

		// Use the main DB instance for lidar data (no separate lidar DB file)
		lidarDB := database

		// Always use tuning config (loaded from --config file; mandatory)
		bgFlushInterval := tuningCfg.GetFlushInterval()
		bgFlushEnable := tuningCfg.GetBackgroundFlush()
		frameBufferTimeout := tuningCfg.GetBufferTimeout()
		minFramePoints := tuningCfg.GetMinFramePoints()

		// Create BackgroundManager from TuningConfig. All tunable parameters
		// come exclusively from the config file (single source of truth).
		bgConfig := l3grid.BackgroundConfigFromActiveTuning(tuningCfg)

		backgroundManager := l3grid.NewBackgroundManager(lidarSensorID, 40, 1800, bgConfig.ToBackgroundParams(), lidarDB)
		if backgroundManager != nil {
			log.Printf("BackgroundManager created and registered for sensor %s", lidarSensorID)
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
					log.Printf("Background flusher stopped unexpectedly: %v. Buffered data may not have been written to disk", err)
				}
			}()
		}

		// Lidar parser and frame builder (optional)
		var parser *parse.Pandar40PParser
		var frameBuilder *l2frames.FrameBuilder
		var tracker *l5tracks.Tracker
		var classifier *l6objects.TrackClassifier
		var pipelineConfig *pipeline.TrackingPipelineConfig // hoisted so BenchmarkMode can be wired post-webserver creation
		var visualiserServer *l9endpoints.Server            // Hoisted so Config callbacks can reference it
		var visualiserPublisher *l9endpoints.Publisher      // Hoisted so OnVRLogLoad callback can reference it
		var vrlogRecorderMu sync.Mutex
		var vrlogRecorder *recorder.Recorder
		var vrlogRecorderPath string

		// Optional foreground-only forwarder (Pandar40-compatible) for live mode
		if *lidarFGForward && lidarFGForwardPortCfg > 0 {
			fg, err := network.NewForegroundForwarder(*lidarFGFwdAddr, lidarFGForwardPortCfg, nil)
			if err != nil {
				log.Printf("failed to create foreground forwarder: %v. Check address and port are not already in use", err)
			} else {
				foregroundForwarder = fg
				foregroundForwarder.Start(ctx)
				defer foregroundForwarder.Close()
				log.Printf("Foreground forwarder enabled to %s:%d", *lidarFGFwdAddr, lidarFGForwardPortCfg)
			}
		}

		if !*lidarNoParse {
			config := mustLoadValidatedPandarConfig(
				parse.LoadEmbeddedPandar40PConfig,
				func(cfg *parse.Pandar40PConfig) error { return cfg.Validate() },
				log.Fatalf,
			)
			parser = parse.NewPandar40PParser(*config)
			parse.ConfigureTimestampMode(parser)

			// Initialise tracking components from tuning config
			trackerCfg := l5tracks.TrackerConfigFromTuning(tuningCfg.L5.CvKfV1)
			tracker = l5tracks.NewTracker(trackerCfg)
			classifier = l6objects.NewTrackClassifierWithMinObservations(
				tuningCfg.GetMinObservationsForClassification(),
			)
			log.Printf("Tracker and classifier initialized for sensor %s", lidarSensorID)

			// Wire per-ring elevation corrections from parser config into BackgroundManager
			// This ensures background ASC exports use the same per-channel elevations as frames.
			if backgroundManager != nil {
				log.Print(ringElevationLogMessage(backgroundManager, lidarSensorID, config))
			}

			// Initialise visualiser components if gRPC mode is enabled
			var frameAdapter *l9endpoints.FrameAdapter
			var lidarViewAdapter *l9endpoints.LidarViewAdapter

			// Validate forward mode
			forwardMode := *lidarForwardMode
			ensureValidForwardMode(forwardMode, log.Fatalf)

			// Initialise gRPC publisher if needed
			if forwardMode == "grpc" || forwardMode == "both" {
				vizConfig := l9endpoints.DefaultConfig()
				vizConfig.ListenAddr = *lidarGRPCListen
				vizConfig.SensorID = lidarSensorID
				vizConfig.EnableDebug = *debugMode
				vizConfig.MaxClients = 5
				visualiserPublisher = l9endpoints.NewPublisher(vizConfig)
				visualiserServer = l9endpoints.NewServer(visualiserPublisher)

				if err := visualiserPublisher.Start(); err != nil {
					log.Fatalf("Could not start visualiser publisher: %v. Check the gRPC listen address is free and not already in use", err)
				}
				defer visualiserPublisher.Stop()

				// Register gRPC service (must happen after Start() to ensure GRPCServer is initialised)
				l9endpoints.RegisterService(visualiserPublisher.GRPCServer(), visualiserServer)

				frameAdapter = l9endpoints.NewFrameAdapter(lidarSensorID)

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
					lidarViewAdapter = l9endpoints.NewLidarViewAdapter(foregroundForwarder)
					log.Printf("LidarView adapter enabled (forwarding to %s:%d)", *lidarFGFwdAddr, lidarFGForwardPortCfg)
				}
			}

			// Create tracking pipeline callback with all necessary dependencies
			pipelineConfig = &pipeline.TrackingPipelineConfig{
				BackgroundManager:   backgroundManager,
				FgForwarder:         foregroundForwarder,
				Tracker:             tracker,
				Classifier:          classifier,
				DB:                  lidarDB.DB, // Pass underlying sql.DB to avoid import cycle
				SensorID:            lidarSensorID,
				VisualiserPublisher: visualiserPublisher,
				VisualiserAdapter:   frameAdapter,
				LidarViewAdapter:    lidarViewAdapter,
				MaxFrameRate:        25, // Must exceed sensor max Hz (20) to avoid dropping live frames
				HeightBandFloor:     tuningCfg.GetHeightBandFloor(),
				HeightBandCeiling:   tuningCfg.GetHeightBandCeiling(),
				RemoveGround:        tuningCfg.GetRemoveGround(),
			}
			callback := pipelineConfig.NewFrameCallback()

			frameBuilder = l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
				SensorID:      lidarSensorID,
				FrameCallback: callback,
				// Use CLI-configurable MinFramePoints and BufferTimeout so devs can tune
				MinFramePoints:  minFramePoints,
				FrameBufferSize: 100,
				BufferTimeout:   frameBufferTimeout,
				CleanupInterval: 250 * time.Millisecond,
				// Larger callback channel buffer absorbs short processing
				// stalls during PCAP replay without dropping frames.
				FrameChCapacity: 32,
			})
		}

		// Packet forwarding (optional): only for LidarView or both modes.
		// In gRPC-only mode there is no LidarView listener, so forwarding raw
		// packets to localhost:2368 causes write-error noise in the log.
		var packetForwarder *network.PacketForwarder
		// Create a PacketStats instance and wire it into the forwarder, listener and webserver
		packetStats := server.NewPacketStats()
		if *lidarForward && lidarForwardPortCfg > 0 && (*lidarForwardMode == "lidarview" || *lidarForwardMode == "both") {
			createdForwarder, err := network.NewPacketForwarder(*lidarFwdAddr, lidarForwardPortCfg, packetStats, time.Minute)
			if err != nil {
				log.Printf("failed to create lidar forwarder: %v", err)
			} else {
				packetForwarder = createdForwarder
				defer packetForwarder.Close()
			}
		}

		udpAddr := fmt.Sprintf(":%d", lidarUDPListenPort)
		udpListenerConfig := network.UDPListenerConfig{
			Address:        udpAddr,
			RcvBuf:         lidarUDPRcvBuf,
			LogInterval:    time.Minute,
			Stats:          packetStats,
			Forwarder:      packetForwarder,
			Parser:         parser,
			FrameBuilder:   frameBuilder,
			DB:             lidarDB,
			DisableParsing: *lidarNoParse,
			UDPPort:        lidarUDPListenPort,
		}

		// Start lidar webserver for monitoring (moved into internal/api)
		// Provide a PacketStats instance if parsing/forwarding is enabled
		// Pass the same PacketStats instance to the webserver so it shows live stats
		lidarServer = server.NewServer(server.Config{
			Address:           *lidarListen,
			Stats:             packetStats,
			ForwardingEnabled: *lidarForward && lidarForwardPortCfg > 0,
			ForwardAddr:       *lidarFwdAddr,
			ForwardPort:       lidarForwardPortCfg,
			ParsingEnabled:    !*lidarNoParse,
			UDPPort:           lidarUDPListenPort,
			DB:                lidarDB,
			SensorID:          lidarSensorID,
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
			TuningConfig:      tuningCfg,
			OnPCAPStarted:     pcapStartedCallback(visualiserPublisher, visualiserServer, log.Printf),
			OnPCAPStopped: func() {
				if visualiserServer != nil {
					visualiserServer.SetReplayMode(false)
					log.Printf("[Visualiser] PCAP stopped: switched to live mode")
				}
			},
			OnPCAPProgress:   pcapProgressCallback(visualiserServer),
			PlaybackProbe:    visualiserPlaybackProbe{server: visualiserServer},
			OnPCAPTimestamps: pcapTimestampsCallback(visualiserServer),
			OnRecordingStart: func(runID string) string {
				if visualiserPublisher == nil {
					log.Printf("[Visualiser] VRLOG recording skipped (publisher not initialised)")
					return ""
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
					return ""
				}
				if err := os.MkdirAll(baseDir, 0755); err != nil {
					log.Printf("[Visualiser] VRLOG recording failed: %v", err)
					return ""
				}
				recordPath := filepath.Join(baseDir, runID)
				rec := newVRLogRecorderOrLog(recorder.NewRecorder, recordPath, lidarSensorID, log.Printf)
				if rec == nil {
					return ""
				}

				applyRecordingMetadata(rec, lidarDB, lidarServer, runID, tuningHash, log.Default())

				vrlogRecorder = rec
				vrlogRecorderPath = rec.Path()
				visualiserPublisher.SetRecorder(rec)
				log.Printf("[Visualiser] VRLOG recording started: %s", vrlogRecorderPath)
				return vrlogRecorderPath
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
			OnVRLogLoad: func(vrlogPath string) (string, error) {
				if visualiserPublisher == nil {
					return "", fmt.Errorf("visualiser publisher not initialised")
				}
				if visualiserServer != nil {
					visualiserServer.SetVRLogMode(true)
				}
				// Stop any existing replay first
				visualiserPublisher.StopVRLogReplay()
				// Open the VRLOG directory as a replayer
				replayer, err := recorder.NewReplayer(vrlogPath)
				if err != nil {
					return "", fmt.Errorf("failed to open vrlog: %w", err)
				}
				frameEncoding := string(replayer.FrameEncoding())
				// Start replay through the publisher
				if err := visualiserPublisher.StartVRLogReplay(replayer); err != nil {
					replayer.Close()
					return "", fmt.Errorf("failed to start vrlog replay: %w", err)
				}
				// Fallback for recordings that hold no background frame of
				// their own: the client has nothing to composite the replayed
				// foreground against. A no-op when the replay already emitted
				// its recorded background.
				if err := visualiserPublisher.SendBackgroundSnapshot(); err != nil {
					log.Printf("[Visualiser] Failed to send background snapshot: %v", err)
				}
				log.Printf("[Visualiser] VRLOG replay started: %s (frame encoding=%s)", vrlogPath, frameEncoding)
				return frameEncoding, nil
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
		// A VRLOG that plays to its end returns the pipeline to live by itself.
		// Nothing else observes the end, so without this the recording stayed
		// the data source indefinitely: the live listener was never restarted,
		// plugging the sensor back in produced nothing, and the replay slot
		// stayed claimed so no new replay could start.
		if visualiserPublisher != nil {
			srv := lidarServer
			visualiserPublisher.SetOnReplayEnded(func() {
				if err := srv.ReturnToLive("VRLOG replay reached the end of its recording"); err != nil {
					log.Printf("[Visualiser] Failed to return to live after VRLOG replay ended: %v", err)
				}
			})
		}

		// Let the streaming layer read the source mode from its single owner
		// rather than keeping a second copy that can drift out of agreement.
		if visualiserServer != nil {
			srv := lidarServer
			visualiserServer.SetSourceModeProvider(func() (string, bool) {
				state := srv.PipelineState()
				return state.DataSourceWire(), state.Recording
			})
		}
		// Wire tracker for in-memory config access via /api/lidar/params
		if tracker != nil {
			lidarServer.SetTracker(tracker)
		}
		if classifier != nil {
			lidarServer.SetClassifier(classifier)
		}
		// Wire benchmark mode toggle from webserver to pipeline so the
		// dashboard checkbox can enable/disable trace logging at runtime.
		if pipelineConfig != nil {
			pipelineConfig.BenchmarkMode = lidarServer.BenchmarkMode()
			pipelineConfig.DisableTrackPersistence = lidarServer.DisableTrackPersistenceFlag()
		}
		// Create and wire sweep runner using direct in-process backend.
		// This eliminates all HTTP overhead for sweep runner ↔ webserver communication.
		sweepBackend := server.NewDirectBackend(lidarSensorID, lidarServer)
		sweepRunner := sweep.NewRunner(sweepBackend)
		lidarServer.SetSweepRunner(sweepRunner)

		// Set up auto-tuner
		autoTuner := sweep.NewAutoTuner(sweepRunner)
		lidarServer.SetAutoTuneRunner(autoTuner)

		// Set up sweep persistence
		sweepStore := sqlite.NewSweepStore(lidarDB)
		recoverOrphanedSweepsOnStart(sweepStore, log.Default())
		lidarServer.SetSweepStore(sweepStore)
		sweepRunner.SetPersister(sweepStore)
		autoTuner.SetPersister(sweepStore)

		// Wire ground truth scorer and scene store for label-aware auto-tuning.
		// The scene store enables persisting optimal params after ground truth sweeps.
		// The scorer closure resolves the scene's reference_run_id at evaluation time.
		sceneStore := sqlite.NewReplayCaseStore(lidarDB)
		analysisRunStore := sqlite.NewAnalysisRunStore(lidarDB)
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
		lidarServer.SetHINTRunner(hintTuner)

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lidarServer.Start(ctx); err != nil {
				log.Printf("Lidar webserver error: %v", err)
			}
		}()
	}

	// run the monitor routine to manage IO on the serial port
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := serialManager.Monitor(ctx); err != nil && err != context.Canceled {
			log.Printf("failed to monitor serial port: %v", err)
		}
		log.Print("monitor routine terminated")
	}()

	// subscribe to the serial port messages
	// and pass them to event handler
	wg.Add(1)
	go func() {
		defer wg.Done()
		id, c := serialManager.Subscribe()
		defer serialManager.Unsubscribe(id)
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
		apiServer := api.NewServer(serialManager, database, *unitsFlag, *timezoneFlag)
		apiServer.SetSerialManager(serialManager)
		// Set the transit controller so API can provide UI controls
		apiServer.SetTransitController(transitController)

		// Tailscale lifecycle manager: drives tailscaled on opt-in,
		// caches the IPN bus login URL, and applies the device policy
		// (Tailscale SSH on, tailscale serve publishing the local Go
		// server on :443 of the tailnet) once the node is up.
		tsManager := tailscale.New(tailscale.WithServeTarget(tailscaleServeTarget(*listen)))
		tsManager.Start(ctx)
		defer tsManager.Stop()
		apiServer.SetTailscaleController(tsManager)

		// Wire capabilities provider so /api/capabilities reports sensor state.
		// When LiDAR is enabled we report "starting" here; the subsystem should
		// call SetLidarReady() once it has completed initialisation successfully,
		// or SetLidarError() if startup fails. This avoids advertising "ready"
		// before the hardware is actually operational.
		capsProvider := newCapabilitiesProvider()
		if lidarServer != nil {
			capsProvider.SetLidarStarting()
		}
		apiServer.SetCapabilitiesProvider(capsProvider)

		// Attach admin routes that belong to other components
		// (these modify the mux returned by apiServer.ServeMux internally)
		mux := apiServer.ServeMux()
		if handler, err := docsite.Handler(*docsSource, docsite.DefaultDiskDir); err != nil {
			log.Printf("Offline docs route %s unavailable on main HTTP server: %v", docsite.DefaultMount, err)
		} else if err := docsite.Mount(mux, docsite.DefaultMount, handler); err != nil {
			log.Printf("Offline docs route %s unavailable on main HTTP server: %v", docsite.DefaultMount, err)
		} else {
			log.Printf("Offline docs available on main HTTP server at %s (source=%s)", docsite.DefaultMount, *docsSource)
		}
		if handler, err := docsite.DiskHandler(docsite.PublicHTMLDiskDir); err != nil {
			log.Printf("Offline homepage route %s unavailable on main HTTP server: %v", docsite.PublicHTMLMount, err)
		} else if err := docsite.Mount(mux, docsite.PublicHTMLMount, handler); err != nil {
			log.Printf("Offline homepage route %s unavailable on main HTTP server: %v", docsite.PublicHTMLMount, err)
		} else {
			log.Printf("Offline homepage available on main HTTP server at %s", docsite.PublicHTMLMount)
		}
		serialManager.AttachAdminRoutes(mux)
		database.AttachAdminRoutes(mux)

		// Attach Lidar routes if enabled
		if lidarServer != nil {
			lidarServer.RegisterRoutes(mux)
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
	return 0
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
		log.Fatalf("Could not parse transits flags: %v. Run 'velocity-report transits --help' for usage", err)
	}

	// Open database without migration check for CLI commands
	database, err := db.OpenDB(*transitDBPath)
	if err != nil {
		log.Fatalf("Could not open database: %v. Check --db-path points to a valid SQLite file", err)
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
			log.Fatalf("Could not analyse transits: %v. Check the database is not locked by another process", err)
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
			log.Fatalf("Could not delete transits: %v. Check the database is not locked by another process", err)
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
			log.Fatalf("Could not migrate transits: %v. Check the database is not locked by another process", err)
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
			log.Fatalf("Could not rebuild transits: %v. Check the database is not locked by another process", err)
		}

	default:
		log.Fatalf("Unknown transits subcommand: %s: run 'velocity-report transits --help' for available commands", subCmd)
	}
}

// backgroundManagerBridge adapts *l3grid.BackgroundManager to satisfy
// l9endpoints.BackgroundManagerInterface, converting between the two
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
	// Convert *l3grid.BackgroundSnapshotData → *l9endpoints.BackgroundSnapshot
	return &l9endpoints.BackgroundSnapshot{
		SequenceNumber: data.SequenceNumber,
		TimestampNanos: data.TimestampNanos,
		X:              data.X,
		Y:              data.Y,
		Z:              data.Z,
		Confidence:     data.Confidence,
		GridMetadata: l9endpoints.GridMetadata{
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

// hintSceneAdapter bridges sqlite.ReplayCaseStore to sweep.SceneGetter.
type hintSceneAdapter struct {
	store *sqlite.ReplayCaseStore
}

func (a *hintSceneAdapter) GetScene(sceneID string) (*sweep.HINTScene, error) {
	scene, err := a.store.GetScene(sceneID)
	if err != nil {
		return nil, err
	}
	return &sweep.HINTScene{
		ReplayCaseID:      scene.ReplayCaseID,
		SensorID:          scene.SensorID,
		PCAPFile:          scene.PCAPFile,
		PCAPStartSecs:     scene.PCAPStartSecs,
		PCAPDurationSecs:  scene.PCAPDurationSecs,
		ReferenceRunID:    scene.ReferenceRunID,
		RecommendedParams: scene.RecommendedParams,
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
	// Stop any lingering sweep in the Runner before starting the reference run.
	// HINT owns the orchestration at this point, so it's safe to reclaim the Runner.
	a.runner.StopAndWait(5 * time.Second)

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
