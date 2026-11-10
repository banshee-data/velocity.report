package l5tracks

// VelocityPriorGated (experiment l5est0010): the honest birth velocity
// variance in the filter, the shipped prior's covariance in a tentative
// track's gate.
//
// l5est0009 found that VelocityPriorInit's (7.5 m/s)² removes the first
// second's along-track over-dispersion but also widens the newborn's
// innovation covariance, so a tentative track could take a cluster from
// further away and confirm on the speed one pairing gave it. Here the track
// carries a second 4x4 covariance, founded as the shipped prior founds P
// (velocity variance 1 m²/s²) and stepped by the same prediction, process
// noise, cap, coast inflation and update as P, each with its own gain. While
// the track is tentative its gate and association cost read that covariance;
// the Kalman update reads the track's own. Once the track confirms the shadow
// is no longer read or stepped.
//
// It is a deliberate inconsistency for a tentative track's frames (at the
// defaults, HitsToConfirm = 4 observed frames plus any coasts between them):
// the gate is stated in a covariance the state does not hold.

// gateCovariance is a tentative track's gate covariance under
// VelocityPriorGated. active is false when the option is off or the track was
// born without it.
type gateCovariance struct {
	active bool
	p      [16]float32
}

// gateShadowLive reports whether the track's gate reads its shadow
// covariance this frame.
func (t *Tracker) gateShadowLive(track *TrackedObject) bool {
	return t.Config.VelocityPriorGated && track.gate.active && track.TrackState == TrackTentative
}

// gateP is the covariance the association gate and cost read for the track:
// the track's own P, or under VelocityPriorGated a tentative track's
// shipped-prior shadow.
func (t *Tracker) gateP(track *TrackedObject) *[16]float32 {
	if t.gateShadowLive(track) {
		return &track.gate.p
	}
	return &track.P
}

// predictGateCovariance steps the shadow as predict stepped P: F P Fᵀ + Q at
// the same (already clamped) dt, then the same cap. The solid body's tracked
// extent block is not part of the shadow; it is never read by the gate's
// position block.
func (t *Tracker) predictGateCovariance(track *TrackedObject, dt float32) {
	if !t.gateShadowLive(track) {
		return
	}
	P := track.gate.p
	var FP [16]float32
	for j := 0; j < 4; j++ {
		FP[0*4+j] = P[0*4+j] + dt*P[2*4+j]
		FP[1*4+j] = P[1*4+j] + dt*P[3*4+j]
		FP[2*4+j] = P[2*4+j]
		FP[3*4+j] = P[3*4+j]
	}
	g := &track.gate.p
	for i := 0; i < 4; i++ {
		g[i*4+0] = FP[i*4+0] + dt*FP[i*4+2]
		g[i*4+1] = FP[i*4+1] + dt*FP[i*4+3]
		g[i*4+2] = FP[i*4+2]
		g[i*4+3] = FP[i*4+3]
	}
	addProcessNoise(g, t.Config.ProcessNoisePos, t.Config.ProcessNoiseVel, dt, t.Config.CoupledProcessNoise)
	t.capGateCovariance(g)
}

// capGateCovariance applies the cap predict applies to P.
func (t *Tracker) capGateCovariance(g *[16]float32) {
	if t.Config.ConsistentCovarianceCap {
		capCovarianceConsistent(g, t.Config.MaxCovarianceDiag)
		return
	}
	for i := 0; i < 4; i++ {
		if g[i*4+i] > t.Config.MaxCovarianceDiag {
			g[i*4+i] = t.Config.MaxCovarianceDiag
		}
	}
}

// inflateGateCovariance adds the coast inflation P received to the shadow's
// position diagonal, capped as P's is.
func (t *Tracker) inflateGateCovariance(track *TrackedObject, add float32) {
	if !t.gateShadowLive(track) {
		return
	}
	g := &track.gate.p
	g[0*4+0] += add
	g[1*4+1] += add
	t.capGateCovariance(g)
}

// updateGateCovariance applies the measurement update to the shadow with its
// own gain and the same R the track's update uses, in the same form (naive or
// Joseph). The covariance update does not depend on the measurement, so this
// is exactly the covariance the shipped prior would hold after the same
// sequence of predictions and updates.
func (t *Tracker) updateGateCovariance(track *TrackedObject, adaptiveR MeasurementCovariance) {
	if !t.gateShadowLive(track) {
		return
	}
	g := &track.gate.p
	S00 := g[0*4+0] + t.Config.MeasurementNoise
	S01 := g[0*4+1]
	S10 := g[1*4+0]
	S11 := g[1*4+1] + t.Config.MeasurementNoise
	if t.Config.AdaptiveMeasurementNoise {
		S00, S01, S10, S11 = adaptiveInnovationCovariance(g, adaptiveR)
	}
	det := S00*S11 - S01*S10
	if det < MinDeterminantThreshold {
		return
	}
	invS00 := S11 / det
	invS01 := -S01 / det
	invS10 := -S10 / det
	invS11 := S00 / det
	var K [8]float32
	for i := 0; i < 4; i++ {
		K[i*2+0] = g[i*4+0]*invS00 + g[i*4+1]*invS10
		K[i*2+1] = g[i*4+0]*invS01 + g[i*4+1]*invS11
	}
	switch {
	case t.Config.JosephCovarianceUpdate && t.Config.AdaptiveMeasurementNoise:
		*g = josephCovarianceUpdateFull(*g, K, adaptiveR)
	case t.Config.JosephCovarianceUpdate:
		*g = josephCovarianceUpdate(*g, K, t.Config.MeasurementNoise)
	default:
		*g = naiveCovarianceUpdate(*g, K)
	}
}
