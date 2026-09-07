package l5tracks

// Extent beliefs: what the tracker may conclude about an object's size from a
// sequence of partial views.
//
// A LiDAR return is censored evidence. Occlusion, range falloff and grazing
// incidence all remove points; nothing adds them. An observed span on a body
// axis therefore says the object is AT LEAST that long, and says nothing about
// how much longer it might be. Two consequences follow, and the first version
// of the axis test fell foul of both:
//
//   - A running mean of observed spans shrinks the object. Most views are
//     partial, and every partial view drags the mean down, so the reference
//     ends up describing a smaller object than the one being tracked. Measured
//     on s2_sf_4, 92% of the axis test's abstentions were observations that
//     matched neither interpretation of such a reference.
//   - A raw maximum ratchets upward on contamination, because one merged
//     cluster looks exactly like one unusually good view.
//
// What is used instead is a corroborated maximum: the largest span that
// several accepted observations have reached, held in a fixed-bin histogram
// and clamped to a road-user range. This is a heuristic and is labelled as
// one: the estimate is not a derived Bayesian posterior, and the spread is a
// dispersion indicator, not a calibrated interval. It is revisable in both
// directions and carries its own support count, so a caller can tell a belief
// built from eighty views from one built from two.
const (
	// extentBeliefBinMetres is the histogram resolution. A quarter of a metre
	// separates a car from a van without pretending to distinguish two cars.
	extentBeliefBinMetres = 0.25
	// extentBeliefBins spans [0, 16) metres, which covers pedestrians through
	// articulated vehicles. A span beyond it is reported as a conflict rather
	// than silently clamped into the top bin.
	extentBeliefBins = 64
	// extentBeliefMaxMetres is the largest representable span.
	extentBeliefMaxMetres = extentBeliefBins * extentBeliefBinMetres
	// extentBeliefMinMetres floors the estimate. Below it a return is noise
	// rather than a body dimension.
	extentBeliefMinMetres = 0.20
	// extentBeliefCorroboration is how many observations must reach a span
	// before it is believed.
	//
	// Under pure censoring the maximum observed span is the right estimate:
	// every observation is a lower bound, so the largest is the most
	// informative. The maximum's weakness is that one merged cluster looks
	// exactly like one unusually good view. Requiring corroboration keeps the
	// maximum's ability to recover the truth from a handful of good views
	// among many partial ones — which a quantile cannot do, because a quantile
	// tracks how OFTEN a span is seen rather than how large it is — while
	// ignoring transient outliers.
	extentBeliefCorroboration = 3
)

// extentBelief is a revisable lower-bound belief about one body dimension.
// The zero value is a belief with no support, which Estimate reports as zero.
type extentBelief struct {
	hist [extentBeliefBins]uint32
	// Support counts the observations admitted as evidence. It is not the
	// number of frames the track has lived: only confidently assigned
	// observations may revise a dimension.
	Support int
	// Conflicts counts spans too large to be a road user. They are refused
	// rather than clamped, so a merged cluster shows up as a disagreement
	// between model and evidence instead of quietly inflating the belief.
	Conflicts int
}

// Observe admits one span as lower-bound evidence.
func (b *extentBelief) Observe(span float32) {
	if span <= 0 {
		return
	}
	if span >= extentBeliefMaxMetres {
		b.Conflicts++
		return
	}
	b.hist[int(span/extentBeliefBinMetres)]++
	b.Support++
}

// Reseed discards the accumulated evidence and starts again from one span.
// It exists for the case where the belief itself has become the obstacle and
// no further observation can revise it.
func (b *extentBelief) Reseed(span float32) {
	b.hist = [extentBeliefBins]uint32{}
	b.Support = 0
	b.Observe(span)
}

// Estimate is the believed dimension in metres, or zero with no support.
//
// Known limitation: a single good view among many partial ones does not move
// the belief, because one good view cannot be told from one merged cluster.
// Recovering it would require membership evidence, which is D2.2's job. A
// merge that persists for the corroboration window will likewise inflate the
// belief; the MergeCandidate guard at the call site is what narrows that.
func (b *extentBelief) Estimate() float32 {
	if b.Support == 0 {
		return 0
	}
	need := extentBeliefCorroboration
	if b.Support < need {
		need = b.Support
	}
	seen := 0
	for i := extentBeliefBins - 1; i >= 0; i-- {
		seen += int(b.hist[i])
		if seen < need {
			continue
		}
		if centre := (float32(i) + 0.5) * extentBeliefBinMetres; centre > extentBeliefMinMetres {
			return centre
		}
		return extentBeliefMinMetres
	}
	return 0
}

// Spread is the gap between the median and the 95th percentile of observed
// spans: wide when views disagree, narrow when they agree. It indicates
// dispersion of the evidence and is not a calibrated standard deviation.
func (b *extentBelief) Spread() float32 {
	if b.Support == 0 {
		return 0
	}
	return b.quantile(0.95) - b.quantile(0.50)
}

// quantile returns the centre of the bin holding the given quantile.
func (b *extentBelief) quantile(q float64) float32 {
	if b.Support == 0 {
		return 0
	}
	target := q * float64(b.Support)
	cumulative := 0.0
	for i, count := range b.hist {
		cumulative += float64(count)
		if cumulative >= target {
			centre := (float32(i) + 0.5) * extentBeliefBinMetres
			if centre < extentBeliefMinMetres {
				return extentBeliefMinMetres
			}
			return centre
		}
	}
	return extentBeliefMaxMetres - extentBeliefBinMetres/2
}
