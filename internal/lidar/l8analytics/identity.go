package l8analytics

import (
	"fmt"
	"math"
	"sort"
)

// Identity metrics (Ristani et al., 2016): IDF1, IDP and IDR.
//
// Why a third family beside CLEAR MOT and HOTA: an identity switch is counted
// when the matching changes hands, so a tracker that swaps two identities
// once and swaps them back scores two switches whether the swap lasted a
// frame or a minute. The identity measures instead ask how much of each
// reference trajectory is covered by the one hypothesis globally assigned to
// it, so a long wrong identity costs more than a brief one. The D2 A/B
// reported IDF1 and identity recall for exactly that reason: a heading or
// reassociation change that shortens wrong identities without changing how
// often they happen is invisible to IDSW.
//
// The matching is a single bipartite assignment over whole trajectories, not a
// per-frame one. Pairing reference i with hypothesis j gains one identity true
// positive for every frame in which both are present and within i's gate; the
// assignment maximises the total, which is the same as minimising Ristani's
// IDFN + IDFP. Every reference or hypothesis detection not covered by its
// assigned partner is an identity false negative or false positive.
//
// Ignore follows the CLEAR MOT pass: ignored reference points are not
// detections, and a hypothesis detection absorbed by one there is removed
// from the hypothesis total. The three families therefore score one
// population.

// IdentityMetrics is the identity family from one global trajectory
// assignment.
type IdentityMetrics struct {
	IDF1 float64 `json:"idf1"`
	// IDP is identity precision, IDTP / (IDTP + IDFP).
	IDP float64 `json:"idp"`
	// IDR is identity recall, IDTP / (IDTP + IDFN): the D2 A/B's "identity
	// recall".
	IDR  float64 `json:"idr"`
	IDTP int     `json:"idtp"`
	IDFP int     `json:"idfp"`
	IDFN int     `json:"idfn"`
}

// ComputeIdentityMetrics scores hypothesis tracks against reference tracks
// under the given gate.
func ComputeIdentityMetrics(reference, hypothesis []TrackSeries, gate MatchGate) IdentityMetrics {
	refIdx, hypIdx := indexSeries(reference), indexSeries(hypothesis)
	return identityFrom(refIdx, hypIdx, gate, matchFrames(refIdx, hypIdx, gate))
}

func identityFrom(refIdx, hypIdx frameIndex, gate MatchGate, matchings []frameMatching) IdentityMetrics {
	overlap := map[pairKey]int{}
	var refTotal, hypTotal int
	for _, m := range matchings {
		refFrame, hypFrame := refIdx[m.timestampNanos], hypIdx[m.timestampNanos]
		for hypID := range hypFrame {
			if _, gone := m.absorbed[hypID]; !gone {
				hypTotal++
			}
		}
		for _, refID := range m.presentRefs {
			refTotal++
			r := refFrame[refID]
			g := gate.forReference(r)
			for hypID, h := range hypFrame {
				if _, gone := m.absorbed[hypID]; gone {
					continue
				}
				if euclid(r.pos, h.pos) <= g {
					overlap[pairKey{refID, hypID}]++
				}
			}
		}
	}

	idtp := maxOverlapAssignment(overlap)
	res := IdentityMetrics{IDTP: idtp, IDFN: refTotal - idtp, IDFP: hypTotal - idtp}
	if d := res.IDTP + res.IDFP; d > 0 {
		res.IDP = float64(res.IDTP) / float64(d)
	}
	if d := res.IDTP + res.IDFN; d > 0 {
		res.IDR = float64(res.IDTP) / float64(d)
	}
	if d := 2*res.IDTP + res.IDFP + res.IDFN; d > 0 {
		res.IDF1 = 2 * float64(res.IDTP) / float64(d)
	}
	return res
}

// maxOverlapAssignment returns the largest total overlap any one-to-one
// pairing of references with hypotheses achieves.
//
// Only trajectories with some overlap take part, since the rest cannot add to
// the total. Every cell is allowed and costs maxOverlap - overlap, so a
// complete assignment of the smaller side minimises cost exactly when it
// maximises the sum; a zero-overlap pairing simply gains nothing. Forbidding
// zero-overlap cells instead would be wrong: the solver would then prefer more
// pairs over a larger total.
func maxOverlapAssignment(overlap map[pairKey]int) int {
	if len(overlap) == 0 {
		return 0
	}
	refSet, hypSet := map[string]struct{}{}, map[string]struct{}{}
	maxOverlap := 0
	for key, n := range overlap {
		refSet[key.ref] = struct{}{}
		hypSet[key.hyp] = struct{}{}
		if n > maxOverlap {
			maxOverlap = n
		}
	}
	refs, hyps := sortedKeys(refSet), sortedKeys(hypSet)
	cost := make([][]float64, len(refs))
	allowed := make([][]bool, len(refs))
	for i, refID := range refs {
		cost[i] = make([]float64, len(hyps))
		allowed[i] = make([]bool, len(hyps))
		for j, hypID := range hyps {
			cost[i][j] = float64(maxOverlap - overlap[pairKey{refID, hypID}])
			allowed[i][j] = true
		}
	}
	total := 0
	for i, j := range assignMinCost(cost, allowed) {
		if j >= 0 {
			total += overlap[pairKey{refs[i], hyps[j]}]
		}
	}
	return total
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PerFrameResult is every metric family for one reference/hypothesis pair.
type PerFrameResult struct {
	CLEARMOT TrackMetrics    `json:"clear_mot"`
	HOTA     HOTAResult      `json:"hota"`
	Identity IdentityMetrics `json:"identity"`
}

// EvaluatePerFrame computes CLEAR MOT, HOTA and the identity family under one
// gate. CLEAR MOT and the identity family share one matching pass; HOTA keeps
// its own matching, for the reason given in hota.go.
func EvaluatePerFrame(reference, hypothesis []TrackSeries, gate MatchGate, alphas []float64) PerFrameResult {
	refIdx, hypIdx := indexSeries(reference), indexSeries(hypothesis)
	matchings := matchFrames(refIdx, hypIdx, gate)
	return PerFrameResult{
		CLEARMOT: tallyTrackMetrics(matchings),
		HOTA:     ComputeHOTAGated(reference, hypothesis, gate, alphas),
		Identity: identityFrom(refIdx, hypIdx, gate, matchings),
	}
}

// Combining results across episodes.
//
// Each episode is its own sequence: its own frames, its own objects. Pooled
// numbers are therefore sums of counts with the ratios recomputed, never
// averages of ratios, so an episode with ten times the references carries ten
// times the weight. HOTA follows TrackEval's combine_sequences: per alpha, the
// true, false and missed detections are summed and AssA is averaged weighted
// by each sequence's true positives.

// CombineTrackMetrics pools CLEAR MOT results from separate sequences.
func CombineTrackMetrics(parts []TrackMetrics) TrackMetrics {
	var out TrackMetrics
	var distanceSum float64
	for _, p := range parts {
		out.NumFrames += p.NumFrames
		out.NumGT += p.NumGT
		out.FN += p.FN
		out.FP += p.FP
		out.IDSwitches += p.IDSwitches
		out.Matches += p.Matches
		out.Fragmentations += p.Fragmentations
		out.IgnoredHypotheses += p.IgnoredHypotheses
		distanceSum += p.MOTP * float64(p.Matches)
	}
	if out.Matches > 0 {
		out.MOTP = distanceSum / float64(out.Matches)
	}
	if out.NumGT > 0 {
		out.MOTA = 1.0 - float64(out.FN+out.FP+out.IDSwitches)/float64(out.NumGT)
	}
	return out
}

// CombineIdentity pools identity results from separate sequences.
func CombineIdentity(parts []IdentityMetrics) IdentityMetrics {
	var out IdentityMetrics
	for _, p := range parts {
		out.IDTP += p.IDTP
		out.IDFP += p.IDFP
		out.IDFN += p.IDFN
	}
	if d := out.IDTP + out.IDFP; d > 0 {
		out.IDP = float64(out.IDTP) / float64(d)
	}
	if d := out.IDTP + out.IDFN; d > 0 {
		out.IDR = float64(out.IDTP) / float64(d)
	}
	if d := 2*out.IDTP + out.IDFP + out.IDFN; d > 0 {
		out.IDF1 = 2 * float64(out.IDTP) / float64(d)
	}
	return out
}

// CombineHOTA pools HOTA results from separate sequences. Every part must
// have been swept over the same alphas.
func CombineHOTA(parts []HOTAResult) (HOTAResult, error) {
	if len(parts) == 0 {
		return HOTAResult{}, nil
	}
	alphas := parts[0].Alphas
	for i, p := range parts {
		if len(p.Alphas) != len(alphas) || len(p.PerAlpha) != len(alphas) {
			return HOTAResult{}, fmt.Errorf("HOTA part %d swept %d alphas, part 0 swept %d", i, len(p.Alphas), len(alphas))
		}
		for k := range alphas {
			if p.Alphas[k] != alphas[k] {
				return HOTAResult{}, fmt.Errorf("HOTA part %d alpha %d is %v, part 0 has %v", i, k, p.Alphas[k], alphas[k])
			}
		}
	}

	out := HOTAResult{Alphas: append([]float64(nil), alphas...)}
	var sumHOTA, sumDetA, sumAssA float64
	for k, alpha := range alphas {
		var tp, fn, fp int
		var assWeighted float64
		for _, p := range parts {
			row := p.PerAlpha[k]
			tp += row.TP
			fn += row.FN
			fp += row.FP
			assWeighted += row.AssA * float64(row.TP)
		}
		detA, assA := 0.0, 0.0
		if d := tp + fn + fp; d > 0 {
			detA = float64(tp) / float64(d)
		}
		if tp > 0 {
			assA = assWeighted / float64(tp)
		}
		hota := math.Sqrt(detA * assA)
		out.PerAlpha = append(out.PerAlpha, HOTAAlphaResult{
			Alpha: alpha, HOTA: hota, DetA: detA, AssA: assA, TP: tp, FN: fn, FP: fp,
		})
		sumHOTA += hota
		sumDetA += detA
		sumAssA += assA
	}
	if n := float64(len(alphas)); n > 0 {
		out.HOTA, out.DetA, out.AssA = sumHOTA/n, sumDetA/n, sumAssA/n
	}
	return out, nil
}
