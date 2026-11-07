package perframeeval

import (
	"fmt"
	"math"
	"sort"
)

// Centre error by range and aspect (geom0015's measurement-bias baseline).
//
// A position computed from the returns a sensor sees sits toward the faces
// it sees, by an amount that depends on how far away the body is and how it
// is turned. This places every reference pose with a compared centre by its
// range and aspect from the sensor, and pools the centre's distance and its
// offset toward the sensor by range band and aspect bin, separately for each
// point the predictions name (a body centre, a cluster mean or medoid, a
// visible box centre). The distance of a point that is not on the body is
// where that point sits relative to the body centre, not a body-centre error;
// grouping by prediction reference keeps the two apart.

// CentreViewRule defines the pose's view and its offset toward the sensor.
const CentreViewRule = "range_m = |reference centre − sensor|, with the sensor at the pack frame's origin; aspect_deg = the angle " +
	"between the line of sight from the sensor to the reference centre and the reference heading's axis, folded to [0, 90]: 0 is end on, " +
	"90 broadside; toward_sensor_m = (prediction − reference centre)·(sensor − reference centre)/range_m, positive where the prediction " +
	"sits nearer the sensor than the reference centre; along and across are the absolute offsets on the reference axis and across it"

// CentreViewRangeBandEdgesM and CentreViewAspectBinEdgesDeg are the upper
// edges of the bands and bins: under 10 m, 10 to 20 m and 20 m or more; 0 to
// 30°, 30 to 60° and 60 to 90°.
var (
	CentreViewRangeBandEdgesM   = []float64{10, 20}
	CentreViewAspectBinEdgesDeg = []float64{30, 60}
)

// CentreViewPose is one reference pose with a compared centre.
type CentreViewPose struct {
	EpisodeID           string  `json:"episode_id"`
	ObjectID            string  `json:"object_id"`
	Label               string  `json:"label"`
	SampleID            int     `json:"sample_id"`
	TrackKey            string  `json:"track_key"`
	PredictionReference string  `json:"prediction_reference"`
	RangeM              float64 `json:"range_m"`
	// AspectDeg is null where the reference states no heading; the pose's
	// aspect bin is then aspect_unknown and it has no along or across.
	AspectDeg       *float64 `json:"aspect_deg"`
	RangeBand       string   `json:"range_band"`
	AspectBin       string   `json:"aspect_bin"`
	ErrorM          float64  `json:"error_m"`
	TowardSensorM   float64  `json:"toward_sensor_m"`
	AlongAbsM       *float64 `json:"along_abs_m,omitempty"`
	AcrossAbsM      *float64 `json:"across_abs_m,omitempty"`
	ReferenceBoundM float64  `json:"reference_bound_m"`
}

// CentreViewCell pools the poses of one prediction reference in one group:
// group "all", "object" (key the object ID), "range_band", "aspect_bin" or
// "range_aspect" (key the band and bin joined by a slash). Medians and means
// are of the signed toward-sensor offset and of the unsigned distances.
type CentreViewCell struct {
	PredictionReference  string  `json:"prediction_reference"`
	Group                string  `json:"group"`
	Key                  string  `json:"key"`
	Poses                int     `json:"poses"`
	ErrorMedianM         float64 `json:"error_median_m"`
	ErrorMeanM           float64 `json:"error_mean_m"`
	TowardSensorMedianM  float64 `json:"toward_sensor_median_m"`
	TowardSensorMeanM    float64 `json:"toward_sensor_mean_m"`
	AlongAbsMedianM      float64 `json:"along_abs_median_m"`
	AcrossAbsMedianM     float64 `json:"across_abs_median_m"`
	WithinReferenceBound int     `json:"within_reference_bound"`
}

// CentreViewSummary is an arm's centre error by range and aspect.
type CentreViewSummary struct {
	Rule              string    `json:"rule"`
	RangeBandEdgesM   []float64 `json:"range_band_edges_m"`
	AspectBinEdgesDeg []float64 `json:"aspect_bin_edges_deg"`
	// ReferencePoses counts the reviewed poses whose reference states a
	// centre; Unmatched those of them no prediction was compared with, by
	// object. Unreviewed counts the poses left out because their reference
	// is not reviewed: they are neither reference poses nor unmatched.
	ReferencePoses    int              `json:"reference_poses"`
	Unreviewed        int              `json:"unreviewed_poses"`
	Unmatched         int              `json:"unmatched_poses"`
	UnmatchedByObject map[string]int   `json:"unmatched_by_object"`
	Poses             []CentreViewPose `json:"poses"`
	Cells             []CentreViewCell `json:"cells"`
	Caveats           []string         `json:"caveats,omitempty"`
}

// summariseCentreView places every compared centre by its view. Without the
// sensor at the pack frame's origin there is no line of sight to measure,
// and the summary says so instead.
func summariseCentreView(instants []PhysicalInstant, labels map[string]string, sensorAtOrigin bool) *CentreViewSummary {
	out := &CentreViewSummary{Rule: CentreViewRule, RangeBandEdgesM: CentreViewRangeBandEdgesM,
		AspectBinEdgesDeg: CentreViewAspectBinEdgesDeg, UnmatchedByObject: map[string]int{},
		Poses: []CentreViewPose{}, Cells: []CentreViewCell{}}
	if !sensorAtOrigin {
		out.Caveats = append(out.Caveats, "The references carry a site transform, so the sensor is not at the pack frame's "+
			"origin and no pose's range or aspect is measured.")
		return out
	}
	for _, in := range instants {
		g := in.Reference
		if g == nil || g.Centre == nil {
			continue
		}
		if in.Outcomes[ComponentCentre].Reason == ReasonReferenceUnreviewed {
			out.Unreviewed++
			continue
		}
		out.ReferencePoses++
		c := in.Comparison.Centre
		if c == nil || in.Prediction == nil {
			out.Unmatched++
			out.UnmatchedByObject[in.ObjectID]++
			continue
		}
		label := labels[in.ObjectID]
		if label == "" {
			label = "unlabelled"
		}
		rx, ry := g.Centre.XM, g.Centre.YM
		r := math.Hypot(rx, ry)
		pose := CentreViewPose{
			EpisodeID: in.EpisodeID, ObjectID: in.ObjectID, Label: label, SampleID: in.SampleID,
			TrackKey: in.Prediction.TrackKey, PredictionReference: c.PredictionReference,
			RangeM: r, RangeBand: rangeBand(r, CentreViewRangeBandEdgesM), AspectBin: "aspect_unknown",
			ErrorM: c.ErrorM, AlongAbsM: c.AlongAxisAbsM, AcrossAbsM: c.AcrossAxisAbsM, ReferenceBoundM: c.ReferenceBoundM,
		}
		if r > 0 {
			// The unit vector toward the sensor is −(rx, ry)/r.
			pose.TowardSensorM = -(c.DXM*rx + c.DYM*ry) / r
		}
		if g.Yaw != nil && r > 0 {
			a := aspectDeg(math.Atan2(ry, rx), g.Yaw.Rad)
			pose.AspectDeg = &a
			pose.AspectBin = aspectBin(a, CentreViewAspectBinEdgesDeg)
		}
		out.Poses = append(out.Poses, pose)
	}
	out.Cells = centreViewCells(out.Poses)
	return out
}

// aspectDeg folds the angle between a line of sight and an undirected body
// axis to [0, 90] degrees.
func aspectDeg(bearingRad, yawRad float64) float64 {
	d := math.Abs(math.Remainder(bearingRad-yawRad, math.Pi)) * 180 / math.Pi
	return math.Min(d, 180-d)
}

// aspectBin names the bin an aspect falls in: aspect_0_30deg, ...,
// aspect_60_90deg.
func aspectBin(a float64, edges []float64) string {
	lower := 0.0
	for _, e := range edges {
		if a < e {
			return fmt.Sprintf("aspect_%g_%gdeg", lower, e)
		}
		lower = e
	}
	return fmt.Sprintf("aspect_%g_90deg", lower)
}

// centreViewCells pools the poses per prediction reference and group, in a
// stable order: prediction reference, then group, then key.
func centreViewCells(poses []CentreViewPose) []CentreViewCell {
	type key struct{ ref, group, key string }
	pools := map[key][]CentreViewPose{}
	for _, p := range poses {
		for _, k := range []key{
			{p.PredictionReference, "all", "all"},
			{p.PredictionReference, "object", p.ObjectID},
			{p.PredictionReference, "range_band", p.RangeBand},
			{p.PredictionReference, "aspect_bin", p.AspectBin},
			{p.PredictionReference, "range_aspect", p.RangeBand + "/" + p.AspectBin},
		} {
			pools[k] = append(pools[k], p)
		}
	}
	order := map[string]int{"all": 0, "object": 1, "range_band": 2, "aspect_bin": 3, "range_aspect": 4}
	keys := make([]key, 0, len(pools))
	for k := range pools {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.ref != b.ref {
			return a.ref < b.ref
		}
		if a.group != b.group {
			return order[a.group] < order[b.group]
		}
		return a.key < b.key
	})
	cells := make([]CentreViewCell, 0, len(keys))
	for _, k := range keys {
		ps := pools[k]
		var errs, toward, along, across []float64
		within := 0
		for _, p := range ps {
			errs, toward = append(errs, p.ErrorM), append(toward, p.TowardSensorM)
			if p.AlongAbsM != nil {
				along = append(along, *p.AlongAbsM)
			}
			if p.AcrossAbsM != nil {
				across = append(across, *p.AcrossAbsM)
			}
			if p.ErrorM <= p.ReferenceBoundM {
				within++
			}
		}
		c := CentreViewCell{PredictionReference: k.ref, Group: k.group, Key: k.key, Poses: len(ps),
			ErrorMedianM: medianOf(errs), ErrorMeanM: meanOf(errs),
			TowardSensorMedianM: medianOf(toward), TowardSensorMeanM: meanOf(toward), WithinReferenceBound: within}
		if len(along) > 0 {
			c.AlongAbsMedianM = medianOf(along)
		}
		if len(across) > 0 {
			c.AcrossAbsMedianM = medianOf(across)
		}
		cells = append(cells, c)
	}
	return cells
}
