package l5tracks

import "math"

// The fragment birth gate (TrackerConfig.FragmentBirthGate).
//
// The fragment guard (isFragmentFor) refuses a metre-scale track a cluster
// smaller than MinAssociableExtentMetres, so that a scrap cannot become the
// track's dimensions. The refused scrap is left unassociated and, unless
// another track takes it, seeds a track of its own. Under the shipped d² cost
// another track often did: a duplicate on the same body, whose own extent is
// under the guard's two-metre belief. The robust likelihood cost removes most
// of those duplicates, and on kirk0 the scraps they used to take seed brief
// confirmed tracks inside the body's box (experiment l5id0021). The gate
// absorbs such a scrap instead: it is part of a body already tracked, so it
// neither seeds a track nor updates one.
//
// The test is narrow on purpose. It needs the guard's own condition (a
// confirmed track that has seen three observations and believes itself two
// metres or more, a cluster under the guard's extent), and the cluster inside
// that track's drawn box, grown by overlapEndBodyMargin and the track's
// position standard deviation. A scrap beside the body, or a small road user
// stepping out from behind it, is outside the box and is born as before.

// fragmentBirthMaxSigmaMetres bounds the position standard deviation that
// grows a box for the test. A coasting track's covariance is capped only at
// MaxCovarianceDiag, whose square root is metres beyond any body, so without
// a bound a long coast would absorb everything small around its prediction.
// One metre is the order of a single coasted frame's growth at urban speeds;
// it is a stated bound, not a fitted value.
const fragmentBirthMaxSigmaMetres = 1.0

// absorbsFragment reports whether FragmentBirthGate absorbs the unassociated
// cluster: whether some confirmed track's guard refuses it and its grown box
// holds it. The tracks are those after this frame's updates and coasts.
func (t *Tracker) absorbsFragment(cluster *WorldCluster) bool {
	for _, track := range t.Tracks {
		if track.TrackState != TrackConfirmed || !t.isFragmentFor(track, *cluster) {
			continue
		}
		r, ok := drawnBoxOf(track)
		if !ok {
			continue
		}
		sigma := math.Sqrt(math.Max(float64(track.P[0]), float64(track.P[5])))
		if math.IsNaN(sigma) || sigma > fragmentBirthMaxSigmaMetres {
			sigma = fragmentBirthMaxSigmaMetres
		}
		grow := 2 * (overlapEndBodyMargin + sigma)
		r.length += grow
		r.wd += grow
		if rectContains(r, float64(cluster.CentroidX), float64(cluster.CentroidY)) {
			return true
		}
	}
	return false
}
