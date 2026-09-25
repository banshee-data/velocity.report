package l8analytics

import (
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

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
}

// matchFrames applies the MOT16 rule frame by frame. maxDistMetres is the
// association gate; a pair further apart than that is never matched.
func matchFrames(reference, hypothesis []TrackSeries, maxDistMetres float64) []frameMatching {
	refIdx, hypIdx := indexSeries(reference), indexSeries(hypothesis)
	frames := collectFrames(refIdx, hypIdx)

	out := make([]frameMatching, 0, len(frames))
	previous := map[string]string{}

	for _, ts := range frames {
		refFrame, hypFrame := refIdx[ts], hypIdx[ts]
		current := map[string]string{}
		distances := map[string]float64{}
		usedRef, usedHyp := map[string]struct{}{}, map[string]struct{}{}

		// Step 1: continuity. previous is one-to-one, so iteration order
		// cannot change the outcome.
		for refID, hypID := range previous {
			r, okR := refFrame[refID]
			h, okH := hypFrame[hypID]
			if !okR || !okH {
				continue
			}
			if d := euclid(r.pos, h.pos); d <= maxDistMetres {
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
			cost := make([][]float32, len(remRef))
			for i, refID := range remRef {
				cost[i] = make([]float32, len(remHyp))
				for j, hypID := range remHyp {
					d := euclid(refFrame[refID].pos, hypFrame[hypID].pos)
					if d <= maxDistMetres {
						cost[i][j] = float32(d)
					} else {
						cost[i][j] = clearMOTForbidden
					}
				}
			}
			for i, j := range l5tracks.HungarianAssign(cost) {
				if j < 0 || j >= len(remHyp) || cost[i][j] >= clearMOTForbidden {
					continue // unassigned, or the solver filled a forbidden cell
				}
				current[remRef[i]] = remHyp[j]
				distances[remRef[i]] = float64(cost[i][j])
			}
		}

		// The ignore rule, applied after matching as MOT16 specifies: a
		// hypothesis that matched an uncertifiable reference point is removed
		// from the tally entirely rather than counted either way.
		ignored := 0
		referenceCount, hypothesisIn := 0, len(hypFrame)
		presentRefs := make([]string, 0, len(refFrame))
		for _, refID := range sortedIDs(refFrame, nil) {
			entry := refFrame[refID]
			if !entry.ignore {
				referenceCount++
				presentRefs = append(presentRefs, refID)
				continue
			}
			if _, matched := current[refID]; matched {
				ignored++
				delete(current, refID)
				delete(distances, refID)
			}
		}

		out = append(out, frameMatching{
			timestampNanos: ts, matches: current, distances: distances,
			presentRefs:    presentRefs,
			referenceCount: referenceCount, hypothesisIn: hypothesisIn - ignored,
			ignoredHyp: ignored,
		})
		previous = current
	}
	return out
}

// ComputeTrackMetrics evaluates hypothesis tracks against reference tracks.
func ComputeTrackMetrics(reference, hypothesis []TrackSeries, maxDistMetres float64) TrackMetrics {
	matchings := matchFrames(reference, hypothesis, maxDistMetres)

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
