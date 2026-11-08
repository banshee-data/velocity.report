package l5tracks

import "math"

// Face innovations: what a near-edge fix measured against what the solid body
// predicted, face by face, as G-EST-1's criteria read it (state plan Section
// 7.3; maths review 2026-11-08, Section 4.3). The fix applies its faces as
// sequential scalar updates, so the NIS it reports is a sum of per-face terms
// each against the state the previous face left. That sum is the joint NIS,
// but its terms are conditional, and a consumer that wants the innovation
// along the track needs the joint form: every face against the one state the
// faces updated, with the joint innovation covariance S = H P Hᵀ + R. That is
// what is recorded here, beside the update and never instead of it: it reads
// the state and covariance and writes only the measurement record, so no
// estimate changes with it.

// maxFixFaces is the most faces one fix applies: at most one longitudinal and
// one lateral face reach it, whose normals are perpendicular.
const maxFixFaces = 2

// FaceInnovation is one applied face's innovation against the state the
// fix's faces updated.
type FaceInnovation struct {
	Face BodyFace
	// NormalX, NormalY is the face's outward unit normal in the site frame.
	NormalX, NormalY float32
	// Innovation is z − h·x⁻: the face's measurement less the prediction of it.
	// Without ExtentState z is the face's implied centre offset (the plane
	// less the believed half-extent) and h·x⁻ the predicted centre along the
	// normal; with it z is the plane offset and h·x⁻ includes the predicted
	// half-extent. Either way it is the residual along the normal, in metres,
	// positive where the face was further out along its normal than predicted.
	Innovation float32
	// Variance is this face's diagonal of S, h P⁻ hᵀ + r, in m².
	Variance float32
	// NoiseVariance is r, the face's measurement noise as the update used it:
	// measurement_noise plus any entry consider or heading term.
	NoiseVariance float32
	// ExtentVariance is the predicted variance of the face's half-extent under
	// ExtentState, which is part of Variance; zero without it, where the
	// filter takes the believed half-extent as exact.
	ExtentVariance float32
}

// faceMeasurementRow returns face i's scalar measurement as the solid body
// applies it, z = h·x + v with v ~ N(0, r), over the joint state's indices.
// Without extentState z is the implied centre offset and h has no extent
// entry, which is the four-state update; with it z is the plane offset and h
// carries the face's half-extent. consider and heading, when not nil, add
// FaceEntryConsider's and HeadingFaceNoise's variances to r. Both updates
// build their rows here, so the record cannot describe a different update.
func (t *Tracker) faceMeasurementRow(e EdgeMeasurement, extentState bool, consider, heading []float64, i int) (h [jointStates]float64, z, r float64) {
	r = float64(t.Config.MeasurementNoise)
	if consider != nil {
		r += consider[i]
	}
	if heading != nil {
		r += heading[i]
	}
	h[0], h[1] = float64(e.NormalX), float64(e.NormalY)
	if extentState {
		h[4+extentIndex(e.Face)] = 1
		return h, float64(e.PlaneOffsetMetres), r
	}
	return h, float64(e.ImpliedCentreOffset()), r
}

// flatJoint embeds a four-state body in the joint layout with no extent
// block, so one routine reads either filter.
func flatJoint(state [4]float32, p [16]float32) jointState {
	var js jointState
	for i := 0; i < 4; i++ {
		js.x[i] = float64(state[i])
		for j := 0; j < 4; j++ {
			js.p[i*jointStates+j] = float64(p[i*4+j])
		}
	}
	return js
}

// recordFaceInnovations writes a fix's joint innovations into m: each face's
// innovation against js, S's diagonal and off-diagonal, and the state the
// faces updated. js is that state, before any face; the rows are built as the
// update builds them. It records nothing, and leaves m's count at zero, for
// more faces than a fix can apply.
func (t *Tracker) recordFaceInnovations(m *SolidBodyMeasurement, js *jointState, edges []EdgeMeasurement, extentState bool, consider, heading []float64) {
	m.FaceInnovationCount = 0
	if len(edges) == 0 || len(edges) > maxFixFaces {
		return
	}
	const n = jointStates
	var rows [maxFixFaces][n]float64
	var ph [maxFixFaces][n]float64
	for k, e := range edges {
		h, z, r := t.faceMeasurementRow(e, extentState, consider, heading, k)
		rows[k] = h
		var predicted float64
		for i := 0; i < n; i++ {
			predicted += h[i] * js.x[i]
			for j := 0; j < n; j++ {
				ph[k][i] += js.p[i*n+j] * h[j]
			}
		}
		var s float64
		for i := 0; i < n; i++ {
			s += h[i] * ph[k][i]
		}
		fi := FaceInnovation{
			Face: e.Face, NormalX: e.NormalX, NormalY: e.NormalY,
			Innovation: float32(z - predicted), Variance: float32(s + r), NoiseVariance: float32(r),
		}
		if extentState {
			x := 4 + extentIndex(e.Face)
			fi.ExtentVariance = float32(js.p[x*n+x])
		}
		m.FaceInnovations[k] = fi
	}
	m.FaceInnovationCross = 0
	if len(edges) == 2 {
		var s01 float64
		for i := 0; i < n; i++ {
			s01 += rows[0][i] * ph[1][i]
		}
		m.FaceInnovationCross = float32(s01)
	}
	m.FaceInnovationCount = len(edges)
	m.PriorX, m.PriorY = float32(js.x[0]), float32(js.x[1])
	m.PriorVX, m.PriorVY = float32(js.x[2]), float32(js.x[3])
}

// JointFaceNIS is the fix's NIS from its recorded joint innovations, yᵀS⁻¹y
// over FaceInnovationCount dimensions, and false when none are recorded or S
// is not positive definite. It equals the sequential NIS the update reported
// (NIS), to rounding: the check that the record describes the update.
func (m SolidBodyMeasurement) JointFaceNIS() (float64, bool) {
	switch m.FaceInnovationCount {
	case 1:
		f := m.FaceInnovations[0]
		s := float64(f.Variance)
		if !(s > 0) {
			return 0, false
		}
		y := float64(f.Innovation)
		return y * y / s, true
	case 2:
		a, b := m.FaceInnovations[0], m.FaceInnovations[1]
		s00, s11, s01 := float64(a.Variance), float64(b.Variance), float64(m.FaceInnovationCross)
		det := s00*s11 - s01*s01
		if !(s00 > 0) || !(det > 0) || math.IsInf(det, 0) {
			return 0, false
		}
		y0, y1 := float64(a.Innovation), float64(b.Innovation)
		return (s11*y0*y0 - 2*s01*y0*y1 + s00*y1*y1) / det, true
	}
	return 0, false
}
