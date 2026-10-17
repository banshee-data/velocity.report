package l5tracks

import "github.com/banshee-data/velocity.report/internal/lidar/l4perception"

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

// coastsOnLive reports whether the coasting track's drawn box shares more
// than overlapEndShare of the smaller box with any live confirmed track's.
func coastsOnLive(track *TrackedObject, live []drawnRect) bool {
	r, ok := drawnBoxOf(track)
	if !ok {
		return false
	}
	for _, l := range live {
		if l.id != r.id && rectOverlapShare(r, l) > overlapEndShare {
			return true
		}
	}
	return false
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
