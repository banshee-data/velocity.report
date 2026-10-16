package l5tracks

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The published box: the one rectangle per track that a viewer draws.
//
// The legacy box is assembled from three estimates that need not agree: the
// filtered position (a medoid on the visible surface, not the body's centre),
// a heading smoothed by an EMA at 8 % a frame behind three guards, and this
// frame's PCA extents, measured along the PCA axis rather than along that
// heading. Drawn together, the box sits off its points, lags a turn by tens
// of degrees, and turns its corners out of the cluster whenever the smoothed
// heading and the PCA axis part. Nothing ties any of the three to the points
// or to how a vehicle can move.
//
// The box model keeps one rectangle whose parts agree by construction:
//
//   - The heading is a two-state filter, heading and yaw rate, observed by
//     the cluster's rectangle-fit axis (modulo 90°, the closeness criterion
//     of l4perception.FitRectangle) and, at speed, by the course. Its yaw
//     rate is bounded by what a road vehicle can do at its speed,
//     |ω| ≤ κ·v + ω₀, so it cannot spin at rest and turns no tighter than a
//     car can. Before the course has labelled which axis is the length, the
//     bound is looser.
//   - The extents are the larger of a corroborated belief (extentBelief) and
//     this frame's span of the points along the heading.
//   - The centre is placed so that the box contains this frame's points: as
//     near as containment allows to where the previous box moved with the
//     track, pulled toward the placement that puts the unseen remainder on
//     the side away from the sensor.
//
// It reads the tracked state and writes only the published box. Association,
// the Kalman state and identity are the tracker's without it.

// BoxAxisSource selects the per-frame observation of the body's axis.
type BoxAxisSource string

const (
	// BoxAxisRectangle fits the rectangle orientation to the cluster's
	// members (or retained sample): the axis of an L or of an end-on strip,
	// where PCA reads the diagonal or the strip.
	BoxAxisRectangle BoxAxisSource = "rectangle_fit"
	// BoxAxisPCA folds the cluster OBB's PCA heading to an axis: for a
	// pipeline that keeps no points.
	BoxAxisPCA BoxAxisSource = "pca"
)

// BoxModelConfig is the published box's configuration. The zero value is
// off; DefaultBoxModelConfig is the configuration the box model was scored
// with.
type BoxModelConfig struct {
	Enabled    bool
	AxisSource BoxAxisSource
	// CourseMinSpeedMps is the speed at and above which the course observes
	// the heading and labels its length and front. CourseMaxSigmaRad refuses a
	// course whose own spread, from the velocity covariance, is wider.
	CourseMinSpeedMps float64
	CourseMaxSigmaRad float64
	// CourseSideslipRad is the course's allowance for a body that does not
	// travel exactly along its axis: sideslip, and the velocity's lag in a
	// turn.
	CourseSideslipRad float64
	// YawAccelSigma is the white yaw acceleration (rad/s²) the heading filter
	// expects: how quickly a yaw rate may build.
	YawAccelSigma float64
	// MaxCurvature (1/m) and MinYawRate (rad/s) bound the yaw rate once the
	// box is labelled: |ω| ≤ MaxCurvature·v + MinYawRate. UnlabelledYawRate
	// replaces the bound's floor before the course has labelled the box.
	MaxCurvature      float64
	MinYawRate        float64
	UnlabelledYawRate float64
	// AxisSigmaScale multiplies the axis observation's standard deviation;
	// AxisGateSigmas refuses an axis further than that many standard
	// deviations of the innovation from the prediction, and AxisRestartRun
	// consecutive refusals reopen the heading to the fit.
	AxisSigmaScale float64
	AxisGateSigmas float64
	AxisRestartRun int
	// CentrePull is the share of the gap between the carried centre and the
	// sensor-anchored placement closed each frame, inside the interval that
	// contains the points.
	CentrePull float64
	// SensorX and SensorY are the sensor's position in the world frame, from
	// which the unseen remainder of a partly visible body lies away.
	SensorX, SensorY float64
}

// DefaultBoxModelConfig is the box model as scored in the 2026-10-16 box
// rotation run (docs/lidar/operations/published-box-rotation-2026-10.md),
// with Enabled left to the caller.
func DefaultBoxModelConfig() BoxModelConfig {
	return BoxModelConfig{
		AxisSource:        BoxAxisRectangle,
		CourseMinSpeedMps: 1.0,
		CourseMaxSigmaRad: 15 * math.Pi / 180,
		CourseSideslipRad: 4 * math.Pi / 180,
		YawAccelSigma:     0.4,
		MaxCurvature:      0.25,
		MinYawRate:        5 * math.Pi / 180,
		UnlabelledYawRate: 45 * math.Pi / 180,
		AxisSigmaScale:    1.5,
		AxisGateSigmas:    3,
		AxisRestartRun:    5,
		CentrePull:        0.3,
	}
}

// BoxInput is one frame's input to the box model: the tracked state after
// this frame's update and the cluster associated with it, nil when coasting.
type BoxInput struct {
	Nanos  int64
	X, Y   float64
	VX, VY float64
	// VelCov is the Kalman velocity covariance, [vxvx, vxvy, vyvy].
	VelCov  [3]float64
	Cluster *WorldCluster
}

// BoxState is the published box.
type BoxState struct {
	X, Y       float64
	HeadingRad float64
	Length     float64
	Width      float64
	// Labelled says the course has resolved which axis is the length.
	Labelled bool
}

// BoxStats counts a box's discrete events.
type BoxStats struct {
	// Relabels are labelling instants that turned the box by 90°; Clamps
	// frames whose heading change the yaw-rate bound cut; Restarts axis
	// refusal runs that reopened the heading.
	Relabels, Clamps, Restarts int
}

// BoxEstimator is one track's published box.
type BoxEstimator struct {
	cfg       BoxModelConfig
	seeded    bool
	labelled  bool
	psi, rate float64
	// p is the covariance of (psi, rate).
	p        [2][2]float64
	refusals int
	// discrete is this frame's turn of the frame that moved no box: a change
	// of front, or the labelling instant's quarter turn. The yaw-rate bound
	// applies to the rest.
	discrete  float64
	lastNanos int64
	length    windowBelief
	width     windowBelief
	// offA and offN place the box centre relative to the track position,
	// along and across the heading.
	offA, offN float64
	box        BoxState
	Stats      BoxStats
}

// NewBoxEstimator starts an empty box.
func NewBoxEstimator(cfg BoxModelConfig) *BoxEstimator {
	return &BoxEstimator{cfg: cfg}
}

// State is the current published box.
func (e *BoxEstimator) State() BoxState { return e.box }

const (
	boxMinExtentMetres = 0.1
	boxMaxDtSeconds    = 0.5
	boxSeedSigmaRad    = 30 * math.Pi / 180
	boxSeedRateSigma   = 0.3
	boxPCAAxisSigmaRad = 8 * math.Pi / 180
	boxPCAMinAspect    = 0.25
	// boxCourseGateRad refuses a course this far from a labelled heading: a
	// manoeuvre or a velocity still settling, not the body's axis.
	boxCourseGateRad = 45 * math.Pi / 180
	// boxCoastRateDecay is the share of the yaw rate kept per coasted frame.
	boxCoastRateDecay = 0.5
)

// Observe advances the box to the input's time. With a cluster it is updated
// from it; without one it coasts with the track.
func (e *BoxEstimator) Observe(in BoxInput) BoxState {
	dt := 0.0
	if e.seeded {
		dt = float64(in.Nanos-e.lastNanos) / 1e9
		if dt < 0 {
			dt = 0
		}
		if dt > boxMaxDtSeconds {
			dt = boxMaxDtSeconds
		}
	}
	speed := math.Hypot(in.VX, in.VY)
	pts := boxPoints(in.Cluster)
	if !e.seeded {
		if len(pts) == 0 {
			return e.box
		}
		e.seed(in, pts, speed)
		e.lastNanos = in.Nanos
		e.place(in, pts, true, nil)
		return e.box
	}
	prev := e.psi
	if len(pts) == 0 {
		// Coasting: nothing observes the turn, so the yaw rate the box last
		// had decays rather than carrying it round on its own.
		e.rate *= boxCoastRateDecay
	}
	e.predict(dt)
	var spans *[2]float64
	if len(pts) > 0 {
		spans = e.correct(in, pts, speed)
	}
	e.limit(prev, speed, dt)
	e.lastNanos = in.Nanos
	e.place(in, pts, false, spans)
	return e.box
}

func (e *BoxEstimator) seed(in BoxInput, pts [][2]float64, speed float64) {
	e.seeded = true
	e.p = [2][2]float64{{boxSeedSigmaRad * boxSeedSigmaRad, 0}, {0, boxSeedRateSigma * boxSeedRateSigma}}
	axis, _, ok := e.axisObservation(in.Cluster, pts)
	course, _, courseOK := e.courseObservation(in, speed)
	switch {
	case courseOK && ok:
		e.psi = nearestCandidate(axis, course)
		e.labelled = true
	case courseOK:
		e.psi = course
		e.labelled = true
	case ok:
		// The longer of the two spans is the length until a course says
		// otherwise; the front is the velocity's, if it has one.
		a, n := spansAlong(pts, axis)
		e.psi = axis
		if n > a {
			e.psi = axis + math.Pi/2
		}
		if speed > 0 && math.Cos(math.Atan2(in.VY, in.VX)-e.psi) < 0 {
			e.psi += math.Pi
		}
	case in.Cluster != nil && in.Cluster.OBB != nil:
		e.psi = float64(in.Cluster.OBB.HeadingRad)
	}
	e.psi = wrapPi(e.psi)
	if e.labelled {
		e.p[0][0] = math.Pow(5*math.Pi/180, 2)
	}
}

func (e *BoxEstimator) predict(dt float64) {
	if dt <= 0 {
		return
	}
	e.psi = wrapPi(e.psi + e.rate*dt)
	p := e.p
	// F = [[1, dt], [0, 1]]
	p00 := p[0][0] + dt*(p[1][0]+p[0][1]) + dt*dt*p[1][1]
	p01 := p[0][1] + dt*p[1][1]
	q := e.cfg.YawAccelSigma * e.cfg.YawAccelSigma
	e.p = [2][2]float64{
		{p00 + q*dt*dt*dt/3, p01 + q*dt*dt/2},
		{p01 + q*dt*dt/2, p[1][1] + q*dt},
	}
}

// correct applies this frame's axis and course observations. When the axis
// agrees with the corrected heading it returns the frame's spans along the
// observed axes, labelled length then width by the heading: extent evidence
// measured along the body's own axes, which a span along a heading a few
// degrees off is not (it gains L·sin δ across).
func (e *BoxEstimator) correct(in BoxInput, pts [][2]float64, speed float64) *[2]float64 {
	var spans *[2]float64
	if axis, sigma, ok := e.axisObservation(in.Cluster, pts); ok {
		r := foldQuarter(axis - e.psi)
		v := sigma * sigma
		gate := e.cfg.AxisGateSigmas
		if gate <= 0 || r*r <= gate*gate*(e.p[0][0]+v) {
			e.refusals = 0
			e.update(r, v)
			if math.Abs(foldQuarter(axis-e.psi)) <= 2*sigma {
				a, n := spansAlong(pts, axis)
				if math.Abs(foldHalf(axis-e.psi)) > math.Pi/4 {
					a, n = n, a
				}
				spans = &[2]float64{a, n}
			}
		} else {
			e.refusals++
			if e.cfg.AxisRestartRun > 0 && e.refusals >= e.cfg.AxisRestartRun {
				e.refusals = 0
				e.Stats.Restarts++
				e.p[0][0] = math.Max(e.p[0][0], boxSeedSigmaRad*boxSeedSigmaRad)
				e.update(r, v)
			}
		}
	}
	if course, sigma, ok := e.courseObservation(in, speed); ok {
		if !e.labelled {
			e.label(course)
		}
		r := wrapPi(course - e.psi)
		if math.Abs(r) > math.Pi/2 {
			// The same axis seen from the other end: the box is symmetric
			// under a half turn, so only the front changes.
			e.reframe(math.Pi)
			r = wrapPi(course - e.psi)
		}
		if math.Abs(r) <= boxCourseGateRad {
			e.update(r, sigma*sigma)
		}
	}
	return spans
}

// label resolves which axis is the length, and the front, from the course.
// A label that puts the length across the unlabelled heading turns the box by
// a quarter at once: which axis is the length was never observed, so the
// turn corrects a belief rather than drawing a motion, and turning at the
// yaw-rate bound would draw a vehicle rotating while it drives straight.
func (e *BoxEstimator) label(course float64) {
	e.labelled = true
	turn := foldHalf(nearestCandidate(e.psi, course) - e.psi)
	if math.Abs(turn) > math.Pi/4 {
		e.Stats.Relabels++
		e.length, e.width = e.width, e.length
		e.reframe(math.Copysign(math.Pi/2, turn))
	}
	if math.Cos(course-e.psi) < 0 {
		e.reframe(math.Pi)
	}
}

// reframe turns the heading by delta without moving the box: the centre's
// offsets are re-expressed in the turned frame.
func (e *BoxEstimator) reframe(delta float64) {
	c, s := math.Cos(delta), math.Sin(delta)
	e.offA, e.offN = e.offA*c+e.offN*s, -e.offA*s+e.offN*c
	e.psi = wrapPi(e.psi + delta)
	e.discrete += delta
}

func (e *BoxEstimator) update(r, v float64) {
	s := e.p[0][0] + v
	if s <= 0 {
		return
	}
	k0, k1 := e.p[0][0]/s, e.p[1][0]/s
	e.psi = wrapPi(e.psi + k0*r)
	e.rate += k1 * r
	p := e.p
	e.p = [2][2]float64{
		{(1 - k0) * p[0][0], (1 - k0) * p[0][1]},
		{p[1][0] - k1*p[0][0], p[1][1] - k1*p[0][1]},
	}
}

// limit holds the yaw rate and this frame's turn of the box to what a
// vehicle can do at its speed. The frame's discrete turns, a change of front
// or a label, moved no box and are not counted.
func (e *BoxEstimator) limit(prev, speed, dt float64) {
	floor := e.cfg.MinYawRate
	if !e.labelled && e.cfg.UnlabelledYawRate > floor {
		floor = e.cfg.UnlabelledYawRate
	}
	bound := e.cfg.MaxCurvature*speed + floor
	if e.rate > bound {
		e.rate = bound
	} else if e.rate < -bound {
		e.rate = -bound
	}
	base := prev + e.discrete
	e.discrete = 0
	turn := wrapPi(e.psi - base)
	if max := bound * dt; math.Abs(turn) > max+1e-12 {
		e.Stats.Clamps++
		e.psi = wrapPi(base + math.Copysign(max, turn))
	}
}

// place sets the extents and the centre from this frame's points, or carries
// the box with the track when there are none.
func (e *BoxEstimator) place(in BoxInput, pts [][2]float64, first bool, evidence *[2]float64) {
	c, s := math.Cos(e.psi), math.Sin(e.psi)
	if len(pts) == 0 {
		e.box = BoxState{
			X: in.X + e.offA*c - e.offN*s, Y: in.Y + e.offA*s + e.offN*c,
			HeadingRad: e.psi, Length: e.box.Length, Width: e.box.Width, Labelled: e.labelled,
		}
		return
	}
	aLo, aHi := math.Inf(1), math.Inf(-1)
	nLo, nHi := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		dx, dy := p[0]-in.X, p[1]-in.Y
		a, n := dx*c+dy*s, -dx*s+dy*c
		aLo, aHi = math.Min(aLo, a), math.Max(aHi, a)
		nLo, nHi = math.Min(nLo, n), math.Max(nHi, n)
	}
	spanA, spanN := aHi-aLo, nHi-nLo
	if evidence != nil && (in.Cluster == nil || !in.Cluster.GroundClipped) {
		e.length.observe(evidence[0])
		e.width.observe(evidence[1])
	}
	length := math.Max(math.Max(e.length.estimate(), spanA), boxMinExtentMetres)
	width := math.Max(math.Max(e.width.estimate(), spanN), boxMinExtentMetres)
	sa := (e.cfg.SensorX-in.X)*c + (e.cfg.SensorY-in.Y)*s
	sn := -(e.cfg.SensorX-in.X)*s + (e.cfg.SensorY-in.Y)*c
	e.offA = boxCentre(aLo, aHi, length, sa, e.offA, first, e.cfg.CentrePull)
	e.offN = boxCentre(nLo, nHi, width, sn, e.offN, first, e.cfg.CentrePull)
	e.box = BoxState{
		X: in.X + e.offA*c - e.offN*s, Y: in.Y + e.offA*s + e.offN*c,
		HeadingRad: e.psi, Length: length, Width: width, Labelled: e.labelled,
	}
}

// boxCentre places a box of the given extent along one axis: inside the
// interval that contains [lo, hi], as near as it allows to the carried
// centre moved part way toward the anchored one, which puts the unseen
// remainder on the side away from the sensor at s.
func boxCentre(lo, hi, extent, s, carried float64, first bool, pull float64) float64 {
	anchor := (lo + hi) / 2
	switch {
	case s <= lo:
		anchor = lo + extent/2
	case s >= hi:
		anchor = hi - extent/2
	}
	target := anchor
	if !first {
		target = carried + pull*(anchor-carried)
	}
	minC, maxC := hi-extent/2, lo+extent/2
	if target < minC {
		return minC
	}
	if target > maxC {
		return maxC
	}
	return target
}

// axisObservation is the frame's axis, modulo 90°, and its standard
// deviation.
func (e *BoxEstimator) axisObservation(cluster *WorldCluster, pts [][2]float64) (float64, float64, bool) {
	if cluster == nil {
		return 0, 0, false
	}
	scale := e.cfg.AxisSigmaScale
	if scale <= 0 {
		scale = 1
	}
	if e.cfg.AxisSource == BoxAxisPCA || (len(cluster.Members) == 0 && len(cluster.RetainedPoints) == 0) {
		b := cluster.OBB
		if b == nil || b.Length <= 0 || b.Width <= 0 {
			return 0, 0, false
		}
		long, short := math.Max(float64(b.Length), float64(b.Width)), math.Min(float64(b.Length), float64(b.Width))
		if (long-short)/long < boxPCAMinAspect {
			return 0, 0, false
		}
		return foldQuarterPositive(float64(b.HeadingRad)), boxPCAAxisSigmaRad * scale, true
	}
	members := cluster.Members
	if len(members) == 0 {
		members = cluster.RetainedPoints
	}
	fit := l4perception.FitRectangle(members)
	if !fit.Known() {
		return 0, 0, false
	}
	return fit.AxisRad, fit.SigmaRad * scale, true
}

// courseObservation is the direction of travel, when the track is fast
// enough and its velocity settled enough for it to say where the body points.
func (e *BoxEstimator) courseObservation(in BoxInput, speed float64) (float64, float64, bool) {
	if e.cfg.CourseMinSpeedMps <= 0 || speed < e.cfg.CourseMinSpeedMps {
		return 0, 0, false
	}
	course := math.Atan2(in.VY, in.VX)
	// The velocity's spread across its own direction, over the speed.
	nx, ny := -math.Sin(course), math.Cos(course)
	across := nx*nx*in.VelCov[0] + 2*nx*ny*in.VelCov[1] + ny*ny*in.VelCov[2]
	sigma := math.Sqrt(math.Max(across, 0)) / speed
	if e.cfg.CourseMaxSigmaRad > 0 && sigma > e.cfg.CourseMaxSigmaRad {
		return 0, 0, false
	}
	return course, math.Hypot(sigma, e.cfg.CourseSideslipRad), true
}

// boxPoints is the cluster's XY: its members, else its retained sample, else
// the corners of its OBB, which contains every member.
func boxPoints(cluster *WorldCluster) [][2]float64 {
	if cluster == nil {
		return nil
	}
	src := cluster.Members
	if len(src) == 0 {
		src = cluster.RetainedPoints
	}
	if len(src) > 0 {
		out := make([][2]float64, len(src))
		for i, p := range src {
			out[i] = [2]float64{p.X, p.Y}
		}
		return out
	}
	b := cluster.OBB
	if b == nil {
		return [][2]float64{{float64(cluster.CentroidX), float64(cluster.CentroidY)}}
	}
	c, s := math.Cos(float64(b.HeadingRad)), math.Sin(float64(b.HeadingRad))
	hl, hw := float64(b.Length)/2, float64(b.Width)/2
	out := make([][2]float64, 0, 4)
	for _, k := range [4][2]float64{{1, 1}, {1, -1}, {-1, -1}, {-1, 1}} {
		a, n := k[0]*hl, k[1]*hw
		out = append(out, [2]float64{float64(b.CenterX) + a*c - n*s, float64(b.CenterY) + a*s + n*c})
	}
	return out
}

// spansAlong is the points' extent along axis and across it.
func spansAlong(pts [][2]float64, axis float64) (float64, float64) {
	c, s := math.Cos(axis), math.Sin(axis)
	aLo, aHi := math.Inf(1), math.Inf(-1)
	nLo, nHi := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		a, n := p[0]*c+p[1]*s, -p[0]*s+p[1]*c
		aLo, aHi = math.Min(aLo, a), math.Max(aHi, a)
		nLo, nHi = math.Min(nLo, n), math.Max(nHi, n)
	}
	return aHi - aLo, nHi - nLo
}

// nearestCandidate is the heading axis + k·90° nearest target.
func nearestCandidate(axis, target float64) float64 {
	best, bestD := axis, math.Inf(1)
	for k := 0; k < 4; k++ {
		h := axis + float64(k)*math.Pi/2
		if d := math.Abs(wrapPi(target - h)); d < bestD {
			best, bestD = h, d
		}
	}
	return wrapPi(best)
}

func wrapPi(a float64) float64 { return math.Atan2(math.Sin(a), math.Cos(a)) }

// foldHalf folds an angle to [-π/2, π/2): the change of an unlabelled front.
func foldHalf(a float64) float64 { return 0.5 * math.Atan2(math.Sin(2*a), math.Cos(2*a)) }

// foldQuarter folds an angle to [-π/4, π/4): the residual between axes.
func foldQuarter(a float64) float64 { return 0.25 * math.Atan2(math.Sin(4*a), math.Cos(4*a)) }

// foldQuarterPositive folds an angle to [0, π/2).
func foldQuarterPositive(a float64) float64 {
	a = math.Mod(a, math.Pi/2)
	if a < 0 {
		a += math.Pi / 2
	}
	return a
}

// windowBelief is a corroborated maximum over the most recent admitted spans:
// the third largest of the last boxBeliefWindow. A body's dimensions do not
// change, but a merged cluster corroborates itself for as long as the merge
// lasts; the window lets the box return to the body once it ends, where a
// whole-life maximum holds the merge's size until the track does.
type windowBelief struct {
	spans []float64
	next  int
}

const (
	boxBeliefWindow        = 50
	boxBeliefCorroboration = 3
)

func (b *windowBelief) observe(span float64) {
	if span <= 0 || math.IsNaN(span) || math.IsInf(span, 0) {
		return
	}
	if len(b.spans) < boxBeliefWindow {
		b.spans = append(b.spans, span)
		return
	}
	b.spans[b.next] = span
	b.next = (b.next + 1) % boxBeliefWindow
}

// estimate is the third largest admitted span, or the smallest of fewer.
func (b *windowBelief) estimate() float64 {
	var top [boxBeliefCorroboration]float64
	n := 0
	for _, v := range b.spans {
		for i := 0; i < boxBeliefCorroboration; i++ {
			if i >= n || v > top[i] {
				copy(top[i+1:], top[i:boxBeliefCorroboration-1])
				top[i] = v
				if n < boxBeliefCorroboration {
					n++
				}
				break
			}
		}
	}
	if n == 0 {
		return 0
	}
	return top[n-1]
}
