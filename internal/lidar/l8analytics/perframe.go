package l8analytics

import "sort"

// Per-frame matching and the MOT16 metric family, over the neutral TrackSeries
// shape (see truth.go).
//
// The matching rule is MOT16's, the same one clearmot.go implements: prefer the
// previous frame's correspondence when it still holds, then solve an optimal
// assignment over what is left. Preferring continuity is what stops an
// identity switch being reported every time two objects are equidistant.
//
// This adds two things the cherry-picked version does not have: MOT16's
// fragmentation count, and the ignore rule. ComputeCLEARMOT delegates here, and
// its imported test asserts the two agree.
//
// Ignored points are transparent to identity. MOT16's devkit removes ignored
// annotations before the CLEAR MOT pass, which drops the previous frame's
// correspondence for an object that was ignored in it: the next scored frame
// then re-solves from scratch, and a nearer stray hypothesis can take the
// object from the one that tracked it through the uncertifiable frames. That
// is an identity switch the tracker did not earn, caused by the annotation.
// Here a correspondence is carried across a frame in which its reference is
// ignored, unless its hypothesis was claimed by a scored match in that frame,
// so an ignored stretch scores exactly as the same stretch scored and tracked
// would. The carried correspondence is not applied while the point is
// ignored: an ignored point never keeps a hypothesis by continuity, so it can
// never take one from a scored object beside it. An absent reference still drops its correspondence, as MOT16 does:
// absence is a fact about the object, ignore a fact about the label. A real
// change of hypothesis across an ignored stretch is still a switch, by MOT16's
// last-known-assignment rule; that is the reassociation failure this evaluator
// exists to see. ignore_rules_test.go pins each of these.

// TrackMetrics is the MOT16 family computed from one per-frame matching.
type TrackMetrics struct {
	MOTA       float64 `json:"mota"`
	MOTP       float64 `json:"motp_metres"`
	NumFrames  int     `json:"num_frames"`
	NumGT      int     `json:"num_gt"`
	FN         int     `json:"fn"`
	FP         int     `json:"fp"`
	IDSwitches int     `json:"id_switches"`
	Matches    int     `json:"matches"`
	// Fragmentations is MOT16's FM: the number of times a reference
	// trajectory goes from tracked to untracked and is picked up again. It is
	// counted per reference track and summed. This is the number that closes
	// the evaluator's hard-coded Fragmentation = 0, and the one that says
	// whether extra candidate tracks are a recall gain or one object cut up.
	Fragmentations int `json:"fragmentations"`
	// IgnoredHypotheses is the count of hypothesis detections dropped because
	// they matched an ignore-class reference point. MOT16 neither penalises
	// nor rewards these.
	IgnoredHypotheses int `json:"ignored_hypotheses"`
}

// frameMatching is one frame's outcome, retained so several metrics can be
// derived from a single pass.
type frameMatching struct {
	timestampNanos int64
	matches        map[string]string // reference ID -> hypothesis ID
	distances      map[string]float64
	// presentRefs are the non-ignored references in this frame, sorted. Only
	// these can change tracked status, which is what keeps fragmentation a
	// count of interruptions rather than of absences.
	presentRefs    []string
	referenceCount int
	hypothesisIn   int
	ignoredHyp     int
	// absorbed are the hypotheses removed because they matched an ignored
	// reference point. The identity metrics exclude exactly these, so every
	// family scores the same hypothesis population.
	absorbed map[string]struct{}
}

// matchFrames applies the MOT16 rule frame by frame. The gate is evaluated per
// reference point; a pair further apart than it is never matched.
func matchFrames(refIdx, hypIdx frameIndex, gate MatchGate) []frameMatching {
	frames := collectFrames(refIdx, hypIdx)

	out := make([]frameMatching, 0, len(frames))
	previous := map[string]string{}

	for _, ts := range frames {
		refFrame, hypFrame := refIdx[ts], hypIdx[ts]
		current := map[string]string{}
		distances := map[string]float64{}
		usedRef, usedHyp := map[string]struct{}{}, map[string]struct{}{}

		// Step 1: continuity. previous is one-to-one, so iteration order
		// cannot change the outcome. An ignored point takes no part: it
		// competes for a hypothesis only in the optimal assignment below, by
		// distance, as MOT16's preprocessing matches distractors. Keeping its
		// hypothesis by continuity would take it from a nearer scored object
		// and make that object a miss, charging the tracker for the label.
		for refID, hypID := range previous {
			r, okR := refFrame[refID]
			h, okH := hypFrame[hypID]
			if !okR || !okH || r.ignore {
				continue
			}
			if d := euclid(r.pos, h.pos); d <= gate.forReference(r) {
				current[refID] = hypID
				distances[refID] = d
				usedRef[refID] = struct{}{}
				usedHyp[hypID] = struct{}{}
			}
		}

		// Step 2: optimal assignment over the remainder, in sorted order so
		// the cost matrix is identical on every run.
		remRef := sortedIDs(refFrame, usedRef)
		remHyp := sortedIDs(hypFrame, usedHyp)
		if len(remRef) > 0 && len(remHyp) > 0 {
			// Distances are rounded to float32, the precision the solver
			// works in, so MOTP is unchanged from the solver-direct version
			// wherever the assignment is.
			cost := make([][]float64, len(remRef))
			allowed := make([][]bool, len(remRef))
			for i, refID := range remRef {
				cost[i] = make([]float64, len(remHyp))
				allowed[i] = make([]bool, len(remHyp))
				r := refFrame[refID]
				g := gate.forReference(r)
				for j, hypID := range remHyp {
					d := euclid(r.pos, hypFrame[hypID].pos)
					cost[i][j] = float64(float32(d))
					allowed[i][j] = d <= g
				}
			}
			for i, j := range assignMinCost(cost, allowed) {
				if j < 0 {
					continue
				}
				current[remRef[i]] = remHyp[j]
				distances[remRef[i]] = cost[i][j]
			}
		}

		// The ignore rule, applied after matching as MOT16 specifies: a
		// hypothesis that matched an uncertifiable reference point is removed
		// from the tally entirely rather than counted either way.
		ignored := 0
		absorbed := map[string]struct{}{}
		referenceCount, hypothesisIn := 0, len(hypFrame)
		presentRefs := make([]string, 0, len(refFrame))
		for _, refID := range sortedIDs(refFrame, nil) {
			entry := refFrame[refID]
			if !entry.ignore {
				referenceCount++
				presentRefs = append(presentRefs, refID)
				continue
			}
			if hypID, matched := current[refID]; matched {
				ignored++
				absorbed[hypID] = struct{}{}
				delete(current, refID)
				delete(distances, refID)
			}
		}

		out = append(out, frameMatching{
			timestampNanos: ts, matches: current, distances: distances,
			presentRefs:    presentRefs,
			referenceCount: referenceCount, hypothesisIn: hypothesisIn - ignored,
			ignoredHyp: ignored, absorbed: absorbed,
		})
		previous = carryThroughIgnored(previous, current, refFrame)
	}
	return out
}

// carryThroughIgnored is the next frame's continuity map: this frame's scored
// matches, plus the previous correspondence of every reference whose point is
// ignored this frame, unless a scored match claimed that hypothesis. The
// result stays one-to-one: previous was, and a carried hypothesis is by
// construction not a value of current.
func carryThroughIgnored(previous, current map[string]string, refFrame map[string]seriesEntry) map[string]string {
	claimed := make(map[string]struct{}, len(current))
	next := make(map[string]string, len(current))
	for refID, hypID := range current {
		claimed[hypID] = struct{}{}
		next[refID] = hypID
	}
	for refID, hypID := range previous {
		entry, present := refFrame[refID]
		if !present || !entry.ignore {
			continue
		}
		if _, taken := claimed[hypID]; taken {
			continue
		}
		next[refID] = hypID
	}
	return next
}

// ComputeTrackMetrics evaluates hypothesis tracks against reference tracks
// with a fixed association gate.
func ComputeTrackMetrics(reference, hypothesis []TrackSeries, maxDistMetres float64) TrackMetrics {
	return ComputeTrackMetricsGated(reference, hypothesis, FixedGate(maxDistMetres))
}

// ComputeTrackMetricsGated evaluates hypothesis tracks against reference
// tracks under an explicit gate rule.
func ComputeTrackMetricsGated(reference, hypothesis []TrackSeries, gate MatchGate) TrackMetrics {
	return tallyTrackMetrics(matchFrames(indexSeries(reference), indexSeries(hypothesis), gate))
}

// tallyTrackMetrics derives the MOT16 family from one matching pass.
func tallyTrackMetrics(matchings []frameMatching) TrackMetrics {
	res := TrackMetrics{NumFrames: len(matchings)}
	var distanceSum float64
	lastHyp := map[string]string{}
	// tracked records whether a reference was matched in the last frame it was
	// *present*, so a gap is counted once rather than per missed frame. A
	// reference that is absent from a frame does not change status: MOT16
	// counts a trajectory going from tracked to untracked, and an object that
	// is not there is neither. Counting absence as a loss would report a
	// fragmentation for every occlusion the reference itself records.
	tracked := map[string]bool{}
	seen := map[string]bool{}

	for _, m := range matchings {
		res.NumGT += m.referenceCount
		res.Matches += len(m.matches)
		res.FN += m.referenceCount - len(m.matches)
		res.FP += m.hypothesisIn - len(m.matches)
		res.IgnoredHypotheses += m.ignoredHyp

		// Identity switches and fragmentations, in sorted order: both write to
		// maps keyed by reference ID, and a stable order keeps the counts
		// reproducible even though neither total depends on it.
		refIDs := make([]string, 0, len(m.matches))
		for refID := range m.matches {
			refIDs = append(refIDs, refID)
		}
		sort.Strings(refIDs)
		for _, refID := range refIDs {
			hypID := m.matches[refID]
			distanceSum += m.distances[refID]
			if last, ok := lastHyp[refID]; ok && last != hypID {
				res.IDSwitches++
			}
			lastHyp[refID] = hypID
			// A reference that was previously tracked, then lost, and is now
			// matched again is one fragmentation.
			if seen[refID] && !tracked[refID] {
				res.Fragmentations++
			}
			tracked[refID] = true
			seen[refID] = true
		}
		// A reference present this frame but unmatched is no longer tracked.
		for _, refID := range m.presentRefs {
			if _, matched := m.matches[refID]; !matched {
				tracked[refID] = false
			}
		}
	}

	if res.Matches > 0 {
		res.MOTP = distanceSum / float64(res.Matches)
	}
	if res.NumGT > 0 {
		res.MOTA = 1.0 - float64(res.FN+res.FP+res.IDSwitches)/float64(res.NumGT)
	}
	return res
}
