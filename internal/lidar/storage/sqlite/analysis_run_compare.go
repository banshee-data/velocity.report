package sqlite

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// compareParams compares two RunParams and returns a map of differences.
func compareParams(p1, p2 *RunParams) map[string]any {
	diff := make(map[string]any)

	// Compare background params
	if p1.Background != p2.Background {
		bgDiff := make(map[string]any)
		if p1.Background.BackgroundUpdateFraction != p2.Background.BackgroundUpdateFraction {
			bgDiff["background_update_fraction"] = map[string]any{
				"run1": p1.Background.BackgroundUpdateFraction,
				"run2": p2.Background.BackgroundUpdateFraction,
			}
		}
		if p1.Background.ClosenessSensitivityMultiplier != p2.Background.ClosenessSensitivityMultiplier {
			bgDiff["closeness_sensitivity_multiplier"] = map[string]any{
				"run1": p1.Background.ClosenessSensitivityMultiplier,
				"run2": p2.Background.ClosenessSensitivityMultiplier,
			}
		}
		if p1.Background.SafetyMarginMeters != p2.Background.SafetyMarginMeters {
			bgDiff["safety_margin_meters"] = map[string]any{
				"run1": p1.Background.SafetyMarginMeters,
				"run2": p2.Background.SafetyMarginMeters,
			}
		}
		if p1.Background.NeighborConfirmationCount != p2.Background.NeighborConfirmationCount {
			bgDiff["neighbor_confirmation_count"] = map[string]any{
				"run1": p1.Background.NeighborConfirmationCount,
				"run2": p2.Background.NeighborConfirmationCount,
			}
		}
		if p1.Background.NoiseRelativeFraction != p2.Background.NoiseRelativeFraction {
			bgDiff["noise_relative_fraction"] = map[string]any{
				"run1": p1.Background.NoiseRelativeFraction,
				"run2": p2.Background.NoiseRelativeFraction,
			}
		}
		if p1.Background.SeedFromFirstObservation != p2.Background.SeedFromFirstObservation {
			bgDiff["seed_from_first_observation"] = map[string]any{
				"run1": p1.Background.SeedFromFirstObservation,
				"run2": p2.Background.SeedFromFirstObservation,
			}
		}
		if len(bgDiff) > 0 {
			diff["background"] = bgDiff
		}
	}

	// Compare clustering params
	if p1.Clustering != p2.Clustering {
		clDiff := make(map[string]any)
		if p1.Clustering.Eps != p2.Clustering.Eps {
			clDiff["eps"] = map[string]any{
				"run1": p1.Clustering.Eps,
				"run2": p2.Clustering.Eps,
			}
		}
		if p1.Clustering.MinPts != p2.Clustering.MinPts {
			clDiff["min_pts"] = map[string]any{
				"run1": p1.Clustering.MinPts,
				"run2": p2.Clustering.MinPts,
			}
		}
		if len(clDiff) > 0 {
			diff["clustering"] = clDiff
		}
	}

	// Compare tracking params
	if p1.Tracking != p2.Tracking {
		trDiff := make(map[string]any)
		if p1.Tracking.MaxTracks != p2.Tracking.MaxTracks {
			trDiff["max_tracks"] = map[string]any{
				"run1": p1.Tracking.MaxTracks,
				"run2": p2.Tracking.MaxTracks,
			}
		}
		if p1.Tracking.MaxMisses != p2.Tracking.MaxMisses {
			trDiff["max_misses"] = map[string]any{
				"run1": p1.Tracking.MaxMisses,
				"run2": p2.Tracking.MaxMisses,
			}
		}
		if p1.Tracking.HitsToConfirm != p2.Tracking.HitsToConfirm {
			trDiff["hits_to_confirm"] = map[string]any{
				"run1": p1.Tracking.HitsToConfirm,
				"run2": p2.Tracking.HitsToConfirm,
			}
		}
		if p1.Tracking.GatingDistanceSquared != p2.Tracking.GatingDistanceSquared {
			trDiff["gating_distance_squared"] = map[string]any{
				"run1": p1.Tracking.GatingDistanceSquared,
				"run2": p2.Tracking.GatingDistanceSquared,
			}
		}
		if len(trDiff) > 0 {
			diff["tracking"] = trDiff
		}
	}

	return diff
}

// computeTemporalIoU calculates temporal IoU for two tracks.
// Delegates to l8analytics.ComputeTemporalIoU for the core algorithm.
func computeTemporalIoU(ref, cand *RunTrack) float64 {
	return l8analytics.ComputeTemporalIoU(ref.StartUnixNanos, ref.EndUnixNanos, cand.StartUnixNanos, cand.EndUnixNanos)
}

// CompareRuns compares two analysis runs by matching their tracks using temporal IoU
// and spatial proximity. It populates RunComparison with matched tracks, split candidates,
// merge candidates, and tracks unique to each run.
func CompareRuns(store *AnalysisRunStore, run1ID, run2ID string) (*RunComparison, error) {
	// Load tracks for both runs
	run1Tracks, err := store.GetRunTracks(run1ID)
	if err != nil {
		return nil, fmt.Errorf("load run1 tracks: %w", err)
	}

	run2Tracks, err := store.GetRunTracks(run2ID)
	if err != nil {
		return nil, fmt.Errorf("load run2 tracks: %w", err)
	}

	comparison := &RunComparison{
		Run1ID: run1ID,
		Run2ID: run2ID,
	}

	// If either run is empty, return early with empty results
	if len(run1Tracks) == 0 || len(run2Tracks) == 0 {
		for _, t := range run1Tracks {
			comparison.TracksOnlyRun1 = append(comparison.TracksOnlyRun1, t.TrackID)
		}
		for _, t := range run2Tracks {
			comparison.TracksOnlyRun2 = append(comparison.TracksOnlyRun2, t.TrackID)
		}
		return comparison, nil
	}

	// Build cost matrix using temporal IoU
	// IoU > 0.3 means potential match (from design doc)
	const iouThreshold = 0.3
	const forbiddenCost = 1e18

	costMatrix := make([][]float32, len(run1Tracks))
	iouMatrix := make([][]float64, len(run1Tracks))

	for i, t1 := range run1Tracks {
		costMatrix[i] = make([]float32, len(run2Tracks))
		iouMatrix[i] = make([]float64, len(run2Tracks))

		for j, t2 := range run2Tracks {
			iou := computeTemporalIoU(t1, t2)
			iouMatrix[i][j] = iou

			if iou > iouThreshold {
				// Valid match: cost = 1.0 - IoU (lower cost is better)
				costMatrix[i][j] = float32(1.0 - iou)
			} else {
				// Forbidden match
				costMatrix[i][j] = forbiddenCost
			}
		}
	}

	// Use Hungarian algorithm for optimal bipartite matching
	assignments := HungarianAssign(costMatrix)

	// Build sets for matched tracks
	run1Matched := make(map[string]bool)
	run2Matched := make(map[string]bool)

	// Track how many run2 tracks are matched to each run1 track (for split detection)
	run1ToRun2 := make(map[string][]string)
	// Track how many run1 tracks are matched to each run2 track (for merge detection)
	run2ToRun1 := make(map[string][]string)

	// Process assignments
	for i, j := range assignments {
		if j >= 0 && j < len(run2Tracks) {
			// Check if this is a valid match (not forbidden)
			if costMatrix[i][j] < forbiddenCost {
				t1 := run1Tracks[i]
				t2 := run2Tracks[j]

				// Record the match
				run1Matched[t1.TrackID] = true
				run2Matched[t2.TrackID] = true

				run1ToRun2[t1.TrackID] = append(run1ToRun2[t1.TrackID], t2.TrackID)
				run2ToRun1[t2.TrackID] = append(run2ToRun1[t2.TrackID], t1.TrackID)

				// Add to matched tracks list
				overlapPct := float32(iouMatrix[i][j] * 100.0)
				comparison.MatchedTracks = append(comparison.MatchedTracks, TrackMatch{
					Track1ID:   t1.TrackID,
					Track2ID:   t2.TrackID,
					OverlapPct: overlapPct,
				})
			}
		}
	}

	// Detect splits: one run1 track matched to multiple run2 tracks
	// NOTE: With the current Hungarian 1:1 matching algorithm, this will never trigger
	// because each run1 track can only be matched to at most one run2 track.
	// Future enhancement: Use a different matching strategy (e.g., IoU threshold without
	// uniqueness constraint) to detect when one reference track overlaps with multiple candidates.
	for t1ID, t2IDs := range run1ToRun2 {
		if len(t2IDs) > 1 {
			split := TrackSplit{
				OriginalTrack: t1ID,
				SplitTracks:   t2IDs,
				Confidence:    0.8, // High confidence for multiple matches
			}

			// Position fields (SplitX/SplitY) remain at zero value — position data
			// is not available from RunTrack; load observations for accurate location.

			comparison.SplitCandidates = append(comparison.SplitCandidates, split)
		}
	}

	// Detect merges: multiple run1 tracks matched to one run2 track
	// NOTE: With the current Hungarian 1:1 matching algorithm, this will never trigger
	// because each run2 track can only be matched to at most one run1 track.
	// Future enhancement: Use a different matching strategy to detect when multiple
	// reference tracks overlap with the same candidate track.
	for t2ID, t1IDs := range run2ToRun1 {
		if len(t1IDs) > 1 {
			merge := TrackMerge{
				MergedTrack:  t2ID,
				SourceTracks: t1IDs,
				Confidence:   0.8, // High confidence for multiple matches
			}

			// Position fields (MergeX/MergeY) remain at zero value — position data
			// is not available from RunTrack; load observations for accurate location.

			comparison.MergeCandidates = append(comparison.MergeCandidates, merge)
		}
	}

	// Collect tracks only in run1
	for _, t := range run1Tracks {
		if !run1Matched[t.TrackID] {
			comparison.TracksOnlyRun1 = append(comparison.TracksOnlyRun1, t.TrackID)
		}
	}

	// Collect tracks only in run2
	for _, t := range run2Tracks {
		if !run2Matched[t.TrackID] {
			comparison.TracksOnlyRun2 = append(comparison.TracksOnlyRun2, t.TrackID)
		}
	}

	// Compare parameters if both runs have param data
	run1, err := store.GetRun(run1ID)
	if err == nil && len(run1.ParamsJSON) > 0 {
		run2, err := store.GetRun(run2ID)
		if err == nil && len(run2.ParamsJSON) > 0 {
			params1, err1 := ParseRunParams(run1.ParamsJSON)
			params2, err2 := ParseRunParams(run2.ParamsJSON)

			if err1 == nil && err2 == nil {
				comparison.ParamDiff = compareParams(params1, params2)
			}
		}
	}

	return comparison, nil
}
