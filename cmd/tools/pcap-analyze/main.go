//go:build pcap
// +build pcap

// Package main provides a PCAP analysis tool for LIDAR data.
// It processes PCAP files through the full tracking pipeline and exports
// categorized tracks and foreground point clouds for ML training.
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	_ "modernc.org/sqlite"
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

	// Run analysis
	result, err := analyzePCAP(config)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	// Print summary
	printSummary(result)

	// Export results
	if err := exportResults(config, result); err != nil {
		log.Fatalf("Export failed: %v", err)
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
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -pcap capture.pcap -output ./results\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -pcap capture.pcap -training -output ./ml_data\n", os.Args[0])
	}

	flag.Parse()
	return config
}

func analyzePCAP(config Config) (*AnalysisResult, error) {
	startTime := time.Now()

	// Open PCAP file
	handle, err := pcap.OpenOffline(config.PCAPFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open PCAP: %w", err)
	}
	defer handle.Close()

	// Set BPF filter for UDP port
	filterStr := fmt.Sprintf("udp port %d", config.UDPPort)
	if err := handle.SetBPFFilter(filterStr); err != nil {
		return nil, fmt.Errorf("failed to set BPF filter: %w", err)
	}

	// Initialize components
	parserConfig := parse.LoadEmbeddedPandar40PConfig()
	parser := parse.NewPandar40PParser(parserConfig)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)

	bgManager := createBackgroundManager(config.SensorID)
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())
	classifier := lidar.NewTrackClassifier()

	// Result tracking
	result := &AnalysisResult{
		PCAPFile:      config.PCAPFile,
		TracksByClass: make(map[string]int),
	}

	// Frame building state
	var framePoints []lidar.PointPolar
	var lastAzimuth float64
	var frameStartTime time.Time
	frameCount := 0
	var firstPacketTime, lastPacketTime time.Time

	// Training data collection
	var trainingFrames []*TrainingFrame

	// Process packets
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	ctx := context.Background()

	for packet := range packetSource.Packets() {
		select {
		case <-ctx.Done():
			break
		default:
		}

		udpLayer := packet.Layer(layers.LayerTypeUDP)
		if udpLayer == nil {
			continue
		}

		udp := udpLayer.(*layers.UDP)
		payload := udp.Payload
		if len(payload) == 0 {
			continue
		}

		result.TotalPackets++

		// Parse packet
		points, err := parser.ParsePacket(payload)
		if err != nil {
			if config.Verbose {
				log.Printf("Parse error: %v", err)
			}
			continue
		}

		if len(points) == 0 {
			continue
		}

		result.TotalPoints += len(points)

		// Track packet timestamps
		pktTime := time.Unix(0, points[0].Timestamp)
		if firstPacketTime.IsZero() {
			firstPacketTime = pktTime
			frameStartTime = pktTime
		}
		lastPacketTime = pktTime

		// Detect frame completion (360° rotation)
		for _, p := range points {
			// Check for azimuth wrap (new frame)
			if lastAzimuth > 270 && p.Azimuth < 90 {
				// Frame complete - process it
				if len(framePoints) > 0 {
					frameResult := processFrame(
						bgManager, tracker, classifier,
						framePoints, frameStartTime, config,
					)

					result.TotalFrames++
					result.ForegroundPoints += frameResult.foregroundCount
					result.BackgroundPoints += len(framePoints) - frameResult.foregroundCount
					result.TotalClusters += frameResult.clusterCount

					// Collect training data if requested
					if config.ExportTraining && frameResult.foregroundCount > 0 {
						trainingFrame := &TrainingFrame{
							FrameID:          frameCount,
							Timestamp:        frameStartTime,
							SensorID:         config.SensorID,
							TotalPoints:      len(framePoints),
							ForegroundPoints: frameResult.foregroundCount,
							Clusters:         frameResult.clusterCount,
							ActiveTracks:     len(tracker.GetActiveTracks()),
							ForegroundBlob:   lidar.EncodeForegroundBlob(frameResult.foregroundPoints),
						}
						trainingFrames = append(trainingFrames, trainingFrame)
					}

					if config.Verbose && frameCount%100 == 0 {
						log.Printf("Frame %d: %d points, %d foreground, %d clusters, %d tracks",
							frameCount, len(framePoints), frameResult.foregroundCount,
							frameResult.clusterCount, len(tracker.GetActiveTracks()))
					}

					frameCount++
				}

				// Start new frame
				framePoints = framePoints[:0]
				frameStartTime = pktTime
			}

			framePoints = append(framePoints, p)
			lastAzimuth = p.Azimuth
		}
	}

	// Process final partial frame
	if len(framePoints) > 0 {
		processFrame(bgManager, tracker, classifier, framePoints, frameStartTime, config)
		result.TotalFrames++
	}

	// Compute duration
	result.Duration = lastPacketTime.Sub(firstPacketTime)
	result.DurationSecs = result.Duration.Seconds()

	// Collect track results
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
			StartX:       track.X, // Current position
			StartY:       track.Y,
		}
		result.Tracks = append(result.Tracks, trackExport)

		if track.AvgSpeedMps > 0 {
			speedSamples = append(speedSamples, track.AvgSpeedMps)
		}
	}

	// Compute classification distribution
	result.ClassificationDist = computeClassStats(result.Tracks)

	// Compute speed statistics
	result.SpeedStats = computeSpeedStats(speedSamples)

	// Processing time
	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	// Training frame count
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

type frameProcessResult struct {
	foregroundCount  int
	clusterCount     int
	foregroundPoints []lidar.PointPolar
}

func processFrame(
	bgManager *lidar.BackgroundManager,
	tracker *lidar.Tracker,
	classifier *lidar.TrackClassifier,
	points []lidar.PointPolar,
	timestamp time.Time,
	config Config,
) frameProcessResult {
	result := frameProcessResult{}

	// Step 1: Foreground extraction
	mask, err := bgManager.ProcessFramePolarWithMask(points)
	if err != nil || mask == nil {
		return result
	}

	// Extract foreground points
	foregroundPoints := lidar.ExtractForegroundPoints(points, mask)
	result.foregroundCount = len(foregroundPoints)
	result.foregroundPoints = foregroundPoints

	if len(foregroundPoints) == 0 {
		return result
	}

	// Step 2: Transform to world frame
	worldPoints := lidar.TransformToWorld(foregroundPoints, nil, config.SensorID)

	// Step 3: Cluster
	clusters := lidar.DBSCAN(worldPoints, lidar.DefaultDBSCANParams())
	result.clusterCount = len(clusters)

	if len(clusters) == 0 {
		return result
	}

	// Step 4: Track
	tracker.Update(clusters, timestamp)

	// Step 5: Classify confirmed tracks
	for _, track := range tracker.GetConfirmedTracks() {
		if track.ObjectClass == "" && track.ObservationCount >= 5 {
			classifier.ClassifyAndUpdate(track)
		}
	}

	return result
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

	// Sort for percentiles
	sorted := make([]float32, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum float32
	for _, s := range sorted {
		sum += s
	}

	n := len(sorted)
	return SpeedStatistics{
		MinSpeed: sorted[0],
		MaxSpeed: sorted[n-1],
		AvgSpeed: sum / float32(n),
		P50Speed: sorted[n/2],
		P85Speed: sorted[int(float64(n)*0.85)],
		P95Speed: sorted[int(float64(n)*0.95)],
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
		VALUES (?, ?, ?, ?, ?, ?)`,
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
