package l5tracks

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The overlap end (TrackerConfig.OverlapEndFrames): a coasting track whose
// drawn box lies on a live confirmed track's is ended. Two road users cannot
// occupy the same ground, so a prediction that does is the live track's
// duplicate: a second track on one body that lost the body's cluster to the
// first, or takes it back in alternate frames. An occluded road user is
// behind its occluder, never inside it, so the rule does not end one. The
// published box is the one a viewer draws; without the box model it is the
// legacy OBB at the tracked position.

// overlapEndShare is the share of the smaller box two boxes must share.
const overlapEndShare = 0.5

// drawnRect is a track's drawn box in the plane.
type drawnRect struct {
	id                    string
	x, y, psi, length, wd float64
}

// drawnBoxOf is the box a viewer draws for the track.
func drawnBoxOf(track *TrackedObject) (drawnRect, bool) {
	if track.BoxLength > 0 && track.BoxWidth > 0 {
		return drawnRect{track.TrackID, float64(track.BoxX), float64(track.BoxY), float64(track.BoxHeadingRad),
			float64(track.BoxLength), float64(track.BoxWidth)}, true
	}
	if track.OBBLength > 0 && track.OBBWidth > 0 {
		return drawnRect{track.TrackID, float64(track.X), float64(track.Y), float64(track.OBBHeadingRad),
			float64(track.OBBLength), float64(track.OBBWidth)}, true
	}
	return drawnRect{}, false
}

// liveConfirmedRects are the drawn boxes of the confirmed tracks matched
// this frame.
func (t *Tracker) liveConfirmedRects(matched map[string]bool) []drawnRect {
	var out []drawnRect
	for id := range matched {
		track := t.Tracks[id]
		if track == nil || track.TrackState != TrackConfirmed {
			continue
		}
		if r, ok := drawnBoxOf(track); ok {
			out = append(out, r)
		}
	}
	return out
}

// overlapEndBodyMargin grows a live body's observed extent for the
// on-body test: a prediction a little outside the returns is still on them.
const overlapEndBodyMargin = 0.25

// coastsOn reports the live confirmed track a coasting track lies on, by the
// configured test: its drawn box over half another's (the default), or its
// predicted position within another's observed returns (OverlapEndOnBody).
func (t *Tracker) coastsOn(track *TrackedObject, live []drawnRect) (string, bool) {
	if t.Config.OverlapEndOnBody {
		for _, l := range live {
			if l.id != track.TrackID && rectContains(l, float64(track.X), float64(track.Y)) {
				return l.id, true
			}
		}
		return "", false
	}
	r, ok := drawnBoxOf(track)
	if !ok {
		return "", false
	}
	for _, l := range live {
		if l.id != r.id && rectOverlapShare(r, l) > overlapEndShare {
			return l.id, true
		}
	}
	return "", false
}

// overlapVictim is the track the overlap end ends: the coasting one, or
// with OverlapEndKeepOlder whichever of the two was created later.
func (t *Tracker) overlapVictim(coasting *TrackedObject, liveID string) *TrackedObject {
	if !t.Config.OverlapEndKeepOlder {
		return coasting
	}
	if l := t.Tracks[liveID]; l != nil && l.TrackState != TrackDeleted && l.CreationSequence > coasting.CreationSequence {
		return l
	}
	return coasting
}

// liveConfirmedBodies are the observed extents of the confirmed tracks
// matched this frame: each cluster's members' extent along the track's drawn
// heading, grown by overlapEndBodyMargin, or the cluster's PCA box without
// members.
func (t *Tracker) liveConfirmedBodies(clusters []WorldCluster, associations []string) []drawnRect {
	var out []drawnRect
	for ci, id := range associations {
		track := t.Tracks[id]
		if id == "" || track == nil || track.TrackState != TrackConfirmed || ci >= len(clusters) {
			continue
		}
		c := &clusters[ci]
		pts := c.Members
		if len(pts) == 0 {
			pts = c.RetainedPoints
		}
		if len(pts) == 0 {
			if c.OBB != nil && c.OBB.Length > 0 {
				out = append(out, drawnRect{id, float64(c.OBB.CenterX), float64(c.OBB.CenterY), float64(c.OBB.HeadingRad),
					float64(c.OBB.Length) + 2*overlapEndBodyMargin, float64(c.OBB.Width) + 2*overlapEndBodyMargin})
			}
			continue
		}
		psi := float64(track.OBBHeadingRad)
		if r, ok := drawnBoxOf(track); ok {
			psi = r.psi
		}
		cs, sn := math.Cos(psi), math.Sin(psi)
		aLo, aHi, nLo, nHi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, p := range pts {
			a, n := p.X*cs+p.Y*sn, -p.X*sn+p.Y*cs
			aLo, aHi, nLo, nHi = math.Min(aLo, a), math.Max(aHi, a), math.Min(nLo, n), math.Max(nHi, n)
		}
		ca, cn := (aLo+aHi)/2, (nLo+nHi)/2
		out = append(out, drawnRect{id, ca*cs - cn*sn, ca*sn + cn*cs, psi,
			aHi - aLo + 2*overlapEndBodyMargin, nHi - nLo + 2*overlapEndBodyMargin})
	}
	return out
}

// rectContains reports whether (x, y) lies within the rectangle.
func rectContains(r drawnRect, x, y float64) bool {
	cs, sn := math.Cos(r.psi), math.Sin(r.psi)
	dx, dy := x-r.x, y-r.y
	return math.Abs(dx*cs+dy*sn) <= r.length/2 && math.Abs(-dx*sn+dy*cs) <= r.wd/2
}

// withoutRect drops the named track's rectangle.
func withoutRect(rs []drawnRect, id string) []drawnRect {
	out := rs[:0:0]
	for _, r := range rs {
		if r.id != id {
			out = append(out, r)
		}
	}
	return out
}

// rectOverlapShare is the area two rectangles share over the smaller one's
// (l4perception.RectOverlapShare, which lidar-box-eval's duplicate count
// reads too).
func rectOverlapShare(p, q drawnRect) float64 {
	return l4perception.RectOverlapShare(p.rect(), q.rect())
}

func (r drawnRect) rect() l4perception.Rect {
	return l4perception.Rect{X: r.x, Y: r.y, HeadingRad: r.psi, Length: r.length, Width: r.wd}
}
