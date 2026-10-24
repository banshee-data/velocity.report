package l5tracks

import "math"

// PublishedBox is a track's box as viewers draw it (l9endpoints publishBox):
// the box model's centre, heading and extents when the model has seen the
// track's points, else the tracked position with the smoothed OBB heading and
// the frame's OBB extents. Coordinates are the tracker's frame, which offline
// replay and the live sensor-frame pipeline both make the sensor frame.
//
// Z is the box's vertical centre: the cluster OBB's centre, or the middle of
// the ground-anchored base and height when the box model stands the box on
// the fitted surface.
type PublishedBox struct {
	X, Y, Z               float32
	Length, Width, Height float32
	HeadingRad            float32
}

// PublishedBox returns the box viewers draw for the track.
func (t *TrackedObject) PublishedBox() PublishedBox {
	b := PublishedBox{
		X: t.X, Y: t.Y, Z: t.LatestZ,
		Length: t.OBBLength, Width: t.OBBWidth, Height: t.OBBHeight,
		HeadingRad: t.OBBHeadingRad,
	}
	if t.BoxLength > 0 {
		b.X, b.Y = t.BoxX, t.BoxY
		b.Length, b.Width = t.BoxLength, t.BoxWidth
		b.HeadingRad = t.BoxHeadingRad
	}
	if t.BoxHeight > 0 {
		b.Height = t.BoxHeight
		b.Z = t.BoxZ + t.BoxHeight/2
	}
	return b
}

// Contains reports whether (x, y, z) lies in the box grown by marginXY on
// each side in the plane and marginZ above its top. The box is never grown
// downwards: what lies below its base is the ground it stands on, not the
// object.
func (b PublishedBox) Contains(x, y, z, marginXY, marginZ float64) bool {
	halfH := float64(b.Height) / 2
	cz := float64(b.Z)
	if z < cz-halfH || z > cz+halfH+marginZ {
		return false
	}
	dx, dy := x-float64(b.X), y-float64(b.Y)
	s, c := math.Sincos(float64(b.HeadingRad))
	along := dx*c + dy*s
	across := -dx*s + dy*c
	return math.Abs(along) <= float64(b.Length)/2+marginXY &&
		math.Abs(across) <= float64(b.Width)/2+marginXY
}
