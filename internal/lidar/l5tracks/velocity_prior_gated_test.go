package l5tracks

import (
	"math"
	"testing"
	"time"
)

// Tests for VelocityPriorGated (experiment l5est0010): the wide birth
// velocity variance in the update, the shipped prior's covariance in a
// tentative track's gate.

func gatedPriorConfig() TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.VelocityPriorInit = true
	cfg.VelocityPriorGated = true
	return cfg
}

func TestVelocityPriorGatedIsOptIn(t *testing.T) {
	if DefaultTrackerConfig().VelocityPriorGated {
		t.Fatal("VelocityPriorGated must be off by default")
	}
	// Off, no track carries a shadow, so the gate reads P.
	tr := NewTracker(DefaultTrackerConfig())
	now := time.Unix(1_700_000_000, 0)
	tr.Update([]WorldCluster{hygieneCluster(5, 2, now)}, now)
	for _, track := range tr.Tracks {
		if track.gate.active || tr.gateP(track) != &track.P {
			t.Fatal("with the option off the gate must read the track's own P")
		}
	}
}

// The founding shadow is the shipped prior; the track's own P is the wide one.
func TestVelocityPriorGatedFoundingCovariances(t *testing.T) {
	tr := NewTracker(gatedPriorConfig())
	now := time.Unix(1_700_000_000, 0)
	tr.Update([]WorldCluster{hygieneCluster(5, 2, now)}, now)
	shipped := [16]float32{10, 0, 0, 0, 0, 10, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	for _, track := range tr.Tracks {
		if !track.gate.active || track.gate.p != shipped {
			t.Errorf("shadow %v (active %v), want the shipped founding covariance", track.gate.p, track.gate.active)
		}
		if track.P[10] != VelocityPriorInitVariance || track.P[15] != VelocityPriorInitVariance {
			t.Errorf("track P velocity diagonal (%v, %v), want the wide prior", track.P[10], track.P[15])
		}
		if tr.gateP(track) != &track.gate.p {
			t.Error("a tentative track's gate must read the shadow")
		}
	}
}

// The shadow is exactly the covariance the shipped prior's track holds after
// the same frames: run the same scene under the defaults and compare.
func TestVelocityPriorGatedShadowIsTheShippedCovariance(t *testing.T) {
	run := func(cfg TrackerConfig) *TrackedObject {
		tr := NewTracker(cfg)
		t0 := time.Unix(1_700_000_000, 0)
		for i := 0; i < 3; i++ {
			now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
			tr.Update([]WorldCluster{hygieneCluster(5+0.65*float32(i), 2, now)}, now)
		}
		// One coasting frame, so the coast inflation reaches both.
		now := t0.Add(300 * time.Millisecond)
		tr.Update(nil, now)
		for _, track := range tr.Tracks {
			return track
		}
		t.Fatal("no track")
		return nil
	}
	shipped := run(DefaultTrackerConfig())
	gated := run(gatedPriorConfig())
	if gated.TrackState != TrackTentative {
		t.Fatalf("track state %v, want tentative", gated.TrackState)
	}
	for i := range shipped.P {
		if math.Abs(float64(shipped.P[i]-gated.gate.p[i])) > 1e-5 {
			t.Fatalf("shadow %v, want the shipped track's P %v", gated.gate.p, shipped.P)
		}
	}
	if gated.P == gated.gate.p {
		t.Fatal("the track's own P must be the wide prior's, not the shadow")
	}
}

// The update reads the wide covariance: the noise-free 6.5 m/s vehicle
// converges in exactly as many updates as under velocity_prior_init_7p5.
func TestVelocityPriorGatedConvergesAsTheWidePrior(t *testing.T) {
	const vx, vy = 6.5, 0
	wide := DefaultTrackerConfig()
	wide.VelocityPriorInit = true
	want := updatesToConverge(t, wide, vx, vy)
	got := updatesToConverge(t, gatedPriorConfig(), vx, vy)
	t.Logf("updates until within 10 %% of 6.5 m/s: wide prior %d, gated %d", want, got)
	if got != want {
		t.Errorf("gated: within 10 %% from update %d, want %d as under the wide prior", got, want)
	}
	if got > 4 {
		t.Errorf("gated: within 10 %% only from update %d, want by the fourth", got)
	}
}

// gateVarianceX is the x variance of the innovation covariance a track's gate
// would read after predicting across dt, for the given covariance.
func gateVarianceX(cfg TrackerConfig, p [16]float32, dt float32) float32 {
	tr := NewTracker(cfg)
	track := &TrackedObject{P: p}
	tr.predict(track, dt)
	return track.P[0] + cfg.MeasurementNoise
}

// A cluster outside the shipped gate but inside the wide one is associated to
// a tentative track under the wide prior and not under the gated form, where
// it seeds its own track instead.
func TestVelocityPriorGatedRefusesWhatOnlyTheWideGateAdmits(t *testing.T) {
	const frames = 3
	const dt = 0.1
	t0 := time.Unix(1_700_000_000, 0)
	// A stationary newborn observed three times: still tentative at the
	// defaults (HitsToConfirm 4), its position variance small, its velocity
	// variance still far apart under the two priors.
	settle := func(cfg TrackerConfig) (*Tracker, *TrackedObject) {
		tr := NewTracker(cfg)
		for i := 0; i < frames; i++ {
			now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
			tr.Update([]WorldCluster{hygieneCluster(5, 2, now)}, now)
		}
		if len(tr.Tracks) != 1 {
			t.Fatalf("%d tracks after settling, want 1", len(tr.Tracks))
		}
		for _, track := range tr.Tracks {
			if track.TrackState != TrackTentative {
				t.Fatalf("track state %v after %d hits, want tentative", track.TrackState, frames)
			}
			return tr, track
		}
		return nil, nil
	}
	_, probe := settle(gatedPriorConfig())
	sShipped := gateVarianceX(gatedPriorConfig(), probe.gate.p, dt)
	sWide := gateVarianceX(gatedPriorConfig(), probe.P, dt)
	gate := DefaultTrackerConfig().GatingDistanceSquared
	reachShipped := math.Sqrt(float64(gate * sShipped))
	reachWide := math.Sqrt(float64(gate * sWide))
	t.Logf("gate reach along x: shipped %.3f m, wide %.3f m", reachShipped, reachWide)
	if reachWide <= reachShipped*1.2 {
		t.Fatalf("the scene does not separate the gates: shipped %.3f m, wide %.3f m", reachShipped, reachWide)
	}
	d := float32((reachShipped + reachWide) / 2)

	jump := func(cfg TrackerConfig) (associated bool) {
		tr, track := settle(cfg)
		id := track.TrackID
		hits := track.Hits
		now := t0.Add(frames * 100 * time.Millisecond)
		tr.Update([]WorldCluster{hygieneCluster(5+d, 2, now)}, now)
		return tr.Tracks[id].Hits > hits
	}
	wide := DefaultTrackerConfig()
	wide.VelocityPriorInit = true
	if !jump(wide) {
		t.Errorf("wide prior: the cluster %.3f m away was not associated; the scene should be inside its gate", d)
	}
	if jump(gatedPriorConfig()) {
		t.Errorf("gated: the cluster %.3f m away was associated to the tentative track; it is outside the shipped gate", d)
	}
	if jump(DefaultTrackerConfig()) {
		t.Errorf("shipped prior: the cluster %.3f m away was associated; the scene's gate is not the shipped one", d)
	}
}

// Once the track confirms, its gate reads its own P.
func TestVelocityPriorGatedConfirmedTrackGatesOnItsOwnCovariance(t *testing.T) {
	cfg := gatedPriorConfig()
	tr := NewTracker(cfg)
	t0 := time.Unix(1_700_000_000, 0)
	for i := 0; i < cfg.HitsToConfirm+1; i++ {
		now := t0.Add(time.Duration(i) * 100 * time.Millisecond)
		tr.Update([]WorldCluster{hygieneCluster(5+0.65*float32(i), 2, now)}, now)
	}
	for _, track := range tr.Tracks {
		if track.TrackState != TrackConfirmed {
			t.Fatalf("track state %v, want confirmed", track.TrackState)
		}
		if tr.gateP(track) != &track.P {
			t.Error("a confirmed track's gate must read its own P")
		}
	}
}

// Tests for VelocityPriorShadowRefusal (experiment l5est0011): a birth in a
// confirmed track's shadow takes the shipped prior, every other the wide one.

func shadowRefusalTracker() *Tracker {
	cfg := gatedPriorConfig()
	cfg.VelocityPriorShadowRefusal = true
	tr := NewTracker(cfg)
	car := &TrackedObject{TrackID: "car", X: 10, Y: 0, OBBLength: 4.6, OBBWidth: 1.9}
	car.TrackState = TrackConfirmed
	tr.Tracks["car"] = car
	return tr
}

// A cluster on the confirmed car's bearing at twice its range is born with
// the shipped velocity variance and no gate shadow, as the shipped tracker
// would found it, and the refusal is counted.
func TestVelocityPriorShadowRefusalShadowBirthTakesTheShippedPrior(t *testing.T) {
	tr := shadowRefusalTracker()
	now := time.Unix(1_700_000_000, 0)
	born := tr.initTrack(hygieneCluster(20, 0, now), now.UnixNano())
	shipped := NewTracker(DefaultTrackerConfig()).initTrack(hygieneCluster(20, 0, now), now.UnixNano())
	if born.P != shipped.P {
		t.Errorf("shadow birth P %v, want the shipped founding covariance %v", born.P, shipped.P)
	}
	if born.gate.active || tr.gateP(born) != &born.P {
		t.Error("a shadow birth must gate on its own (shipped) P")
	}
	if got := tr.ContinuityStats().ShadowPriorBirths; got != 1 {
		t.Errorf("ShadowPriorBirths %d, want 1", got)
	}
}

// A car in clear view, off every confirmed box's bearings, is born with the
// wide variance and the shipped-prior gate shadow, as under VelocityPriorGated.
func TestVelocityPriorShadowRefusalClearBirthTakesTheWidePrior(t *testing.T) {
	tr := shadowRefusalTracker()
	now := time.Unix(1_700_000_000, 0)
	born := tr.initTrack(hygieneCluster(0, 15, now), now.UnixNano())
	if born.P[10] != VelocityPriorInitVariance || born.P[15] != VelocityPriorInitVariance {
		t.Errorf("clear birth velocity diagonal (%v, %v), want the wide prior", born.P[10], born.P[15])
	}
	shipped := [16]float32{10, 0, 0, 0, 0, 10, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	if !born.gate.active || born.gate.p != shipped {
		t.Errorf("clear birth gate shadow %v (active %v), want the shipped founding covariance", born.gate.p, born.gate.active)
	}
	if got := tr.ContinuityStats().ShadowPriorBirths; got != 0 {
		t.Errorf("ShadowPriorBirths %d, want 0", got)
	}
}

// Without the refusal the gated form widens a shadow birth as before: the
// option changes nothing unless it is named.
func TestVelocityPriorShadowRefusalIsOptIn(t *testing.T) {
	if DefaultTrackerConfig().VelocityPriorShadowRefusal {
		t.Fatal("VelocityPriorShadowRefusal must be off by default")
	}
	tr := shadowRefusalTracker()
	tr.Config.VelocityPriorShadowRefusal = false
	now := time.Unix(1_700_000_000, 0)
	born := tr.initTrack(hygieneCluster(20, 0, now), now.UnixNano())
	if born.P[10] != VelocityPriorInitVariance || !born.gate.active {
		t.Error("without the refusal the gated form must widen a shadow birth")
	}
	if got := tr.ContinuityStats().ShadowPriorBirths; got != 0 {
		t.Errorf("ShadowPriorBirths %d, want 0", got)
	}
}
