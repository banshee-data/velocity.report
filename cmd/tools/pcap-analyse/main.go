//go:build pcap
// +build pcap

// Package main provides a PCAP analysis tool for LIDAR data.
// It processes PCAP files through the full tracking pipeline and exports
// categorised tracks and foreground point clouds for ML training.
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	_ "modernc.org/sqlite"
)

// Benchmark constants
const (
	// commitHashLength is the number of characters to use for abbreviated git commit hashes
	commitHashLength = 12

	// defaultFrameCapacity is the pre-allocated capacity for frame timing samples
	defaultFrameCapacity = 10000

	// bytesPerKB is the binary unit for byte size formatting (1 KB = 1024 bytes)
	bytesPerKB = 1024
)

// Config holds configuration for the PCAP analysis.
type Config struct {
	PCAPFile       string
	OutputDir      string
	SensorID       string
	UDPPort        int
	DBPath         string
	ExportCSV      bool
	ExportJSON     bool
	ExportTraining bool
	Verbose        bool
	FrameRate      float64 // Expected frame rate in Hz

	// Benchmark settings
	Benchmark           bool
	BenchmarkOutput     string
	Quiet               bool
	CompareBaseline     string
	RegressionThreshold float64
}

// AnalysisResult holds the results of PCAP analysis.
type AnalysisResult struct {
	PCAPFile           string                `json:"pcap_file"`
	Duration           time.Duration         `json:"duration_ns"`
	DurationSecs       float64               `json:"duration_secs"`
	TotalPackets       int                   `json:"total_packets"`
	TotalPoints        int                   `json:"total_points"`
	TotalFrames        int                   `json:"total_frames"`
	ForegroundPoints   int                   `json:"foreground_points"`
	BackgroundPoints   int                   `json:"background_points"`
	TotalClusters      int                   `json:"total_clusters"`
	TotalTracks        int                   `json:"total_tracks"`
	ConfirmedTracks    int                   `json:"confirmed_tracks"`
	TracksByClass      map[string]int        `json:"tracks_by_class"`
	ProcessingTimeMs   int64                 `json:"processing_time_ms"`
	Tracks             []*TrackExport        `json:"tracks,omitempty"`
	ClassificationDist map[string]ClassStats `json:"classification_distribution"`
	SpeedStats         SpeedStatistics       `json:"speed_statistics"`
	TrainingFrames     int                   `json:"training_frames,omitempty"`
}

// TrackExport represents a track for export.
type TrackExport struct {
	TrackID       string  `json:"track_id"`
	Class         string  `json:"class"`
	Confidence    float32 `json:"confidence"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	DurationSecs  float64 `json:"duration_secs"`
	Observations  int     `json:"observations"`
	AvgSpeedMps   float32 `json:"avg_speed_mps"`
	PeakSpeedMps  float32 `json:"peak_speed_mps"`
	P50SpeedMps   float32 `json:"p50_speed_mps"`
	P85SpeedMps   float32 `json:"p85_speed_mps"`
	P95SpeedMps   float32 `json:"p95_speed_mps"`
	AvgHeight     float32 `json:"avg_height_m"`
	AvgLength     float32 `json:"avg_length_m"`
	AvgWidth      float32 `json:"avg_width_m"`
	HeightP95Max  float32 `json:"height_p95_max_m"`
	StartX        float32 `json:"start_x_m"`
	StartY        float32 `json:"start_y_m"`
	EndX          float32 `json:"end_x_m"`
	EndY          float32 `json:"end_y_m"`
	TotalDistance float32 `json:"total_distance_m"`
}

// ClassStats holds statistics for a classification category.
type ClassStats struct {
	Count           int     `json:"count"`
	AvgSpeed        float32 `json:"avg_speed_mps"`
	AvgDuration     float32 `json:"avg_duration_secs"`
	AvgObservations float32 `json:"avg_observations"`
}

// SpeedStatistics holds overall speed statistics.
type SpeedStatistics struct {
	MinSpeed float32 `json:"min_speed_mps"`
	MaxSpeed float32 `json:"max_speed_mps"`
	AvgSpeed float32 `json:"avg_speed_mps"`
	P50Speed float32 `json:"p50_speed_mps"`
	P85Speed float32 `json:"p85_speed_mps"`
	P95Speed float32 `json:"p95_speed_mps"`
}

// TrainingFrame represents a frame prepared for ML ingestion.
type TrainingFrame struct {
	FrameID          int       `json:"frame_id"`
	Timestamp        time.Time `json:"timestamp"`
	SensorID         string    `json:"sensor_id"`
	TotalPoints      int       `json:"total_points"`
	ForegroundPoints int       `json:"foreground_points"`
	Clusters         int       `json:"clusters"`
	ActiveTracks     int       `json:"active_tracks"`
	ForegroundBlob   []byte    `json:"-"` // Binary blob not included in JSON
}

// FrameTimeStats holds statistics for per-frame processing times.
type FrameTimeStats struct {
	MinMs   float64 `json:"min_ms"`
	MaxMs   float64 `json:"max_ms"`
	AvgMs   float64 `json:"avg_ms"`
	P50Ms   float64 `json:"p50_ms"`
	P95Ms   float64 `json:"p95_ms"`
	P99Ms   float64 `json:"p99_ms"`
	Samples int     `json:"samples"`
}

// PerformanceMetrics captures comprehensive performance metrics for benchmarking.
type PerformanceMetrics struct {
	// Timing
	WallClockMs    int64          `json:"wall_clock_ms"`
	FrameTimeStats FrameTimeStats `json:"frame_time_stats"`

	// Throughput
	FramesPerSecond  float64 `json:"frames_per_second"`
	PacketsPerSecond float64 `json:"packets_per_second"`
	PointsPerSecond  float64 `json:"points_per_second"`

	// Memory (from runtime.MemStats)
	HeapAllocBytes  uint64 `json:"heap_alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	NumGC           uint32 `json:"num_gc"`
	GCPauseNs       uint64 `json:"gc_pause_ns"`

	// Pipeline Stage Timing
	PipelineTimeMs int64 `json:"pipeline_time_ms"` // Total PCAP reading + frame processing time
	ClusterTimeMs  int64 `json:"cluster_time_ms"`
	TrackingTimeMs int64 `json:"tracking_time_ms"`
	ClassifyTimeMs int64 `json:"classify_time_ms"`
}

// SystemInfo captures system information for benchmark reproducibility.
type SystemInfo struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	NumCPU     int    `json:"num_cpu"`
	GoVersion  string `json:"go_version"`
	CommitHash string `json:"commit_hash,omitempty"`
}

// BenchmarkResult is the output format for benchmark runs.
type BenchmarkResult struct {
	Version    string               `json:"version"`
	Timestamp  string               `json:"timestamp"`
	PCAPFile   string               `json:"pcap_file"`
	SystemInfo SystemInfo           `json:"system_info"`
	Metrics    PerformanceMetrics   `json:"metrics"`
	Comparison *BenchmarkComparison `json:"comparison,omitempty"`
}

// BenchmarkComparison holds comparison results against a baseline.
type BenchmarkComparison struct {
	BaselineFile string             `json:"baseline_file"`
	Regressions  []MetricDifference `json:"regressions,omitempty"`
	Improvements []MetricDifference `json:"improvements,omitempty"`
}

// MetricDifference represents a change in a specific metric.
type MetricDifference struct {
	Metric        string  `json:"metric"`
	BaselineValue float64 `json:"baseline_value"`
	CurrentValue  float64 `json:"current_value"`
	ChangePercent float64 `json:"change_percent"`
}

func main() {
	config := parseFlags()

	if config.PCAPFile == "" {
		fmt.Fprintln(os.Stderr, "Error: PCAP file is required")
		flag.Usage()
		os.Exit(1)
	}

	if _, err := os.Stat(config.PCAPFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: PCAP file not found: %s\n", config.PCAPFile)
		os.Exit(1)
	}

	// Create output directory
	if config.OutputDir != "" {
		if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// In benchmark mode with quiet flag, suppress verbose output
	if config.Benchmark && config.Quiet {
		config.Verbose = false
		log.SetOutput(io.Discard) // Suppress all logging to avoid measurement interference
	}

	// Run analysis with benchmark metrics collection
	var benchMetrics *PerformanceMetrics
	var result *AnalysisResult
	var err error

	if config.Benchmark {
		result, benchMetrics, err = analyzePCAPWithBenchmark(config)
	} else {
		result, err = analyzePCAP(config)
	}
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	// Print summary (unless in quiet mode)
	if !config.Quiet {
		printSummary(result)
	}

	// Export results
	if err := exportResults(config, result); err != nil {
		log.Fatalf("Export failed: %v", err)
	}

	// Handle benchmark output and comparison
	if config.Benchmark && benchMetrics != nil {
		exitCode := handleBenchmarkOutput(config, result, benchMetrics)
		os.Exit(exitCode)
	}
}

func parseFlags() Config {
	config := Config{}

	flag.StringVar(&config.PCAPFile, "pcap", "", "Path to PCAP file (required)")
	flag.StringVar(&config.OutputDir, "output", ".", "Output directory for results")
	flag.StringVar(&config.SensorID, "sensor-id", "hesai-pandar40p", "Sensor ID")
	flag.IntVar(&config.UDPPort, "port", 2369, "UDP port for LIDAR data")
	flag.StringVar(&config.DBPath, "db", "", "SQLite database path (optional, for persistence)")
	flag.BoolVar(&config.ExportCSV, "csv", true, "Export tracks to CSV")
	flag.BoolVar(&config.ExportJSON, "json", true, "Export full results to JSON")
	flag.BoolVar(&config.ExportTraining, "training", false, "Export training data (foreground blobs)")
	flag.BoolVar(&config.Verbose, "v", false, "Verbose output")
	flag.Float64Var(&config.FrameRate, "fps", 10.0, "Expected frame rate in Hz")

	// Benchmark flags (short and long forms bind to same variable for convenience)
	flag.BoolVar(&config.Benchmark, "benchmark", false, "Enable performance measurement mode")
	flag.BoolVar(&config.Benchmark, "bench", false, "Enable performance measurement mode (alias for -benchmark)")
	flag.StringVar(&config.BenchmarkOutput, "benchmark-output", "", "Output file for benchmark JSON (default: {pcap}_benchmark.json)")
	flag.BoolVar(&config.Quiet, "quiet", false, "Suppress verbose output to prevent logging from affecting measurements")
	flag.BoolVar(&config.Quiet, "q", false, "Suppress verbose output (alias for -quiet)")
	flag.StringVar(&config.CompareBaseline, "compare-baseline", "", "Compare against a baseline benchmark file")
	flag.Float64Var(&config.RegressionThreshold, "regression-threshold", 0.10, "Threshold for flagging regressions (default: 0.10 = 10%)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "PCAP Analysis Tool for LIDAR Track Categorization and ML Training Data Extraction\n\n")
		fmt.Fprintf(os.Stderr, "This tool processes PCAP files through the full LIDAR tracking pipeline:\n")
		fmt.Fprintf(os.Stderr, "  1. Parse UDP packets to extract LIDAR points\n")
		fmt.Fprintf(os.Stderr, "  2. Build 360° frames from point stream\n")
		fmt.Fprintf(os.Stderr, "  3. Classify foreground/background using adaptive background model\n")
		fmt.Fprintf(os.Stderr, "  4. Cluster foreground points using DBSCAN\n")
		fmt.Fprintf(os.Stderr, "  5. Track clusters using Kalman filter\n")
		fmt.Fprintf(os.Stderr, "  6. Classify tracks (pedestrian, car, bird, other)\n")
		fmt.Fprintf(os.Stderr, "  7. Export results for ML training\n\n")
		fmt.Fprintf(os.Stderr, "Benchmark Mode:\n")
		fmt.Fprintf(os.Stderr, "  -benchmark              Enable performance measurement\n")
		fmt.Fprintf(os.Stderr, "  -benchmark-output FILE  Output benchmark JSON to FILE\n")
		fmt.Fprintf(os.Stderr, "  -quiet                  Suppress output to reduce measurement noise\n")
		fmt.Fprintf(os.Stderr, "  -compare-baseline FILE  Compare against baseline, exit 1 on regression\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -pcap capture.pcap -output ./results\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -pcap capture.pcap -training -output ./ml_data\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -pcap capture.pcap -benchmark -quiet -benchmark-output perf.json\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -pcap capture.pcap -benchmark -compare-baseline baseline.json\n", os.Args[0])
	}

	flag.Parse()
	return config
}

// analysisStats implements network.PacketStatsInterface for tracking analysis statistics.
type analysisStats struct {
	mu       sync.Mutex
	packets  int
	points   int
	dropped  int
	firstPkt time.Time
	lastPkt  time.Time
}

func (s *analysisStats) AddPacket(bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets++
	now := time.Now()
	if s.firstPkt.IsZero() {
		s.firstPkt = now
	}
	s.lastPkt = now
}

func (s *analysisStats) AddDropped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropped++
}

func (s *analysisStats) AddPoints(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points += count
}

func (s *analysisStats) LogStats(parsePackets bool) {
	// No-op for analysis mode - we report at the end
}

func (s *analysisStats) getStats() (packets, points int, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.packets, s.points, s.lastPkt.Sub(s.firstPkt)
}

// analysisFrameBuilder implements network.FrameBuilder for collecting frames and processing.
type analysisFrameBuilder struct {
	mu             sync.Mutex
	points         []lidar.PointPolar
	lastAzimuth    float64
	frameStartTime time.Time
	frameCount     int
	motorSpeed     uint16

	// Processing components
	bgManager  *lidar.BackgroundManager
	tracker    *lidar.Tracker
	classifier *lidar.TrackClassifier
	config     Config

	// Results
	result         *AnalysisResult
	trainingFrames []*TrainingFrame

	// Benchmark metrics (only populated in benchmark mode)
	benchmarkMode  bool
	frameTimes     []float64 // Per-frame processing times in milliseconds
	clusterTimeNs  int64     // Cumulative clustering time
	trackTimeNs    int64     // Cumulative tracking time
	classifyTimeNs int64     // Cumulative classification time
}

func newAnalysisFrameBuilder(config Config, result *AnalysisResult) *analysisFrameBuilder {
	fb := &analysisFrameBuilder{
		points:        make([]lidar.PointPolar, 0, 50000),
		bgManager:     createBackgroundManager(config.SensorID),
		tracker:       lidar.NewTracker(lidar.DefaultTrackerConfig()),
		classifier:    lidar.NewTrackClassifier(),
		config:        config,
		result:        result,
		benchmarkMode: config.Benchmark,
	}
	if config.Benchmark {
		// Pre-allocate frame times array (estimate based on typical PCAP duration)
		fb.frameTimes = make([]float64, 0, defaultFrameCapacity)
	}
	return fb
}

func (fb *analysisFrameBuilder) AddPointsPolar(points []lidar.PointPolar) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if len(points) == 0 {
		return
	}

	// Track packet timestamps
	pktTime := time.Unix(0, points[0].Timestamp)
	if fb.frameStartTime.IsZero() {
		fb.frameStartTime = pktTime
	}

	// Detect frame completion (360° rotation)
	for _, p := range points {
		// Check for azimuth wrap (new frame)
		if fb.lastAzimuth > 270 && p.Azimuth < 90 {
			// Frame complete - process it
			if len(fb.points) > 0 {
				fb.processCurrentFrame()
				fb.frameCount++
			}

			// Start new frame
			fb.points = fb.points[:0]
			fb.frameStartTime = pktTime
		}

		fb.points = append(fb.points, p)
		fb.lastAzimuth = p.Azimuth
	}
}

func (fb *analysisFrameBuilder) SetMotorSpeed(rpm uint16) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.motorSpeed = rpm
}

// processCurrentFrame processes the accumulated points as a complete frame.
// MUST be called while holding fb.mu lock (caller is responsible for locking).
func (fb *analysisFrameBuilder) processCurrentFrame() {
	var frameStart time.Time
	if fb.benchmarkMode {
		frameStart = time.Now()
	}

	// Step 1: Foreground extraction
	mask, err := fb.bgManager.ProcessFramePolarWithMask(fb.points)
	if err != nil || mask == nil {
		if fb.benchmarkMode {
			fb.frameTimes = append(fb.frameTimes, float64(time.Since(frameStart).Nanoseconds())/1e6)
		}
		return
	}

	// Extract foreground points
	foregroundPoints := lidar.ExtractForegroundPoints(fb.points, mask)
	foregroundCount := len(foregroundPoints)

	fb.result.TotalFrames++
	fb.result.ForegroundPoints += foregroundCount
	fb.result.BackgroundPoints += len(fb.points) - foregroundCount

	if foregroundCount == 0 {
		if fb.benchmarkMode {
			fb.frameTimes = append(fb.frameTimes, float64(time.Since(frameStart).Nanoseconds())/1e6)
		}
		return
	}

	// Step 2: Transform to world frame
	worldPoints := lidar.TransformToWorld(foregroundPoints, nil, fb.config.SensorID)

	// Step 3: Cluster (respect runtime foreground clustering params)
	var clusterStart time.Time
	if fb.benchmarkMode {
		clusterStart = time.Now()
	}
	dbscanParams := lidar.DefaultDBSCANParams()
	if fb.bgManager != nil {
		p := fb.bgManager.GetParams()
		if p.ForegroundMinClusterPoints > 0 {
			dbscanParams.MinPts = p.ForegroundMinClusterPoints
		}
		if p.ForegroundDBSCANEps > 0 {
			dbscanParams.Eps = float64(p.ForegroundDBSCANEps)
		}
	}
	clusters := lidar.DBSCAN(worldPoints, dbscanParams)
	if fb.benchmarkMode {
		fb.clusterTimeNs += time.Since(clusterStart).Nanoseconds()
	}
	fb.result.TotalClusters += len(clusters)

	if len(clusters) == 0 {
		if fb.benchmarkMode {
			fb.frameTimes = append(fb.frameTimes, float64(time.Since(frameStart).Nanoseconds())/1e6)
		}
		return
	}

	// Step 4: Track
	var trackStart time.Time
	if fb.benchmarkMode {
		trackStart = time.Now()
	}
	fb.tracker.Update(clusters, fb.frameStartTime)
	if fb.benchmarkMode {
		fb.trackTimeNs += time.Since(trackStart).Nanoseconds()
	}

	// Step 5: Classify confirmed tracks
	var classifyStart time.Time
	if fb.benchmarkMode {
		classifyStart = time.Now()
	}
	for _, track := range fb.tracker.GetConfirmedTracks() {
		if track.ObjectClass == "" && track.ObservationCount >= 5 {
			fb.classifier.ClassifyAndUpdate(track)
		}
	}
	if fb.benchmarkMode {
		fb.classifyTimeNs += time.Since(classifyStart).Nanoseconds()
	}

	// Collect training data if requested
	if fb.config.ExportTraining && foregroundCount > 0 {
		trainingFrame := &TrainingFrame{
			FrameID:          fb.frameCount,
			Timestamp:        fb.frameStartTime,
			SensorID:         fb.config.SensorID,
			TotalPoints:      len(fb.points),
			ForegroundPoints: foregroundCount,
			Clusters:         len(clusters),
			ActiveTracks:     len(fb.tracker.GetActiveTracks()),
			ForegroundBlob:   lidar.EncodeForegroundBlob(foregroundPoints),
		}
		fb.trainingFrames = append(fb.trainingFrames, trainingFrame)
	}

	if fb.config.Verbose && fb.frameCount%100 == 0 {
		log.Printf("Frame %d: %d points, %d foreground, %d clusters, %d tracks",
			fb.frameCount, len(fb.points), foregroundCount,
			len(clusters), len(fb.tracker.GetActiveTracks()))
	}

	// Record total frame processing time
	if fb.benchmarkMode {
		fb.frameTimes = append(fb.frameTimes, float64(time.Since(frameStart).Nanoseconds())/1e6)
	}
}

func (fb *analysisFrameBuilder) finalise() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	// Process final partial frame
	if len(fb.points) > 0 {
		fb.processCurrentFrame()
	}
}

func (fb *analysisFrameBuilder) getTracker() *lidar.Tracker {
	return fb.tracker
}

func (fb *analysisFrameBuilder) getClassifier() *lidar.TrackClassifier {
	return fb.classifier
}

func (fb *analysisFrameBuilder) getTrainingFrames() []*TrainingFrame {
	return fb.trainingFrames
}

// getBenchmarkData returns the collected benchmark timing data.
// NOTE: This method must be called after processing is complete (after finalise()),
// when the frame builder is no longer being modified concurrently.
func (fb *analysisFrameBuilder) getBenchmarkData() (frameTimes []float64, clusterNs, trackNs, classifyNs int64) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return fb.frameTimes, fb.clusterTimeNs, fb.trackTimeNs, fb.classifyTimeNs
}

// collectTrackResults processes tracks from the frame builder and populates the result.
// Returns the list of all tracks for optional persistence.
func collectTrackResults(frameBuilder *analysisFrameBuilder, result *AnalysisResult) []*lidar.TrackedObject {
	tracker := frameBuilder.getTracker()
	classifier := frameBuilder.getClassifier()
	allTracks := tracker.GetAllTracks()

	result.TotalTracks = len(allTracks)
	result.Tracks = make([]*TrackExport, 0, len(allTracks))

	var speedSamples []float32

	for _, track := range allTracks {
		// Classify if not already done
		if track.ObjectClass == "" && track.ObservationCount >= 5 {
			classifier.ClassifyAndUpdate(track)
		}

		if track.State == lidar.TrackConfirmed {
			result.ConfirmedTracks++
		}

		class := track.ObjectClass
		if class == "" {
			class = "other"
		}
		result.TracksByClass[class]++

		// Export track data
		p50, p85, p95 := lidar.ComputeSpeedPercentiles(track.SpeedHistory())

		trackExport := &TrackExport{
			TrackID:      track.TrackID,
			Class:        class,
			Confidence:   track.ObjectConfidence,
			StartTime:    time.Unix(0, track.FirstUnixNanos).Format(time.RFC3339),
			EndTime:      time.Unix(0, track.LastUnixNanos).Format(time.RFC3339),
			DurationSecs: float64(track.LastUnixNanos-track.FirstUnixNanos) / 1e9,
			Observations: track.ObservationCount,
			AvgSpeedMps:  track.AvgSpeedMps,
			PeakSpeedMps: track.PeakSpeedMps,
			P50SpeedMps:  p50,
			P85SpeedMps:  p85,
			P95SpeedMps:  p95,
			AvgHeight:    track.BoundingBoxHeightAvg,
			AvgLength:    track.BoundingBoxLengthAvg,
			AvgWidth:     track.BoundingBoxWidthAvg,
			HeightP95Max: track.HeightP95Max,
			StartX:       track.X,
			StartY:       track.Y,
		}
		result.Tracks = append(result.Tracks, trackExport)

		if track.AvgSpeedMps > 0 {
			speedSamples = append(speedSamples, track.AvgSpeedMps)
		}
	}

	// Compute classification distribution and speed statistics
	result.ClassificationDist = computeClassStats(result.Tracks)
	result.SpeedStats = computeSpeedStats(speedSamples)

	return allTracks
}

func analyzePCAP(config Config) (*AnalysisResult, error) {
	startTime := time.Now()

	// Initialise parser
	parserConfig, _ := parse.LoadEmbeddedPandar40PConfig()
	parser := parse.NewPandar40PParser(*parserConfig)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)

	// Result tracking
	result := &AnalysisResult{
		PCAPFile:      config.PCAPFile,
		TracksByClass: make(map[string]int),
	}

	// Create analysis-specific frame builder that processes tracking pipeline
	stats := &analysisStats{}
	frameBuilder := newAnalysisFrameBuilder(config, result)

	// Use shared PCAP reading infrastructure from internal/lidar/network
	// No forwarder needed for offline analysis
	ctx := context.Background()
	if err := network.ReadPCAPFile(ctx, config.PCAPFile, config.UDPPort, parser, frameBuilder, stats, nil); err != nil {
		return nil, fmt.Errorf("failed to read PCAP: %w", err)
	}

	// Finalise any remaining frame data
	frameBuilder.finalise()

	// Get statistics from the shared reader
	packets, points, duration := stats.getStats()
	result.TotalPackets = packets
	result.TotalPoints = points
	result.Duration = duration
	result.DurationSecs = duration.Seconds()

	// Collect track results using shared helper
	allTracks := collectTrackResults(frameBuilder, result)

	// Processing time
	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	// Training frame count
	trainingFrames := frameBuilder.getTrainingFrames()
	result.TrainingFrames = len(trainingFrames)

	// Persist to DB if requested
	if config.DBPath != "" {
		if err := persistToDatabase(config.DBPath, result, allTracks); err != nil {
			log.Printf("Warning: database persistence failed: %v", err)
		}
	}

	// Export training data
	if config.ExportTraining && len(trainingFrames) > 0 {
		if err := exportTrainingData(config.OutputDir, trainingFrames); err != nil {
			log.Printf("Warning: training data export failed: %v", err)
		}
	}

	return result, nil
}

// analyzePCAPWithBenchmark runs PCAP analysis with performance metrics collection.
func analyzePCAPWithBenchmark(config Config) (*AnalysisResult, *PerformanceMetrics, error) {
	// Force GC before starting to get clean memory baseline
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()

	// Initialise parser
	parserConfig, _ := parse.LoadEmbeddedPandar40PConfig()
	parser := parse.NewPandar40PParser(*parserConfig)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)

	parseStart := time.Now()

	// Result tracking
	result := &AnalysisResult{
		PCAPFile:      config.PCAPFile,
		TracksByClass: make(map[string]int),
	}

	// Create analysis-specific frame builder that processes tracking pipeline
	stats := &analysisStats{}
	frameBuilder := newAnalysisFrameBuilder(config, result)

	// Use shared PCAP reading infrastructure from internal/lidar/network
	ctx := context.Background()
	if err := network.ReadPCAPFile(ctx, config.PCAPFile, config.UDPPort, parser, frameBuilder, stats, nil); err != nil {
		return nil, nil, fmt.Errorf("failed to read PCAP: %w", err)
	}

	// Finalise any remaining frame data
	frameBuilder.finalise()

	pipelineTimeMs := time.Since(parseStart).Milliseconds()

	// Get statistics from the shared reader
	packets, points, duration := stats.getStats()
	result.TotalPackets = packets
	result.TotalPoints = points
	result.Duration = duration
	result.DurationSecs = duration.Seconds()

	// Collect track results using shared helper
	allTracks := collectTrackResults(frameBuilder, result)

	wallClockMs := time.Since(startTime).Milliseconds()
	result.ProcessingTimeMs = wallClockMs

	trainingFrames := frameBuilder.getTrainingFrames()
	result.TrainingFrames = len(trainingFrames)

	// Collect memory stats BEFORE persistence/export operations
	// to get accurate LIDAR pipeline memory footprint
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Get benchmark data from frame builder
	frameTimes, clusterNs, trackNs, classifyNs := frameBuilder.getBenchmarkData()

	// Compute frame time statistics
	frameStats := computeFrameTimeStats(frameTimes)

	// Build performance metrics
	metrics := &PerformanceMetrics{
		WallClockMs:    wallClockMs,
		FrameTimeStats: frameStats,

		// Throughput (based on wall clock time)
		FramesPerSecond:  float64(result.TotalFrames) / (float64(wallClockMs) / 1000.0),
		PacketsPerSecond: float64(result.TotalPackets) / (float64(wallClockMs) / 1000.0),
		PointsPerSecond:  float64(result.TotalPoints) / (float64(wallClockMs) / 1000.0),

		// Memory
		HeapAllocBytes:  memAfter.HeapAlloc,
		TotalAllocBytes: memAfter.TotalAlloc - memBefore.TotalAlloc,
		NumGC:           memAfter.NumGC - memBefore.NumGC,
		GCPauseNs:       memAfter.PauseTotalNs - memBefore.PauseTotalNs,

		// Pipeline stage timing
		PipelineTimeMs: pipelineTimeMs,
		ClusterTimeMs:  clusterNs / 1e6,
		TrackingTimeMs: trackNs / 1e6,
		ClassifyTimeMs: classifyNs / 1e6,
	}

	// Persist to DB if requested (after memory stats collection)
	if config.DBPath != "" {
		if err := persistToDatabase(config.DBPath, result, allTracks); err != nil {
			log.Printf("Warning: database persistence failed: %v", err)
		}
	}

	// Export training data (after memory stats collection)
	if config.ExportTraining && len(trainingFrames) > 0 {
		if err := exportTrainingData(config.OutputDir, trainingFrames); err != nil {
			log.Printf("Warning: training data export failed: %v", err)
		}
	}

	return result, metrics, nil
}

func createBackgroundManager(sensorID string) *lidar.BackgroundManager {
	grid := &lidar.BackgroundGrid{
		SensorID:    sensorID,
		Rings:       40,
		AzimuthBins: 1800, // 0.2° resolution
		Cells:       make([]lidar.BackgroundCell, 40*1800),
		Params: lidar.BackgroundParams{
			BackgroundUpdateFraction:       0.02,
			ClosenessSensitivityMultiplier: 3.0,
			SafetyMarginMeters:             0.5,
			NeighborConfirmationCount:      3,
			NoiseRelativeFraction:          0.315,
			SeedFromFirstObservation:       true, // Important for PCAP replay
			FreezeDurationNanos:            int64(5 * time.Second),
		},
	}

	return &lidar.BackgroundManager{
		Grid:      grid,
		StartTime: time.Now(),
	}
}

func computeClassStats(tracks []*TrackExport) map[string]ClassStats {
	stats := make(map[string]ClassStats)
	byClass := make(map[string][]*TrackExport)

	for _, t := range tracks {
		byClass[t.Class] = append(byClass[t.Class], t)
	}

	for class, classTracks := range byClass {
		var sumSpeed, sumDuration float32
		var sumObs int
		for _, t := range classTracks {
			sumSpeed += t.AvgSpeedMps
			sumDuration += float32(t.DurationSecs)
			sumObs += t.Observations
		}
		n := float32(len(classTracks))
		stats[class] = ClassStats{
			Count:           len(classTracks),
			AvgSpeed:        sumSpeed / n,
			AvgDuration:     sumDuration / n,
			AvgObservations: float32(sumObs) / n,
		}
	}

	return stats
}

func computeSpeedStats(samples []float32) SpeedStatistics {
	if len(samples) == 0 {
		return SpeedStatistics{}
	}

	// Sort for min/max and average
	sorted := make([]float32, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum float32
	for _, s := range sorted {
		sum += s
	}

	// Use the shared percentile computation from lidar package
	p50, p85, p95 := lidar.ComputeSpeedPercentiles(samples)

	n := len(sorted)
	return SpeedStatistics{
		MinSpeed: sorted[0],
		MaxSpeed: sorted[n-1],
		AvgSpeed: sum / float32(n),
		P50Speed: p50,
		P85Speed: p85,
		P95Speed: p95,
	}
}

func printSummary(result *AnalysisResult) {
	fmt.Println("\n========== PCAP Analysis Summary ==========")
	fmt.Printf("File: %s\n", result.PCAPFile)
	fmt.Printf("Duration: %.1f seconds (%.1f minutes)\n", result.DurationSecs, result.DurationSecs/60)
	fmt.Printf("Processing time: %d ms\n", result.ProcessingTimeMs)
	fmt.Println()
	fmt.Printf("Packets: %d\n", result.TotalPackets)
	fmt.Printf("Points: %d total, %d foreground (%.1f%%), %d background\n",
		result.TotalPoints, result.ForegroundPoints,
		100*float64(result.ForegroundPoints)/float64(result.TotalPoints),
		result.BackgroundPoints)
	fmt.Printf("Frames: %d (%.1f fps)\n", result.TotalFrames, float64(result.TotalFrames)/result.DurationSecs)
	fmt.Printf("Clusters: %d\n", result.TotalClusters)
	fmt.Println()
	fmt.Printf("Tracks: %d total, %d confirmed\n", result.TotalTracks, result.ConfirmedTracks)
	fmt.Println("\nTracks by Class:")
	for class, count := range result.TracksByClass {
		pct := 100 * float64(count) / float64(result.TotalTracks)
		fmt.Printf("  %s: %d (%.1f%%)\n", class, count, pct)
	}
	fmt.Println("\nSpeed Statistics (confirmed tracks):")
	fmt.Printf("  Min: %.2f m/s (%.1f km/h)\n", result.SpeedStats.MinSpeed, result.SpeedStats.MinSpeed*3.6)
	fmt.Printf("  Max: %.2f m/s (%.1f km/h)\n", result.SpeedStats.MaxSpeed, result.SpeedStats.MaxSpeed*3.6)
	fmt.Printf("  Avg: %.2f m/s (%.1f km/h)\n", result.SpeedStats.AvgSpeed, result.SpeedStats.AvgSpeed*3.6)
	fmt.Printf("  P85: %.2f m/s (%.1f km/h)\n", result.SpeedStats.P85Speed, result.SpeedStats.P85Speed*3.6)
	fmt.Println()
	if result.TrainingFrames > 0 {
		fmt.Printf("Training frames exported: %d\n", result.TrainingFrames)
	}
	fmt.Println("=============================================")
}

func exportResults(config Config, result *AnalysisResult) error {
	baseName := strings.TrimSuffix(filepath.Base(config.PCAPFile), filepath.Ext(config.PCAPFile))

	// Export JSON
	if config.ExportJSON {
		jsonPath := filepath.Join(config.OutputDir, baseName+"_analysis.json")
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("JSON marshal: %w", err)
		}
		if err := os.WriteFile(jsonPath, data, 0644); err != nil {
			return fmt.Errorf("write JSON: %w", err)
		}
		fmt.Printf("JSON results: %s\n", jsonPath)
	}

	// Export CSV
	if config.ExportCSV && len(result.Tracks) > 0 {
		csvPath := filepath.Join(config.OutputDir, baseName+"_tracks.csv")
		if err := exportTracksCSV(csvPath, result.Tracks); err != nil {
			return fmt.Errorf("write CSV: %w", err)
		}
		fmt.Printf("CSV tracks: %s\n", csvPath)
	}

	return nil
}

func exportTracksCSV(path string, tracks []*TrackExport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header
	header := []string{
		"track_id", "class", "confidence", "start_time", "end_time",
		"duration_secs", "observations", "avg_speed_mps", "peak_speed_mps",
		"p50_speed_mps", "p85_speed_mps", "p95_speed_mps",
		"avg_height_m", "avg_length_m", "avg_width_m", "height_p95_max_m",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	// Data rows
	for _, t := range tracks {
		row := []string{
			t.TrackID,
			t.Class,
			strconv.FormatFloat(float64(t.Confidence), 'f', 3, 32),
			t.StartTime,
			t.EndTime,
			strconv.FormatFloat(t.DurationSecs, 'f', 2, 64),
			strconv.Itoa(t.Observations),
			strconv.FormatFloat(float64(t.AvgSpeedMps), 'f', 2, 32),
			strconv.FormatFloat(float64(t.PeakSpeedMps), 'f', 2, 32),
			strconv.FormatFloat(float64(t.P50SpeedMps), 'f', 2, 32),
			strconv.FormatFloat(float64(t.P85SpeedMps), 'f', 2, 32),
			strconv.FormatFloat(float64(t.P95SpeedMps), 'f', 2, 32),
			strconv.FormatFloat(float64(t.AvgHeight), 'f', 3, 32),
			strconv.FormatFloat(float64(t.AvgLength), 'f', 3, 32),
			strconv.FormatFloat(float64(t.AvgWidth), 'f', 3, 32),
			strconv.FormatFloat(float64(t.HeightP95Max), 'f', 3, 32),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

func exportTrainingData(outputDir string, frames []*TrainingFrame) error {
	trainingDir := filepath.Join(outputDir, "training_data")
	if err := os.MkdirAll(trainingDir, 0755); err != nil {
		return err
	}

	// Export metadata JSON
	metadataPath := filepath.Join(trainingDir, "frames_metadata.json")
	metadata := make([]map[string]interface{}, len(frames))
	for i, f := range frames {
		metadata[i] = map[string]interface{}{
			"frame_id":          f.FrameID,
			"timestamp":         f.Timestamp.Format(time.RFC3339Nano),
			"sensor_id":         f.SensorID,
			"total_points":      f.TotalPoints,
			"foreground_points": f.ForegroundPoints,
			"clusters":          f.Clusters,
			"active_tracks":     f.ActiveTracks,
			"blob_file":         fmt.Sprintf("frame_%06d.bin", f.FrameID),
		}
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return err
	}

	// Export binary blobs
	for _, f := range frames {
		blobPath := filepath.Join(trainingDir, fmt.Sprintf("frame_%06d.bin", f.FrameID))
		if err := os.WriteFile(blobPath, f.ForegroundBlob, 0644); err != nil {
			return err
		}
	}

	fmt.Printf("Training data: %s (%d frames)\n", trainingDir, len(frames))
	return nil
}

func persistToDatabase(dbPath string, result *AnalysisResult, tracks []*lidar.TrackedObject) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Create tables if they don't exist
	schema := `
		CREATE TABLE IF NOT EXISTS analysis_runs (
			run_id INTEGER PRIMARY KEY,
			pcap_file TEXT NOT NULL,
			analysis_time TEXT NOT NULL,
			duration_secs REAL,
			total_packets INTEGER,
			total_frames INTEGER,
			total_tracks INTEGER,
			confirmed_tracks INTEGER
		);

		CREATE TABLE IF NOT EXISTS analyzed_tracks (
			track_id TEXT PRIMARY KEY,
			run_id INTEGER,
			class TEXT,
			confidence REAL,
			duration_secs REAL,
			observations INTEGER,
			avg_speed_mps REAL,
			peak_speed_mps REAL,
			p50_speed_mps REAL,
			p85_speed_mps REAL,
			p95_speed_mps REAL,
			avg_height REAL,
			avg_length REAL,
			avg_width REAL,
			FOREIGN KEY (run_id) REFERENCES analysis_runs(run_id)
		);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Insert run
	res, err := db.Exec(`
		INSERT INTO analysis_runs (pcap_file, analysis_time, duration_secs, total_packets, total_frames, total_tracks, confirmed_tracks)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		result.PCAPFile,
		time.Now().Format(time.RFC3339),
		result.DurationSecs,
		result.TotalPackets,
		result.TotalFrames,
		result.TotalTracks,
		result.ConfirmedTracks,
	)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}

	runID, _ := res.LastInsertId()

	// Insert tracks
	for _, t := range result.Tracks {
		_, err := db.Exec(`
			INSERT OR REPLACE INTO analyzed_tracks
			(track_id, run_id, class, confidence, duration_secs, observations, avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps, avg_height, avg_length, avg_width)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.TrackID, runID, t.Class, t.Confidence, t.DurationSecs, t.Observations,
			t.AvgSpeedMps, t.PeakSpeedMps, t.P50SpeedMps, t.P85SpeedMps, t.P95SpeedMps,
			t.AvgHeight, t.AvgLength, t.AvgWidth,
		)
		if err != nil {
			log.Printf("Warning: failed to insert track %s: %v", t.TrackID, err)
		}
	}

	return nil
}

// computeFrameTimeStats computes statistics for frame processing times.
// Uses floor-based indexing for percentiles, matching ComputeSpeedPercentiles
// in the lidar package. For small sample sizes (n<3), percentiles may be similar.
func computeFrameTimeStats(frameTimes []float64) FrameTimeStats {
	if len(frameTimes) == 0 {
		return FrameTimeStats{}
	}

	// Make a copy and sort for percentiles
	sorted := make([]float64, len(frameTimes))
	copy(sorted, frameTimes)
	sort.Float64s(sorted)

	// Compute basic stats
	var sum float64
	minVal := sorted[0]
	maxVal := sorted[len(sorted)-1]

	for _, v := range sorted {
		sum += v
	}
	avg := sum / float64(len(sorted))

	// Compute percentiles using floor-based indexing (consistent with lidar.ComputeSpeedPercentiles)
	n := len(sorted)
	p50Idx := int(float64(n) * 0.50)
	p95Idx := int(float64(n) * 0.95)
	p99Idx := int(float64(n) * 0.99)

	// Clamp indices
	if p50Idx >= n {
		p50Idx = n - 1
	}
	if p95Idx >= n {
		p95Idx = n - 1
	}
	if p99Idx >= n {
		p99Idx = n - 1
	}

	return FrameTimeStats{
		MinMs:   minVal,
		MaxMs:   maxVal,
		AvgMs:   avg,
		P50Ms:   sorted[p50Idx],
		P95Ms:   sorted[p95Idx],
		P99Ms:   sorted[p99Idx],
		Samples: len(frameTimes),
	}
}

// getSystemInfo collects system information for benchmark reproducibility.
func getSystemInfo() SystemInfo {
	info := SystemInfo{
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}

	// Try to get commit hash from build info
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				if len(setting.Value) > commitHashLength {
					info.CommitHash = setting.Value[:commitHashLength]
				} else {
					info.CommitHash = setting.Value
				}
				break
			}
		}
	}

	return info
}

// handleBenchmarkOutput writes benchmark results and handles comparison.
// Returns exit code (0 for success, 1 for regression detected).
func handleBenchmarkOutput(config Config, result *AnalysisResult, metrics *PerformanceMetrics) int {
	// Build benchmark result
	benchResult := BenchmarkResult{
		Version:    "1.0",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		PCAPFile:   filepath.Base(config.PCAPFile),
		SystemInfo: getSystemInfo(),
		Metrics:    *metrics,
	}

	exitCode := 0

	// Compare with baseline if provided
	if config.CompareBaseline != "" {
		comparison, hasRegression := compareWithBaseline(config.CompareBaseline, metrics, config.RegressionThreshold)
		benchResult.Comparison = comparison

		if hasRegression {
			exitCode = 1
			printComparisonSummary(comparison, config.RegressionThreshold)
		} else if comparison != nil {
			printComparisonSummary(comparison, config.RegressionThreshold)
		}
	}

	// Determine output path
	outputPath := config.BenchmarkOutput
	if outputPath == "" {
		baseName := strings.TrimSuffix(filepath.Base(config.PCAPFile), filepath.Ext(config.PCAPFile))
		outputPath = filepath.Join(config.OutputDir, baseName+"_benchmark.json")
	}

	// Write benchmark JSON
	data, err := json.MarshalIndent(benchResult, "", "  ")
	if err != nil {
		log.Printf("Error marshaling benchmark result: %v", err)
		return 1
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		log.Printf("Error writing benchmark file: %v", err)
		return 1
	}

	if !config.Quiet {
		fmt.Printf("\nBenchmark results: %s\n", outputPath)
		printBenchmarkSummary(metrics)
	}

	return exitCode
}

// compareWithBaseline compares current metrics against a baseline file.
func compareWithBaseline(baselinePath string, current *PerformanceMetrics, threshold float64) (*BenchmarkComparison, bool) {
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		log.Printf("Warning: failed to read baseline file: %v", err)
		return nil, false
	}

	var baseline BenchmarkResult
	if err := json.Unmarshal(data, &baseline); err != nil {
		log.Printf("Warning: failed to parse baseline file: %v", err)
		return nil, false
	}

	comparison := &BenchmarkComparison{
		BaselineFile: filepath.Base(baselinePath),
	}

	hasRegression := false

	// Compare key metrics (higher is worse for time metrics)
	metricsToCompare := []struct {
		name        string
		baseline    float64
		current     float64
		higherIsBad bool
	}{
		{"wall_clock_ms", float64(baseline.Metrics.WallClockMs), float64(current.WallClockMs), true},
		{"frame_time_avg_ms", baseline.Metrics.FrameTimeStats.AvgMs, current.FrameTimeStats.AvgMs, true},
		{"frame_time_p95_ms", baseline.Metrics.FrameTimeStats.P95Ms, current.FrameTimeStats.P95Ms, true},
		{"frames_per_second", baseline.Metrics.FramesPerSecond, current.FramesPerSecond, false},
		{"heap_alloc_bytes", float64(baseline.Metrics.HeapAllocBytes), float64(current.HeapAllocBytes), true},
		{"cluster_time_ms", float64(baseline.Metrics.ClusterTimeMs), float64(current.ClusterTimeMs), true},
		{"tracking_time_ms", float64(baseline.Metrics.TrackingTimeMs), float64(current.TrackingTimeMs), true},
	}

	for _, m := range metricsToCompare {
		if m.baseline == 0 {
			continue // Skip if baseline is zero to avoid division by zero
		}

		changePercent := (m.current - m.baseline) / m.baseline

		diff := MetricDifference{
			Metric:        m.name,
			BaselineValue: m.baseline,
			CurrentValue:  m.current,
			ChangePercent: changePercent * 100, // Convert to percentage
		}

		if m.higherIsBad {
			if changePercent > threshold {
				comparison.Regressions = append(comparison.Regressions, diff)
				hasRegression = true
			} else if changePercent < -threshold {
				comparison.Improvements = append(comparison.Improvements, diff)
			}
		} else {
			// For metrics where higher is better (e.g., frames_per_second)
			if changePercent < -threshold {
				comparison.Regressions = append(comparison.Regressions, diff)
				hasRegression = true
			} else if changePercent > threshold {
				comparison.Improvements = append(comparison.Improvements, diff)
			}
		}
	}

	return comparison, hasRegression
}

// printComparisonSummary prints a human-readable comparison summary.
func printComparisonSummary(comparison *BenchmarkComparison, threshold float64) {
	fmt.Printf("\n========== Benchmark Comparison ==========\n")
	fmt.Printf("Baseline: %s\n", comparison.BaselineFile)
	fmt.Printf("Regression threshold: %.0f%%\n\n", threshold*100)

	if len(comparison.Regressions) > 0 {
		fmt.Printf("⚠️  REGRESSIONS DETECTED:\n")
		for _, r := range comparison.Regressions {
			fmt.Printf("  - %s: %.2f → %.2f (%+.1f%%)\n",
				r.Metric, r.BaselineValue, r.CurrentValue, r.ChangePercent)
		}
		fmt.Println()
	}

	if len(comparison.Improvements) > 0 {
		fmt.Printf("✓ Improvements:\n")
		for _, i := range comparison.Improvements {
			fmt.Printf("  - %s: %.2f → %.2f (%+.1f%%)\n",
				i.Metric, i.BaselineValue, i.CurrentValue, i.ChangePercent)
		}
		fmt.Println()
	}

	if len(comparison.Regressions) == 0 && len(comparison.Improvements) == 0 {
		fmt.Printf("✓ No significant changes detected.\n")
	}

	fmt.Println("===========================================")
}

// printBenchmarkSummary prints a human-readable benchmark summary.
func printBenchmarkSummary(metrics *PerformanceMetrics) {
	fmt.Printf("\n========== Benchmark Summary ==========\n")
	fmt.Printf("Wall clock time: %d ms\n", metrics.WallClockMs)
	fmt.Printf("Throughput: %.1f frames/sec, %.1f packets/sec\n",
		metrics.FramesPerSecond, metrics.PacketsPerSecond)
	fmt.Printf("Frame time: avg=%.2fms p50=%.2fms p95=%.2fms p99=%.2fms (n=%d)\n",
		metrics.FrameTimeStats.AvgMs, metrics.FrameTimeStats.P50Ms,
		metrics.FrameTimeStats.P95Ms, metrics.FrameTimeStats.P99Ms,
		metrics.FrameTimeStats.Samples)
	fmt.Printf("Pipeline: total=%dms cluster=%dms track=%dms classify=%dms\n",
		metrics.PipelineTimeMs, metrics.ClusterTimeMs, metrics.TrackingTimeMs, metrics.ClassifyTimeMs)
	fmt.Printf("Memory: heap=%s alloc=%s GC=%d (pause=%dµs)\n",
		formatBytes(metrics.HeapAllocBytes), formatBytes(metrics.TotalAllocBytes),
		metrics.NumGC, metrics.GCPauseNs/1000)
	fmt.Println("=========================================")
}

// formatBytes formats bytes as human-readable string using binary units (1 KB = 1024 bytes).
func formatBytes(b uint64) string {
	if b < bytesPerKB {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(bytesPerKB), 0
	for n := b / bytesPerKB; n >= bytesPerKB; n /= bytesPerKB {
		div *= bytesPerKB
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
