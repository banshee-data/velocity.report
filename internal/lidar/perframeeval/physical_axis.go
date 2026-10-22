package perframeeval

import (
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/stats"
)

// Axis error, criterion G1 of the geometry convergence criteria
// (docs/lidar/operations/geometry-convergence-criteria.md).
//
// The axis is the body's orientation modulo 90°, which no length label
// touches, so a right axis with the wrong length label is a 0° axis error
// (and a 90° directed error, which G2 scores). Each scored pose's yaw error is
// folded modulo 90° to [0°, 45°], the reference's own yaw bound is taken off
// and the result floored at zero, since an error inside the bound is not
// resolvable. Only established predictions moving at AxisMinSpeedMPS or more
// are pooled. The median and p90 are nearest rank (stats.NearestRank), the
// owner's choice in the maths decision review (E-11): nearest rank never reads
// lower than a truncated or interpolated p90, so the estimator cannot loosen
// the level.

// AxisMinSpeedMPS is G1's speed floor: below it a course says little about
// the body, and the population is the moving vehicles.
const AxisMinSpeedMPS = 2.0

// AxisErrorRule states the computation, recorded with every result.
const AxisErrorRule = "per scored pose whose prediction is established and moving at min_speed_mps or more: " +
	"|yaw error| folded modulo 90° to [0°, 45°], less the reference's yaw bound, floored at zero; " +
	"median and p90 by nearest rank, the value at 1-based rank ceil(p·n)"

// Reasons a yaw-scored pose is left out of the axis pool.
const (
	AxisExcludedNotEstablished = "prediction_not_established"
	AxisExcludedBelowSpeed     = "prediction_below_min_speed"
)

// AxisErrorPose is one pooled pose.
type AxisErrorPose struct {
	EpisodeID string  `json:"episode_id"`
	ObjectID  string  `json:"object_id"`
	SampleID  int     `json:"sample_id"`
	SpeedMPS  float64 `json:"speed_mps"`
	FoldedDeg float64 `json:"folded_deg"`
	BoundDeg  float64 `json:"bound_deg"`
	// ErrorDeg is FoldedDeg less BoundDeg, floored at zero: what G1 reads.
	ErrorDeg float64 `json:"error_deg"`
}

// AxisErrorStats summarises pooled poses' ErrorDeg. With no poses every
// figure is zero and Poses says so.
type AxisErrorStats struct {
	Poses     int     `json:"poses"`
	MedianDeg float64 `json:"median_deg"`
	P90Deg    float64 `json:"p90_deg"`
	MaxDeg    float64 `json:"max_deg"`
}

// AxisErrorVehicle is one object's poses, and every other object's beside
// them: G1 sets a vehicle apart (truck 2 on kirk0), and nearest-rank
// percentiles do not combine across groups, so the level is read from the
// set-apart vehicle's Others.
type AxisErrorVehicle struct {
	ObjectID string `json:"object_id"`
	AxisErrorStats
	Others AxisErrorStats `json:"others"`
}

// AxisErrorSummary is an arm's G1 reading.
type AxisErrorSummary struct {
	Rule        string  `json:"rule"`
	MinSpeedMPS float64 `json:"min_speed_mps"`
	// Excluded counts yaw-scored poses left out of the pool, by reason.
	Excluded map[string]int     `json:"excluded"`
	Poses    []AxisErrorPose    `json:"poses"`
	Vehicles []AxisErrorVehicle `json:"vehicles"`
	Pooled   AxisErrorStats     `json:"pooled"`
}

// foldAxisDeg folds a yaw error to the axis error in [0°, 45°].
func foldAxisDeg(errRad float64) float64 {
	return math.Abs(math.Remainder(errRad, math.Pi/2)) * 180 / math.Pi
}

// summariseAxis pools the scored poses' axis errors in instant order.
func summariseAxis(instants []PhysicalInstant) *AxisErrorSummary {
	out := &AxisErrorSummary{Rule: AxisErrorRule, MinSpeedMPS: AxisMinSpeedMPS,
		Excluded: map[string]int{}, Poses: []AxisErrorPose{}, Vehicles: []AxisErrorVehicle{}}
	for _, in := range instants {
		y := in.Comparison.Yaw
		if y == nil || in.Prediction == nil || in.Outcomes[ComponentYaw].Category != OutcomeScored {
			continue
		}
		s := in.Prediction.sample
		if s.Estimation != l8behaviour.EstimationEstablished {
			out.Excluded[AxisExcludedNotEstablished]++
			continue
		}
		speed := math.Hypot(s.VX, s.VY)
		if speed < AxisMinSpeedMPS {
			out.Excluded[AxisExcludedBelowSpeed]++
			continue
		}
		folded, bound := foldAxisDeg(y.ErrorRad), y.ReferenceBoundRad*180/math.Pi
		out.Poses = append(out.Poses, AxisErrorPose{EpisodeID: in.EpisodeID, ObjectID: in.ObjectID, SampleID: in.SampleID,
			SpeedMPS: speed, FoldedDeg: folded, BoundDeg: bound, ErrorDeg: math.Max(folded-bound, 0)})
	}
	errs := func(keep func(AxisErrorPose) bool) []float64 {
		var v []float64
		for _, p := range out.Poses {
			if keep(p) {
				v = append(v, p.ErrorDeg)
			}
		}
		return v
	}
	out.Pooled = axisStats(errs(func(AxisErrorPose) bool { return true }))
	seen := map[string]bool{}
	var ids []string
	for _, p := range out.Poses {
		if !seen[p.ObjectID] {
			seen[p.ObjectID] = true
			ids = append(ids, p.ObjectID)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		out.Vehicles = append(out.Vehicles, AxisErrorVehicle{ObjectID: id,
			AxisErrorStats: axisStats(errs(func(p AxisErrorPose) bool { return p.ObjectID == id })),
			Others:         axisStats(errs(func(p AxisErrorPose) bool { return p.ObjectID != id }))})
	}
	return out
}

// axisStats is the nearest-rank median and p90, and the maximum, of v.
func axisStats(v []float64) AxisErrorStats {
	if len(v) == 0 {
		return AxisErrorStats{}
	}
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	return AxisErrorStats{Poses: len(sorted), MedianDeg: stats.NearestRank(sorted, 0.5),
		P90Deg: stats.NearestRank(sorted, 0.9), MaxDeg: sorted[len(sorted)-1]}
}
