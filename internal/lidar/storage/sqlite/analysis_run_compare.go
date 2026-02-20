package sqlite

import "github.com/banshee-data/velocity.report/internal/lidar/l6objects"

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
// Delegates to l6objects.ComputeTemporalIoU for the core algorithm.
func computeTemporalIoU(ref, cand *RunTrack) float64 {
	return l6objects.ComputeTemporalIoU(ref.StartUnixNanos, ref.EndUnixNanos, cand.StartUnixNanos, cand.EndUnixNanos)
}
