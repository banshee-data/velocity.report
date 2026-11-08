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
// Lower bounds. A frame's span along a body axis, less its noise, is a lower
// bound b on the dimension, so the half-extent h = gᵀx satisfies h ≥ l = b/2.
// Under ExtentState the inequality is imposed on the joint Gaussian N(x̂, P)
// by estimate projection: when gᵀx̂ < l the mean moves to the nearest point of
// {gᵀx ≥ l} in the P⁻¹ metric,
//
//	x̂' = x̂ + P g (l − gᵀx̂) / (gᵀ P g),
//
// which is the joint regression on h, puts h exactly at l, and is the mode of
// the Gaussian truncated at the bound; when gᵀx̂ ≥ l nothing moves. P is left
// as it is. Both half-extents' constraints together are the two-dimensional
// problem min (h − ĥ)ᵀ P_hh⁻¹ (h − ĥ) subject to h ≥ l, solved by its active
// set, and every state moves by P_xh λ with λ ≥ 0 its multipliers
// (holdHalfExtentsAbove).
//
// The bound was first a moment-matched truncation, mean μ + σλ(α) with
// λ(α) = φ(α)/(1 − Φ(α)) and α = (l − μ)/σ, and that overshot. Its mean lands
// past the bound by σλ(α): on the box pass of
// TestSpanBoundsDoNotCarryTheExtentStatePastTheFaces the first frame's
// bound, l = 2.186 m, took hL from 1.562 m (sigma 0.75 m) to 2.606 m against a
// true 2.25 m: a 5.21 m length against the 4.494 m span the frame observed. And
// the Gaussian it returns has mass below the bound again, so the next frame's
// bound, from the same returns a few millimetres longer, truncated it again as
// if it were independent evidence: a 5 mm rise in l moved hL 78 mm and shrank
// its sigma from 0.294 to 0.237 m. That is an assumed-density filter applying
// a correlated sequence of bounds as independent ones, and it ended the pass
// at hL 2.457 m with a departure error of 0.190 m, against 2.214 m and
// 0.054 m without the bounds. The projection is idempotent: any sequence of
// bounds leaves the mean where the highest alone puts it, and since P is
// unchanged no number of bounds narrows the posterior, which is Section
// 9.2.2's rule. Leaving P is conservative, as a truncation only ever narrows
// a Gaussian. What it gives up is the bound's narrowing of sigma: only faces
// narrow it.
//
// Floor. Nothing else keeps a half-extent positive. A face measures n·c + h,
// so a face whose plane lies far inboard of the prediction (a cluster that
// has shed its far returns, or a face of the wrong identity) moves h by its
// gain, through zero, and the medoid update and the speed clamp move it by its
// covariance with the position and velocity. (A negative 2h was published as
// 0 m: assembleSolidBody raises the length to the containment floor, which is
// zero when containment is off.) Every step that moves the half-extents
// therefore ends with the same projection at extentStateMinHalfMetres:
// holdHalfExtentsAbove(extentFloor) on a jointState, holdExtentFloor where
// the blocks are written in place. A frame's bounds are taken together with
// it (applyExtentBounds).
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
	// extentStateMinHalfMetres is the half-extents' floor under ExtentState:
	// half the extent belief's own floor, below which a return is noise
	// (extentBeliefMinMetres).
	extentStateMinHalfMetres = extentBeliefMinMetres / 2
)

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
	// bound updated it: the dimension's admissible-frame count. A bound counts
	// only on a frame it moved the half-extent.
	updates [2]int
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
	return true
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
	scaleCappedJoint(&sb.p, &sb.ext, uncapped)
}

// scaleCappedJoint is scaleCappedRows on a given four-state block and extent
// block: the shadow's own, or under NearEdgeTracking the tracked filter's P
// and the body's extent block.
func scaleCappedJoint(p *[16]float32, ext *solidBodyExtent, uncapped [4]float32) {
	var d [4]float64
	scaled := false
	for i := range d {
		d[i] = 1
		if c, u := p[i*4+i], uncapped[i]; c < u && u > 0 {
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
				p[i*4+j] = float32(float64(p[i*4+j]) * d[i] * d[j])
			}
		}
		for j := 0; j < 2; j++ {
			ext.pxh[i*2+j] = float32(float64(ext.pxh[i*2+j]) * d[i])
		}
	}
}

// extentBlock is the body's extent block when it carries one, and nil
// otherwise, for the helpers that take the block optionally.
func (sb *solidBodyTrack) extentBlock() *solidBodyExtent {
	if !sb.ext.on {
		return nil
	}
	return &sb.ext
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
	return jointFrom(state, p, &sb.ext)
}

// jointFrom assembles the joint state from a four-state block and an extent
// block.
func jointFrom(state [4]float32, p [16]float32, ext *solidBodyExtent) jointState {
	var js jointState
	for i := 0; i < 4; i++ {
		js.x[i] = float64(state[i])
		for j := 0; j < 4; j++ {
			js.p[i*jointStates+j] = float64(p[i*4+j])
		}
		for j := 0; j < 2; j++ {
			v := float64(ext.pxh[i*2+j])
			js.p[i*jointStates+4+j] = v
			js.p[(4+j)*jointStates+i] = v
		}
	}
	for i := 0; i < 2; i++ {
		js.x[4+i] = float64(ext.h[i])
		for j := 0; j < 2; j++ {
			js.p[(4+i)*jointStates+4+j] = float64(ext.phh[i*2+j])
		}
	}
	return js
}

// writeJointMean writes a joint state's mean, and only its mean, back to a
// four-state block and an extent block: the write a projection needs, which
// leaves the covariance as it was.
func writeJointMean(state *[4]float32, ext *solidBodyExtent, js jointState) {
	for i := 0; i < 4; i++ {
		state[i] = float32(js.x[i])
	}
	for i := 0; i < 2; i++ {
		ext.h[i] = float32(js.x[4+i])
	}
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

// applyMedoidGain carries a two-dimensional position update of the
// four-state filter, with gain K (4x2, row-major) and inverse innovation
// covariance invS (2x2), to the extent block: the joint update the tracked
// filter's medoid update is the four-state part of. With A = H P_xh, the
// position rows of the cross block, the half-extents' gain is
// K_h = Aᵀ S⁻¹, and
//
//	h ← h + K_h y,   P_hh ← P_hh − K_h A,   P_xh ← P_xh − K A,
//
// the optimal-gain update's extent rows and columns, which is what the Joseph
// form gives at that gain too. P_hh is symmetrised after, as the joint matrix
// is held exactly symmetric.
func (e *solidBodyExtent) applyMedoidGain(k [8]float32, invS [4]float32, yX, yY float32) {
	var a [4]float64
	for i := range a {
		a[i] = float64(e.pxh[i])
	}
	y := [2]float64{float64(yX), float64(yY)}
	var kh [4]float64 // kh[j*2+r]
	for j := 0; j < 2; j++ {
		for r := 0; r < 2; r++ {
			kh[j*2+r] = a[0*2+j]*float64(invS[0*2+r]) + a[1*2+j]*float64(invS[1*2+r])
		}
		e.h[j] = float32(float64(e.h[j]) + kh[j*2+0]*y[0] + kh[j*2+1]*y[1])
	}
	var phh [4]float64
	for j := 0; j < 2; j++ {
		for l := 0; l < 2; l++ {
			phh[j*2+l] = float64(e.phh[j*2+l]) - (kh[j*2+0]*a[0*2+l] + kh[j*2+1]*a[1*2+l])
		}
	}
	off := (phh[1] + phh[2]) / 2
	e.phh = [4]float32{float32(math.Max(phh[0], 0)), float32(off), float32(off), float32(math.Max(phh[3], 0))}
	for i := 0; i < 4; i++ {
		for l := 0; l < 2; l++ {
			v := float64(e.pxh[i*2+l]) - (float64(k[i*2+0])*a[0*2+l] + float64(k[i*2+1])*a[1*2+l])
			e.pxh[i*2+l] = float32(v)
		}
	}
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
	var k [n]float64
	for i := range k {
		k[i] = pg[i] / sigma2
		js.x[i] += k[i] * mean
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
	return mean, true
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
		h, z, r := t.faceMeasurementRow(e, true, nil, heading, i)
		term, ok := next.scalarUpdateNIS(h, z, r)
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
// (spanAxisBelieved). Zero is no bound.
func (t *Tracker) frameSpanBounds(sb *solidBodyTrack, cluster WorldCluster, axis float32, axisIsCourse, mergeCandidate bool) [2]float32 {
	var out [2]float32
	if mergeCandidate || sb.estimation == EstimationInitialising || !spanAxisBelieved(sb, axis, axisIsCourse) {
		return out
	}
	points := nearEdgePoints(cluster)
	if len(points) < DefaultMinFaceSupport {
		return out
	}
	along, across := windowMinimumSpans(points, float64(axis))
	noise := spanBoundNoise(math.Hypot(float64(sb.state[0]-t.Config.SolidBody.SensorX), float64(sb.state[1]-t.Config.SolidBody.SensorY)))
	for i, span := range [2]float32{along, across} {
		if b := float64(span) - noise; b > 0 {
			out[i] = float32(b)
		}
	}
	return out
}

// spanBoundNoise is a span bound's noise at a range: the extreme's spread and
// the azimuth step's spacing in quadrature, containment's bound noise without
// its shortfall term.
func spanBoundNoise(rangeMetres float64) float64 {
	spacing := rangeMetres * containmentAngularSpacingRad
	return math.Sqrt(spanBoundNoiseMetres*spanBoundNoiseMetres + spacing*spacing)
}

// applyExtentBounds holds each half-extent at or above half its dimension's
// bound and at or above extentStateMinHalfMetres, by projection (see the file
// comment), and says which half-extents a bound, rather than the floor, moved.
// The covariance is not changed, so a bound repeated or a millimetre higher on
// the next frame cannot narrow it, and moves the mean only as far as the
// higher bound requires.
func (sb *solidBodyTrack) applyExtentBounds(bounds [2]float32) [2]bool {
	lower := [2]float64{extentStateMinHalfMetres, extentStateMinHalfMetres}
	for i, b := range bounds {
		if b > 0 {
			lower[i] = math.Max(lower[i], float64(b)/2)
		}
	}
	js := sb.jointOf(sb.state, sb.p)
	active := js.holdHalfExtentsAbove(lower)
	if active == ([2]bool{}) {
		return active
	}
	writeJointMean(&sb.state, &sb.ext, js)
	var touched [2]bool
	for i, a := range active {
		touched[i] = a && bounds[i] > 0 && lower[i] == float64(bounds[i])/2
	}
	return touched
}

// extentFloor is the half-extents' lower limits with no frame bound.
var extentFloor = [2]float64{extentStateMinHalfMetres, extentStateMinHalfMetres}

// holdExtentFloor holds an extent block's half-extents at or above
// extentStateMinHalfMetres, for a joint state whose four-state block is
// (x, p): the steps that write the blocks in place rather than through a
// jointState (the tracked medoid update and the consistent speed clamp). Only
// the mean moves; p and the extent block's covariance are not touched.
func holdExtentFloor(x *[4]float32, p [16]float32, ext *solidBodyExtent) {
	if float64(ext.h[0]) >= extentStateMinHalfMetres && float64(ext.h[1]) >= extentStateMinHalfMetres {
		return
	}
	js := jointFrom(*x, p, ext)
	if js.holdHalfExtentsAbove(extentFloor) != ([2]bool{}) {
		writeJointMean(x, ext, js)
	}
}

// holdHalfExtentsAbove imposes h ≥ lower on the joint state's half-extents by
// estimate projection: the mean moves to the point of the feasible set
// nearest it in the P⁻¹ metric, and P is not changed. It returns which
// constraints are active, those the projection put the half-extent on.
//
// The cost of a move in the full state, minimised over the states the
// constraint does not name, is the half-extents' own (h − ĥ)ᵀ P_hh⁻¹ (h − ĥ),
// and the minimising move of every state is P_xh λ with λ = P_hh⁻¹ (h − ĥ).
// The Karush-Kuhn-Tucker conditions make λ ≥ 0 the constraints' multipliers,
// zero on an inactive one. With two constraints the active set is one of
// {hL}, {hW} or both, and the problem is strictly convex, so the first set
// whose solution is feasible with λ ≥ 0 is the solution: one constraint j
// active moves h by P_hh e_j d_j / P_jj, with d_j = lower_j − ĥ_j > 0, and is
// the answer when it leaves the other half-extent feasible; otherwise both
// are active and λ = P_hh⁻¹ d.
//
// A half-extent with no variance has no correlation either (P is positive
// semi-definite), and a P_hh that is singular has its two half-extents
// exactly correlated; in either case the constraints are taken one at a time,
// each by its own regression where it has variance and by setting the
// half-extent where it has none.
func (js *jointState) holdHalfExtentsAbove(lower [2]float64) [2]bool {
	const n = jointStates
	h := [2]float64{js.x[4], js.x[5]}
	d := [2]float64{lower[0] - h[0], lower[1] - h[1]}
	if !(d[0] > 0) && !(d[1] > 0) {
		return [2]bool{}
	}
	s := [2]float64{js.p[4*n+4], js.p[5*n+5]}
	s01 := (js.p[4*n+5] + js.p[5*n+4]) / 2
	det := s[0]*s[1] - s01*s01
	var lambda [2]float64
	var active [2]bool
	if s[0] > 0 && s[1] > 0 && det > 1e-12*s[0]*s[1] {
		single := func(j int) bool {
			o := 1 - j
			if !(d[j] > 0) || h[o]+s01/s[j]*d[j] < lower[o] {
				return false
			}
			lambda[j], active[j] = d[j]/s[j], true
			return true
		}
		if !single(0) && !single(1) {
			lambda = [2]float64{(s[1]*d[0] - s01*d[1]) / det, (s[0]*d[1] - s01*d[0]) / det}
			active = [2]bool{true, true}
		}
		for i := 0; i < n; i++ {
			js.x[i] += js.p[i*n+4]*lambda[0] + js.p[i*n+5]*lambda[1]
		}
	} else {
		for j := 0; j < 2; j++ {
			dj := lower[j] - js.x[4+j]
			if !(dj > 0) {
				continue
			}
			active[j] = true
			if sjj := js.p[(4+j)*n+4+j]; sjj > 0 {
				for i := 0; i < n; i++ {
					js.x[i] += js.p[i*n+4+j] / sjj * dj
				}
			}
		}
	}
	// On the constraint exactly, not a rounding below it.
	for j, a := range active {
		if a {
			js.x[4+j] = lower[j]
		}
	}
	return active
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
