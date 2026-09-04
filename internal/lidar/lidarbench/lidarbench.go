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
	"math"
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

	// Profile overrides the tuning config's pipeline.profile for this run.
	// Empty means "use the config", which is the normal case.
	Profile config.Profile

	BenchmarkOutput     string
	CompareBaseline     string
	RegressionThreshold float64

	// MaxFramesOverBudgetPct is the share of frames allowed to exceed the
	// per-frame budget before the run fails. It is a percentage, and it is
	// checked whether or not a baseline is supplied.
	MaxFramesOverBudgetPct float64

	// Repeats runs the benchmark this many times and reports the run whose
	// wall clock is the median. One sample on a shared CI runner is not a
	// measurement: five repeats of identical code on one machine spanned
	// 9 675-11 547 ms, a 19% range.
	Repeats int

	Quiet        bool
	ProgressSecs float64
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
	ConfirmedTracks  int
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

// WorkCounters records how much work the pipeline actually did, as opposed to
// how long it took. Without these a benchmark cannot tell a genuine regression
// from a run that quietly stopped detecting anything: the CI baseline
// committed in June 2026 measured 832 frames, zero foreground points and zero
// clusters, and read for three months as a healthy full-pipeline run.
type WorkCounters struct {
	Frames           int `json:"frames"`
	ForegroundPoints int `json:"foreground_points"`
	BackgroundPoints int `json:"background_points"`
	Clusters         int `json:"clusters"`
	ConfirmedTracks  int `json:"confirmed_tracks"`
}

// FrameBudget records the per-frame wall-clock ceiling and how often the run
// exceeded it. This is an absolute measure, not a comparison: a baseline can
// only say whether the pipeline got slower than it was, never whether it is
// fast enough for the sensor feeding it.
type FrameBudget struct {
	BudgetMs      float64 `json:"budget_ms"`
	FramesOver    int     `json:"frames_over"`
	FramesOverPct float64 `json:"frames_over_pct"`
	WorstMs       float64 `json:"worst_ms"`
}

// PerformanceMetrics captures the benchmark measurements. Committed baselines
// (internal/lidar/perf/baseline/*.json) decode into it, so existing field
// names and tags must not change without regenerating baselines; new fields
// may be added, and a baseline missing them is refused rather than compared.
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

	Work        WorkCounters `json:"work"`
	FrameBudget FrameBudget  `json:"frame_budget"`
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
//
// Profile and TuningFingerprint are the workload identity. A baseline records
// hardware (goos, goarch, num_cpu, go_version) and, before these fields,
// nothing at all about what it measured — so two runs with different layer
// depth or different tuning produced incomparable numbers and the file could
// not say which one you were holding.
type BenchmarkResult struct {
	Version           string               `json:"version"`
	Timestamp         string               `json:"timestamp"`
	PCAPFile          string               `json:"pcap_file"`
	Profile           string               `json:"profile"`
	TuningFingerprint string               `json:"tuning_fingerprint"`
	SystemInfo        SystemInfo           `json:"system_info"`
	Metrics           PerformanceMetrics   `json:"metrics"`
	RepeatSpread      *RepeatSpread        `json:"repeat_spread,omitempty"`
	Comparison        *BenchmarkComparison `json:"comparison,omitempty"`
}

// RepeatSpread records what a multi-run capture saw, so a baseline carries
// evidence of its own noise rather than being one sample presented as fact.
type RepeatSpread struct {
	Runs        int     `json:"runs"`
	WallClockMs []int64 `json:"wall_clock_ms"`
	MedianMs    int64   `json:"median_ms"`
	SpreadPct   float64 `json:"spread_pct"`
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

	res, metrics, spread, err := runRepeated(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Benchmark failed: %v\n", err)
		return 1
	}
	return handleBenchmarkOutput(cfg, res, metrics, spread)
}

// runRepeated runs the benchmark cfg.Repeats times and returns the median run
// by wall clock, along with the spread across all runs.
//
// The median run is returned whole rather than a per-metric median, so every
// field in the emitted document came from the same execution and stays
// mutually consistent — a p95 from one run beside a heap figure from another
// would describe a run that never happened.
func runRepeated(cfg Config) (*result, *PerformanceMetrics, *RepeatSpread, error) {
	repeats := cfg.Repeats
	if repeats < 1 {
		repeats = 1
	}

	type run struct {
		res     *result
		metrics *PerformanceMetrics
	}
	runs := make([]run, 0, repeats)

	for i := 0; i < repeats; i++ {
		if repeats > 1 && !cfg.Quiet {
			fmt.Printf("Run %d of %d...\n", i+1, repeats)
		}
		res, metrics, err := runBenchmark(cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		runs = append(runs, run{res: res, metrics: metrics})
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].metrics.WallClockMs < runs[j].metrics.WallClockMs
	})
	median := runs[len(runs)/2]

	if repeats == 1 {
		return median.res, median.metrics, nil, nil
	}

	spread := &RepeatSpread{
		Runs:        len(runs),
		WallClockMs: make([]int64, 0, len(runs)),
		MedianMs:    median.metrics.WallClockMs,
	}
	for _, r := range runs {
		spread.WallClockMs = append(spread.WallClockMs, r.metrics.WallClockMs)
	}
	if lo := spread.WallClockMs[0]; lo > 0 {
		hi := spread.WallClockMs[len(spread.WallClockMs)-1]
		spread.SpreadPct = float64(hi-lo) / float64(lo) * 100
	}
	return median.res, median.metrics, spread, nil
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

	// Collect before reading. HeapAlloc without a preceding collection is
	// whatever the heap happened to be mid-cycle: across five runs of
	// identical code over the same capture it read 18.8-40.0 MB, a 2.1x
	// spread, which is what produced the "+6763% heap regression" headline
	// the gate reported in September 2026. With the collection it is stable
	// to three significant figures.
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	frameTimes, clusterNs, trackNs, classifyNs := fb.getBenchmarkData()
	wallSec := float64(wallClockMs) / 1000.0
	budgetMs := config.DefaultFrameBudgetMs
	if cfg.Tuning != nil {
		budgetMs = cfg.Tuning.GetFrameBudgetMs()
	}
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
		Work: WorkCounters{
			Frames:           res.TotalFrames,
			ForegroundPoints: res.ForegroundPoints,
			BackgroundPoints: res.BackgroundPoints,
			Clusters:         res.TotalClusters,
			ConfirmedTracks:  res.ConfirmedTracks,
		},
		FrameBudget: computeFrameBudget(frameTimes, budgetMs),
	}
	return res, metrics, nil
}

// computeFrameBudget counts frames whose processing exceeded the per-frame
// ceiling. Beyond the ceiling the pipeline is in alarm territory: it is no
// longer keeping up with the sensor, and on a 10 Hz capture the next frame is
// already waiting.
func computeFrameBudget(frameTimes []float64, budgetMs float64) FrameBudget {
	fb := FrameBudget{BudgetMs: budgetMs}
	for _, ms := range frameTimes {
		if ms > fb.WorstMs {
			fb.WorstMs = ms
		}
		if budgetMs > 0 && ms > budgetMs {
			fb.FramesOver++
		}
	}
	if len(frameTimes) > 0 {
		fb.FramesOverPct = float64(fb.FramesOver) / float64(len(frameTimes)) * 100
	}
	return fb
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
	profile    config.Profile

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
		profile:        benchProfile(cfg),
		frameTimes:     make([]float64, 0, defaultFrameCapacity),
	}
}

// benchProfile resolves the profile the benchmark should run, preferring an
// explicit override so a single tuning file can be measured at several depths.
func benchProfile(cfg Config) config.Profile {
	if cfg.Profile != "" {
		return cfg.Profile
	}
	if cfg.Tuning != nil {
		return cfg.Tuning.GetProfile()
	}
	return config.DefaultProfile
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

	// Profile gate: l3-only stops here, mirroring the live pipeline's gate at
	// the same boundary. The two must agree on what a profile means, or the
	// benchmark measures a workload the deployment never runs.
	if !fb.profile.RunsLayer(4) {
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

	// Profile gate: detect stops here. Clusters exist but no tracker state is
	// created, which is the distinction that justifies the profile.
	if !fb.profile.RunsLayer(5) {
		fb.frameTimes = append(fb.frameTimes, msSince(frameStart))
		return
	}

	trackStart := time.Now()
	fb.tracker.Update(clusters, fb.frameStartTime)
	atomic.AddInt64(&fb.trackTimeNs, time.Since(trackStart).Nanoseconds())

	confirmed := fb.tracker.GetConfirmedTracks()
	fb.res.ConfirmedTracks = len(confirmed)

	if !fb.profile.RunsLayer(6) {
		fb.frameTimes = append(fb.frameTimes, msSince(frameStart))
		return
	}

	classifyStart := time.Now()
	for _, track := range confirmed {
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
func handleBenchmarkOutput(cfg Config, res *result, metrics *PerformanceMetrics, spread *RepeatSpread) int {
	benchResult := BenchmarkResult{
		Version:           "2.0",
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		PCAPFile:          filepath.Base(cfg.PCAPFile),
		Profile:           string(benchProfile(cfg)),
		TuningFingerprint: tuningFingerprint(cfg),
		SystemInfo:        getSystemInfo(),
		Metrics:           *metrics,
		RepeatSpread:      spread,
	}

	exitCode := 0

	// The budget check runs first and runs unconditionally. It is the only
	// check that does not need a baseline, and it answers the question a
	// relative gate cannot: is the pipeline fast enough for the sensor?
	if err := checkFrameBudget(metrics.FrameBudget, cfg.MaxFramesOverBudgetPct); err != nil {
		fmt.Fprintf(os.Stderr, "\n[FRAME BUDGET] %v\n", err)
		exitCode = 1
	}

	if cfg.CompareBaseline != "" {
		baseline, err := readBaseline(cfg.CompareBaseline)
		switch {
		case err != nil:
			log.Printf("Warning: %v", err)
		default:
			if idErr := checkWorkloadIdentity(baseline, &benchResult); idErr != nil {
				fmt.Fprintf(os.Stderr, "\n[BASELINE REFUSED] %v\n", idErr)
				fmt.Fprintf(os.Stderr,
					"Not reporting a regression: these runs did not measure the same workload.\n"+
						"Regenerate the baseline for this profile before reading any comparison.\n")
				exitCode = 1
			} else {
				comparison, hasRegression := compareMetrics(
					filepath.Base(cfg.CompareBaseline), baseline, metrics, cfg.RegressionThreshold)
				benchResult.Comparison = comparison
				printComparisonSummary(comparison, cfg.RegressionThreshold)
				if hasRegression {
					exitCode = 1
				}
			}
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
		printBenchmarkSummary(&benchResult)
	}
	return exitCode
}

// compareWithBaseline diffs current metrics against a baseline file, flagging
// changes beyond threshold as regressions or improvements.
func compareWithBaseline(baselinePath string, current *PerformanceMetrics, threshold float64) (*BenchmarkComparison, bool) {
	baseline, err := readBaseline(baselinePath)
	if err != nil {
		log.Printf("Warning: %v", err)
		return nil, false
	}
	return compareMetrics(filepath.Base(baselinePath), baseline, current, threshold)
}

// readBaseline loads and decodes a committed baseline document.
func readBaseline(path string) (*BenchmarkResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}
	var baseline BenchmarkResult
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to parse baseline file: %w", err)
	}
	return &baseline, nil
}

// tuningFingerprint returns the fingerprint of the config this run measured.
func tuningFingerprint(cfg Config) string {
	if cfg.Tuning == nil {
		return ""
	}
	return cfg.Tuning.Fingerprint()
}

// compareMetrics diffs current metrics against an already-decoded baseline.
func compareMetrics(baselineName string, baseline *BenchmarkResult, current *PerformanceMetrics, threshold float64) (*BenchmarkComparison, bool) {
	comparison := &BenchmarkComparison{BaselineFile: baselineName}
	hasRegression := false

	// frames_per_second is deliberately absent: it is frames divided by the
	// same wall clock already gated above, so including it reported one
	// runner slowdown as two independent regressions. total_alloc_bytes is
	// present in its place — it is stable to under 1% across repeats, where
	// heap_alloc_bytes needed a forced collection before it meant anything.
	metricsToCompare := []struct {
		name        string
		baseline    float64
		current     float64
		higherIsBad bool
	}{
		{"wall_clock_ms", float64(baseline.Metrics.WallClockMs), float64(current.WallClockMs), true},
		{"frame_time_avg_ms", baseline.Metrics.FrameTimeStats.AvgMs, current.FrameTimeStats.AvgMs, true},
		{"frame_time_p95_ms", baseline.Metrics.FrameTimeStats.P95Ms, current.FrameTimeStats.P95Ms, true},
		{"heap_alloc_bytes", float64(baseline.Metrics.HeapAllocBytes), float64(current.HeapAllocBytes), true},
		{"total_alloc_bytes", float64(baseline.Metrics.TotalAllocBytes), float64(current.TotalAllocBytes), true},
		{"cluster_time_ms", float64(baseline.Metrics.ClusterTimeMs), float64(current.ClusterTimeMs), true},
		{"tracking_time_ms", float64(baseline.Metrics.TrackingTimeMs), float64(current.TrackingTimeMs), true},
	}

	for _, m := range metricsToCompare {
		diff, verdict := classifyMetricChange(m.name, m.baseline, m.current, m.higherIsBad, threshold)
		switch verdict {
		case verdictRegression:
			comparison.Regressions = append(comparison.Regressions, diff)
			hasRegression = true
		case verdictImprovement:
			comparison.Improvements = append(comparison.Improvements, diff)
		}
	}
	return comparison, hasRegression
}

// metricVerdict is the outcome of comparing one metric against its baseline.
type metricVerdict int

const (
	verdictUnchanged metricVerdict = iota
	verdictRegression
	verdictImprovement
)

// unboundedChangePercent stands in for a ratio against a zero baseline, which
// has no finite value. It is reported rather than skipped: the comparator used
// to `continue` past every zero-baseline metric, and cluster_time_ms and
// tracking_time_ms were both zero in the June 2026 baseline — so the two
// numbers that would have said "clustering is not running" were the two it
// ignored, for three months.
const unboundedChangePercent = 1e6

// classifyMetricChange decides whether one metric moved enough to matter.
// A zero baseline is handled rather than skipped: zero to non-zero is an
// unbounded increase, and non-zero to zero an unbounded decrease.
func classifyMetricChange(name string, baseline, current float64, higherIsBad bool, threshold float64) (MetricDifference, metricVerdict) {
	diff := MetricDifference{Metric: name, BaselineValue: baseline, CurrentValue: current}

	var changePercent float64
	switch {
	case baseline == 0 && current == 0:
		return diff, verdictUnchanged
	case baseline == 0:
		// Any appearance from nothing is unbounded growth.
		changePercent = unboundedChangePercent
	default:
		changePercent = (current - baseline) / baseline
	}
	diff.ChangePercent = changePercent * 100

	if higherIsBad {
		switch {
		case changePercent > threshold:
			return diff, verdictRegression
		case changePercent < -threshold:
			return diff, verdictImprovement
		}
		return diff, verdictUnchanged
	}
	switch {
	case changePercent < -threshold:
		return diff, verdictRegression
	case changePercent > threshold:
		return diff, verdictImprovement
	}
	return diff, verdictUnchanged
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

func printBenchmarkSummary(result *BenchmarkResult) {
	metrics := &result.Metrics
	fmt.Printf("\n========== Benchmark Summary ==========\n")
	fmt.Printf("Profile: %s  tuning: %s\n", result.Profile, result.TuningFingerprint)
	fmt.Printf("Work: frames=%d foreground=%d clusters=%d tracks=%d\n",
		metrics.Work.Frames, metrics.Work.ForegroundPoints,
		metrics.Work.Clusters, metrics.Work.ConfirmedTracks)
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
	fmt.Printf("Frame budget: %d of %d frames over %.0f ms (%.2f%%), worst %.1f ms\n",
		metrics.FrameBudget.FramesOver, metrics.FrameTimeStats.Samples,
		metrics.FrameBudget.BudgetMs, metrics.FrameBudget.FramesOverPct,
		metrics.FrameBudget.WorstMs)
	if result.RepeatSpread != nil {
		fmt.Printf("Repeats: %d runs, median %d ms, spread %.1f%% %v\n",
			result.RepeatSpread.Runs, result.RepeatSpread.MedianMs,
			result.RepeatSpread.SpreadPct, result.RepeatSpread.WallClockMs)
	}
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

// workTolerance is the fraction by which a work counter may drift from the
// baseline before the two runs are treated as different workloads. Counts are
// deterministic for a given code, config and capture, so any movement is a
// behaviour change; the tolerance exists so that a genuine but small detection
// improvement does not have to be re-baselined as an emergency.
const workTolerance = 0.10

// identityError reports that two benchmark runs are not comparable. It is
// deliberately distinct from a regression: a regression says the pipeline got
// slower, this says the question was never well formed.
type identityError struct {
	reasons []string
}

func (e *identityError) Error() string {
	return "baseline workload mismatch: " + strings.Join(e.reasons, "; ")
}

// checkWorkloadIdentity refuses to compare two runs that did not measure the
// same thing. This is the guard the September 2026 failure needed: the
// baseline it compared against had recorded 832 frames, zero foreground points
// and zero clusters, and every "regression" it reported was the cost of the
// pipeline finally doing its job.
func checkWorkloadIdentity(baseline *BenchmarkResult, current *BenchmarkResult) error {
	var reasons []string

	if baseline.Profile == "" {
		reasons = append(reasons,
			"the baseline predates pipeline profiles and cannot state which layers it measured")
	} else if baseline.Profile != current.Profile {
		reasons = append(reasons, fmt.Sprintf("profile %q vs %q", baseline.Profile, current.Profile))
	}

	if baseline.TuningFingerprint == "" {
		reasons = append(reasons, "the baseline carries no tuning fingerprint")
	} else if baseline.TuningFingerprint != current.TuningFingerprint {
		reasons = append(reasons, fmt.Sprintf("tuning fingerprint %s vs %s",
			baseline.TuningFingerprint, current.TuningFingerprint))
	}

	if baseline.PCAPFile != "" && current.PCAPFile != "" && baseline.PCAPFile != current.PCAPFile {
		reasons = append(reasons, fmt.Sprintf("capture %s vs %s", baseline.PCAPFile, current.PCAPFile))
	}

	// Architecture is part of the workload identity, not a footnote. Timings
	// from arm64 say nothing about amd64, and a committed local baseline is
	// otherwise a trap for the next person to run the gate on a different
	// machine. CPU count and Go version only warn: they shift the numbers
	// without making the comparison meaningless.
	if b, c := baseline.SystemInfo, current.SystemInfo; b.GOOS != "" || b.GOARCH != "" {
		if b.GOOS != c.GOOS || b.GOARCH != c.GOARCH {
			reasons = append(reasons, fmt.Sprintf("platform %s/%s vs %s/%s",
				b.GOOS, b.GOARCH, c.GOOS, c.GOARCH))
		} else {
			if b.NumCPU != 0 && b.NumCPU != c.NumCPU {
				log.Printf("Warning: baseline was captured on %d CPUs, this run has %d; timings are not directly comparable",
					b.NumCPU, c.NumCPU)
			}
			if b.GoVersion != "" && b.GoVersion != c.GoVersion {
				log.Printf("Warning: baseline was captured with %s, this run uses %s",
					b.GoVersion, c.GoVersion)
			}
		}
	}

	reasons = append(reasons, workDifferences(baseline.Metrics.Work, current.Metrics.Work)...)

	if len(reasons) > 0 {
		return &identityError{reasons: reasons}
	}
	return nil
}

// workDifferences reports the work counters that moved beyond tolerance.
func workDifferences(baseline, current WorkCounters) []string {
	counters := []struct {
		name     string
		baseline int
		current  int
	}{
		{"frames", baseline.Frames, current.Frames},
		{"foreground_points", baseline.ForegroundPoints, current.ForegroundPoints},
		{"clusters", baseline.Clusters, current.Clusters},
	}

	var out []string
	for _, c := range counters {
		if c.baseline == 0 && c.current == 0 {
			continue
		}
		if c.baseline == 0 || c.current == 0 {
			out = append(out, fmt.Sprintf("%s %d vs %d", c.name, c.baseline, c.current))
			continue
		}
		drift := math.Abs(float64(c.current-c.baseline)) / float64(c.baseline)
		if drift > workTolerance {
			out = append(out, fmt.Sprintf("%s %d vs %d (%+.1f%%)",
				c.name, c.baseline, c.current, drift*100))
		}
	}
	return out
}

// checkFrameBudget reports whether too many frames breached the per-frame
// ceiling. Unlike every other check here it needs no baseline: it asks whether
// the pipeline is fast enough for the sensor, which no comparison against a
// previous run can answer.
func checkFrameBudget(budget FrameBudget, maxOverPct float64) error {
	if budget.BudgetMs <= 0 || budget.FramesOver == 0 {
		return nil
	}
	if budget.FramesOverPct <= maxOverPct {
		return nil
	}
	return fmt.Errorf(
		"%d frames (%.2f%%) exceeded the %.0f ms budget, above the %.2f%% allowance; worst frame %.1f ms",
		budget.FramesOver, budget.FramesOverPct, budget.BudgetMs, maxOverPct, budget.WorstMs)
}
