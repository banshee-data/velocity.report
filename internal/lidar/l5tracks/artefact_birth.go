package l5tracks

import "math"

// The artefact options (experiment l6cls0005): small clutter tracks refused
// at birth, or their clusters taken by the neighbouring body, instead of
// filtered from the background. The November benchmark found small
// artefacts the largest clutter class at every case it read, and its
// recurrence grid found most of them one-offs that no background filter can
// reach. Both options read only the clusters a track is born from, so
// neither can end a track that has confirmed, which is how the l6cls0002
// quality gates removed stopped cars.
//
// The thresholds are physical anchors, not values fitted to a capture:
//
//   - artefactMaxExtentMetres: D13's class floor, about the width of the
//     smallest car. A cluster this long or wide on either side is a road
//     user's, so a stopped car is never held;
//   - artefactStaticMetres: D13's near-static speed, 0.5 m/s, held over the
//     artefactPersistNanos a held track can wait;
//   - artefactMaxMeanPoints: three times L4's shipped minimum cluster of 8
//     returns. A well-supported cluster is not held whatever its size;
//   - artefactPersistNanos: D13's brief bound. A person standing still is
//     seen on most frames and persists past it, which is what a birth rule
//     can read in place of D13's class test, since a track has no class at
//     birth;
//   - artefactAbsorbMarginMetres: D13's footprint margin (0.25 m) and its
//     1 m, so absorb takes what D13 would call a fragment of the body.
const (
	artefactMaxExtentMetres    = 1.5
	artefactStaticMetres       = 0.5
	artefactMaxMeanPoints      = 24
	artefactPersistNanos       = int64(1e9)
	artefactAbsorbMarginMetres = 1.25
)

// artefactEvidence is what the birth gate reads of a tentative track's
// clusters: where it was first seen, where it was last seen, its largest
// extents and its returns. With the gate off it is never written.
type artefactEvidence struct {
	firstX, firstY float32
	lastX, lastY   float32
	maxLength      float32
	maxWidth       float32
	points         int
	clusters       int
	// held records that the gate has held the track back at least once,
	// so its release is counted once.
	held bool
}

// found records the cluster a track was born from.
func (a *artefactEvidence) found(c *WorldCluster) {
	a.firstX, a.firstY = c.CentroidX, c.CentroidY
	a.observe(*c)
}

// observe records a cluster the track has taken.
func (a *artefactEvidence) observe(c WorldCluster) {
	a.lastX, a.lastY = c.CentroidX, c.CentroidY
	if c.BoundingBoxLength > a.maxLength {
		a.maxLength = c.BoundingBoxLength
	}
	if c.BoundingBoxWidth > a.maxWidth {
		a.maxWidth = c.BoundingBoxWidth
	}
	a.points += c.PointsCount
	a.clusters++
}

// artefactLike reports whether every clause of ArtefactBirthGate holds: the
// track is small, near-static, sparse and has not persisted.
func (a *artefactEvidence) artefactLike(sinceFirstNanos int64) bool {
	if a.clusters == 0 {
		return false
	}
	small := a.maxLength < artefactMaxExtentMetres && a.maxWidth < artefactMaxExtentMetres
	static := math.Hypot(float64(a.lastX-a.firstX), float64(a.lastY-a.firstY)) < artefactStaticMetres
	sparse := a.points < artefactMaxMeanPoints*a.clusters
	brief := sinceFirstNanos < artefactPersistNanos
	return small && static && sparse && brief
}

// holdArtefactBirth applies TrackerConfig.ArtefactBirthGate to a tentative
// track that has earned confirmation: it reports true, and the track stays
// tentative, while the track is artefact-like. Releasing a track the gate
// once held is counted.
func (t *Tracker) holdArtefactBirth(track *TrackedObject, nowNanos int64) bool {
	if track.artefact.artefactLike(nowNanos - track.StartUnixNanos) {
		if !track.artefact.held {
			track.artefact.held = true
			t.continuity.ArtefactBirthsHeld++
			diagf("Track held tentative as an artefact: track_id=%s hits=%d observations=%d length=%.2f width=%.2f",
				track.TrackID, track.Hits, track.ObservationCount, track.artefact.maxLength, track.artefact.maxWidth)
		}
		return true
	}
	if track.artefact.held {
		t.continuity.ArtefactBirthsReleased++
	}
	return false
}

// absorbArtefact applies TrackerConfig.ArtefactAbsorb to an unassociated
// cluster: it reports true, and no track is born from it, when the cluster
// is small and sparse and lies within artefactAbsorbMarginMetres of one of
// the live confirmed bodies.
func (t *Tracker) absorbArtefact(c *WorldCluster, live []drawnRect) bool {
	if c.BoundingBoxLength >= artefactMaxExtentMetres || c.BoundingBoxWidth >= artefactMaxExtentMetres ||
		c.PointsCount >= artefactMaxMeanPoints {
		return false
	}
	x, y := float64(c.CentroidX), float64(c.CentroidY)
	for _, r := range live {
		grown := r
		grown.length += 2 * artefactAbsorbMarginMetres
		grown.wd += 2 * artefactAbsorbMarginMetres
		if rectContains(grown, x, y) {
			t.continuity.ArtefactClustersAbsorbed++
			diagf("Cluster absorbed by a live track: track_id=%s cluster_id=%d points=%d length=%.2f width=%.2f",
				r.id, c.ClusterID, c.PointsCount, c.BoundingBoxLength, c.BoundingBoxWidth)
			return true
		}
	}
	return false
}
