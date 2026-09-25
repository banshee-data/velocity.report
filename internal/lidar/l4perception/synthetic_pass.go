package l4perception

// A synthetic Pandar40P observing a box vehicle on a straight pass.
//
// This is the seed of the synthetic corpus in Section 16 of
// docs/plans/lidar-state-estimation-plan.md, promoted from the prototype that
// produced Section 3's evidence.
//
// Its purpose is to isolate measurement bias from noise. There is no range
// error, no intensity model, no background-subtraction error and no dropped
// returns beyond an explicitly injected occluder: every point is exactly where
// the geometry says it should be. Whatever error a measurement definition shows
// against this generator is therefore a property of the definition, not of the
// sensor — which is what makes it possible to say that the medoid's lateral
// offset is deterministic given viewing geometry rather than noise to be
// filtered out.
//
// The simplifications all make results *conservative*: the vehicle is a box,
// the road is flat, there is no ground return and no second object. Each of
// those would add error to the production path rather than remove it.

import (
	"math"
	"sort"
	"time"
)

// SyntheticSensor models the sensor's sampling pattern.
type SyntheticSensor struct {
	// AzimuthStepDeg is the horizontal angular resolution.
	AzimuthStepDeg float64
	// ElevationsDeg are the rings to fire, in degrees above the horizon
	// (negative looks down).
	ElevationsDeg []float64
	// HeightMetres is the sensor's height above the road plane. The sensor sits
	// at the origin in X and Y.
	HeightMetres float64
}

// Pandar40PElevationsDeg is the real per-ring elevation table read from a
// deployed hesai-pandar40p, spanning +15.21 to -24.57 degrees with the
// characteristic dense band around the horizon.
//
// The real table is used rather than a hand-picked band because the band
// Section 3.1 describes — "fourteen elevation rings spanning the dense band" —
// cannot see the plan's own vehicle for most of the pass. The steepest ring in
// that band is about -6.7 degrees, which from a 3 m mounting only descends to
// the 1.5 m roof height at 12.7 m; the vehicle passes within 5.1 m. Section
// 3.2's table nonetheless reports 248 points at that frame, so §3.1's stated
// configuration cannot be the one that produced §3.2's numbers. Rather than
// guess at the difference, this generator uses the hardware's own table, which
// covers the whole pass, and the discrepancy is recorded in the plan.
var Pandar40PElevationsDeg = []float64{
	15.21, 11.36, 8.387, 5.385, 3.368, 2.356, 2.016, 1.679,
	1.341, 1.003, 0.665, 0.328, -0.009, -0.347, -0.685, -1.023,
	-1.36, -1.7, -2.037, -2.372, -2.712, -3.047, -3.384, -3.722,
	-4.057, -4.392, -4.729, -5.063, -5.398, -5.733, -6.735, -7.731,
	-8.732, -9.557, -10.704, -11.678, -12.646, -13.602, -18.561, -24.569,
}

// DefaultSyntheticSensor is Section 3.1's sampling geometry — 0.2 degree
// azimuth steps, 3 m above the road — with the real Pandar40P ring table.
func DefaultSyntheticSensor() SyntheticSensor {
	elevations := make([]float64, len(Pandar40PElevationsDeg))
	copy(elevations, Pandar40PElevationsDeg)
	return SyntheticSensor{
		AzimuthStepDeg: 0.2,
		ElevationsDeg:  elevations,
		HeightMetres:   3.0,
	}
}

// SyntheticVehicle is an axis-aligned box travelling in a straight line.
type SyntheticVehicle struct {
	// Length, Width, Height are the body's true dimensions in metres.
	Length, Width, Height float64
	// LateralOffsetMetres is the vehicle centreline's Y offset from the sensor.
	LateralOffsetMetres float64
	// SpeedMps is the constant speed along +X.
	SpeedMps float64
	// StartXMetres is the body centre's X at frame zero.
	StartXMetres float64
}

// DefaultSyntheticVehicle is Section 3.1's vehicle: a 4.5 x 1.8 x 1.5 m box
// travelling dead straight at exactly 12.0 m/s, 5 m to the side of the sensor.
func DefaultSyntheticVehicle() SyntheticVehicle {
	return SyntheticVehicle{
		Length:              4.5,
		Width:               1.8,
		Height:              1.5,
		LateralOffsetMetres: 5.0,
		SpeedMps:            12.0,
		StartXMetres:        -24.0,
	}
}

// CentreAt is the true body centre at a frame index, given the frame interval.
func (v SyntheticVehicle) CentreAt(frame int, interval time.Duration) (x, y, z float64) {
	t := float64(frame) * interval.Seconds()
	return v.StartXMetres + v.SpeedMps*t, v.LateralOffsetMetres, v.Height / 2
}

// SyntheticOccluder deletes an azimuth wedge from a range of frames, modelling
// a foreground obstruction. Section 3.1 used a 1.2 degree wedge on frames 18
// to 20, which is what produces the only frames in the pass where the near-edge
// measurement fails.
type SyntheticOccluder struct {
	// CentreAzimuthDeg and WidthDeg describe the deleted wedge. A WidthDeg of
	// zero disables the occluder.
	CentreAzimuthDeg float64
	WidthDeg         float64
	// FirstFrame and LastFrame are inclusive.
	FirstFrame, LastFrame int
}

// blocks reports whether this occluder deletes a given azimuth on a given
// frame.
func (o SyntheticOccluder) blocks(frame int, azimuthDeg float64) bool {
	if o.WidthDeg <= 0 || frame < o.FirstFrame || frame > o.LastFrame {
		return false
	}
	half := o.WidthDeg / 2
	return math.Abs(angleDeltaDeg(azimuthDeg, o.CentreAzimuthDeg)) <= half
}

// SyntheticPass describes a whole pass to be generated.
type SyntheticPass struct {
	Sensor   SyntheticSensor
	Vehicle  SyntheticVehicle
	Occluder SyntheticOccluder
	// Frames is how many frames to generate and Interval the time between
	// them.
	Frames   int
	Interval time.Duration
	SensorID string
}

// DefaultSyntheticPass is Section 3.1's forty-frame pass sampled every 100 ms,
// with the occluder positioned to clip the vehicle as it passes closest to the
// sensor.
//
// The occluder's azimuth is derived rather than hard-coded, so that changing
// the vehicle's geometry keeps the obstruction pointed at it instead of
// silently becoming a wedge of empty sky.
func DefaultSyntheticPass() SyntheticPass {
	vehicle := DefaultSyntheticVehicle()
	pass := SyntheticPass{
		Sensor:   DefaultSyntheticSensor(),
		Vehicle:  vehicle,
		Frames:   40,
		Interval: 100 * time.Millisecond,
		SensorID: "synthetic-pandar40p",
	}
	// Frame 19 is the middle of the occluded range; point the wedge at where
	// the vehicle is then.
	x, y, _ := vehicle.CentreAt(19, pass.Interval)
	pass.Occluder = SyntheticOccluder{
		CentreAzimuthDeg: math.Atan2(y, x) * 180 / math.Pi,
		WidthDeg:         1.2,
		FirstFrame:       18,
		LastFrame:        20,
	}
	return pass
}

// SyntheticFrame is one generated frame: the returns, and the truth they came
// from, so a test never has to re-derive the answer it is checking against.
type SyntheticFrame struct {
	Index  int
	Points []WorldPoint
	// TrueCentreX/Y/Z is the body centre that produced these points.
	TrueCentreX, TrueCentreY, TrueCentreZ float64
	// Occluded reports whether the occluder was active on this frame.
	Occluded  bool
	Timestamp time.Time
}

// GenerateSyntheticPass produces the whole pass.
//
// Returns are generated by slab intersection against the box, and only on
// faces the sensor can actually see: a ray that would strike the far side is
// not reported, because the near side stopped it. That single property is what
// the whole of Section 3 rests on — the far face is not observed at all and
// must come from a prior.
func GenerateSyntheticPass(pass SyntheticPass) []SyntheticFrame {
	if pass.Frames <= 0 || pass.Interval <= 0 || pass.Sensor.AzimuthStepDeg <= 0 {
		return nil
	}
	origin := time.Unix(0, 0).UTC()

	frames := make([]SyntheticFrame, 0, pass.Frames)
	for i := 0; i < pass.Frames; i++ {
		cx, cy, cz := pass.Vehicle.CentreAt(i, pass.Interval)
		frame := SyntheticFrame{
			Index:       i,
			TrueCentreX: cx,
			TrueCentreY: cy,
			TrueCentreZ: cz,
			Occluded:    pass.Occluder.WidthDeg > 0 && i >= pass.Occluder.FirstFrame && i <= pass.Occluder.LastFrame,
			Timestamp:   origin.Add(time.Duration(i) * pass.Interval),
		}

		// Only sweep the azimuth window the box can possibly occupy, so the
		// generator's cost does not scale with the whole 360 degree sweep.
		minAz, maxAz := boxAzimuthSpanDeg(pass.Vehicle, cx, cy)
		for az := minAz; az <= maxAz; az += pass.Sensor.AzimuthStepDeg {
			if pass.Occluder.blocks(i, az) {
				continue
			}
			for _, elev := range pass.Sensor.ElevationsDeg {
				p, hit := castRayAtBox(pass.Sensor, pass.Vehicle, cx, cy, az, elev)
				if !hit {
					continue
				}
				p.Timestamp = frame.Timestamp
				p.SensorID = pass.SensorID
				frame.Points = append(frame.Points, p)
			}
		}
		frames = append(frames, frame)
	}
	return frames
}

// boxAzimuthSpanDeg bounds the azimuths at which the box could be hit, with a
// degree of slack on each side.
func boxAzimuthSpanDeg(v SyntheticVehicle, cx, cy float64) (minDeg, maxDeg float64) {
	halfL, halfW := v.Length/2, v.Width/2
	minDeg, maxDeg = math.Inf(1), math.Inf(-1)
	for _, corner := range [4][2]float64{
		{cx - halfL, cy - halfW}, {cx + halfL, cy - halfW},
		{cx - halfL, cy + halfW}, {cx + halfL, cy + halfW},
	} {
		deg := math.Atan2(corner[1], corner[0]) * 180 / math.Pi
		minDeg = math.Min(minDeg, deg)
		maxDeg = math.Max(maxDeg, deg)
	}
	return minDeg - 1, maxDeg + 1
}

// castRayAtBox intersects one ray with the axis-aligned box and returns the
// nearest surface point, which is the only one the sensor would see.
//
// The slab method: for each axis, the ray enters the box's extent at one
// parameter and leaves at another. The ray hits the box exactly when the latest
// entry precedes the earliest exit, and the surface point is at that latest
// entry.
func castRayAtBox(s SyntheticSensor, v SyntheticVehicle, cx, cy, azimuthDeg, elevationDeg float64) (WorldPoint, bool) {
	az := azimuthDeg * math.Pi / 180
	el := elevationDeg * math.Pi / 180

	// Unit direction from the sensor, which sits at (0, 0, HeightMetres).
	dx := math.Cos(el) * math.Cos(az)
	dy := math.Cos(el) * math.Sin(az)
	dz := math.Sin(el)
	ox, oy, oz := 0.0, 0.0, s.HeightMetres

	// The box spans these bounds. Its base sits on the road plane at z = 0.
	minX, maxX := cx-v.Length/2, cx+v.Length/2
	minY, maxY := cy-v.Width/2, cy+v.Width/2
	minZ, maxZ := 0.0, v.Height

	tEnter, tExit := math.Inf(-1), math.Inf(1)
	for _, slab := range [3]struct {
		origin, direction, lo, hi float64
	}{
		{ox, dx, minX, maxX},
		{oy, dy, minY, maxY},
		{oz, dz, minZ, maxZ},
	} {
		if math.Abs(slab.direction) < 1e-12 {
			// Parallel to this slab: the ray either starts inside it or can
			// never enter the box at all.
			if slab.origin < slab.lo || slab.origin > slab.hi {
				return WorldPoint{}, false
			}
			continue
		}
		t1 := (slab.lo - slab.origin) / slab.direction
		t2 := (slab.hi - slab.origin) / slab.direction
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tEnter = math.Max(tEnter, t1)
		tExit = math.Min(tExit, t2)
		if tEnter > tExit {
			return WorldPoint{}, false
		}
	}
	// Behind the sensor, or degenerate.
	if tExit < 0 || tEnter <= 0 {
		return WorldPoint{}, false
	}

	return WorldPoint{
		X: ox + tEnter*dx,
		Y: oy + tEnter*dy,
		Z: oz + tEnter*dz,
		// Uniform: there is deliberately no intensity model, so nothing
		// downstream can come to depend on one that was invented here.
		Intensity: 100,
	}, true
}

// NearEdgePercentile is the percentile of a cluster's lateral coordinate that
// Section 3.3 found to work: the 5th. It is not the minimum, because a single
// stray return would then define the edge, and not the median, because half the
// near face lies beyond that.
const NearEdgePercentile = 5.0

// LateralPercentile returns the requested percentile of the points' coordinate
// along the given unit direction, measured from the origin.
//
// This is the observable the near-edge measurement is built on: the position of
// the surface that was actually sampled, as opposed to the centre of a box
// fitted to a partial view.
func LateralPercentile(points []WorldPoint, dirX, dirY, percentile float64) (float64, bool) {
	if len(points) == 0 || percentile < 0 || percentile > 100 {
		return 0, false
	}
	norm := math.Hypot(dirX, dirY)
	if norm < 1e-12 {
		return 0, false
	}
	dirX, dirY = dirX/norm, dirY/norm

	projections := make([]float64, len(points))
	for i, p := range points {
		projections[i] = p.X*dirX + p.Y*dirY
	}
	sort.Float64s(projections)

	// Nearest-rank on a sorted sample: for the 5th percentile of 200 points
	// this is the 10th smallest, which is robust to a handful of strays
	// without reaching into the body of the face.
	idx := int(percentile / 100 * float64(len(projections)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(projections) {
		idx = len(projections) - 1
	}
	return projections[idx], true
}

// angleDeltaDeg is the shortest signed angle from b to a, in degrees.
func angleDeltaDeg(a, b float64) float64 {
	d := math.Mod(a-b+540, 360) - 180
	return d
}
