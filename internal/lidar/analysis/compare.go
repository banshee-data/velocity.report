package analysis

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

// CompareReports loads analysis.json from two .vrlog directories and produces
// a ComparisonReport. If outPath is empty the caller is responsible for
// serialisation; otherwise the report is written to outPath.
//
// If analysis.json does not yet exist for either input, it is generated
// automatically via [GenerateReport].
func CompareReports(pathA, pathB, outPath string) (*ComparisonReport, error) {
	reportA, err := loadOrGenerate(pathA)
	if err != nil {
		return nil, fmt.Errorf("load A: %w", err)
	}
	reportB, err := loadOrGenerate(pathB)
	if err != nil {
		return nil, fmt.Errorf("load B: %w", err)
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
	// Index speeds by track ID for O(1) lookup in correlation and per_pair.
	speedByA := make(map[string]float64, len(tracksA))
	for _, t := range tracksA {
		speedByA[t.id] = float64(t.avgSpeed)
	}
	speedByB := make(map[string]float64, len(tracksB))
	for _, t := range tracksB {
		speedByB[t.id] = float64(t.avgSpeed)
	}

	speedCorr := 0.0
	if len(matches) >= 2 {
		xs := make([]float64, len(matches))
		ys := make([]float64, len(matches))
		for i, m := range matches {
			xs[i] = speedByA[m.ATrackID]
			ys[i] = speedByB[m.BTrackID]
		}
		speedCorr = pearsonR(xs, ys)
	}

	// Per-pair speed breakdown (§8.4)
	perPair := make([]MatchedPairSpeed, 0, len(matches))
	for _, m := range matches {
		aSpd := speedByA[m.ATrackID]
		bSpd := speedByB[m.BTrackID]
		perPair = append(perPair, MatchedPairSpeed{
			ATrackID:      m.ATrackID,
			BTrackID:      m.BTrackID,
			AAvgSpeedMps:  aSpd,
			BAvgSpeedMps:  bSpd,
			SpeedDeltaMps: math.Abs(aSpd - bSpd),
		})
	}

	// Earth Mover Distance (Wasserstein-1) between speed histograms (§8.4)
	emd := histogramEMD(reportA.SpeedHistogram, reportB.SpeedHistogram)

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
			MeanAbsSpeedDeltaMps:    meanAbsDelta,
			MaxAbsSpeedDeltaMps:     maxAbsDelta,
			SpeedCorrelation:        speedCorr,
			HistogramEarthMoverDist: emd,
			PerPair:                 perPair,
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

	if outPath != "" {
		data, err := json.MarshalIndent(comparison, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal comparison: %w", err)
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", outPath, err)
		}
	}

	return comparison, nil
}

// LoadAnalysis reads analysis.json from inside a .vrlog directory.
func LoadAnalysis(vrlogPath string) (*AnalysisReport, error) {
	p := filepath.Join(vrlogPath, "analysis.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if err := validateAnalysisSchema(data); err != nil {
		return nil, fmt.Errorf("validate %s: %w", p, err)
	}
	var report AnalysisReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &report, nil
}

func validateAnalysisSchema(data []byte) error {
	var raw struct {
		Tracks []map[string]json.RawMessage `json:"tracks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for i, track := range raw.Tracks {
		if _, ok := track["max_speed_mps"]; ok {
			continue
		}
		if _, ok := track["MaxSpeedMps"]; ok {
			continue
		}
		return fmt.Errorf("track %d missing required key max_speed_mps", i)
	}
	return nil
}

// loadOrGenerate returns the cached analysis if analysis.json exists,
// otherwise runs GenerateReport to create it. Regenerates for any error
// including missing file, corrupt JSON, or stale schema.
func loadOrGenerate(vrlogPath string) (*AnalysisReport, error) {
	report, err := LoadAnalysis(vrlogPath)
	if err == nil {
		return report, nil
	}
	// analysis.json missing, corrupt, or stale-schema — generate it.
	diagf("Generating analysis for %s ...", vrlogPath)
	report, _, genErr := GenerateReport(vrlogPath)
	if genErr != nil {
		return nil, genErr
	}
	return report, nil
}

// isJSONError returns true if the error chain contains a JSON syntax or
// unmarshal type error.
func isJSONError(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		switch e.(type) {
		case *json.SyntaxError, *json.UnmarshalTypeError:
			return true
		}
	}
	return false
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

// histogramEMD computes the Wasserstein-1 (Earth Mover's Distance) between
// two speed histograms. Both histograms are normalised to unit mass before
// comparison. Returns 0 if either histogram is empty.
//
// The algorithm merges the bin boundaries of both histograms into a single
// sorted grid, normalises the counts to form probability mass functions, then
// integrates |CDF_A(x) − CDF_B(x)| dx across all bins.
func histogramEMD(a, b SpeedHistogram) float64 {
	if len(a.Bins) == 0 || len(b.Bins) == 0 {
		return 0
	}

	// Collect all unique bin boundaries from both histograms.
	boundarySet := make(map[float64]struct{})
	for _, bin := range a.Bins {
		boundarySet[bin.Lower] = struct{}{}
		boundarySet[bin.Upper] = struct{}{}
	}
	for _, bin := range b.Bins {
		boundarySet[bin.Lower] = struct{}{}
		boundarySet[bin.Upper] = struct{}{}
	}

	boundaries := make([]float64, 0, len(boundarySet))
	for v := range boundarySet {
		boundaries = append(boundaries, v)
	}
	sort.Float64s(boundaries)

	if len(boundaries) < 2 {
		return 0
	}

	// Build a lookup: bin lower → normalised mass for A and B.
	totalA := 0
	for _, bin := range a.Bins {
		totalA += bin.Count
	}
	totalB := 0
	for _, bin := range b.Bins {
		totalB += bin.Count
	}
	if totalA == 0 || totalB == 0 {
		return 0
	}

	massA := make(map[float64]float64, len(a.Bins))
	for _, bin := range a.Bins {
		massA[bin.Lower] = float64(bin.Count) / float64(totalA)
	}
	massB := make(map[float64]float64, len(b.Bins))
	for _, bin := range b.Bins {
		massB[bin.Lower] = float64(bin.Count) / float64(totalB)
	}

	// Walk merged grid accumulating CDFs and integrating |CDF_A - CDF_B|.
	var cdfA, cdfB float64
	var emd float64
	for i := 0; i+1 < len(boundaries); i++ {
		lo := boundaries[i]
		width := boundaries[i+1] - lo
		cdfA += massA[lo]
		cdfB += massB[lo]
		emd += math.Abs(cdfA-cdfB) * width
	}
	return emd
}
