package l8analytics

import (
	"math"
	"sort"
)

// HOTA (Luiten et al., 2021), gap-analysis row M2.
//
// Why this exists beside CLEAR MOT rather than replacing it: MOTA counts
// detection and association errors in one sum, where a false negative and an
// identity switch cost the same, and it is dominated by detection because
// there are far more detections than identity decisions. HOTA separates them,
// so "recall improved but identities got worse" is readable as two numbers
// instead of one that moved slightly.
//
// The matching is HOTA's own, not MOT16's, and the difference is deliberate.
// CLEAR MOT prefers the previous frame's correspondence, which makes its
// identity-switch count depend on the order frames are visited. HOTA instead
// weights each candidate pairing by how often that reference and that
// hypothesis coincide across the *whole* sequence, so the matching is a
// property of the sequence rather than of a walk through it. Conflating the
// two matchings would make both numbers wrong, so each keeps its own.
//
// Similarity for point data is the paper's: one minus the distance normalised
// by the association gate, clamped at zero, so a pair exactly on the gate
// scores 0 and a perfect overlap scores 1.

// HOTAResult is the score and its decomposition.
type HOTAResult struct {
	// HOTA, DetA and AssA are averaged over the alpha thresholds.
	HOTA float64 `json:"hota"`
	DetA float64 `json:"det_a"`
	AssA float64 `json:"ass_a"`
	// Alphas are the localisation thresholds swept, and PerAlpha the score at
	// each, so a single-threshold reading is still available.
	Alphas   []float64         `json:"alphas"`
	PerAlpha []HOTAAlphaResult `json:"per_alpha"`
}

// HOTAAlphaResult is one localisation threshold's outcome.
type HOTAAlphaResult struct {
	Alpha float64 `json:"alpha"`
	HOTA  float64 `json:"hota"`
	DetA  float64 `json:"det_a"`
	AssA  float64 `json:"ass_a"`
	TP    int     `json:"tp"`
	FN    int     `json:"fn"`
	FP    int     `json:"fp"`
}

// DefaultHOTAAlphas is the benchmark's sweep: 0.05 to 0.95 in steps of 0.05.
func DefaultHOTAAlphas() []float64 {
	out := make([]float64, 0, 19)
	for i := 1; i <= 19; i++ {
		out = append(out, float64(i)*0.05)
	}
	return out
}

// pairKey identifies one (reference, hypothesis) pairing across the sequence.
type pairKey struct{ ref, hyp string }

// ComputeHOTA scores hypothesis tracks against reference tracks. maxDistMetres
// is the distance at which similarity reaches zero — the paper's one metre for
// point data, but it belongs to the sensor and the scene, so it is a parameter.
// Reference points marked Ignore are excluded entirely, as are hypotheses that
// match only them.
func ComputeHOTA(reference, hypothesis []TrackSeries, maxDistMetres float64, alphas []float64) HOTAResult {
	return ComputeHOTAGated(reference, hypothesis, FixedGate(maxDistMetres), alphas)
}

// ComputeHOTAGated is ComputeHOTA under an explicit gate rule. Similarity
// reaches zero at the reference point's own gate, so under GateFootprint a
// large object's similarity falls off more slowly than a small one's, exactly
// as its matching tolerance does.
//
// Ignore absorption here is decided per localisation threshold, inside each
// alpha's own assignment, not once before the sweep as TrackEval's
// preprocessing does. A hypothesis close to an ignored point is absorbed at
// every alpha its similarity clears; at a stricter alpha it is scored like any
// other unmatched hypothesis.
func ComputeHOTAGated(reference, hypothesis []TrackSeries, gate MatchGate, alphas []float64) HOTAResult {
	if len(alphas) == 0 {
		alphas = DefaultHOTAAlphas()
	}
	refIdx, hypIdx := indexSeries(reference), indexSeries(hypothesis)
	frames := collectFrames(refIdx, hypIdx)

	// Pre-pass: how often each pairing is plausible, and how often each side
	// appears. This is what makes the matching order-independent.
	coincidence := map[pairKey]float64{}
	refCount, hypCount := map[string]float64{}, map[string]float64{}
	for _, ts := range frames {
		refFrame, hypFrame := refIdx[ts], hypIdx[ts]
		for refID, r := range refFrame {
			if r.ignore {
				continue
			}
			refCount[refID]++
			g := gate.forReference(r)
			for hypID, h := range hypFrame {
				if similarity(r.pos, h.pos, g) > 0 {
					coincidence[pairKey{refID, hypID}]++
				}
			}
		}
		for hypID := range hypFrame {
			hypCount[hypID]++
		}
	}
	// Jaccard over the whole sequence: how much of the time these two are the
	// same object, rather than merely close once.
	alignment := make(map[pairKey]float64, len(coincidence))
	for key, both := range coincidence {
		union := refCount[key.ref] + hypCount[key.hyp] - both
		if union > 0 {
			alignment[key] = both / union
		}
	}

	result := HOTAResult{Alphas: append([]float64(nil), alphas...)}
	var sumHOTA, sumDetA, sumAssA float64

	for _, alpha := range alphas {
		tpPairs := map[pairKey]float64{}
		var tp, fn, fp int

		for _, ts := range frames {
			refFrame, hypFrame := refIdx[ts], hypIdx[ts]
			refIDs := sortedIDs(refFrame, nil)
			hypIDs := sortedIDs(hypFrame, nil)

			// Ignore-class reference points take part in matching so a
			// hypothesis can be absorbed by one, but are never counted.
			scored := 0
			for _, refID := range refIDs {
				if !refFrame[refID].ignore {
					scored++
				}
			}

			matchedRef, matchedHyp := map[string]bool{}, map[string]bool{}
			if len(refIDs) > 0 && len(hypIDs) > 0 {
				// Maximise alignment-weighted similarity. The solver
				// minimises, so the cost is the negated score shifted positive;
				// pairs below the threshold are forbidden outright.
				cost := make([][]float64, len(refIDs))
				allowed := make([][]bool, len(refIDs))
				for i, refID := range refIDs {
					cost[i] = make([]float64, len(hypIDs))
					allowed[i] = make([]bool, len(hypIDs))
					r := refFrame[refID]
					g := gate.forReference(r)
					for j, hypID := range hypIDs {
						s := similarity(r.pos, hypFrame[hypID].pos, g)
						if s < alpha {
							continue
						}
						weight := alignment[pairKey{refID, hypID}]
						cost[i][j] = 2.0 - s*(1.0+weight)
						allowed[i][j] = true
					}
				}
				for i, j := range assignMinCost(cost, allowed) {
					if j < 0 {
						continue
					}
					refID, hypID := refIDs[i], hypIDs[j]
					matchedRef[refID], matchedHyp[hypID] = true, true
					if refFrame[refID].ignore {
						continue // absorbed, counted for neither side
					}
					tp++
					tpPairs[pairKey{refID, hypID}]++
				}
			}

			for _, refID := range refIDs {
				if !refFrame[refID].ignore && !matchedRef[refID] {
					fn++
				}
			}
			for _, hypID := range hypIDs {
				if !matchedHyp[hypID] {
					fp++
				}
			}
		}

		detA := 0.0
		if denom := tp + fn + fp; denom > 0 {
			detA = float64(tp) / float64(denom)
		}

		// Association: for every true positive, how much of that pairing's own
		// history it shares. Summed over true positives, so a pairing that
		// holds for a long time counts for more than one that holds once.
		assA := 0.0
		if tp > 0 {
			var sum float64
			keys := make([]pairKey, 0, len(tpPairs))
			for key := range tpPairs {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool {
				if keys[i].ref != keys[j].ref {
					return keys[i].ref < keys[j].ref
				}
				return keys[i].hyp < keys[j].hyp
			})
			for _, key := range keys {
				tpa := tpPairs[key]
				// Every true positive of this reference that went elsewhere,
				// and every true positive of this hypothesis that belonged to
				// another reference.
				var refTotal, hypTotal float64
				for other, count := range tpPairs {
					if other.ref == key.ref {
						refTotal += count
					}
					if other.hyp == key.hyp {
						hypTotal += count
					}
				}
				fna, fpa := refTotal-tpa, hypTotal-tpa
				if denom := tpa + fna + fpa; denom > 0 {
					sum += tpa * (tpa / denom)
				}
			}
			assA = sum / float64(tp)
		}

		hota := math.Sqrt(detA * assA)
		result.PerAlpha = append(result.PerAlpha, HOTAAlphaResult{
			Alpha: alpha, HOTA: hota, DetA: detA, AssA: assA, TP: tp, FN: fn, FP: fp,
		})
		sumHOTA += hota
		sumDetA += detA
		sumAssA += assA
	}

	if n := float64(len(alphas)); n > 0 {
		result.HOTA, result.DetA, result.AssA = sumHOTA/n, sumDetA/n, sumAssA/n
	}
	return result
}

// similarity is one minus the normalised distance, clamped to [0, 1].
func similarity(a, b point2, maxDistMetres float64) float64 {
	if maxDistMetres <= 0 {
		return 0
	}
	s := 1.0 - euclid(a, b)/maxDistMetres
	if s < 0 {
		return 0
	}
	return s
}
