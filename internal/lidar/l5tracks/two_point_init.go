package l5tracks

import "math"

// Guards for TwoPointVelocityInit (initialiseTwoPoint). A pair of
// measurements differences two reference points, and that difference is a
// velocity only when both are the same body's, seen the same way. Two
// conditions break that on real captures (l5est0006, kirk0 and the first
// five minutes of ashbury-downey):
//
//   - The second measurement is a reflection. A thin, sparse cluster on a
//     nearer car's bearing, at k times its range, moves at k times its
//     speed: 1.6 to 3 times on these captures, 13 to 29 m/s. The shipped
//     tracker never confirms one, because it cannot hold anything born that
//     fast; the pair initialises it exactly, and it confirms and coasts at
//     that speed. The sensor cannot see a surface directly behind a nearer
//     body on the same bearing, so a measurement in a confirmed track's
//     shadow is not evidence of a body's motion.
//   - The visible extent changed. A body first seen as a fragment, then
//     whole, moves its reference point by up to half the change in extent
//     along the displacement without moving at all.
//
// The first declines; the second widens the pair's velocity covariance by
// that slack, and declines through the existing covariance-cap test when the
// widened variance cannot be represented. Either way a declined pair takes
// the shipped update, so the option is never further from the shipped
// tracker than the initialisation it was built to replace.

// clusterFootprint is a cluster's footprint in the plane: its box's length
// and width and, when oriented, the heading of its length axis.
type clusterFootprint struct {
	length, width, heading float32
	oriented               bool
}

// footprintOf reads a cluster's footprint: its OBB when it has a valid one,
// else its bounding box's dimensions without a heading.
func footprintOf(cluster WorldCluster) clusterFootprint {
	if obb := cluster.OBB; obb != nil && finiteMeasurementCoordinate(obb.Length) &&
		finiteMeasurementCoordinate(obb.Width) && finiteMeasurementCoordinate(obb.HeadingRad) {
		return clusterFootprint{length: obb.Length, width: obb.Width, heading: obb.HeadingRad, oriented: true}
	}
	return clusterFootprint{length: cluster.BoundingBoxLength, width: cluster.BoundingBoxWidth}
}

// extentAlong is the footprint's extent along the unit direction (ux, uy).
// Without a heading the direction is unknown, so it is the larger dimension:
// an upper bound.
func (f clusterFootprint) extentAlong(ux, uy float64) float64 {
	l, w := math.Abs(float64(f.length)), math.Abs(float64(f.width))
	if !f.oriented {
		return math.Max(l, w)
	}
	c, s := math.Cos(float64(f.heading)), math.Sin(float64(f.heading))
	along := ux*c + uy*s
	return l*math.Abs(along) + w*math.Sqrt(math.Max(0, 1-along*along))
}

// referenceSlack is how far the reference point can move between two
// footprints of one body without the body moving: half the change in extent
// along the displacement (ux, uy). With a zero displacement there is no
// direction, and it is half the larger change in either dimension.
func referenceSlack(first, second clusterFootprint, ux, uy float64, haveDirection bool) float64 {
	if !haveDirection {
		dl := math.Abs(float64(second.length - first.length))
		dw := math.Abs(float64(second.width - first.width))
		return math.Max(dl, dw) / 2
	}
	d := math.Abs(second.extentAlong(ux, uy) - first.extentAlong(ux, uy))
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return math.Inf(1)
	}
	return d / 2
}

// inConfirmedShadow reports whether (x, y) lies in the shadow of another
// confirmed track's box as the sensor sees it: on a bearing between the
// box's corner bearings and farther than its nearest corner. The sensor is
// at (NoiseSensorX, NoiseSensorY), the origin the adaptive noise model reads;
// the pipeline clusters without a pose, so the tracker frame is the sensor
// frame and zero is correct today. A track without a positive, finite box
// casts no shadow. Every track is visited and the answer is an any, so the
// map's order does not reach the result.
func (t *Tracker) inConfirmedShadow(self *TrackedObject, x, y float32) bool {
	sx, sy := float64(t.Config.NoiseSensorX), float64(t.Config.NoiseSensorY)
	mx, my := float64(x)-sx, float64(y)-sy
	rng := math.Hypot(mx, my)
	if !(rng > 0) {
		return false
	}
	bearing := math.Atan2(my, mx)
	for _, other := range t.Tracks {
		if other == self || other.TrackState != TrackConfirmed {
			continue
		}
		l, w := float64(other.OBBLength), float64(other.OBBWidth)
		if !(l > 0) || !(w > 0) || math.IsInf(l, 0) || math.IsInf(w, 0) ||
			!finiteMeasurementCoordinate(other.OBBHeadingRad) {
			continue
		}
		c, s := math.Cos(float64(other.OBBHeadingRad)), math.Sin(float64(other.OBBHeadingRad))
		ox, oy := float64(other.X)-sx, float64(other.Y)-sy
		lo, hi, nearest := math.Inf(1), math.Inf(-1), math.Inf(1)
		for _, a := range [2]float64{-l / 2, l / 2} {
			for _, b := range [2]float64{-w / 2, w / 2} {
				cx, cy := ox+a*c-b*s, oy+a*s+b*c
				rel := wrapAngle(math.Atan2(cy, cx) - bearing)
				lo, hi = math.Min(lo, rel), math.Max(hi, rel)
				nearest = math.Min(nearest, math.Hypot(cx, cy))
			}
		}
		// A box that straddles the opposite bearing, or holds the sensor,
		// spans π or more: it is not a shadow this test can read.
		if hi-lo < math.Pi && lo <= 0 && hi >= 0 && rng > nearest {
			return true
		}
	}
	return false
}

// wrapAngle wraps a in radians to [−π, π).
func wrapAngle(a float64) float64 {
	return math.Mod(math.Mod(a+math.Pi, 2*math.Pi)+2*math.Pi, 2*math.Pi) - math.Pi
}
