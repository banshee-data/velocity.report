package l5tracks

import "math"

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

// rectOverlapShare is the area two rectangles share over the smaller one's.
func rectOverlapShare(p, q drawnRect) float64 {
	if math.Hypot(p.x-q.x, p.y-q.y) > (math.Hypot(p.length, p.wd)+math.Hypot(q.length, q.wd))/2 {
		return 0
	}
	small := math.Min(p.length*p.wd, q.length*q.wd)
	if small <= 0 {
		return 0
	}
	return convexArea(clipConvexPolygon(rectCorners(p), rectCorners(q))) / small
}

// rectCorners are a rectangle's corners, anticlockwise.
func rectCorners(r drawnRect) [][2]float64 {
	c, s := math.Cos(r.psi), math.Sin(r.psi)
	out := make([][2]float64, 0, 4)
	for _, k := range [4][2]float64{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
		a, n := k[0]*r.length/2, k[1]*r.wd/2
		out = append(out, [2]float64{r.x + a*c - n*s, r.y + a*s + n*c})
	}
	return out
}

// clipConvexPolygon clips subject to the convex, anticlockwise clip polygon
// (Sutherland-Hodgman).
func clipConvexPolygon(subject, clip [][2]float64) [][2]float64 {
	out := subject
	for i := range clip {
		a, b := clip[i], clip[(i+1)%len(clip)]
		in := out
		out = nil
		if len(in) == 0 {
			break
		}
		side := func(p [2]float64) float64 { return (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0]) }
		prev := in[len(in)-1]
		for _, cur := range in {
			sp, sc := side(prev), side(cur)
			if sc >= 0 {
				if sp < 0 {
					out = append(out, crossPoint(prev, cur, sp, sc))
				}
				out = append(out, cur)
			} else if sp >= 0 {
				out = append(out, crossPoint(prev, cur, sp, sc))
			}
			prev = cur
		}
	}
	return out
}

func crossPoint(p, q [2]float64, sp, sq float64) [2]float64 {
	t := sp / (sp - sq)
	return [2]float64{p[0] + t*(q[0]-p[0]), p[1] + t*(q[1]-p[1])}
}

func convexArea(pts [][2]float64) float64 {
	a := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		a += pts[i][0]*pts[j][1] - pts[j][0]*pts[i][1]
	}
	return math.Abs(a) / 2
}
