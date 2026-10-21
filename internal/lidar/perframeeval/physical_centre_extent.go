package perframeeval

import (
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Centre against extent (W9, D-28's reading of Schäfer et al.).
//
// A box anchored on the end the sensor sees, but too short, puts its centre
// half the shortfall toward that end. This splits each scored pose's
// along-axis centre error into that part, which the length error explains,
// and the remainder, which it does not, with the across-axis error beside
// them; per vehicle, per reference label and pooled.

// CentreExtentSeenEndRule is the rule that names the seen end. It is the
// same for every arm, since it reads only the reference and the sensor: a
// published box's ends are all temporally inferred, and an arm's own claim
// about which end it saw would let each arm choose its own decomposition.
const CentreExtentSeenEndRule = "reference_end_nearer_sensor: the reference bumper (centre ± half the reference length " +
	"along the reference heading) nearer the sensor, which sits at the pack frame's origin; that is the front when " +
	"(sensor − reference centre)·u > 0 for u the reference heading's unit vector, the rear when < 0, and the front on a tie"

// CentreExtentShareDefinition is the one share reported.
const CentreExtentShareDefinition = "explained_share = Σ|length_explained_m| / Σ|along_m| over the group's decomposed poses; " +
	"null when Σ|along_m| is zero. It exceeds 1 where half the length error predicts more along-axis error than there is, " +
	"as when a short box is not on the seen end"

// CentreExtentPose is one scored pose's decomposition. Along and across are
// in the reference's own axis frame: u = (cos ψ, sin ψ) for ψ the reference
// heading, along = (prediction − reference centre)·u, positive toward the
// reference front, and across = the same difference on u turned a quarter
// anticlockwise, positive to the reference left. Under an ambiguous
// reference axis ψ is the stored heading, one of two, and "front" is only
// its +u end: the signs of along, the explained part and the remainder flip
// together, and their magnitudes and the share do not.
//
// LengthErrorM is predicted minus reference length. SeenEnd is +1 for the
// front, −1 for the rear (CentreExtentSeenEndRule). LengthExplainedM is
// −SeenEnd·LengthErrorM/2: a body short by ΔL whose seen end is right has
// its centre ΔL/2 toward the seen end, so that is the whole along error.
// RemainderM is AlongM − LengthExplainedM.
type CentreExtentPose struct {
	EpisodeID    string `json:"episode_id"`
	ObjectID     string `json:"object_id"`
	Label        string `json:"label"`
	SampleID     int    `json:"sample_id"`
	TrackKey     string `json:"track_key"`
	AxisResolved bool   `json:"axis_resolved"`
	// SeenEnd is "front" or "rear"; SeenEndSign is +1 or −1 along u.
	SeenEnd     string `json:"seen_end"`
	SeenEndSign int    `json:"seen_end_sign"`
	// SeenEndReferenceEvidence is the reference keyframe's own evidence for
	// the end the rule names: observed, inferred, prior_only or unknown.
	SeenEndReferenceEvidence string  `json:"seen_end_reference_evidence"`
	AlongM                   float64 `json:"along_m"`
	AcrossM                  float64 `json:"across_m"`
	ReferenceLengthM         float64 `json:"reference_length_m"`
	PredictedLengthM         float64 `json:"predicted_length_m"`
	LengthErrorM             float64 `json:"length_error_m"`
	LengthExplainedM         float64 `json:"length_explained_m"`
	RemainderM               float64 `json:"remainder_m"`
}

// CentreExtentSummary pools one group's decomposed poses: one vehicle, one
// reference label or all. Means and medians are of the signed values unless
// named abs.
type CentreExtentSummary struct {
	// ObjectID is empty for a pool; Label is the reference class, or "all".
	ObjectID string `json:"object_id,omitempty"`
	Label    string `json:"label"`
	// CentreScored counts the group's poses whose centre was scored;
	// Poses those of them decomposed. Excluded says why the rest were not.
	CentreScored  int            `json:"centre_scored"`
	Poses         int            `json:"poses"`
	Excluded      map[string]int `json:"excluded,omitempty"`
	AxisAmbiguous int            `json:"axis_ambiguous_poses"`
	SeenFront     int            `json:"seen_front"`
	SeenRear      int            `json:"seen_rear"`
	// SeenEndObserved counts poses whose rule-named seen end the reference
	// keyframe marks observed.
	SeenEndObserved int `json:"seen_end_observed_in_reference"`

	AlongMeanM        float64  `json:"along_mean_m"`
	AlongMedianM      float64  `json:"along_median_m"`
	AlongMeanAbsM     float64  `json:"along_mean_abs_m"`
	LengthErrorMeanM  float64  `json:"length_error_mean_m"`
	ExplainedMeanM    float64  `json:"length_explained_mean_m"`
	ExplainedMedianM  float64  `json:"length_explained_median_m"`
	ExplainedMeanAbsM float64  `json:"length_explained_mean_abs_m"`
	RemainderMeanM    float64  `json:"remainder_mean_m"`
	RemainderMedianM  float64  `json:"remainder_median_m"`
	RemainderMeanAbsM float64  `json:"remainder_mean_abs_m"`
	AcrossMeanAbsM    float64  `json:"across_mean_abs_m"`
	ExplainedShare    *float64 `json:"explained_share"`
}

// CentreExtentDecomposition is an arm's centre-against-extent section.
type CentreExtentDecomposition struct {
	SeenEndRule     string `json:"seen_end_rule"`
	ShareDefinition string `json:"share_definition"`
	// Poses are the decomposed poses in instant order.
	Poses []CentreExtentPose `json:"poses"`
	// Vehicles is one summary per object, by object ID; ByLabel one per
	// reference label; Pooled every decomposed pose.
	Vehicles []CentreExtentSummary `json:"vehicles"`
	ByLabel  []CentreExtentSummary `json:"by_label"`
	Pooled   CentreExtentSummary   `json:"pooled"`
	Caveats  []string              `json:"caveats,omitempty"`
}

// referenceEnds is a keyframe's evidence for its front and rear.
type referenceEnds struct {
	front, rear annotation.EvidenceStatus
}

// decomposeCentreExtent decomposes every pose whose centre and length were
// both scored against a reference with a heading. labels names each
// object's reference class; ends is each keyframe's end evidence, by object
// and sample; sensorAtOrigin is false when the reference frame is not the
// sensor's, so the origin is not where the sensor is.
func decomposeCentreExtent(instants []PhysicalInstant, labels map[string]string,
	ends map[string]map[int]referenceEnds, sensorAtOrigin bool) *CentreExtentDecomposition {
	out := &CentreExtentDecomposition{SeenEndRule: CentreExtentSeenEndRule, ShareDefinition: CentreExtentShareDefinition,
		Poses: []CentreExtentPose{}}
	type group struct {
		summary CentreExtentSummary
		poses   []CentreExtentPose
	}
	vehicles, byLabel := map[string]*group{}, map[string]*group{}
	pooled := &group{summary: CentreExtentSummary{Label: "all"}}
	groupsOf := func(obj, label string) []*group {
		if vehicles[obj] == nil {
			vehicles[obj] = &group{summary: CentreExtentSummary{ObjectID: obj, Label: label}}
		}
		if byLabel[label] == nil {
			byLabel[label] = &group{summary: CentreExtentSummary{Label: label}}
		}
		return []*group{vehicles[obj], byLabel[label], pooled}
	}
	for _, in := range instants {
		if in.Outcomes[ComponentCentre].Category != OutcomeScored {
			continue
		}
		label := labels[in.ObjectID]
		if label == "" {
			label = "unlabelled"
		}
		groups := groupsOf(in.ObjectID, label)
		pose, excluded := decomposePose(in, label, ends[in.ObjectID][in.SampleID])
		for _, g := range groups {
			g.summary.CentreScored++
			if excluded != "" {
				if g.summary.Excluded == nil {
					g.summary.Excluded = map[string]int{}
				}
				g.summary.Excluded[excluded]++
				continue
			}
			g.poses = append(g.poses, pose)
		}
		if excluded == "" {
			out.Poses = append(out.Poses, pose)
		}
	}
	for _, key := range sortedMapKeys(vehicles) {
		out.Vehicles = append(out.Vehicles, summariseCentreExtent(vehicles[key].summary, vehicles[key].poses))
	}
	for _, key := range sortedMapKeys(byLabel) {
		out.ByLabel = append(out.ByLabel, summariseCentreExtent(byLabel[key].summary, byLabel[key].poses))
	}
	out.Pooled = summariseCentreExtent(pooled.summary, pooled.poses)
	if !sensorAtOrigin {
		out.Caveats = append(out.Caveats, "The reference frame carries a sensor-to-site transform, so the sensor is not at "+
			"the frame's origin and the seen end the rule names may not be the end facing the sensor.")
	}
	return out
}

// decomposePose splits one centre-scored pose, or says why it cannot.
func decomposePose(in PhysicalInstant, label string, ends referenceEnds) (CentreExtentPose, string) {
	switch {
	case in.Outcomes[ComponentLength].Category != OutcomeScored:
		o := in.Outcomes[ComponentLength]
		return CentreExtentPose{}, fmt.Sprintf("length:%s/%s", o.Category, o.Reason)
	case in.Reference == nil || in.Reference.Yaw == nil || in.Reference.Centre == nil:
		return CentreExtentPose{}, "reference_heading_unavailable"
	case in.Comparison.Centre == nil || in.Comparison.Length == nil || in.Prediction == nil:
		return CentreExtentPose{}, "comparison_incomplete"
	}
	g, c, l := in.Reference, in.Comparison.Centre, in.Comparison.Length
	ux, uy := math.Cos(g.Yaw.Rad), math.Sin(g.Yaw.Rad)
	p := CentreExtentPose{
		EpisodeID: in.EpisodeID, ObjectID: in.ObjectID, Label: label, SampleID: in.SampleID,
		TrackKey: in.Prediction.TrackKey, AxisResolved: g.Yaw.Axis == annotation.AxisResolved,
		AlongM: c.DXM*ux + c.DYM*uy, AcrossM: -c.DXM*uy + c.DYM*ux,
		ReferenceLengthM: l.ReferenceValueM, PredictedLengthM: l.PredictedM, LengthErrorM: l.ErrorM,
	}
	// The sensor is at the origin: (0 − centre)·u.
	p.SeenEnd, p.SeenEndSign, p.SeenEndReferenceEvidence = "front", 1, string(ends.front)
	if -(g.Centre.XM*ux + g.Centre.YM*uy) < 0 {
		p.SeenEnd, p.SeenEndSign, p.SeenEndReferenceEvidence = "rear", -1, string(ends.rear)
	}
	if p.SeenEndReferenceEvidence == "" {
		p.SeenEndReferenceEvidence = string(annotation.EvidenceUnknown)
	}
	p.LengthExplainedM = -float64(p.SeenEndSign) * p.LengthErrorM / 2
	p.RemainderM = p.AlongM - p.LengthExplainedM
	return p, ""
}

func summariseCentreExtent(s CentreExtentSummary, poses []CentreExtentPose) CentreExtentSummary {
	s.Poses = len(poses)
	if s.Poses == 0 {
		return s
	}
	var along, explained, remainder []float64
	var sumAbsAlong, sumAbsExplained, sumAbsRemainder, sumAbsAcross, sumLength float64
	for _, p := range poses {
		along, explained, remainder = append(along, p.AlongM), append(explained, p.LengthExplainedM), append(remainder, p.RemainderM)
		sumAbsAlong += math.Abs(p.AlongM)
		sumAbsExplained += math.Abs(p.LengthExplainedM)
		sumAbsRemainder += math.Abs(p.RemainderM)
		sumAbsAcross += math.Abs(p.AcrossM)
		sumLength += p.LengthErrorM
		if !p.AxisResolved {
			s.AxisAmbiguous++
		}
		if p.SeenEndSign > 0 {
			s.SeenFront++
		} else {
			s.SeenRear++
		}
		if p.SeenEndReferenceEvidence == string(annotation.EvidenceObserved) {
			s.SeenEndObserved++
		}
	}
	n := float64(s.Poses)
	s.AlongMeanM, s.AlongMedianM, s.AlongMeanAbsM = meanOf(along), medianOf(along), sumAbsAlong/n
	s.ExplainedMeanM, s.ExplainedMedianM, s.ExplainedMeanAbsM = meanOf(explained), medianOf(explained), sumAbsExplained/n
	s.RemainderMeanM, s.RemainderMedianM, s.RemainderMeanAbsM = meanOf(remainder), medianOf(remainder), sumAbsRemainder/n
	s.AcrossMeanAbsM, s.LengthErrorMeanM = sumAbsAcross/n, sumLength/n
	if sumAbsAlong > 0 {
		share := sumAbsExplained / sumAbsAlong
		s.ExplainedShare = &share
	}
	return s
}

func meanOf(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func medianOf(v []float64) float64 {
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	return quantileSorted(sorted, 0.5)
}
