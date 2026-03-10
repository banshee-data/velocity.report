// Command analyse-vrlog generates analysis reports from .vrlog recordings.
//
// Usage:
//
//	analyse-vrlog report <path.vrlog>              # generate analysis.json
//	analyse-vrlog compare <a.vrlog> <b.vrlog>      # compare two recordings
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
	"github.com/banshee-data/velocity.report/internal/version"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  analyse-vrlog report <path.vrlog>\n")
		fmt.Fprintf(os.Stderr, "  analyse-vrlog compare <a.vrlog> <b.vrlog> [-o output.json]\n")
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "report":
		if err := runReport(os.Args[2]); err != nil {
			log.Fatalf("report failed: %v", err)
		}
	case "compare":
		if len(os.Args) < 4 {
			log.Fatalf("compare requires two .vrlog paths")
		}
		outPath := ""
		if len(os.Args) >= 6 && os.Args[4] == "-o" {
			outPath = os.Args[5]
		}
		if err := runCompare(os.Args[2], os.Args[3], outPath); err != nil {
			log.Fatalf("compare failed: %v", err)
		}
	default:
		log.Fatalf("unknown command: %s (use 'report' or 'compare')", cmd)
	}
}

// ---------------------------------------------------------------------------
// Report types
// ---------------------------------------------------------------------------

// AnalysisReport is the top-level analysis output for a single .vrlog.
type AnalysisReport struct {
	Version     string `json:"version"`
	GeneratedAt string `json:"generated_at"`
	ToolVersion string `json:"tool_version"`
	Source      string `json:"source"`

	Recording                  RecordingMeta         `json:"recording"`
	FrameSummary               FrameSummary          `json:"frame_summary"`
	TrackSummary               TrackSummary          `json:"track_summary"`
	Tracks                     []TrackDetail         `json:"tracks"`
	SpeedHistogram             SpeedHistogram        `json:"speed_histogram"`
	ClassificationDistribution map[string]ClassStats `json:"classification_distribution"`
}

// RecordingMeta is §2 in the spec.
type RecordingMeta struct {
	SensorID        string  `json:"sensor_id"`
	TotalFrames     uint64  `json:"total_frames"`
	StartNs         int64   `json:"start_ns"`
	EndNs           int64   `json:"end_ns"`
	DurationSecs    float64 `json:"duration_secs"`
	CoordinateFrame string  `json:"coordinate_frame"`
}

// FrameSummary is §3 in the spec.
type FrameSummary struct {
	TotalFrames                 int        `json:"total_frames"`
	FramesWithTracks            int        `json:"frames_with_tracks"`
	FramesWithClusters          int        `json:"frames_with_clusters"`
	AvgPointsPerFrame           float64    `json:"avg_points_per_frame"`
	AvgForegroundPointsPerFrame float64    `json:"avg_foreground_points_per_frame"`
	ForegroundPct               float64    `json:"foreground_pct"`
	AvgClustersPerFrame         float64    `json:"avg_clusters_per_frame"`
	AvgTracksPerFrame           float64    `json:"avg_tracks_per_frame"`
	FrameIntervalMs             *DistStats `json:"frame_interval_ms,omitempty"`
}

// TrackSummary is §4 in the spec.
type TrackSummary struct {
	TotalTracks        int                `json:"total_tracks"`
	ConfirmedTracks    int                `json:"confirmed_tracks"`
	TentativeTracks    int                `json:"tentative_tracks"`
	DeletedTracks      int                `json:"deleted_tracks"`
	FragmentationRatio float64            `json:"fragmentation_ratio"`
	ObservationCount   *DistStats         `json:"observation_count,omitempty"`
	TrackDurationSecs  *DistStats         `json:"track_duration_secs,omitempty"`
	TrackLengthMetres  *DistStats         `json:"track_length_metres,omitempty"`
	Occlusion          *OcclusionSummary  `json:"occlusion"`
	MergeSplit         *MergeSplitSummary `json:"merge_split"`
}

// OcclusionSummary captures aggregate occlusion metrics.
type OcclusionSummary struct {
	MeanOcclusionCount float64 `json:"mean_occlusion_count"`
	MaxOcclusionFrames int     `json:"max_occlusion_frames"`
	TotalOcclusions    int     `json:"total_occlusions"`
}

// MergeSplitSummary captures merge/split candidate counts.
type MergeSplitSummary struct {
	MergeCandidates int `json:"merge_candidates"`
	SplitCandidates int `json:"split_candidates"`
}

// TrackDetail is §5 in the spec — one entry per track.
type TrackDetail struct {
	TrackID         string  `json:"track_id"`
	State           string  `json:"state"`
	ObjectClass     string  `json:"object_class"`
	ClassConfidence float32 `json:"class_confidence"`

	ObservationCount int     `json:"observation_count"`
	Hits             int     `json:"hits"`
	Misses           int     `json:"misses"`
	FirstSeenNs      int64   `json:"first_seen_ns"`
	LastSeenNs       int64   `json:"last_seen_ns"`
	DurationSecs     float64 `json:"duration_secs"`

	AvgSpeedMps  float32   `json:"avg_speed_mps"`
	PeakSpeedMps float32   `json:"peak_speed_mps"`
	SpeedSamples []float32 `json:"speed_samples,omitempty"`

	StartX            float32 `json:"start_x"`
	StartY            float32 `json:"start_y"`
	EndX              float32 `json:"end_x"`
	EndY              float32 `json:"end_y"`
	TrackLengthMetres float32 `json:"track_length_metres"`

	AvgBBox      BBoxDims `json:"avg_bbox"`
	HeightP95Max float32  `json:"height_p95_max"`

	OcclusionCount     int     `json:"occlusion_count"`
	MaxOcclusionFrames int     `json:"max_occlusion_frames"`
	MotionModel        string  `json:"motion_model"`
	Confidence         float32 `json:"confidence"`
}

// BBoxDims captures averaged bounding box dimensions.
type BBoxDims struct {
	Length float32 `json:"length"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// SpeedHistogram is §6 in the spec.
type SpeedHistogram struct {
	BinWidthMps float64          `json:"bin_width_mps"`
	Bins        []HistogramBin   `json:"bins"`
	Percentiles SpeedPercentiles `json:"percentiles"`
	TotalTracks int              `json:"total_tracks"`
}

// HistogramBin is a single bin in the speed histogram.
type HistogramBin struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	Count int     `json:"count"`
}

// SpeedPercentiles captures P50/P85/P95 speeds.
type SpeedPercentiles struct {
	P50 float32 `json:"p50"`
	P85 float32 `json:"p85"`
	P95 float32 `json:"p95"`
}

// ClassStats is §7 in the spec — per-class aggregates.
type ClassStats struct {
	Count           int     `json:"count"`
	AvgSpeedMps     float64 `json:"avg_speed_mps"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	AvgObservations float64 `json:"avg_observations"`
}

// DistStats captures min/max/avg/p50/p85/p98 for a distribution.
type DistStats struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Avg     float64 `json:"avg"`
	P50     float64 `json:"p50"`
	P85     float64 `json:"p85"`
	P98     float64 `json:"p98"`
	Samples int     `json:"samples"`
}

// ---------------------------------------------------------------------------
// Comparison types (§8)
// ---------------------------------------------------------------------------

// ComparisonReport is the two-file comparison output.
type ComparisonReport struct {
	Version     string `json:"version"`
	GeneratedAt string `json:"generated_at"`
	RunA        string `json:"run_a"`
	RunB        string `json:"run_b"`

	FrameOverlap  FrameOverlap  `json:"frame_overlap"`
	TrackMatching TrackMatching `json:"track_matching"`
	SpeedDelta    SpeedDelta    `json:"speed_delta"`
	QualityDelta  QualityDelta  `json:"quality_delta"`
}

// FrameOverlap is §8.2.
type FrameOverlap struct {
	AFrames          int     `json:"a_frames"`
	BFrames          int     `json:"b_frames"`
	TemporalOverlapS float64 `json:"temporal_overlap_secs"`
	TemporalUnionS   float64 `json:"temporal_union_secs"`
	TemporalIoU      float64 `json:"temporal_iou"`
}

// TrackMatching is §8.3.
type TrackMatching struct {
	ATotalTracks int         `json:"a_total_tracks"`
	BTotalTracks int         `json:"b_total_tracks"`
	MatchedPairs int         `json:"matched_pairs"`
	AOnlyTracks  int         `json:"a_only_tracks"`
	BOnlyTracks  int         `json:"b_only_tracks"`
	Matches      []MatchPair `json:"matches"`
}

// MatchPair is a single matched track pair.
type MatchPair struct {
	ATrackID         string  `json:"a_track_id"`
	BTrackID         string  `json:"b_track_id"`
	TemporalIoU      float64 `json:"temporal_iou"`
	SpeedDeltaMps    float64 `json:"speed_delta_mps"`
	ObservationRatio float64 `json:"observation_ratio"`
	ClassMatch       bool    `json:"class_match"`
}

// SpeedDelta is §8.4.
type SpeedDelta struct {
	MeanAbsSpeedDeltaMps float64 `json:"mean_abs_speed_delta_mps"`
	MaxAbsSpeedDeltaMps  float64 `json:"max_abs_speed_delta_mps"`
	SpeedCorrelation     float64 `json:"speed_correlation"`
}

// QualityDelta is §8.5.
type QualityDelta struct {
	FragmentationRatio DeltaPair `json:"fragmentation_ratio"`
	MeanObservations   DeltaPair `json:"mean_observations"`
	MeanOcclusionCount DeltaPair `json:"mean_occlusion_count"`
}

// DeltaPair shows a and b values with their difference.
type DeltaPair struct {
	A     float64 `json:"a"`
	B     float64 `json:"b"`
	Delta float64 `json:"delta"`
}

// ---------------------------------------------------------------------------
// Report generation
// ---------------------------------------------------------------------------

func runReport(vrlogPath string) error {
	replayer, err := recorder.NewReplayer(vrlogPath)
	if err != nil {
		return fmt.Errorf("open vrlog: %w", err)
	}
	defer replayer.Close()

	header := replayer.Header()

	// Accumulators
	var (
		totalPoints        int64
		totalFGPoints      int64
		totalClusters      int64
		totalTrackCount    int64
		framesWithTracks   int
		framesWithClusters int
		frameTimestamps    []int64
	)

	// Per-track accumulators — keyed by TrackID.
	type trackAccum struct {
		firstSeen           int64
		lastSeen            int64
		firstX, firstY      float32
		lastX, lastY        float32
		speeds              []float32
		bboxL, bboxW, bboxH []float32
		peakSpeed           float32
		heightP95Max        float32
		occlusionCount      int
		maxOccFrames        int
		motionModel         visualiser.MotionModel
		confidence          float32
		hits, misses        int
		state               visualiser.TrackState
		objectClass         string
		classConf           float32
		obsCount            int
		trackLengthM        float32
	}
	tracks := make(map[string]*trackAccum)

	frameCount := 0
	for {
		frame, err := replayer.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read frame %d: %w", frameCount, err)
		}
		frameCount++
		frameTimestamps = append(frameTimestamps, frame.TimestampNanos)

		// Point cloud stats
		if pc := frame.PointCloud; pc != nil {
			nPts := len(pc.X)
			totalPoints += int64(nPts)
			for _, c := range pc.Classification {
				if c == 1 { // foreground
					totalFGPoints++
				}
			}
		}

		// Cluster stats
		if cs := frame.Clusters; cs != nil {
			n := len(cs.Clusters)
			totalClusters += int64(n)
			if n > 0 {
				framesWithClusters++
			}
		}

		// Track stats
		if ts := frame.Tracks; ts != nil {
			n := len(ts.Tracks)
			totalTrackCount += int64(n)
			if n > 0 {
				framesWithTracks++
			}

			for i := range ts.Tracks {
				t := &ts.Tracks[i]
				acc, ok := tracks[t.TrackID]
				if !ok {
					acc = &trackAccum{
						firstSeen: t.FirstSeenNanos,
						firstX:    t.X,
						firstY:    t.Y,
					}
					tracks[t.TrackID] = acc
				}
				acc.lastSeen = t.LastSeenNanos
				acc.lastX = t.X
				acc.lastY = t.Y
				acc.speeds = append(acc.speeds, t.SpeedMps)
				acc.bboxL = append(acc.bboxL, t.BBoxLength)
				acc.bboxW = append(acc.bboxW, t.BBoxWidth)
				acc.bboxH = append(acc.bboxH, t.BBoxHeight)
				if t.PeakSpeedMps > acc.peakSpeed {
					acc.peakSpeed = t.PeakSpeedMps
				}
				if t.HeightP95Max > acc.heightP95Max {
					acc.heightP95Max = t.HeightP95Max
				}
				acc.state = t.State
				acc.objectClass = t.ObjectClass
				acc.classConf = t.ClassConfidence
				acc.hits = t.Hits
				acc.misses = t.Misses
				acc.obsCount = t.ObservationCount
				acc.occlusionCount = t.OcclusionCount
				acc.motionModel = t.MotionModel
				acc.confidence = t.Confidence
				acc.trackLengthM = t.TrackLengthMetres
			}
		}
	}

	// Build frame interval distribution
	var intervalDist *DistStats
	if len(frameTimestamps) > 1 {
		intervals := make([]float64, 0, len(frameTimestamps)-1)
		for i := 1; i < len(frameTimestamps); i++ {
			dtNs := frameTimestamps[i] - frameTimestamps[i-1]
			intervals = append(intervals, float64(dtNs)/1e6) // ms
		}
		intervalDist = computeDistStats(intervals)
	}

	// Build per-track detail
	trackDetails := make([]TrackDetail, 0, len(tracks))
	var confirmedSpeeds []float32

	// Collect summary accumulators for confirmed tracks
	var (
		confirmedObsCounts []float64
		confirmedDurations []float64
		confirmedLengths   []float64
		totalOcclusions    int
		maxOccFramesGlobal int
		sumOcclusionCount  float64
		mergeCandidates    int
		splitCandidates    int
		confirmedCount     int
		tentativeCount     int
		deletedCount       int
	)

	classDist := make(map[string]*classAccum)

	for id, acc := range tracks {
		dur := float64(acc.lastSeen-acc.firstSeen) / 1e9

		avgSpeed := meanFloat32(acc.speeds)
		td := TrackDetail{
			TrackID:           id,
			State:             trackStateName(acc.state),
			ObjectClass:       acc.objectClass,
			ClassConfidence:   acc.classConf,
			ObservationCount:  acc.obsCount,
			Hits:              acc.hits,
			Misses:            acc.misses,
			FirstSeenNs:       acc.firstSeen,
			LastSeenNs:        acc.lastSeen,
			DurationSecs:      dur,
			AvgSpeedMps:       avgSpeed,
			PeakSpeedMps:      acc.peakSpeed,
			SpeedSamples:      acc.speeds,
			StartX:            acc.firstX,
			StartY:            acc.firstY,
			EndX:              acc.lastX,
			EndY:              acc.lastY,
			TrackLengthMetres: acc.trackLengthM,
			AvgBBox: BBoxDims{
				Length: meanFloat32(acc.bboxL),
				Width:  meanFloat32(acc.bboxW),
				Height: meanFloat32(acc.bboxH),
			},
			HeightP95Max:   acc.heightP95Max,
			OcclusionCount: acc.occlusionCount,
			MotionModel:    motionModelName(acc.motionModel),
			Confidence:     acc.confidence,
		}
		trackDetails = append(trackDetails, td)

		switch acc.state {
		case visualiser.TrackStateConfirmed:
			confirmedCount++
			confirmedSpeeds = append(confirmedSpeeds, avgSpeed)
			confirmedObsCounts = append(confirmedObsCounts, float64(acc.obsCount))
			confirmedDurations = append(confirmedDurations, dur)
			confirmedLengths = append(confirmedLengths, float64(acc.trackLengthM))
			totalOcclusions += acc.occlusionCount
			sumOcclusionCount += float64(acc.occlusionCount)
			if acc.occlusionCount > maxOccFramesGlobal {
				maxOccFramesGlobal = acc.occlusionCount
			}
			// Classification distribution
			cls := acc.objectClass
			if cls == "" {
				cls = "unclassified"
			}
			ca, ok := classDist[cls]
			if !ok {
				ca = &classAccum{}
				classDist[cls] = ca
			}
			ca.count++
			ca.sumSpeed += float64(avgSpeed)
			ca.sumDur += dur
			ca.sumObs += float64(acc.obsCount)
		case visualiser.TrackStateTentative:
			tentativeCount++
		case visualiser.TrackStateDeleted:
			deletedCount++
		}
	}

	// Sort tracks by first_seen_ns
	sort.Slice(trackDetails, func(i, j int) bool {
		return trackDetails[i].FirstSeenNs < trackDetails[j].FirstSeenNs
	})

	// Speed histogram (1 m/s bins)
	binWidth := 1.0
	histogram := buildSpeedHistogram(confirmedSpeeds, binWidth)

	// Speed percentiles across confirmed tracks
	p50, p85, p95 := l6objects.ComputeSpeedPercentiles(confirmedSpeeds)

	// Classification distribution
	classDistOut := make(map[string]ClassStats, len(classDist))
	for cls, ca := range classDist {
		n := float64(ca.count)
		classDistOut[cls] = ClassStats{
			Count:           ca.count,
			AvgSpeedMps:     ca.sumSpeed / n,
			AvgDurationSecs: ca.sumDur / n,
			AvgObservations: ca.sumObs / n,
		}
	}

	totalTracks := len(tracks)
	fragRatio := 0.0
	if totalTracks > 0 {
		fragRatio = float64(tentativeCount) / float64(totalTracks)
	}
	meanOcc := 0.0
	if confirmedCount > 0 {
		meanOcc = sumOcclusionCount / float64(confirmedCount)
	}

	durationSecs := float64(header.EndNs-header.StartNs) / 1e9
	if durationSecs < 0 {
		// Fallback: use frame timestamps if header timestamps are inconsistent
		if len(frameTimestamps) >= 2 {
			durationSecs = float64(frameTimestamps[len(frameTimestamps)-1]-frameTimestamps[0]) / 1e9
		}
	}

	avgPtsPerFrame := 0.0
	avgFGPerFrame := 0.0
	avgClusPerFrame := 0.0
	avgTracksPerFrame := 0.0
	fgPct := 0.0
	if frameCount > 0 {
		avgPtsPerFrame = float64(totalPoints) / float64(frameCount)
		avgFGPerFrame = float64(totalFGPoints) / float64(frameCount)
		avgClusPerFrame = float64(totalClusters) / float64(frameCount)
		avgTracksPerFrame = float64(totalTrackCount) / float64(frameCount)
		if totalPoints > 0 {
			fgPct = float64(totalFGPoints) / float64(totalPoints) * 100
		}
	}

	report := &AnalysisReport{
		Version:     "1.0",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ToolVersion: version.Version,
		Source:      filepath.Base(vrlogPath),
		Recording: RecordingMeta{
			SensorID:        header.SensorID,
			TotalFrames:     header.TotalFrames,
			StartNs:         header.StartNs,
			EndNs:           header.EndNs,
			DurationSecs:    durationSecs,
			CoordinateFrame: header.CoordinateFrame.ReferenceFrame,
		},
		FrameSummary: FrameSummary{
			TotalFrames:                 frameCount,
			FramesWithTracks:            framesWithTracks,
			FramesWithClusters:          framesWithClusters,
			AvgPointsPerFrame:           avgPtsPerFrame,
			AvgForegroundPointsPerFrame: avgFGPerFrame,
			ForegroundPct:               fgPct,
			AvgClustersPerFrame:         avgClusPerFrame,
			AvgTracksPerFrame:           avgTracksPerFrame,
			FrameIntervalMs:             intervalDist,
		},
		TrackSummary: TrackSummary{
			TotalTracks:        totalTracks,
			ConfirmedTracks:    confirmedCount,
			TentativeTracks:    tentativeCount,
			DeletedTracks:      deletedCount,
			FragmentationRatio: fragRatio,
			ObservationCount:   computeDistStats(confirmedObsCounts),
			TrackDurationSecs:  computeDistStats(confirmedDurations),
			TrackLengthMetres:  computeDistStats(confirmedLengths),
			Occlusion: &OcclusionSummary{
				MeanOcclusionCount: meanOcc,
				MaxOcclusionFrames: maxOccFramesGlobal,
				TotalOcclusions:    totalOcclusions,
			},
			MergeSplit: &MergeSplitSummary{
				MergeCandidates: mergeCandidates,
				SplitCandidates: splitCandidates,
			},
		},
		Tracks: trackDetails,
		SpeedHistogram: SpeedHistogram{
			BinWidthMps: binWidth,
			Bins:        histogram,
			Percentiles: SpeedPercentiles{P50: p50, P85: p85, P95: p95},
			TotalTracks: confirmedCount,
		},
		ClassificationDistribution: classDistOut,
	}

	outPath := filepath.Join(vrlogPath, "analysis.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	log.Printf("Wrote %s (%d frames, %d tracks, %d confirmed)",
		outPath, frameCount, totalTracks, confirmedCount)
	return nil
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

func runCompare(pathA, pathB, outPath string) error {
	reportA, err := loadAnalysis(pathA)
	if err != nil {
		return fmt.Errorf("load A: %w", err)
	}
	reportB, err := loadAnalysis(pathB)
	if err != nil {
		return fmt.Errorf("load B: %w", err)
	}

	// Frame overlap (§8.2)
	aStart := reportA.Recording.StartNs
	aEnd := reportA.Recording.EndNs
	bStart := reportB.Recording.StartNs
	bEnd := reportB.Recording.EndNs

	overlap := l6objects.ComputeTemporalIoU(aStart, aEnd, bStart, bEnd)

	overlapStart := max(aStart, bStart)
	overlapEnd := min(aEnd, bEnd)
	overlapSecs := 0.0
	if overlapEnd > overlapStart {
		overlapSecs = float64(overlapEnd-overlapStart) / 1e9
	}
	unionStart := min(aStart, bStart)
	unionEnd := max(aEnd, bEnd)
	unionSecs := float64(unionEnd-unionStart) / 1e9

	// Track matching (§8.3) — build lightweight track descriptors for Hungarian
	type trackDesc struct {
		id       string
		startNs  int64
		endNs    int64
		avgSpeed float32
		obsCount int
		class    string
	}

	extractTracks := func(r *AnalysisReport) []trackDesc {
		out := make([]trackDesc, 0, len(r.Tracks))
		for _, t := range r.Tracks {
			if t.State != "confirmed" {
				continue
			}
			out = append(out, trackDesc{
				id:       t.TrackID,
				startNs:  t.FirstSeenNs,
				endNs:    t.LastSeenNs,
				avgSpeed: t.AvgSpeedMps,
				obsCount: t.ObservationCount,
				class:    t.ObjectClass,
			})
		}
		return out
	}

	tracksA := extractTracks(reportA)
	tracksB := extractTracks(reportB)

	// Build cost matrix for Hungarian matching
	const iouThreshold = 0.3
	const forbiddenCost float32 = 1e18

	costMatrix := make([][]float32, len(tracksA))
	iouMatrix := make([][]float64, len(tracksA))
	for i, tA := range tracksA {
		costMatrix[i] = make([]float32, len(tracksB))
		iouMatrix[i] = make([]float64, len(tracksB))
		for j, tB := range tracksB {
			iou := l6objects.ComputeTemporalIoU(tA.startNs, tA.endNs, tB.startNs, tB.endNs)
			iouMatrix[i][j] = iou
			if iou > iouThreshold {
				costMatrix[i][j] = float32(1.0 - iou)
			} else {
				costMatrix[i][j] = forbiddenCost
			}
		}
	}

	var matches []MatchPair
	matchedA := make(map[int]bool)
	matchedB := make(map[int]bool)

	if len(tracksA) > 0 && len(tracksB) > 0 {
		assignments := l5tracks.HungarianAssign(costMatrix)
		for i, j := range assignments {
			if j >= 0 && j < len(tracksB) && costMatrix[i][j] < forbiddenCost {
				tA := tracksA[i]
				tB := tracksB[j]

				obsMin := tA.obsCount
				obsMax := tB.obsCount
				if obsMin > obsMax {
					obsMin, obsMax = obsMax, obsMin
				}
				obsRatio := 0.0
				if obsMax > 0 {
					obsRatio = float64(obsMin) / float64(obsMax)
				}

				matches = append(matches, MatchPair{
					ATrackID:         tA.id,
					BTrackID:         tB.id,
					TemporalIoU:      iouMatrix[i][j],
					SpeedDeltaMps:    math.Abs(float64(tA.avgSpeed) - float64(tB.avgSpeed)),
					ObservationRatio: obsRatio,
					ClassMatch:       tA.class == tB.class,
				})
				matchedA[i] = true
				matchedB[j] = true
			}
		}
	}

	// Speed delta (§8.4)
	var sumAbsDelta, maxAbsDelta float64
	for _, m := range matches {
		if m.SpeedDeltaMps > maxAbsDelta {
			maxAbsDelta = m.SpeedDeltaMps
		}
		sumAbsDelta += m.SpeedDeltaMps
	}
	meanAbsDelta := 0.0
	if len(matches) > 0 {
		meanAbsDelta = sumAbsDelta / float64(len(matches))
	}

	// Speed correlation (Pearson r)
	speedCorr := 0.0
	if len(matches) >= 2 {
		// Index speeds by track ID to avoid O(matches × tracks) lookups.
		speedByA := make(map[string]float64, len(tracksA))
		for _, t := range tracksA {
			speedByA[t.id] = float64(t.avgSpeed)
		}
		speedByB := make(map[string]float64, len(tracksB))
		for _, t := range tracksB {
			speedByB[t.id] = float64(t.avgSpeed)
		}
		xs := make([]float64, len(matches))
		ys := make([]float64, len(matches))
		for i, m := range matches {
			xs[i] = speedByA[m.ATrackID]
			ys[i] = speedByB[m.BTrackID]
		}
		speedCorr = pearsonR(xs, ys)
	}

	// Quality delta (§8.5)
	aObs := reportA.TrackSummary.ObservationCount
	bObs := reportB.TrackSummary.ObservationCount
	aOcc := reportA.TrackSummary.Occlusion
	bOcc := reportB.TrackSummary.Occlusion

	meanObsA := 0.0
	meanObsB := 0.0
	if aObs != nil {
		meanObsA = aObs.Avg
	}
	if bObs != nil {
		meanObsB = bObs.Avg
	}
	meanOccA := 0.0
	meanOccB := 0.0
	if aOcc != nil {
		meanOccA = aOcc.MeanOcclusionCount
	}
	if bOcc != nil {
		meanOccB = bOcc.MeanOcclusionCount
	}

	comparison := &ComparisonReport{
		Version:     "1.0",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RunA:        filepath.Base(pathA),
		RunB:        filepath.Base(pathB),
		FrameOverlap: FrameOverlap{
			AFrames:          reportA.FrameSummary.TotalFrames,
			BFrames:          reportB.FrameSummary.TotalFrames,
			TemporalOverlapS: overlapSecs,
			TemporalUnionS:   unionSecs,
			TemporalIoU:      overlap,
		},
		TrackMatching: TrackMatching{
			ATotalTracks: len(tracksA),
			BTotalTracks: len(tracksB),
			MatchedPairs: len(matches),
			AOnlyTracks:  len(tracksA) - len(matchedA),
			BOnlyTracks:  len(tracksB) - len(matchedB),
			Matches:      matches,
		},
		SpeedDelta: SpeedDelta{
			MeanAbsSpeedDeltaMps: meanAbsDelta,
			MaxAbsSpeedDeltaMps:  maxAbsDelta,
			SpeedCorrelation:     speedCorr,
		},
		QualityDelta: QualityDelta{
			FragmentationRatio: DeltaPair{
				A:     reportA.TrackSummary.FragmentationRatio,
				B:     reportB.TrackSummary.FragmentationRatio,
				Delta: reportB.TrackSummary.FragmentationRatio - reportA.TrackSummary.FragmentationRatio,
			},
			MeanObservations: DeltaPair{
				A: meanObsA, B: meanObsB, Delta: meanObsB - meanObsA,
			},
			MeanOcclusionCount: DeltaPair{
				A: meanOccA, B: meanOccB, Delta: meanOccB - meanOccA,
			},
		},
	}

	data, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal comparison: %w", err)
	}

	if outPath == "" {
		fmt.Println(string(data))
	} else {
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		log.Printf("Wrote %s", outPath)
	}
	return nil
}

// loadAnalysis reads analysis.json from inside a .vrlog directory.
func loadAnalysis(vrlogPath string) (*AnalysisReport, error) {
	p := filepath.Join(vrlogPath, "analysis.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var report AnalysisReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &report, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type classAccum struct {
	count    int
	sumSpeed float64
	sumDur   float64
	sumObs   float64
}

func trackStateName(s visualiser.TrackState) string {
	switch s {
	case visualiser.TrackStateTentative:
		return "tentative"
	case visualiser.TrackStateConfirmed:
		return "confirmed"
	case visualiser.TrackStateDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

func motionModelName(m visualiser.MotionModel) string {
	switch m {
	case visualiser.MotionModelCV:
		return "CV"
	case visualiser.MotionModelCA:
		return "CA"
	default:
		return "unknown"
	}
}

func meanFloat32(vals []float32) float32 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += float64(v)
	}
	return float32(sum / float64(len(vals)))
}

func computeDistStats(vals []float64) *DistStats {
	if len(vals) == 0 {
		return nil
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	n := len(sorted)
	var sum float64
	for _, v := range sorted {
		sum += v
	}

	percentileIdx := func(p float64) int {
		idx := int(math.Floor(float64(n) * p))
		if idx >= n {
			idx = n - 1
		}
		return idx
	}

	return &DistStats{
		Min:     sorted[0],
		Max:     sorted[n-1],
		Avg:     sum / float64(n),
		P50:     sorted[percentileIdx(0.50)],
		P85:     sorted[percentileIdx(0.85)],
		P98:     sorted[percentileIdx(0.98)],
		Samples: n,
	}
}

func buildSpeedHistogram(speeds []float32, binWidth float64) []HistogramBin {
	if len(speeds) == 0 {
		return nil
	}

	maxSpeed := float64(0)
	for _, s := range speeds {
		if float64(s) > maxSpeed {
			maxSpeed = float64(s)
		}
	}

	nBins := int(math.Ceil(maxSpeed/binWidth)) + 1
	bins := make([]HistogramBin, nBins)
	for i := range bins {
		bins[i].Lower = float64(i) * binWidth
		bins[i].Upper = float64(i+1) * binWidth
	}

	for _, s := range speeds {
		idx := int(float64(s) / binWidth)
		if idx < 0 {
			idx = 0
		}
		if idx >= nBins {
			idx = nBins - 1
		}
		bins[idx].Count++
	}

	return bins
}

func pearsonR(xs, ys []float64) float64 {
	n := len(xs)
	if n < 2 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := 0; i < n; i++ {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}

	nf := float64(n)
	num := nf*sumXY - sumX*sumY
	den := math.Sqrt((nf*sumX2 - sumX*sumX) * (nf*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}
