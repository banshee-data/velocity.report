//go:build pcap
// +build pcap

// Package lidarbench runs a PCAP through the full LiDAR L1–L6 tracking pipeline
// (foreground extraction → DBSCAN clustering → Kalman tracking → classification)
// purely to measure its performance, and compares the result against a committed
// baseline. It is the end-to-end perf-regression gate exercised by
// `make test-perf` and nightly CI; it writes and reads benchmark JSON whose
// schema matches the historical pcap-analyse -benchmark output, so existing
// baselines remain valid. It is a dev/CI tool, not part of the `velocity lidar`
// operational surface.
package lidarbench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

const (
	// commitHashLength abbreviates git commit hashes in benchmark output.
	commitHashLength = 12
	// defaultFrameCapacity pre-allocates the per-frame timing slice.
	defaultFrameCapacity = 10000
	// bytesPerKB is the binary unit for byte-size formatting (1 KiB = 1024 B).
	bytesPerKB = 1024
)

var (
	loadBenchmarkPandarConfig = parse.LoadEmbeddedPandar40PConfig
	readBenchmarkBuildInfo    = debug.ReadBuildInfo
)

// Config holds the benchmark run parameters. Flag parsing lives in the caller.
type Config struct {
	PCAPFile        string
	OutputDir       string
	SensorID        string
	UDPPort         int
	StartSeconds    float64
	DurationSeconds float64

	// Tuning is the loaded tuning config (from -config); nil falls back to the
	// embedded defaults, so the measured pipeline matches live observation.
	Tuning *config.TuningConfig

	BenchmarkOutput     string
	CompareBaseline     string
	RegressionThreshold float64
	Quiet               bool
	ProgressSecs        float64
}

// result holds the pipeline counts gathered during a benchmark run.
type result struct {
	PCAPFile         string
	Duration         time.Duration
	DurationSecs     float64
	TotalPackets     int
	TotalPoints      int
	TotalFrames      int
	ForegroundPoints int
	BackgroundPoints int
	TotalClusters    int
	ProcessingTimeMs int64
}

// FrameTimeStats holds the distribution of per-frame processing times.
type FrameTimeStats struct {
	MinMs   float64 `json:"min_ms"`
	MaxMs   float64 `json:"max_ms"`
	AvgMs   float64 `json:"avg_ms"`
	P50Ms   float64 `json:"p50_ms"`
	P95Ms   float64 `json:"p95_ms"`
	P99Ms   float64 `json:"p99_ms"`
	Samples int     `json:"samples"`
}

// PerformanceMetrics captures the benchmark measurements. The JSON schema is
// frozen: committed baselines (internal/lidar/perf/baseline/*.json) decode into
// it, so field names and tags must not change without regenerating baselines.
type PerformanceMetrics struct {
	WallClockMs    int64          `json:"wall_clock_ms"`
	FrameTimeStats FrameTimeStats `json:"frame_time_stats"`

	FramesPerSecond  float64 `json:"frames_per_second"`
	PacketsPerSecond float64 `json:"packets_per_second"`
	PointsPerSecond  float64 `json:"points_per_second"`

	HeapAllocBytes  uint64 `json:"heap_alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	NumGC           uint32 `json:"num_gc"`
	GCPauseNs       uint64 `json:"gc_pause_ns"`

	PipelineTimeMs int64 `json:"pipeline_time_ms"`
	ClusterTimeMs  int64 `json:"cluster_time_ms"`
	TrackingTimeMs int64 `json:"tracking_time_ms"`
	ClassifyTimeMs int64 `json:"classify_time_ms"`
}

// SystemInfo captures host details for benchmark reproducibility.
type SystemInfo struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	NumCPU     int    `json:"num_cpu"`
	GoVersion  string `json:"go_version"`
	CommitHash string `json:"commit_hash,omitempty"`
}

// BenchmarkResult is the JSON document written per run and read as a baseline.
type BenchmarkResult struct {
	Version    string               `json:"version"`
	Timestamp  string               `json:"timestamp"`
	PCAPFile   string               `json:"pcap_file"`
	SystemInfo SystemInfo           `json:"system_info"`
	Metrics    PerformanceMetrics   `json:"metrics"`
	Comparison *BenchmarkComparison `json:"comparison,omitempty"`
}

// BenchmarkComparison holds the diff against a baseline.
type BenchmarkComparison struct {
	BaselineFile string             `json:"baseline_file"`
	Regressions  []MetricDifference `json:"regressions,omitempty"`
	Improvements []MetricDifference `json:"improvements,omitempty"`
}

// MetricDifference is one metric's baseline→current change.
type MetricDifference struct {
	Metric        string  `json:"metric"`
	BaselineValue float64 `json:"baseline_value"`
	CurrentValue  float64 `json:"current_value"`
	ChangePercent float64 `json:"change_percent"`
}

// Run executes the benchmark and returns a process exit code: 0 on success, 1 on
// failure, or 1 when a baseline comparison detects a regression.
func Run(cfg Config) int {
	if cfg.PCAPFile == "" {
		fmt.Fprintln(os.Stderr, "Error: PCAP file is required")
		return 1
	}
	if _, err := os.Stat(cfg.PCAPFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: PCAP file not found: %s\n", cfg.PCAPFile)
		return 1
	}
	if cfg.OutputDir != "" {
		if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
			log.Printf("Failed to create output directory: %v", err)
			return 1
		}
	}
	if cfg.Quiet {
		log.SetOutput(io.Discard)
	}

	res, metrics, err := runBenchmark(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Benchmark failed: %v\n", err)
		return 1
	}
	return handleBenchmarkOutput(cfg, res, metrics)
}

// runBenchmark replays the capture through the tracking pipeline with timing
// instrumentation and assembles the performance metrics.
func runBenchmark(cfg Config) (*result, *PerformanceMetrics, error) {
	cfg.DurationSeconds = normaliseReplayDuration(cfg.DurationSeconds)
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	startTime := time.Now()

	parserConfig, err := loadBenchmarkPandarConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserConfig)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)

	parseStart := time.Now()
	res := &result{PCAPFile: cfg.PCAPFile}
	stats := &analysisStats{}
	fb := newAnalysisFrameBuilder(cfg, res)

	reader := wrapProgress(cfg, stats, "benchmark")
	if err := network.ReadPCAPFile(
		context.Background(), cfg.PCAPFile, cfg.UDPPort,
		parser, fb, reader, nil, cfg.StartSeconds, cfg.DurationSeconds, 0, 0, nil,
	); err != nil {
		return nil, nil, fmt.Errorf("read PCAP: %w", err)
	}
	fb.finalise()
	pipelineTimeMs := time.Since(parseStart).Milliseconds()

	packets, points, duration := stats.getStats()
	res.TotalPackets = packets
	res.TotalPoints = points
	res.Duration = duration
	res.DurationSecs = duration.Seconds()

	wallClockMs := time.Since(startTime).Milliseconds()
	res.ProcessingTimeMs = wallClockMs

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	frameTimes, clusterNs, trackNs, classifyNs := fb.getBenchmarkData()
	wallSec := float64(wallClockMs) / 1000.0
	metrics := &PerformanceMetrics{
		WallClockMs:      wallClockMs,
		FrameTimeStats:   computeFrameTimeStats(frameTimes),
		FramesPerSecond:  perSecond(res.TotalFrames, wallSec),
		PacketsPerSecond: perSecond(res.TotalPackets, wallSec),
		PointsPerSecond:  perSecond(res.TotalPoints, wallSec),
		HeapAllocBytes:   memAfter.HeapAlloc,
		TotalAllocBytes:  memAfter.TotalAlloc - memBefore.TotalAlloc,
		NumGC:            memAfter.NumGC - memBefore.NumGC,
		GCPauseNs:        memAfter.PauseTotalNs - memBefore.PauseTotalNs,
		PipelineTimeMs:   pipelineTimeMs,
		ClusterTimeMs:    clusterNs / 1e6,
		TrackingTimeMs:   trackNs / 1e6,
		ClassifyTimeMs:   classifyNs / 1e6,
	}
	return res, metrics, nil
}

// normaliseReplayDuration preserves the historical zero-value full-capture
// behaviour while retaining -1 as the explicit full-capture setting.
func normaliseReplayDuration(durationSeconds float64) float64 {
	if durationSeconds == 0 {
		return -1
	}
	return durationSeconds
}

func perSecond(count int, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(count) / seconds
}

// analysisStats implements network.PacketStatsInterface to count packets,
// points, and the wall-clock span of the read.
type analysisStats struct {
	mu       sync.Mutex
	packets  int
	points   int
	dropped  int
	firstPkt time.Time
	lastPkt  time.Time
}

func (s *analysisStats) AddPacket(int) {
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

func (s *analysisStats) LogStats(bool) {}

func (s *analysisStats) getStats() (packets, points int, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.packets, s.points, s.lastPkt.Sub(s.firstPkt)
}

// wrapProgress decorates inner with a wall-clock-paced progress line when
// cfg.ProgressSecs > 0.
func wrapProgress(cfg Config, inner network.PacketStatsInterface, label string) network.PacketStatsInterface {
	if cfg.ProgressSecs <= 0 {
		return inner
	}
	var size int64
	if fi, err := os.Stat(cfg.PCAPFile); err == nil {
		size = fi.Size()
	}
	interval := time.Duration(cfg.ProgressSecs * float64(time.Second))
	return network.NewProgressStats(inner, size, interval, label, os.Stderr)
}

// analysisFrameBuilder implements network.FrameBuilder, assembling complete
// rotations from raw blocks and timing each L4–L6 pipeline stage.
type analysisFrameBuilder struct {
	mu             sync.Mutex
	points         []l2frames.PointPolar
	lastRawAzimuth float64 // raw block azimuth (deg); -1 until first point
	frameStartTime time.Time
	frameCount     int

	bgManager  *l3grid.BackgroundManager
	tracker    *l5tracks.Tracker
	classifier *l6objects.TrackClassifier
	cfg        Config
	res        *result

	frameTimes     []float64
	clusterTimeNs  int64
	trackTimeNs    int64
	classifyTimeNs int64
}

func newAnalysisFrameBuilder(cfg Config, res *result) *analysisFrameBuilder {
	return &analysisFrameBuilder{
		points:         make([]l2frames.PointPolar, 0, 50000),
		lastRawAzimuth: -1,
		bgManager:      createBackgroundManager(cfg.SensorID, cfg.Tuning),
		tracker:        l5tracks.NewTracker(l5tracks.DefaultTrackerConfig()),
		classifier:     l6objects.NewTrackClassifier(),
		cfg:            cfg,
		res:            res,
		frameTimes:     make([]float64, 0, defaultFrameCapacity),
	}
}

func (fb *analysisFrameBuilder) AddPointsPolar(points []l2frames.PointPolar) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(points) == 0 {
		return
	}
	pktTime := time.Unix(0, points[0].Timestamp)
	if fb.frameStartTime.IsZero() {
		fb.frameStartTime = pktTime
	}
	// Detect frame completion on the monotonic RAW block azimuth (centi-degrees);
	// the per-point corrected azimuth swings ±~5° and shatters rotations.
	for _, p := range points {
		rawAz := float64(p.RawBlockAzimuth) / 100.0
		if fb.lastRawAzimuth >= 0 && fb.lastRawAzimuth > 270 && rawAz < 90 {
			if len(fb.points) > 0 {
				fb.processCurrentFrame()
				fb.frameCount++
			}
			fb.points = fb.points[:0]
			fb.frameStartTime = pktTime
		}
		fb.points = append(fb.points, p)
		fb.lastRawAzimuth = rawAz
	}
}

// SetMotorSpeed satisfies network.FrameBuilder. Frame boundaries come from the
// azimuth wrap, so the benchmark builder does not use motor speed.
func (fb *analysisFrameBuilder) SetMotorSpeed(uint16) {}

func msSince(t time.Time) float64 { return float64(time.Since(t).Nanoseconds()) / 1e6 }

// processCurrentFrame runs the accumulated points through foreground extraction,
// clustering, tracking, and classification, accumulating per-stage timing. The
// caller must hold fb.mu.
func (fb *analysisFrameBuilder) processCurrentFrame() {
	frameStart := time.Now()

	mask, err := fb.bgManager.ProcessFramePolarWithMask(fb.points)
	if err != nil || mask == nil {
		fb.frameTimes = append(fb.frameTimes, msSince(frameStart))
		return
	}
	foregroundPoints := l3grid.ExtractForegroundPoints(fb.points, mask)
	foregroundCount := len(foregroundPoints)

	fb.res.TotalFrames++
	fb.res.ForegroundPoints += foregroundCount
	fb.res.BackgroundPoints += len(fb.points) - foregroundCount
	if foregroundCount == 0 {
		fb.frameTimes = append(fb.frameTimes, msSince(frameStart))
		return
	}

	worldPoints := l4perception.TransformToWorld(foregroundPoints, nil, fb.cfg.SensorID)

	clusterStart := time.Now()
	dbscanParams := l4perception.DefaultDBSCANParams()
	if fb.bgManager != nil {
		p := fb.bgManager.GetParams()
		if p.ForegroundMinClusterPoints > 0 {
			dbscanParams.MinPts = p.ForegroundMinClusterPoints
		}
		if p.ForegroundDBSCANEps > 0 {
			dbscanParams.Eps = float64(p.ForegroundDBSCANEps)
		}
	}
	clusters := l4perception.DBSCAN(worldPoints, dbscanParams)
	atomic.AddInt64(&fb.clusterTimeNs, time.Since(clusterStart).Nanoseconds())
	fb.res.TotalClusters += len(clusters)
	if len(clusters) == 0 {
		fb.frameTimes = append(fb.frameTimes, msSince(frameStart))
		return
	}

	trackStart := time.Now()
	fb.tracker.Update(clusters, fb.frameStartTime)
	atomic.AddInt64(&fb.trackTimeNs, time.Since(trackStart).Nanoseconds())

	classifyStart := time.Now()
	for _, track := range fb.tracker.GetConfirmedTracks() {
		if track.ObjectClass == "" && track.ObservationCount >= 5 {
			fb.classifier.ClassifyAndUpdate(track)
		}
	}
	atomic.AddInt64(&fb.classifyTimeNs, time.Since(classifyStart).Nanoseconds())

	fb.frameTimes = append(fb.frameTimes, msSince(frameStart))
}

func (fb *analysisFrameBuilder) finalise() {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.points) > 0 {
		fb.processCurrentFrame()
	}
}

// getBenchmarkData returns the collected timing data. Call after finalise().
func (fb *analysisFrameBuilder) getBenchmarkData() (frameTimes []float64, clusterNs, trackNs, classifyNs int64) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return fb.frameTimes, atomic.LoadInt64(&fb.clusterTimeNs),
		atomic.LoadInt64(&fb.trackTimeNs), atomic.LoadInt64(&fb.classifyTimeNs)
}

// createBackgroundManager builds the L3 model from tuning exactly as the live
// server does, so the measured pipeline matches live observation.
func createBackgroundManager(sensorID string, tuningCfg *config.TuningConfig) *l3grid.BackgroundManager {
	tuningCfg = tuningOrEmbedded(tuningCfg)
	bgConfig := l3grid.BackgroundConfigFromActiveTuning(tuningCfg)
	return l3grid.NewBackgroundManager(sensorID, 40, 1800, bgConfig.ToBackgroundParams(), nil)
}

// tuningOrEmbedded returns t when non-nil, else the embedded defaults (validated
// at build time, so a load failure is a build invariant).
func tuningOrEmbedded(t *config.TuningConfig) *config.TuningConfig {
	if t != nil {
		return t
	}
	cfg, err := config.LoadTuningConfigOrEmbedded(config.DefaultConfigPath, radarassets.TuningDefaults)
	if err != nil {
		panic(fmt.Sprintf("lidarbench: load embedded tuning: %v", err))
	}
	return cfg
}

func computeFrameTimeStats(frameTimes []float64) FrameTimeStats {
	if len(frameTimes) == 0 {
		return FrameTimeStats{}
	}
	sorted := make([]float64, len(frameTimes))
	copy(sorted, frameTimes)
	sort.Float64s(sorted)

	var sum float64
	for _, v := range sorted {
		sum += v
	}
	n := len(sorted)
	// Floor-based percentile indexing (consistent with l6objects percentiles);
	// for n >= 1, int(n*0.99) < n so the indices are in range.
	return FrameTimeStats{
		MinMs:   sorted[0],
		MaxMs:   sorted[n-1],
		AvgMs:   sum / float64(n),
		P50Ms:   sorted[int(float64(n)*0.50)],
		P95Ms:   sorted[int(float64(n)*0.95)],
		P99Ms:   sorted[int(float64(n)*0.99)],
		Samples: n,
	}
}

func getSystemInfo() SystemInfo {
	info := SystemInfo{
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}
	if buildInfo, ok := readBenchmarkBuildInfo(); ok {
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

// handleBenchmarkOutput writes the benchmark JSON and, when a baseline is given,
// compares against it. Returns 1 when a regression is detected.
func handleBenchmarkOutput(cfg Config, res *result, metrics *PerformanceMetrics) int {
	benchResult := BenchmarkResult{
		Version:    "1.0",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		PCAPFile:   filepath.Base(cfg.PCAPFile),
		SystemInfo: getSystemInfo(),
		Metrics:    *metrics,
	}

	exitCode := 0
	if cfg.CompareBaseline != "" {
		comparison, hasRegression := compareWithBaseline(cfg.CompareBaseline, metrics, cfg.RegressionThreshold)
		benchResult.Comparison = comparison
		if comparison != nil {
			printComparisonSummary(comparison, cfg.RegressionThreshold)
		}
		if hasRegression {
			exitCode = 1
		}
	}

	outputPath := cfg.BenchmarkOutput
	if outputPath == "" {
		baseName := strings.TrimSuffix(filepath.Base(cfg.PCAPFile), filepath.Ext(cfg.PCAPFile))
		outputPath = filepath.Join(cfg.OutputDir, baseName+"_benchmark.json")
	}
	data, err := json.MarshalIndent(benchResult, "", "  ")
	if err != nil {
		log.Printf("Error marshaling benchmark result: %v", err)
		return 1
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		log.Printf("Error writing benchmark file: %v", err)
		return 1
	}

	if !cfg.Quiet {
		fmt.Printf("\nBenchmark results: %s\n", outputPath)
		printBenchmarkSummary(metrics)
	}
	return exitCode
}

// compareWithBaseline diffs current metrics against a baseline file, flagging
// changes beyond threshold as regressions or improvements.
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

	comparison := &BenchmarkComparison{BaselineFile: filepath.Base(baselinePath)}
	hasRegression := false

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
			continue
		}
		changePercent := (m.current - m.baseline) / m.baseline
		diff := MetricDifference{
			Metric:        m.name,
			BaselineValue: m.baseline,
			CurrentValue:  m.current,
			ChangePercent: changePercent * 100,
		}
		if m.higherIsBad {
			if changePercent > threshold {
				comparison.Regressions = append(comparison.Regressions, diff)
				hasRegression = true
			} else if changePercent < -threshold {
				comparison.Improvements = append(comparison.Improvements, diff)
			}
		} else {
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
