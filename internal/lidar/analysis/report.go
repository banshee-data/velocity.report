package analysis

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
	"github.com/banshee-data/velocity.report/internal/version"
)

// GenerateReport analyses a .vrlog recording and writes analysis.json into it.
// It returns the generated report and the path written.
func GenerateReport(vrlogPath string) (*AnalysisReport, string, error) {
	replayer, err := recorder.NewReplayer(vrlogPath)
	if err != nil {
		return nil, "", fmt.Errorf("open vrlog: %w", err)
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
		headings            []float32 // per-frame HeadingRad, for jitter
		xs, ys              []float32 // per-frame position, for alignment
		vxs, vys            []float32 // per-frame velocity, for alignment
		bboxL, bboxW, bboxH []float32
		maxSpeed            float32
		heightP95Max        float32
		occlusionCount      int
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
			return nil, "", fmt.Errorf("read frame %d: %w", frameCount, err)
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
				acc.headings = append(acc.headings, t.HeadingRad)
				acc.xs = append(acc.xs, t.X)
				acc.ys = append(acc.ys, t.Y)
				acc.vxs = append(acc.vxs, t.VX)
				acc.vys = append(acc.vys, t.VY)
				acc.bboxL = append(acc.bboxL, t.BBoxLength)
				acc.bboxW = append(acc.bboxW, t.BBoxWidth)
				acc.bboxH = append(acc.bboxH, t.BBoxHeight)
				// Older .vrlog frames may lack the renamed max-speed field.
				maxSpeedCandidate := t.MaxSpeedMps
				if maxSpeedCandidate == 0 {
					maxSpeedCandidate = t.SpeedMps
				}
				if maxSpeedCandidate > acc.maxSpeed {
					acc.maxSpeed = maxSpeedCandidate
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
		confirmedObsCounts    []float64
		confirmedDurations    []float64
		confirmedLengths      []float64
		totalOcclusions       int
		maxOccCountGlobal     int
		sumOcclusionCount     float64
		confirmedCount        int
		tentativeCount        int
		deletedCount          int
		confirmedHeadJitters  []float64
		confirmedSpeedJitters []float64
		confirmedAlignMeans   []float64
		confirmedMisalignRats []float64
	)

	classDist := make(map[string]*classAccum)

	for id, acc := range tracks {
		dur := float64(acc.lastSeen-acc.firstSeen) / 1e9

		avgSpeed := meanFloat32(acc.speeds)

		// §12.1 per-track implementable-now metrics
		speedVar := speedVariance(acc.speeds)
		headJitter := headingJitterDeg(acc.headings)
		speedJitter := speedJitterMps(acc.speeds)
		alignMean, misalignRatio := alignmentMetrics(acc.xs, acc.ys, acc.vxs, acc.vys)

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
					MaxSpeedMps:       acc.maxSpeed,
			SpeedSamples:      acc.speeds,
			SpeedVariance:     speedVar,
			HeadingJitterDeg:  headJitter,
			SpeedJitterMps:    speedJitter,
			AlignmentMeanDeg:  alignMean,
			MisalignmentRatio: misalignRatio,
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
			if acc.occlusionCount > maxOccCountGlobal {
				maxOccCountGlobal = acc.occlusionCount
			}
			confirmedHeadJitters = append(confirmedHeadJitters, float64(headJitter))
			confirmedSpeedJitters = append(confirmedSpeedJitters, float64(speedJitter))
			confirmedAlignMeans = append(confirmedAlignMeans, float64(alignMean))
			confirmedMisalignRats = append(confirmedMisalignRats, float64(misalignRatio))
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

	// Speed percentiles across confirmed tracks (reuse computeDistStats for P50/P85/P98)
	speedVals := make([]float64, len(confirmedSpeeds))
	for i, s := range confirmedSpeeds {
		speedVals[i] = float64(s)
	}
	speedDist := computeDistStats(speedVals)

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
	// Clamp to 0 for single-frame or degenerate recordings.
	if durationSecs < 0 {
		durationSecs = 0
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

	// Compute frame rate and inferred replay speed.
	// The LiDAR nominally runs at ~10 Hz. If the actual frame rate differs
	// significantly, the recording was likely replayed at a non-1x speed.
	const nominalHz = 10.0
	frameRateHz := 0.0
	if durationSecs > 0 {
		frameRateHz = float64(frameCount) / durationSecs
	}
	inferredReplaySpeed := 0.0
	if frameRateHz > 0 {
		inferredReplaySpeed = frameRateHz / nominalHz
	}

	report := &AnalysisReport{
		Version:     "1.0",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ToolVersion: version.Version,
		Source:      filepath.Base(vrlogPath),
		Recording: RecordingMeta{
			SensorID:            header.SensorID,
			TotalFrames:         header.TotalFrames,
			CreatedNs:           header.CreatedNs,
			StartNs:             header.StartNs,
			EndNs:               header.EndNs,
			DurationSecs:        durationSecs,
			FrameRateHz:         frameRateHz,
			InferredReplaySpeed: inferredReplaySpeed,
			CoordinateFrame:     header.CoordinateFrame.ReferenceFrame,
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
				MaxOcclusionCount:  maxOccCountGlobal,
				TotalOcclusions:    totalOcclusions,
			},
		},
		Tracks: trackDetails,
		SpeedHistogram: SpeedHistogram{
			BinWidthMps: binWidth,
			Bins:        histogram,
			Percentiles: speedDist,
			TotalTracks: confirmedCount,
		},
		ClassificationDistribution: classDistOut,
	}

	// Only populate jitter/alignment aggregates when there are confirmed tracks
	// with data, so that omitempty correctly omits them when empty.
	if len(confirmedHeadJitters) > 0 || len(confirmedSpeedJitters) > 0 {
		report.TrackSummary.Jitter = &JitterSummary{
			HeadingJitterDeg: computeDistStats(confirmedHeadJitters),
			SpeedJitterMps:   computeDistStats(confirmedSpeedJitters),
		}
	}
	if len(confirmedAlignMeans) > 0 || len(confirmedMisalignRats) > 0 {
		report.TrackSummary.Alignment = &AlignmentSummary{
			AlignmentMeanDeg:  computeDistStats(confirmedAlignMeans),
			MisalignmentRatio: computeDistStats(confirmedMisalignRats),
		}
	}

	outPath := filepath.Join(vrlogPath, "analysis.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return nil, "", fmt.Errorf("write %s: %w", outPath, err)
	}

	return report, outPath, nil
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

// speedVariance computes the population variance of speed samples.
func speedVariance(speeds []float32) float32 {
	if len(speeds) < 2 {
		return 0
	}
	mean := float64(meanFloat32(speeds))
	var sumSq float64
	for _, s := range speeds {
		d := float64(s) - mean
		sumSq += d * d
	}
	return float32(sumSq / float64(len(speeds)))
}

// headingJitterDeg computes the RMS of frame-to-frame angular heading changes
// in degrees. Angular wrap-around is handled by normalising the diff to [-π, π].
func headingJitterDeg(headings []float32) float32 {
	if len(headings) < 2 {
		return 0
	}
	var sumSq float64
	for i := 1; i < len(headings); i++ {
		diff := float64(headings[i]) - float64(headings[i-1])
		// Normalise to [-π, π]
		for diff > math.Pi {
			diff -= 2 * math.Pi
		}
		for diff < -math.Pi {
			diff += 2 * math.Pi
		}
		sumSq += diff * diff
	}
	rmsRad := math.Sqrt(sumSq / float64(len(headings)-1))
	return float32(rmsRad * 180.0 / math.Pi)
}

// speedJitterMps computes the RMS of frame-to-frame speed changes in m/s.
func speedJitterMps(speeds []float32) float32 {
	if len(speeds) < 2 {
		return 0
	}
	var sumSq float64
	for i := 1; i < len(speeds); i++ {
		d := float64(speeds[i]) - float64(speeds[i-1])
		sumSq += d * d
	}
	return float32(math.Sqrt(sumSq / float64(len(speeds)-1)))
}

// alignmentMetrics computes the mean alignment angle (degrees) between the
// instantaneous velocity vector and the displacement vector, and the fraction
// of frames where this angle exceeds 45°.
// Requires at least 2 observations with non-zero velocity.
func alignmentMetrics(xs, ys, vxs, vys []float32) (meanDeg float32, misalignRatio float32) {
	n := len(xs)
	if n < 2 || len(ys) < 2 || len(vxs) < 2 || len(vys) < 2 {
		return 0, 0
	}
	var sumAngle float64
	var nMisalign int
	var nValid int
	for i := 1; i < n; i++ {
		dx := float64(xs[i]) - float64(xs[i-1])
		dy := float64(ys[i]) - float64(ys[i-1])
		vx := float64(vxs[i])
		vy := float64(vys[i])

		dispMag := math.Sqrt(dx*dx + dy*dy)
		velMag := math.Sqrt(vx*vx + vy*vy)
		if dispMag < 1e-6 || velMag < 1e-6 {
			continue
		}

		// Angle between displacement and velocity vectors using atan2
		cross := dx*vy - dy*vx
		dot := dx*vx + dy*vy
		angle := math.Abs(math.Atan2(math.Abs(cross), dot))
		angleDeg := angle * 180.0 / math.Pi

		sumAngle += angleDeg
		if angleDeg > 45.0 {
			nMisalign++
		}
		nValid++
	}
	if nValid == 0 {
		return 0, 0
	}
	return float32(sumAngle / float64(nValid)), float32(nMisalign) / float32(nValid)
}
