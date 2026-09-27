package l5tracks

// Reacquisition and identity: the Sprint 0.5.2.2 options that decide who may
// take a cluster when more than one track could, after gap analysis rows S2
// (tentative against confirmed) and K4 (unlike-class identity). The third
// option of the set, ContestedRejoin, extends the reacquisition guard and
// lives beside it in continuity.go.
//
// Every option here is default-off and a Go-level TrackerConfig field, not a
// tuning key, for the fingerprint reason the other estimator options give.
// Each either forbids a pairing or reorders who is offered a cluster first;
// none makes a pairing cheaper, so none can deepen the coasting-track
// discount of gap S3. With every option off, association is exactly the
// shipped joint assignment.

// tentativePriorityGate is the d² bound inside which a cluster is
// statistically a confirmed track's: the 99% quantile of chi-squared with two
// degrees of freedom. The shipped gate (36) has a tail of 1.5e-8 and admits
// almost everything within reach (gap analysis K10), which is the right width
// for letting a track keep its object and the wrong one for claiming priority
// over another track's. 99% rather than DeepSORT's 95% because K9 measured
// moving-track innovations heavier-tailed than chi-squared.
const tentativePriorityGate = 9.21

// associateWithTentativePriority is TentativePriority's association: two
// stages over one frame. Confirmed tracks are matched first, to clusters
// inside their 99% ellipse only. Every track that stage left unmatched,
// confirmed or tentative, then competes jointly under the shipped gate for
// the clusters it left. Creation-sequence order holds within each stage.
//
// The first stage is what stops a tentative track stealing a confirmed
// track's measurement (S2): a track born a frame ago has a position variance
// of 10 m², so its d² for anything within reach is tiny, and in one joint
// assignment it outbids a confirmed track for a cluster the confirmed track
// predicted. The second stage is what stops the confirmed track stealing in
// return. A confirmed track whose object dropped out this frame meets the
// newborn's cluster only there, on equal terms, and the newborn's own
// prediction wins it; under CascadedAssociation the confirmed track takes it
// first, from anywhere inside the shipped gate.
//
// Inside the 99% ellipse the two readings, "the confirmed track's object came
// back" and "a new object stands where the confirmed track predicted", are
// not separable by position, and the confirmed track is preferred. That is
// the option's premise, and the held-out per-frame run is what tests it.
func (t *Tracker) associateWithTentativePriority(clusters []WorldCluster, allClusters []int, activeTrackIDs []string, dt float32) []string {
	associations := make([]string, len(clusters))
	gate := t.Config.GatingDistanceSquared
	priorityGate := float32(tentativePriorityGate)
	if gate < priorityGate {
		priorityGate = gate
	}

	var confirmed []string
	for _, id := range activeTrackIDs {
		if t.Tracks[id].TrackState == TrackConfirmed {
			confirmed = append(confirmed, id)
		}
	}
	taken := t.assignClusters(clusters, allClusters, confirmed, dt, priorityGate)
	matched := make(map[string]bool, len(taken))
	for ci, id := range taken {
		associations[ci] = id
		matched[id] = true
	}

	var rest []string
	for _, id := range activeTrackIDs {
		if !matched[id] {
			rest = append(rest, id)
		}
	}
	remaining := make([]int, 0, len(clusters)-len(taken))
	for ci := range clusters {
		if _, done := taken[ci]; !done {
			remaining = append(remaining, ci)
		}
	}
	for ci, id := range t.assignClusters(clusters, remaining, rest, dt, gate) {
		associations[ci] = id
	}
	return associations
}

// ClassExtent is the largest horizontal extent L6 accepts for a label: Long
// for the longer of a body's two horizontal extents, Short for the shorter.
type ClassExtent struct {
	Long, Short float32
}

// The envelopes are l6objects' own classification thresholds, restated here
// because l6objects imports this package and not the reverse. A test in
// l6objects pins each value to the threshold it mirrors, so the two cannot
// drift. They are L6's rules, not new ones: a cluster outside a label's
// envelope is one L6 would refuse to call that label.
var classExtents = map[string]ClassExtent{
	// isPedestrian: AvgLength < VehicleLengthMin, AvgWidth < VehicleWidthMin.
	"pedestrian": {Long: 3.0, Short: 1.5},
	// isCyclist: AvgLength < CyclistLengthMax, AvgWidth < CyclistWidthMax.
	"cyclist": {Long: 2.5, Short: 1.2},
	// isMotorcyclist: AvgLength <= MotorcyclistLengthMax, AvgWidth <= MotorcyclistWidthMax.
	"motorcyclist": {Long: 3.0, Short: 1.2},
}

// ClassExtentEnvelope returns the extent envelope ClassIdentity holds a
// labelled track to, and false for a label that has none: every vehicle
// label, since a partial view of a vehicle can be any size below it, and
// every label that is not a road user.
func ClassExtentEnvelope(label string) (ClassExtent, bool) {
	e, ok := classExtents[label]
	return e, ok
}

const (
	// classEnvelopeSlackMetres is added to each envelope before a cluster is
	// refused. L6 judges running averages; one frame's extent carries
	// segmentation noise and occasional ground returns. It is the
	// reacquisition guard's extent slack, reused rather than invented.
	classEnvelopeSlackMetres = reacquisitionExtentSlackMetre
	// classIdentityMinPosterior is the class belief below which no refusal
	// is made: the label must be more probable than not (state-estimation
	// plan Section 5.5, rule 1). L6 clamps every road-user label to at least
	// 0.5, so today this admits every labelled track; it exists so a future
	// classifier's weak labels cannot become hard refusals.
	classIdentityMinPosterior = 0.5
)

// classMismatch reports whether a cluster lies outside the envelope of the
// label the track carries, so that pairing them would carry the identity
// across to a body the label cannot be. A track with no envelope, an
// unconfident label or a cluster reporting no extent is never a mismatch.
func classMismatch(track *TrackedObject, c *WorldCluster) bool {
	env, ok := ClassExtentEnvelope(track.ObjectClass)
	if !ok {
		return false
	}
	if class, strength := trackMotionClass(track); class == MotionUnknown || strength < classIdentityMinPosterior {
		return false
	}
	long, short := c.BoundingBoxLength, c.BoundingBoxWidth
	if short > long {
		long, short = short, long
	}
	if long <= 0 {
		return false
	}
	return long > env.Long+classEnvelopeSlackMetres || short > env.Short+classEnvelopeSlackMetres
}
