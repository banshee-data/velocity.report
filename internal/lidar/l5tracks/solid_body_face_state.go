package l5tracks

// The face-state options on the solid body: Sections 3.1 to 3.5 of
// data/maths/proposals/20261108-face-state-and-cv-ca-maths-review.md, and
// Section 9.2.4 of docs/plans/lidar-state-estimation-plan.md. Each is a
// default-off SolidBodyOptions field; off, nothing here runs and the shipped
// arithmetic is unchanged (TestFaceStateOptionsOffLeaveTheShippedArithmetic).
//
// ExtentState. The near-edge face measures its plane, which lies a
// half-extent outboard of the centre. The shipped update subtracts a believed
// half-extent and trusts the result to the face's own noise, so an extent
// believed short moves the centre toward the sensor by half the shortfall and
// the filter reports it at face precision (the F0 report, Section 4.1). Under
// ExtentState the half-extents are states: the joint state is
//
//	x = [x, y, vx, vy, hL, hW]
//
// and an end face with outward normal n measures z = n·c + hL, with
// H = [n_x, n_y, 0, 0, 1, 0]; a side face uses the hW column. The half-extents
// are constant with a small process noise (extentStateProcessNoise). The
// four-state block stays in solidBodyTrack.state and p, and the extent block,
// with its 4x2 cross-covariance, in solidBodyExtent, so every four-state path
// is untouched; any step that reads or writes the extents assembles the 6x6
// joint state (jointOf), operates on it in float64, and writes both blocks
// back (setJoint). One face cannot separate n·c from h: its row has a null
// direction there, and the joint covariance says so. A rear face seen on
// approach and a front face on departure have rows +n and -n against one hL,
// which fixes hL and the centre.
//
// Lower bounds. A frame's span along a body axis, less the span noise, is a
// lower bound on the dimension, and every body has a positive one
// (extentStateFloorMetres). Bounds on one half-extent nest: together they say
// h >= the highest of them, once. Under ExtentState that one constraint is
// held as a Gaussian site on the half-extent's marginal, in the manner of
// expectation propagation (Minka 2001). Each refinement divides the old site
// out of the marginal to get the cavity, truncates the cavity below at the
// bound (the moment-matched one-sided truncation), and moves the joint state
// to the truncated moments through the regression on the half-extent, the
// form truncateAxis uses for position. The new site is the truncated moments
// divided by the cavity's. Re-truncating the posterior instead counts its
// Gaussian tail below the last bound as mass the next bound removes: a bound
// rising a millimetre a frame then moves the mean up and halves sigma with
// frame count alone, which Section 9.2.2 forbids. The site is refined before
// the frame's faces, so they see half-extents the bound does not contradict,
// and after them, so no face leaves a half-extent below its floor. With the
// cavity unchanged a refinement changes nothing, whatever the frame count;
// as faces narrow the cavity above the bound, the site's pull fades. The
// floor joins the site only once evidence has updated the half-extent: the
// seed is the class prior as declared.
//
// Full spans. A span is a lower bound only when the frame may have missed an
// end. A frame that measured a side face shows the length whole, since a
// convex body hides none of a face it shows, and one that measured an end
// face shows the width whole (fullSpans): Section 9.2.1 admits such a span as
// a measurement, z = 2h with the span noise. Treated as a bound, a span at
// the true length lifts the mean about 0.8 cavity sigmas above it. A full span
// is one Gaussian site per half-extent, which the next full span replaces
// rather than joins, so repeated views of one span never narrow the extent.
// The prediction's process noise does dilute the old site between frames.
// Once the posterior is wider than one span's noise, the next span adds to
// what is left of it, so sigma settles where the extent's process noise and
// one span's noise balance (0.043 to 0.058 m on the synthetic box pass)
// rather than shrinking with frame count.
//
// CensoredExtent, without ExtentState, reports the class prior truncated
// below at the largest lower bound (censoredDimension). SpanBoundsBelief makes
// the frame's own window-minimum span one of those bounds.
//
// HeadingFaceNoise adds sigma_psi^2 · t_eff^2 to a face's variance, and
// AnisotropicReferenceBias states the medoid's bias as (L/2)^2 along the body
// and (W/2)^2 across it.

import (
	"math"
)

const (
	// extentStateProcessNoise is the half-extents' process noise spectral
	// density under ExtentState, (0.02 m)^2/s per half-extent: a body does not
	// change size, but its membership drifts as returns enter and leave the
	// cluster and an articulated vehicle flexes. It is small on purpose: the
	// extent must not follow a manoeuvre, which goes through the velocity.
	extentStateProcessNoise = 0.02 * 0.02
	// jointStates is the joint state's dimension under ExtentState.
	jointStates = 6
	// spanBoundNoiseMetres is the span bound's sampling noise: the trimmed
	// extreme's spread, the same constant containment's bound noise starts
	// from. The Pandar40P's azimuth step adds in quadrature with range
	// (spanBoundNoise).
	spanBoundNoiseMetres = containmentExtremeNoiseMetres
	// censoredHistoryNoiseMetres is the noise the censored posterior takes
	// off an accumulated belief's bound. The belief keeps only a bin, not the
	// ranges its spans were measured at, so the bound is the bin's lower edge
	// less the extreme's spread.
	censoredHistoryNoiseMetres = containmentExtremeNoiseMetres
	// oneSidedTruncationSigmas is how far above the bound, in sigmas, a
	// one-sided truncation puts its upper limit: far enough that the density
	// beyond it is below float64 precision.
	oneSidedTruncationSigmas = 20
	// extentStateFloorMetres is the least dimension the extent state admits,
	// a tenth of a metre: below any road user it holds a body for (a
	// pedestrian is about a quarter of a metre through), and above the
	// returns' few centimetres of spread. It is a lower bound held in the
	// same site as the span's, not a clamp, so it moves a half-extent
	// appreciably only when the evidence takes it near zero. The class
	// prior's minimum is not used: the rigid-vehicle minimum, 2.4 m, sits
	// within one prior sigma of the mean, so it would truncate every car whose
	// evidence is still short, which is the F0 case the extent state exists
	// to correct, and only the rigid-vehicle class states a minimum.
	extentStateFloorMetres = 0.1
	// negligibleBoundMass is the cavity mass below a bound under which the
	// bound starts no site: float32's epsilon, 2^-23. Below it the
	// truncation moves the half-extent's mean by under a ten-millionth of its
	// sigma, which the stored float32 state cannot hold.
	negligibleBoundMass = 0x1p-23
)

// extentSite is a Gaussian site on one half-extent, the factor a lower bound
// or a full span contributes, in information form: precision and
// precision-weighted mean, so that the posterior marginal is the cavity's
// times exp(-precision h^2/2 + weighted h). Zero is no site.
type extentSite struct {
	precision float64
	weighted  float64
}

// solidBodyExtent is the half-extent block of the solid body's joint state
// under ExtentState. The four-state block is solidBodyTrack.state and p.
type solidBodyExtent struct {
	// on says the block is carried: ExtentState on a shadow body.
	on bool
	// h is [hL, hW] in metres.
	h [2]float32
	// pxh is the covariance between [x, y, vx, vy] and [hL, hW], row-major:
	// pxh[i*2+j] = P[i][4+j].
	pxh [8]float32
	// phh is the half-extents' own covariance, row-major.
	phh [4]float32
	// updates counts, per half-extent, the frames on which a face or a lower
	// bound updated it: the dimension's admissible-frame count.
	updates [2]int
	// appliedBound is, per dimension, the highest full-dimension lower bound
	// admitted from a frame's span.
	appliedBound [2]float32
	// site is, per half-extent, the Gaussian site of its lower bound: half
	// the higher of appliedBound and extentStateFloorMetres.
	site [2]extentSite
	// spanSite is, per half-extent, the Gaussian site of the latest full
	// span (fullSpans): one frame's measurement, replaced by the next, so the
	// spans never count more than once whatever the frame count.
	spanSite [2]extentSite
}

// finite says every value in the block is finite.
func (e *solidBodyExtent) finite() bool {
	for _, v := range e.h {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	for _, v := range e.phh {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	for _, v := range e.pxh {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	for i := range e.site {
		if !e.site[i].finite() || !e.spanSite[i].finite() {
			return false
		}
	}
	return true
}

// finite says both of the site's terms are finite.
func (s extentSite) finite() bool {
	return !math.IsNaN(s.precision) && !math.IsInf(s.precision, 0) && !math.IsNaN(s.weighted) && !math.IsInf(s.weighted, 0)
}

// seedSolidBodyExtent starts the extent block from the extent beliefs when
// they have evidence, and from the class prior otherwise: each half-extent at
// half the dimension with half its sigma, uncorrelated with the position.
func seedSolidBodyExtent(sb *solidBodyTrack, prior classDimensionPrior) {
	length := dimensionFromBelief(sb.lengthBelief, prior.lengthMetres, prior.sigmaMetres)
	width := dimensionFromBelief(sb.widthBelief, prior.widthMetres, prior.sigmaMetres)
	sl, sw := length.SigmaMetres/2, width.SigmaMetres/2
	sb.ext = solidBodyExtent{
		on:  true,
		h:   [2]float32{length.Metres / 2, width.Metres / 2},
		phh: [4]float32{sl * sl, 0, 0, sw * sw},
	}
}

// extentIndex is the half-extent a face measures: hL for an end face, hW for
// a side face.
func extentIndex(f BodyFace) int {
	if f.IsLongitudinal() {
		return 0
	}
	return 1
}

// extentDimension is a half-extent as a dimension belief: twice the state,
// with twice its standard deviation, accumulated once a face or a bound has
// updated it and the class prior before.
func (sb *solidBodyTrack) extentDimension(i int) DimensionBelief {
	v := math.Max(float64(sb.ext.phh[i*2+i]), 0)
	d := DimensionBelief{
		Metres:      2 * sb.ext.h[i],
		SigmaMetres: float32(2 * math.Sqrt(v)),
		Provenance:  ProvenanceClassPrior,
	}
	if n := sb.ext.updates[i]; n > 0 {
		d.Provenance, d.AdmissibleFrames = ProvenanceAccumulated, n
	}
	return d
}

// extentStateEdges is every face MeasureNearEdge found, with the half-extent
// each carries set to the state's: under ExtentState no face is excluded for
// a class-prior extent, and the implied centre offset (ImpliedCentreOffset),
// which translateToFaces and the application record read, is the plane less
// the state's half-extent.
func (sb *solidBodyTrack) extentStateEdges(edges []EdgeMeasurement) []EdgeMeasurement {
	out := make([]EdgeMeasurement, len(edges))
	for i, e := range edges {
		e.HalfExtentMetres = sb.ext.h[extentIndex(e.Face)]
		out[i] = e
	}
	return out
}

// symmetriseFour replaces a four-state covariance with the mean of it and its
// transpose. The constant-velocity propagation is symmetric in exact
// arithmetic but rounds its two triangles differently in float32; under
// ExtentState the joint matrix is held exactly symmetric.
func symmetriseFour(p *[16]float32) {
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			v := float32((float64(p[i*4+j]) + float64(p[j*4+i])) / 2)
			p[i*4+j], p[j*4+i] = v, v
		}
	}
}

// predictExtent carries the extent block across dt: the transition is the
// constant-velocity one on [x, y, vx, vy] and the identity on the
// half-extents, so the cross block's position rows gain dt times its velocity
// rows, and the half-extents gain their process noise.
func (sb *solidBodyTrack) predictExtent(dt float32) {
	for j := 0; j < 2; j++ {
		sb.ext.pxh[0*2+j] += dt * sb.ext.pxh[2*2+j]
		sb.ext.pxh[1*2+j] += dt * sb.ext.pxh[3*2+j]
	}
	q := float32(extentStateProcessNoise) * dt
	sb.ext.phh[0] += q
	sb.ext.phh[3] += q
}

// scaleCappedRows keeps the joint covariance positive semi-definite where the
// tracked filter's diagonal cap reduced a variance: the capped state's row
// and column are scaled by the ratio of the capped to the uncapped standard
// deviation, P <- D P D, instead of leaving the off-diagonals as they were.
// It runs only under ExtentState; the four-state paths keep their cap.
func (sb *solidBodyTrack) scaleCappedRows(uncapped [4]float32) {
	var d [4]float64
	scaled := false
	for i := range d {
		d[i] = 1
		if c, u := sb.p[i*4+i], uncapped[i]; c < u && u > 0 {
			d[i] = math.Sqrt(float64(c) / float64(u))
			scaled = true
		}
	}
	if !scaled {
		return
	}
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			if i != j {
				sb.p[i*4+j] = float32(float64(sb.p[i*4+j]) * d[i] * d[j])
			}
		}
		for j := 0; j < 2; j++ {
			sb.ext.pxh[i*2+j] = float32(float64(sb.ext.pxh[i*2+j]) * d[i])
		}
	}
}

// jointState is the solid body's joint state under ExtentState, in float64:
// x = [x, y, vx, vy, hL, hW] and its covariance, row-major.
type jointState struct {
	x [jointStates]float64
	p [jointStates * jointStates]float64
}

// jointOf assembles the joint state from a four-state block and the body's
// extent block.
func (sb *solidBodyTrack) jointOf(state [4]float32, p [16]float32) jointState {
	var js jointState
	for i := 0; i < 4; i++ {
		js.x[i] = float64(state[i])
		for j := 0; j < 4; j++ {
			js.p[i*jointStates+j] = float64(p[i*4+j])
		}
		for j := 0; j < 2; j++ {
			v := float64(sb.ext.pxh[i*2+j])
			js.p[i*jointStates+4+j] = v
			js.p[(4+j)*jointStates+i] = v
		}
	}
	for i := 0; i < 2; i++ {
		js.x[4+i] = float64(sb.ext.h[i])
		for j := 0; j < 2; j++ {
			js.p[(4+i)*jointStates+4+j] = float64(sb.ext.phh[i*2+j])
		}
	}
	return js
}

// setJoint writes a joint state back to the body's two blocks, symmetrising
// the covariance first so the stored blocks describe one symmetric matrix.
func (sb *solidBodyTrack) setJoint(js jointState) {
	js.symmetrise()
	for i := 0; i < 4; i++ {
		sb.state[i] = float32(js.x[i])
		for j := 0; j < 4; j++ {
			sb.p[i*4+j] = float32(js.p[i*jointStates+j])
		}
		for j := 0; j < 2; j++ {
			sb.ext.pxh[i*2+j] = float32(js.p[i*jointStates+4+j])
		}
	}
	for i := 0; i < 2; i++ {
		sb.ext.h[i] = float32(js.x[4+i])
		for j := 0; j < 2; j++ {
			sb.ext.phh[i*2+j] = float32(js.p[(4+i)*jointStates+4+j])
		}
	}
}

// symmetrise replaces the covariance with the mean of it and its transpose.
func (js *jointState) symmetrise() {
	for i := 0; i < jointStates; i++ {
		for j := i + 1; j < jointStates; j++ {
			v := (js.p[i*jointStates+j] + js.p[j*jointStates+i]) / 2
			js.p[i*jointStates+j], js.p[j*jointStates+i] = v, v
		}
	}
}

// scalarUpdate is scalarUpdateNIS without the NIS.
func (js *jointState) scalarUpdate(h [jointStates]float64, z, r float64) bool {
	_, ok := js.scalarUpdateNIS(h, z, r)
	return ok
}

// scalarUpdateNIS applies one scalar measurement z = h·x + v, v ~ N(0, r), to
// the joint state: scalarPositionUpdate generalised to any row h over the n
// states, in the Joseph form, P' = (I - k hᵀ) P (I - k hᵀ)ᵀ + r k kᵀ, which
// keeps P symmetric positive semi-definite under rounding. It returns the
// update's NIS, and false without changing anything when the innovation
// variance is not positive.
func (js *jointState) scalarUpdateNIS(h [jointStates]float64, z, r float64) (float64, bool) {
	const n = jointStates
	// ph = P h, and hp = hᵀ P, which equal for a symmetric P but are kept
	// apart so the update does not assume it.
	var ph, hp [n]float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			ph[i] += js.p[i*n+j] * h[j]
			hp[i] += h[j] * js.p[j*n+i]
		}
	}
	var s, predicted float64
	for i := 0; i < n; i++ {
		s += h[i] * ph[i]
		predicted += h[i] * js.x[i]
	}
	s += r
	if !(s > 0) || math.IsInf(s, 0) {
		return 0, false
	}
	innovation := z - predicted
	var k [n]float64
	for i := range k {
		k[i] = ph[i] / s
		js.x[i] += k[i] * innovation
	}
	// A P with A = I - k hᵀ is P - k hp; (A P) Aᵀ = A P - (A P h) kᵀ.
	var ap [n * n]float64
	var aph [n]float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			v := js.p[i*n+j] - k[i]*hp[j]
			ap[i*n+j] = v
			aph[i] += v * h[j]
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			js.p[i*n+j] = ap[i*n+j] - aph[i]*k[j] + r*k[i]*k[j]
		}
	}
	js.symmetrise()
	return innovation * innovation / s, true
}

// truncate holds the linear functional g·x of the joint state to
// [g·x̂ + a, g·x̂ + b]: the marginal of g·x is truncated to the interval and
// replaces the prior marginal through the state's regression on it, as
// truncateAxis does for position. lowerOnly makes the interval [a, inf). It
// returns the move of g·x and false when it did nothing.
func (js *jointState) truncate(g [jointStates]float64, a, b float64, lowerOnly bool) (float64, bool) {
	const n = jointStates
	var pg [n]float64
	var sigma2 float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			pg[i] += js.p[i*n+j] * g[j]
		}
		sigma2 += g[i] * pg[i]
	}
	if !(sigma2 > 0) || math.IsInf(sigma2, 0) {
		return 0, false
	}
	sigma := math.Sqrt(sigma2)
	var mean, variance float64
	if lowerOnly {
		mean, variance = lowerTruncatedMoments(0, sigma, a)
	} else {
		var ok bool
		mean, variance, ok = truncatedNormalMoments(0, sigma, a, b)
		if !ok {
			return 0, false
		}
	}
	variance = math.Max(variance, containmentVarianceFloor)
	js.replaceMarginal(pg, sigma2, mean, variance)
	return mean, true
}

// replaceMarginal replaces the marginal of a linear functional g·x, whose
// prior variance is sigma2 and whose covariance with the state is pg = P g,
// by one moved by shift with the given variance, through the state's
// regression on it: the conditional of x given g·x is kept. A scalar site on
// g·x changes only that marginal, so this is also how a site is replaced.
func (js *jointState) replaceMarginal(pg [jointStates]float64, sigma2, shift, variance float64) {
	const n = jointStates
	var k [n]float64
	for i := range k {
		k[i] = pg[i] / sigma2
		js.x[i] += k[i] * shift
	}
	d := variance - sigma2
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			js.p[i*n+j] += d * k[i] * k[j]
		}
		if js.p[i*n+i] < 0 {
			js.p[i*n+i] = 0
		}
	}
	js.symmetrise()
}

// lowerTruncatedMoments is the mean and variance of N(mu, sigma^2) truncated
// below at lower. It reads the upper tail through erfc, which keeps its
// precision where 1 - Phi does not, and past the tail's underflow uses the
// Mills-ratio asymptote, mean lower + sigma^2/(lower - mu) and standard
// deviation sigma^2/(lower - mu).
func lowerTruncatedMoments(mu, sigma, lower float64) (mean, variance float64) {
	alpha := (lower - mu) / sigma
	tail := 0.5 * math.Erfc(alpha/math.Sqrt2)
	if !(tail > 1e-300) {
		excess := lower - mu
		m := sigma * sigma / excess
		return lower + m, m * m
	}
	phi := math.Exp(-alpha*alpha/2) / math.Sqrt(2*math.Pi)
	lambda := phi / tail
	mean = mu + sigma*lambda
	variance = sigma * sigma * (1 + alpha*lambda - lambda*lambda)
	if !(variance > 0) {
		// Cancellation far in the tail: the asymptote.
		m := sigma * sigma / math.Max(lower-mu, sigma)
		variance = m * m
	}
	return mean, variance
}

// applyEdgeMeasurementsJoint is applyEdgeMeasurements on the joint state:
// each face measures its plane offset, z = n·c + h, with the face's
// half-extent in H rather than subtracted from z. heading, when not nil, adds
// HeadingFaceNoise's variance per face. Nothing is changed unless every face
// applied.
func (t *Tracker) applyEdgeMeasurementsJoint(js *jointState, edges []EdgeMeasurement, heading []float64) (float32, VisibleFaces, bool) {
	var nis float64
	var faces VisibleFaces
	next := *js
	for i, e := range edges {
		r := float64(t.Config.MeasurementNoise)
		if heading != nil {
			r += heading[i]
		}
		var h [jointStates]float64
		h[0], h[1] = float64(e.NormalX), float64(e.NormalY)
		h[4+extentIndex(e.Face)] = 1
		term, ok := next.scalarUpdateNIS(h, float64(e.PlaneOffsetMetres), r)
		if !ok {
			return 0, 0, false
		}
		nis += term
		faces |= faceBit(e.Face)
	}
	*js = next
	return float32(nis), faces, true
}

// containmentTruncationJoint is containmentTruncation on the joint state.
// The interval is truncateAxis's, with the dimension's upper tail read from
// the state (f's beliefs are the state's under ExtentState: 2h and 2 sigma_h),
// and the truncation moves every state, the half-extents included, by its
// covariance with the position along the direction.
func (t *Tracker) containmentTruncationJoint(js *jointState, f nearEdgeFrame, cluster WorldCluster, alongOpen, acrossOpen bool) (along, across float32) {
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport || f.length.Metres <= 0 || f.width.Metres <= 0 {
		return 0, 0
	}
	scratch := make([]float64, len(points))
	c, s := math.Cos(float64(f.axis)), math.Sin(float64(f.axis))
	rangeMetres := math.Hypot(js.x[0]-float64(t.Config.SolidBody.SensorX), js.x[1]-float64(t.Config.SolidBody.SensorY))
	axis := func(ux, uy float64, belief DimensionBelief) float32 {
		a, b, ok := containmentInterval(points, scratch, ux, uy, ux*js.x[0]+uy*js.x[1], belief, rangeMetres)
		if !ok {
			return 0
		}
		move, ok := js.truncate([jointStates]float64{ux, uy}, a, b, false)
		if !ok {
			return 0
		}
		return float32(move)
	}
	if alongOpen {
		along = axis(c, s, f.length)
	}
	if acrossOpen {
		across = axis(-s, c, f.width)
	}
	return along, across
}

// takesSpanBounds says whether this frame's spans are taken as lower bounds:
// under SpanBoundsBelief, and under ExtentState with CensoredExtent, where
// they are the censored evidence the state truncates on.
func (t *Tracker) takesSpanBounds(sb *solidBodyTrack) bool {
	return t.Config.SolidBody.SpanBoundsBelief || (sb.ext.on && t.Config.SolidBody.CensoredExtent)
}

// frameSpanBounds is the frame's lower bounds on length and width: the
// window-minimum spans along and across the face axis (windowMinimumSpans,
// the value containment's floor reports), less their noise. The admission
// rules that make a span a lower bound apply (Section 9.2.1): none on a merge
// candidate, whose span is two bodies'; none while the estimation lifecycle is
// initialising; and none along an axis the course contradicts
// (spanAxisBelieved). Zero is no bound. It also returns the spans themselves
// and their noise, which ExtentState reads as a measurement where the frame
// shows the dimension whole (fullSpans).
func (t *Tracker) frameSpanBounds(sb *solidBodyTrack, cluster WorldCluster, axis float32, axisIsCourse, mergeCandidate bool) (bounds, spans [2]float32, noise float32) {
	if mergeCandidate || sb.estimation == EstimationInitialising || !spanAxisBelieved(sb, axis, axisIsCourse) {
		return bounds, spans, 0
	}
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport {
		return bounds, spans, 0
	}
	along, across := windowMinimumSpans(points, float64(axis))
	e := spanBoundNoise(math.Hypot(float64(sb.state[0]-t.Config.SolidBody.SensorX), float64(sb.state[1]-t.Config.SolidBody.SensorY)))
	spans = [2]float32{along, across}
	for i, span := range spans {
		if b := float64(span) - e; b > 0 {
			bounds[i] = float32(b)
		}
	}
	return bounds, spans, float32(e)
}

// spanBoundNoise is a span bound's noise at a range: the extreme's spread and
// the azimuth step's spacing in quadrature, containment's bound noise without
// its shortfall term.
func spanBoundNoise(rangeMetres float64) float64 {
	spacing := rangeMetres * containmentAngularSpacingRad
	return math.Sqrt(spanBoundNoiseMetres*spanBoundNoiseMetres + spacing*spacing)
}

// fullSpans says which of the frame's spans run the dimension's whole
// extent: the length when the frame measured a side face, which lies along
// it, and the width when it measured an end face. A convex body hides none of
// a face it shows, so that face's returns reach the body's extreme at each
// end: reliable physical endpoints, which Section 9.2.1 admits as a
// measurement. Any other span is a lower bound. An external occlusion that
// cuts the face short is not detected, as the tracker detects no truncation.
func fullSpans(edges []EdgeMeasurement) [2]bool {
	var full [2]bool
	for _, e := range edges {
		if e.Face.IsLongitudinal() {
			full[1] = true
		} else {
			full[0] = true
		}
	}
	return full
}

// applyExtentSpans is the frame's span evidence on the half-extents under
// ExtentState, before its faces: a full span replaces the dimension's span
// site, and the others are lower bounds (applyExtentBounds). It says which
// half-extents the evidence updated.
func (sb *solidBodyTrack) applyExtentSpans(f nearEdgeFrame) [2]bool {
	bounds := f.spanBound
	var touched [2]bool
	full := fullSpans(f.edges.Edges)
	for i := range bounds {
		if !full[i] || !(f.span[i] > 0) || !(f.spanNoise > 0) {
			continue
		}
		// A full span is a measurement of the dimension, not a bound on it.
		bounds[i] = 0
		half, sd := float64(f.span[i])/2, float64(f.spanNoise)/2
		touched[i] = sb.refineSite(i, &sb.ext.spanSite[i], func(cm, cv float64) (float64, float64, bool) {
			r := sd * sd
			tv := cv * r / (cv + r)
			return tv * (cm/cv + half/r), tv, true
		})
	}
	bounded := sb.applyExtentBounds(bounds)
	return [2]bool{touched[0] || bounded[0], touched[1] || bounded[1]}
}

// applyExtentBounds admits the frame's lower bounds and refines each
// half-extent's bound site, and says which half-extents a newly admitted
// bound updated. A bound is admitted when it is higher than every bound
// before it; a lower or repeated one says nothing the site does not already
// hold. The site is refined on every call once it holds a bound or the
// half-extent has evidence, so a half-extent that has fallen toward zero is
// held above the floor (see the file comment).
func (sb *solidBodyTrack) applyExtentBounds(bounds [2]float32) [2]bool {
	var touched [2]bool
	for i, b := range bounds {
		raised := b > 0 && b > sb.ext.appliedBound[i]
		highest := sb.ext.appliedBound[i]
		if raised {
			highest = b
		}
		if !raised && !sb.boundBinds(i, false) {
			continue
		}
		if !sb.refineBoundSite(i, halfExtentLowerBound(highest)) || !raised {
			continue
		}
		sb.ext.appliedBound[i] = b
		touched[i] = true
	}
	return touched
}

// holdExtentBounds refines both bound sites at the bounds already admitted,
// after a frame's faces, which can move a half-extent anywhere their gain
// takes it. touched names the half-extents this frame's evidence updated.
func (sb *solidBodyTrack) holdExtentBounds(touched [2]bool) {
	for i := range sb.ext.site {
		if sb.boundBinds(i, touched[i]) {
			sb.refineBoundSite(i, halfExtentLowerBound(sb.ext.appliedBound[i]))
		}
	}
}

// boundBinds says whether half-extent i's bound site is refined: once a span
// bound has been admitted, or evidence has updated the half-extent (now says
// this frame's did). Until then the half-extent is the class prior as
// declared, Gaussian, and the floor does not truncate it: the floor is a
// constraint on what the evidence makes of the half-extent, applied after
// each update as the span bounds are.
func (sb *solidBodyTrack) boundBinds(i int, now bool) bool {
	return now || sb.ext.appliedBound[i] > 0 || sb.ext.updates[i] > 0 || sb.ext.site[i].precision > 0
}

// halfExtentLowerBound is a half-extent's lower bound given the highest
// admitted full-dimension bound: half of it, or half the floor.
func halfExtentLowerBound(bound float32) float64 {
	return math.Max(float64(bound), extentStateFloorMetres) / 2
}

// refineBoundSite is one expectation-propagation step for half-extent i's
// lower bound: the target is the cavity truncated below at lower. A bound
// that holds no site yet and would cut off less than negligibleBoundMass of
// the cavity starts none: what it would change is below float32's resolution
// of the half-extent, and a site of that size is rounding, not evidence.
func (sb *solidBodyTrack) refineBoundSite(i int, lower float64) bool {
	open := sb.ext.site[i].precision == 0
	return sb.refineSite(i, &sb.ext.site[i], func(cm, cv float64) (float64, float64, bool) {
		sd := math.Sqrt(cv)
		below := 0.5 * math.Erfc((cm-lower)/(sd*math.Sqrt2))
		if open && below < negligibleBoundMass {
			return 0, 0, false
		}
		tm, tv := lowerTruncatedMoments(cm, sd, lower)
		return tm, math.Max(tv, containmentVarianceFloor), true
	})
}

// refineSite replaces one Gaussian site on half-extent i: the cavity is the
// marginal with the old site divided out, target gives the new marginal from
// the cavity, and the joint state is moved from its marginal to the target
// through the regression on h, which keeps every other state's conditional
// on h. The new site is the target divided by the cavity. It returns false,
// changing nothing, when the marginal is degenerate or the target would not
// narrow the cavity.
//
// The site was fitted at an earlier frame and the prediction has since added
// process noise, so dividing it out is the filtering approximation. Where the
// posterior has grown wider than the site alone, the site's information has
// diffused out of the state, and the posterior stands in for the cavity.
func (sb *solidBodyTrack) refineSite(i int, site *extentSite, target func(cm, cv float64) (float64, float64, bool)) bool {
	const n = jointStates
	js := sb.jointOf(sb.state, sb.p)
	k := 4 + i
	m, v := js.x[k], js.p[k*n+k]
	if !(v > 0) || math.IsInf(v, 0) {
		return false
	}
	cm, cv := m, v
	if c := 1/v - site.precision; site.precision > 0 && c > 0 {
		cv = 1 / c
		cm = cv * (m/v - site.weighted)
	}
	tm, tv, ok := target(cm, cv)
	if !ok || !(tv < cv) || math.IsNaN(tm) || math.IsInf(tm, 0) {
		return false
	}
	var pg [n]float64
	for j := 0; j < n; j++ {
		pg[j] = js.p[j*n+k]
	}
	js.replaceMarginal(pg, v, tm-m, tv)
	sb.setJoint(js)
	*site = extentSite{precision: 1/tv - 1/cv, weighted: tm/tv - cm/cv}
	if !(site.precision > 0) {
		// The target equals the cavity to float64 precision: no site.
		*site = extentSite{}
	}
	return true
}

// censoredDimension is CensoredExtent's dimension: the class prior
// N(mu0, sigma0^2) truncated below at the largest lower bound the evidence
// gives, with the truncated normal's mean and standard deviation. The bounds
// are the frame's (bound, already less its noise) and the accumulated
// belief's corroborated maximum, read at its bin's lower edge less
// censoredHistoryNoiseMetres. The corroborated maximum stands in for the
// largest admitted span because the raw maximum is the one-way ratchet
// Section 9.2.3 rules out: one merged frame would hold the bound for the
// rest of the track. Lower bounds alone never narrow the posterior below
// what the highest of them implies, whatever the frame count. A conflict
// widens it, as extentBeliefSigma's does, up to the prior's sigma.
// ExtentPriorFloor and VehicleExtentFloor do not apply: the posterior's mean
// already lies at or above the prior's.
func censoredDimension(b extentBelief, priorMetres, priorSigma, bound float32) DimensionBelief {
	d := DimensionBelief{Metres: priorMetres, SigmaMetres: priorSigma, Provenance: ProvenanceClassPrior}
	if b.Support > 0 {
		d.Provenance, d.AdmissibleFrames = ProvenanceAccumulated, b.Support
	}
	lower := float64(bound)
	if estimate := b.Estimate(); b.Support > 0 && estimate > 0 {
		lower = math.Max(lower, float64(estimate)-extentBeliefBinMetres/2-censoredHistoryNoiseMetres)
	}
	if !(lower > 0) || !(priorSigma > 0) {
		return d
	}
	mean, variance := lowerTruncatedMoments(float64(priorMetres), float64(priorSigma), lower)
	sigma := math.Sqrt(variance)
	if b.Conflicts > 0 {
		sigma = math.Min(sigma*float64(1+b.Conflicts), float64(priorSigma))
	}
	d.Metres, d.SigmaMetres = float32(mean), float32(sigma)
	return d
}

// headingFaceVariance is HeadingFaceNoise's added variance per face,
// sigma_psi^2 · t_eff^2, appended to buf; nil with the option off.
func headingFaceVariance(on bool, edges []EdgeMeasurement, varianceRad2 float32, buf []float64) []float64 {
	if !on {
		return nil
	}
	for _, e := range edges {
		lever := float64(e.TangentialLeverArmMetres)
		buf = append(buf, float64(varianceRad2)*lever*lever)
	}
	return buf
}

// faceAxisVariance is the orientation variance of the axis the faces are
// measured along: the orientation belief's, or the course's when faceAxis
// took the course instead (CourseAlignedFaces).
func faceAxisVariance(sb *solidBodyTrack, axis float32) float32 {
	if axis != sb.orientation.PsiRad {
		if course, ok := courseOrientation(sb.state, sb.p); ok {
			return course.VarianceRad2
		}
	}
	return sb.orientation.VarianceRad2
}

// anisotropicReferenceBias is AnisotropicReferenceBias's position inflation
// for a body whose axis is psi: R diag((L/2)^2, (W/2)^2) Rᵀ from the class
// prior, as [xx, xy, yy]. The medoid sits on the sensor-facing surface, which
// along the body can be an end face half a length from the centre.
func anisotropicReferenceBias(prior classDimensionPrior, psi float64) [3]float32 {
	a := float64(prior.lengthMetres) / 2
	b := float64(prior.widthMetres) / 2
	c, s := math.Cos(psi), math.Sin(psi)
	a2, b2 := a*a, b*b
	return [3]float32{
		float32(a2*c*c + b2*s*s),
		float32((a2 - b2) * c * s),
		float32(a2*s*s + b2*c*c),
	}
}

// isotropicReferenceBias is the inflation without a resolved axis: the
// larger half-extent squared on both axes.
func isotropicReferenceBias(prior classDimensionPrior) float32 {
	h := prior.lengthMetres
	if prior.widthMetres > h {
		h = prior.widthMetres
	}
	return (h / 2) * (h / 2)
}

// addReferenceBias adds [xx, xy, yy] to a four-state covariance's position
// block.
func addReferenceBias(p *[16]float32, b [3]float32) {
	p[0*4+0] += b[0]
	p[0*4+1] += b[1]
	p[1*4+0] += b[1]
	p[1*4+1] += b[2]
}
