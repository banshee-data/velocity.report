package l5tracks

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Existence separate from observation, and the default-off continuity
// options. Scenario-level evidence, with ground truth, is in
// continuity_scenario_test.go; these tests pin each rule on its own.

// ccCluster is a cluster with a body: an OBB of the given extent, heading
// along +X, and enough points for PCA.
func ccCluster(x, y, length, width float32) WorldCluster {
	return WorldCluster{
		CentroidX: x, CentroidY: y, SensorID: "continuity",
		BoundingBoxLength: length, BoundingBoxWidth: width, BoundingBoxHeight: 1.5,
		HeightP95: 1.5, PointsCount: 40,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: x, CenterY: y, Length: length, Width: width, Height: 1.5,
		},
	}
}

// confirmedTrack tracks a stationary body at (x, y) at 10 Hz from capture
// time 1 s until it is confirmed and has a trusted extent, and returns the
// tracker, the track and the capture time of its last observation.
func confirmedTrack(t *testing.T, cfg TrackerConfig, x, y, length, width float32) (*Tracker, *TrackedObject, float64) {
	t.Helper()
	tk := NewTracker(cfg)
	now := 1.0
	for i := 0; i < 8; i++ {
		tk.Update([]WorldCluster{ccCluster(x, y, length, width)}, tdAt(now))
		now += 0.1
	}
	track := soleActive(t, tk)
	if track.TrackState != TrackConfirmed {
		t.Fatalf("setup: track is %s, want confirmed", track.TrackState)
	}
	return tk, track, now - 0.1
}

func continuityConfig(mutate func(*OcclusionContinuityConfig)) TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.OcclusionContinuity = DefaultOcclusionContinuity()
	cfg.OcclusionContinuity.ExplainAbsence = false
	cfg.OcclusionContinuity.CaptureTimeInflation = false
	cfg.OcclusionContinuity.ClassCoastBounds = false
	cfg.OcclusionContinuity.ReacquisitionGuard = false
	if mutate != nil {
		mutate(&cfg.OcclusionContinuity)
	}
	return cfg
}

// The support tokens are the behaviour plan's closed vocabulary, read from
// the plan itself, so l5tracks and l8behaviour cannot drift apart silently.
func TestSupportTokensMatchTheBehaviourPlanVocabulary(t *testing.T) {
	var doc []byte
	var err error
	for _, prefix := range []string{"", "../../../"} {
		if doc, err = os.ReadFile(filepath.Join(prefix, "docs/plans/lidar-behaviour-analytics-plan.md")); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("read behaviour plan: %v", err)
	}
	text := string(doc)
	start := strings.Index(text, "### 7.3 Observation support")
	if start < 0 {
		t.Fatal("behaviour plan has no Section 7.3")
	}
	section := text[start:]
	section = section[:strings.Index(section[4:], "\n### ")+4]
	var planTokens []string
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cell := strings.TrimSpace(strings.Split(strings.Trim(line, "|"), "|")[0])
		planTokens = append(planTokens, strings.Trim(cell, "`"))
	}
	if len(planTokens) != int(supportTokenCount)-1 {
		t.Fatalf("plan lists %d support states %v; l5tracks declares %d", len(planTokens), planTokens, supportTokenCount-1)
	}
	for i, token := range planTokens {
		s := ObservationSupport(i + 1)
		if s.String() != token {
			t.Errorf("support %d is %q in l5tracks and %q in the plan", i+1, s, token)
		}
		if parsed, ok := ParseObservationSupport(token); !ok || parsed != s {
			t.Errorf("%q does not round-trip: %v %v", token, parsed, ok)
		}
	}
	if _, ok := ParseObservationSupport(""); ok {
		t.Error("the empty token parsed")
	}
	if _, ok := ParseObservationSupport("occluded"); ok {
		t.Error("an unregistered token parsed")
	}
}

// The zero value of the continuity options is the shipped tracker.
func TestContinuityOptionsAreOffByDefault(t *testing.T) {
	if got := DefaultTrackerConfig().OcclusionContinuity; got != (OcclusionContinuityConfig{}) {
		t.Fatalf("a continuity option is set by default: %+v", got)
	}
	d := DefaultOcclusionContinuity()
	if !d.ExplainAbsence || !d.CaptureTimeInflation || !d.ClassCoastBounds || !d.ReacquisitionGuard {
		t.Fatalf("DefaultOcclusionContinuity should switch every option on: %+v", d)
	}
	for class, p := range d.Policies {
		if p.UnexplainedSecs <= 0 || p.ExplainedSecs < p.UnexplainedSecs || p.InflationPerSec < 0 {
			t.Errorf("class %s policy %+v: bounds must be positive and the explained one no shorter", MotionClass(class), p)
		}
	}
}

// By default every history point says whether it was observed or coasted,
// the track's existence follows, and the frame-count rule expires it with
// its reason recorded. Nothing here changes an estimate.
func TestSupportAndExistenceAreRecordedByDefault(t *testing.T) {
	tk, track, last := confirmedTrack(t, DefaultTrackerConfig(), 10, 5, 4.5, 1.8)
	if track.Existence != ExistenceObserved || track.LastSupport != SupportObserved {
		t.Fatalf("observed track: existence=%s support=%s", track.Existence, track.LastSupport)
	}
	for i, p := range track.History {
		if p.Support != SupportObserved {
			t.Fatalf("history point %d is %q, want observed", i, p.Support)
		}
	}
	now := last
	for i := 0; i < tk.Config.MaxMissesConfirmed; i++ {
		now += 0.1
		tk.Update(nil, tdAt(now))
		if i < tk.Config.MaxMissesConfirmed-1 && track.Existence != ExistenceCoasting {
			t.Fatalf("miss %d: existence %s, want coasting", i+1, track.Existence)
		}
	}
	if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryMisses || track.Existence != ExistenceExpired {
		t.Fatalf("after the miss budget: state=%s reason=%s existence=%s", track.TrackState, track.ExpiryReason, track.Existence)
	}
	coasted := 0
	for _, p := range track.History {
		if p.Support == SupportCoasted {
			coasted++
		}
	}
	if coasted != tk.Config.MaxMissesConfirmed {
		t.Fatalf("%d coasted history points, want %d", coasted, tk.Config.MaxMissesConfirmed)
	}
	stats := tk.ContinuityStats()
	if stats.SupportInstants.Coasted != int64(coasted) || stats.SupportInstants.OccludedInferred != 0 || stats.SupportInstants.MissedUnknown != 0 {
		t.Fatalf("support counts %+v", stats.SupportInstants)
	}
	if stats.ExpiredByReason.Misses != 1 || stats.TracksBorn != 1 || stats.TracksConfirmed != 1 {
		t.Fatalf("stats %+v", stats)
	}
	if stats.CoastAgeAtExpiry.UpperEdgesSecs != coastAgeEdgesSecs {
		t.Fatal("the snapshot does not describe its histogram bins")
	}
	if solid := SolidBodyFromTrack(track, MotionClassBelief{}, DefaultConvergenceBounds()); solid.Support.Instant != SupportCoasted {
		t.Fatalf("solid-body support instant %q, want coasted", solid.Support.Instant)
	}
}

// Absence explanation is diagnostic: the same stream with and without it
// gives the same estimates, frame by frame. Only the tokens differ.
func TestExplainAbsenceChangesNoEstimate(t *testing.T) {
	run := func(explain bool) ([]float32, []ObservationSupport) {
		tk := NewTracker(continuityConfig(func(c *OcclusionContinuityConfig) { c.ExplainAbsence = explain }))
		var trace []float32
		var support []ObservationSupport
		for k := 0; k < 40; k++ {
			now := 1 + 0.1*float64(k)
			target := ccCluster(-10+0.5*float32(k), 12, 1.8, 0.6)
			clusters := []WorldCluster{ccCluster(0, 6, 5, 2)}
			if k < 15 || k > 22 {
				clusters = append(clusters, target)
			}
			tk.Update(clusters, tdAt(now))
			for _, tr := range tk.Tracks {
				if tr.TrackState != TrackDeleted && tr.Y > 9 {
					trace = append(trace, tr.X, tr.Y, tr.VX, tr.P[0], tr.P[5])
					support = append(support, tr.LastSupport)
				}
			}
		}
		return trace, support
	}
	plainTrace, plainSupport := run(false)
	explainedTrace, explainedSupport := run(true)
	if len(plainTrace) != len(explainedTrace) {
		t.Fatalf("trace lengths differ: %d and %d", len(plainTrace), len(explainedTrace))
	}
	for i := range plainTrace {
		if plainTrace[i] != explainedTrace[i] {
			t.Fatalf("explanation changed an estimate at %d: %v vs %v", i, plainTrace[i], explainedTrace[i])
		}
	}
	var sawCoasted, sawExplained bool
	for i := range plainSupport {
		sawCoasted = sawCoasted || plainSupport[i] == SupportCoasted
		sawExplained = sawExplained || explainedSupport[i] == SupportOccludedInferred || explainedSupport[i] == SupportMissedUnknown
	}
	if !sawCoasted || !sawExplained {
		t.Fatalf("the stream did not exercise explanation: plain %v explained %v", plainSupport, explainedSupport)
	}
}

// explainAbsence on hand-placed geometry. The sensor is at the origin.
func TestExplainAbsenceGeometry(t *testing.T) {
	cfg := continuityConfig(func(c *OcclusionContinuityConfig) {
		c.ExplainAbsence = true
		c.Coverage = SensorCoverage{MaxRangeMetres: 40, AzimuthCentreDeg: 90, AzimuthHalfWidthDeg: 120}
	})
	tk := NewTracker(cfg)
	// A pedestrian-sized belief 12 m out along +Y.
	track := &TrackedObject{X: 0, Y: 12, TrackMeasurement: TrackMeasurement{BoundingBoxLengthAvg: 0.6, BoundingBoxWidthAvg: 0.5}}
	van := ccCluster(0, 6, 5, 2) // broadside at 6 m: hides ±22° at 6 m
	cases := []struct {
		name     string
		track    TrackedObject
		clusters []WorldCluster
		want     ObservationSupport
	}{
		{"nothing nearer", *track, nil, SupportMissedUnknown},
		{"van in front", *track, []WorldCluster{van}, SupportOccludedInferred},
		{"van behind", *track, []WorldCluster{ccCluster(0, 18, 5, 2)}, SupportMissedUnknown},
		{"van beside, same range", *track, []WorldCluster{ccCluster(2, 12, 2, 1)}, SupportMissedUnknown},
		{"sliver in front", TrackedObject{X: 0, Y: 12, TrackMeasurement: TrackMeasurement{BoundingBoxLengthAvg: 4.5}},
			[]WorldCluster{ccCluster(0.9, 6, 0.3, 0.3)}, SupportMissedUnknown},
		// End-on, the same van hides only its width; its long extent does not count.
		{"van end-on is narrow", TrackedObject{X: 12, Y: 12, TrackMeasurement: TrackMeasurement{BoundingBoxLengthAvg: 4.5}},
			[]WorldCluster{func() WorldCluster {
				c := ccCluster(6.5, 6.5, 5, 0.4)
				c.OBB.HeadingRad = math.Pi / 4
				return c
			}()}, SupportMissedUnknown},
		{"two occluders together", TrackedObject{X: 0, Y: 12, TrackMeasurement: TrackMeasurement{BoundingBoxLengthAvg: 4}},
			[]WorldCluster{ccCluster(-0.6, 6, 1.0, 0.5), ccCluster(0.6, 6, 1.0, 0.5)}, SupportOccludedInferred},
		{"beyond range", TrackedObject{X: 0, Y: 41}, []WorldCluster{van}, SupportOutOfFOV},
		{"outside the sector", TrackedObject{X: 0, Y: -12}, nil, SupportOutOfFOV},
	}
	for _, c := range cases {
		c := c
		if got := tk.explainAbsence(&c.track, c.clusters); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	// The azimuth wraps at ±π without losing the occluder.
	wrap := NewTracker(continuityConfig(func(c *OcclusionContinuityConfig) { c.ExplainAbsence = true }))
	behind := &TrackedObject{X: -12, Y: 0.1, TrackMeasurement: TrackMeasurement{BoundingBoxLengthAvg: 0.6}}
	occluder := ccCluster(-6, -0.1, 0.5, 3)
	if got := wrap.explainAbsence(behind, []WorldCluster{occluder}); got != SupportOccludedInferred {
		t.Fatalf("across the azimuth wrap: %s, want occluded_inferred", got)
	}

	// A sensor origin elsewhere moves the geometry with it.
	moved := NewTracker(continuityConfig(func(c *OcclusionContinuityConfig) {
		c.ExplainAbsence = true
		c.SensorX, c.SensorY = 0, 20
	}))
	if got := moved.explainAbsence(track, []WorldCluster{van}); got != SupportMissedUnknown {
		t.Fatalf("seen from (0, 20) the van at y=6 is behind the track: %s", got)
	}
}

// With the shipped per-frame inflation, the same unobserved interval widens
// the covariance by however many frames reached the tracker in it. Under
// CaptureTimeInflation it widens by the time that passed.
func TestCaptureTimeInflationFollowsCoastSecondsNotFrames(t *testing.T) {
	inflation := func(cfg TrackerConfig, frames int) float32 {
		// Isolate inflation from prediction: no process noise, and no velocity
		// or velocity covariance for F P Fᵀ to carry into position.
		cfg.ProcessNoisePos, cfg.ProcessNoiseVel = 0, 0
		cfg.MaxMisses, cfg.MaxMissesConfirmed = 100, 100
		tk, track, last := confirmedTrack(t, cfg, 10, 5, 1.8, 0.6)
		track.VX, track.VY = 0, 0
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				if i >= 2 || j >= 2 {
					track.P[i*4+j] = 0
				}
			}
		}
		before := track.P[0]
		for i := 1; i <= frames; i++ {
			tk.Update(nil, tdAt(last+0.4*float64(i)/float64(frames)))
		}
		return track.P[0] - before
	}
	shipped := DefaultTrackerConfig()
	if a, b := inflation(shipped, 4), inflation(shipped, 1); math.Abs(float64(a-4*b)) > 1e-4 {
		t.Fatalf("shipped inflation over 0.4 s: %v in four frames, %v in one; want four times", a, b)
	}

	option := continuityConfig(func(c *OcclusionContinuityConfig) { c.CaptureTimeInflation = true })
	frequent, sparse := inflation(option, 4), inflation(option, 1)
	want := 0.4 * option.OcclusionContinuity.Policies[MotionUnknown].InflationPerSec
	if math.Abs(float64(frequent-want)) > 1e-4 || math.Abs(float64(sparse-want)) > 1e-4 {
		t.Fatalf("capture-time inflation over 0.4 s: %v in four frames, %v in one; want %v in both", frequent, sparse, want)
	}

	pure := continuityConfig(func(c *OcclusionContinuityConfig) {
		c.CaptureTimeInflation = true
		for i := range c.Policies {
			c.Policies[i].InflationPerSec = 0
		}
	})
	if got := inflation(pure, 4); got != 0 {
		t.Fatalf("a zero rate is pure CV growth, and with no process noise adds nothing: got %v", got)
	}
}

// Gap S3: OcclusionCovInflation discounts a coasting track's association
// cost. At the nominal 10 Hz the capture-time rates must never make that
// discount deeper than the shipped per-frame inflation does, for any class,
// at any point in a coast the shipped budget allows.
func TestCaptureTimeInflationDoesNotDeepenTheS3Discount(t *testing.T) {
	for _, label := range []string{"", "car", "cyclist", "pedestrian"} {
		coastP := func(cfg TrackerConfig) []float32 {
			cfg.MaxMisses, cfg.MaxMissesConfirmed = 100, 100
			tk, track, last := confirmedTrack(t, cfg, 10, 5, 1.8, 0.6)
			if label != "" {
				tk.UpdateClassification(track.TrackID, label, 1, "test")
			}
			var ps []float32
			for i := 1; i <= DefaultTrackerConfig().MaxMissesConfirmed; i++ {
				tk.Update(nil, tdAt(last+0.1*float64(i)))
				ps = append(ps, track.P[0])
			}
			return ps
		}
		shipped := coastP(DefaultTrackerConfig())
		option := coastP(continuityConfig(func(c *OcclusionContinuityConfig) { c.CaptureTimeInflation = true }))
		for i := range shipped {
			if option[i] > shipped[i]+1e-5 {
				t.Errorf("class %q, miss %d: position variance %.3f under the option exceeds the shipped %.3f", label, i+1, option[i], shipped[i])
			}
		}
	}
}

// ClassCoastBounds replaces the frame count with capture time, per class, and
// records why the hypothesis lapsed.
func TestClassCoastBoundsExpireByCaptureTimeAndRecordTheReason(t *testing.T) {
	cfg := continuityConfig(func(c *OcclusionContinuityConfig) { c.ClassCoastBounds = true })
	policies := cfg.OcclusionContinuity.Policies

	t.Run("unexplained absence at 20 Hz outlives the frame budget", func(t *testing.T) {
		tk, track, last := confirmedTrack(t, cfg, 10, 5, 4.5, 1.8)
		tk.UpdateClassification(track.TrackID, "car", 1, "test")
		bound := policies[MotionRigidVehicle].UnexplainedSecs
		now := last
		for track.TrackState != TrackDeleted && now < last+10 {
			now += 0.05
			tk.Update(nil, tdAt(now))
		}
		if track.ExpiryReason != ExpiryMissedUnknown {
			t.Fatalf("reason %s, want missed_unknown", track.ExpiryReason)
		}
		if track.CoastAgeSecs < bound || track.CoastAgeSecs > bound+0.051 {
			t.Fatalf("expired at coast age %.3f s, want the %.2f s bound", track.CoastAgeSecs, bound)
		}
		if track.Misses <= tk.Config.MaxMissesConfirmed {
			t.Fatalf("%d misses: at 20 Hz the capture-time bound should outlast the %d-frame budget", track.Misses, tk.Config.MaxMissesConfirmed)
		}
	})

	t.Run("an explained absence gets the longer allowance", func(t *testing.T) {
		tk, track, last := confirmedTrack(t, cfg, 0, 12, 1.8, 0.6)
		tk.UpdateClassification(track.TrackID, "cyclist", 1, "test")
		van := ccCluster(0, 6, 5, 2)
		now := last
		for track.TrackState != TrackDeleted && now < last+10 {
			now += 0.1
			tk.Update([]WorldCluster{van}, tdAt(now))
		}
		bound := policies[MotionTwoWheeler].ExplainedSecs
		if track.ExpiryReason != ExpiryOccludedInferred || track.CoastAgeSecs < bound || track.CoastAgeSecs > bound+0.101 {
			t.Fatalf("reason %s at %.3f s, want occluded_inferred at %.2f s", track.ExpiryReason, track.CoastAgeSecs, bound)
		}
	})

	t.Run("an explanation that disappears lapses the hypothesis at once", func(t *testing.T) {
		tk, track, last := confirmedTrack(t, cfg, 0, 12, 1.8, 0.6)
		tk.UpdateClassification(track.TrackID, "cyclist", 1, "test")
		now := last
		for i := 0; i < 20; i++ { // 2 s behind a van: beyond the unexplained bound
			now += 0.1
			tk.Update([]WorldCluster{ccCluster(0, 6, 5, 2)}, tdAt(now))
		}
		if track.TrackState == TrackDeleted {
			t.Fatalf("expired while occluded at %.2f s", track.CoastAgeSecs)
		}
		now += 0.1
		tk.Update([]WorldCluster{ccCluster(30, 6, 5, 2)}, tdAt(now)) // the van leaves; the cyclist is not there
		if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryMissedUnknown {
			t.Fatalf("the object was not behind the occluder when it left: state=%s reason=%s", track.TrackState, track.ExpiryReason)
		}
	})

	t.Run("out of coverage is a departure, with the short allowance", func(t *testing.T) {
		oc := cfg
		oc.OcclusionContinuity.Coverage = SensorCoverage{MaxRangeMetres: 20}
		tk, track, last := confirmedTrack(t, oc, 19.9, 0, 1.8, 0.6)
		tk.UpdateClassification(track.TrackID, "cyclist", 1, "test")
		track.VX = 5
		now := last
		for track.TrackState != TrackDeleted && now < last+10 {
			now += 0.1
			tk.Update(nil, tdAt(now))
			if track.TrackState != TrackDeleted && track.LastSupport != SupportOutOfFOV {
				t.Fatalf("leaving coverage at x=%.1f: %s", track.X, track.LastSupport)
			}
		}
		if track.ExpiryReason != ExpiryOutOfFOV || track.Existence != ExistenceExpired {
			t.Fatalf("reason %s existence %s", track.ExpiryReason, track.Existence)
		}
		if bound := policies[MotionTwoWheeler].UnexplainedSecs; track.CoastAgeSecs > bound+0.101 {
			t.Fatalf("expired at %.2f s, beyond the %.2f s departure allowance", track.CoastAgeSecs, bound)
		}
	})

	t.Run("a tentative track has the tentative bound whatever explains it", func(t *testing.T) {
		tk := NewTracker(cfg)
		tk.Update([]WorldCluster{ccCluster(0, 12, 1.8, 0.6)}, tdAt(1))
		track := soleActive(t, tk)
		now := 1.0
		for track.TrackState != TrackDeleted && now < 5 {
			now += 0.1
			tk.Update([]WorldCluster{ccCluster(0, 6, 5, 2)}, tdAt(now))
		}
		if tb := cfg.OcclusionContinuity.TentativeCoastSecs; track.CoastAgeSecs < tb || track.CoastAgeSecs > tb+0.101 {
			t.Fatalf("tentative expired at %.2f s, want %.2f s", track.CoastAgeSecs, tb)
		}
	})

	t.Run("an unclassified or doubtful class relaxes toward unknown", func(t *testing.T) {
		tk, track, _ := confirmedTrack(t, cfg, 10, 5, 4.5, 1.8)
		unknown := policies[MotionUnknown].UnexplainedSecs
		if got, _ := tk.classCoastBound(track, SupportMissedUnknown); got != unknown {
			t.Fatalf("unclassified bound %.2f, want unknown's %.2f", got, unknown)
		}
		tk.UpdateClassification(track.TrackID, "car", 0.5, "test")
		want := unknown + 0.5*(policies[MotionRigidVehicle].UnexplainedSecs-unknown)
		if got, _ := tk.classCoastBound(track, SupportMissedUnknown); math.Abs(float64(got-want)) > 1e-6 {
			t.Fatalf("half-confident car bound %.3f, want %.3f", got, want)
		}
		tk.UpdateClassification(track.TrackID, "noise", 1, "test")
		if got, _ := tk.classCoastBound(track, SupportMissedUnknown); got != unknown {
			t.Fatalf("a non-road-user label should read as unknown: %.2f", got)
		}
	})

	t.Run("a lapsed hypothesis is expired before association", func(t *testing.T) {
		tk, track, last := confirmedTrack(t, cfg, 10, 5, 4.5, 1.8)
		// Two seconds with no frame at all, then the object where it was.
		tk.Update([]WorldCluster{ccCluster(10, 5, 4.5, 1.8)}, tdAt(last+2))
		if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryMissedUnknown {
			t.Fatalf("state=%s reason=%s: nothing supported the 2 s interval", track.TrackState, track.ExpiryReason)
		}
		if tk.TracksCreated != 2 {
			t.Fatalf("TracksCreated = %d: the returning cluster must seed a new identity", tk.TracksCreated)
		}
	})

	t.Run("AdvanceMisses applies the class bound", func(t *testing.T) {
		tk, track, last := confirmedTrack(t, cfg, 10, 5, 4.5, 1.8)
		tk.AdvanceMisses(tdAt(last + 0.5))
		if track.TrackState == TrackDeleted {
			t.Fatal("expired at 0.5 s")
		}
		tk.AdvanceMisses(tdAt(last + 1.1))
		if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryMissedUnknown {
			t.Fatalf("state=%s reason=%s after 1.1 s unclassified", track.TrackState, track.ExpiryReason)
		}
	})
}

// A coasting car-sized track must not reclaim a pedestrian-sized cluster
// because it happens to be the nearest box. The shipped tracker does.
func TestReacquisitionGuardRefusesAnIncompatibleBody(t *testing.T) {
	scene := func(cfg TrackerConfig, returning WorldCluster) (*Tracker, *TrackedObject) {
		tk, car, last := confirmedTrack(t, cfg, 10, 5, 4.5, 1.8)
		tk.Update(nil, tdAt(last+0.1))
		tk.Update([]WorldCluster{returning}, tdAt(last+0.2))
		return tk, car
	}
	pedestrian := ccCluster(10.5, 5.2, 0.6, 0.5)

	_, car := scene(DefaultTrackerConfig(), pedestrian)
	if car.LastSupport != SupportObserved {
		t.Fatalf("shipped: the car track did not take the pedestrian cluster (%s); the pinned defect has moved", car.LastSupport)
	}

	guarded := continuityConfig(func(c *OcclusionContinuityConfig) { c.ReacquisitionGuard = true })
	tk, car := scene(guarded, pedestrian)
	if car.LastSupport == SupportObserved {
		t.Fatal("guarded: the car track reclaimed a pedestrian-sized cluster")
	}
	if tk.TracksCreated != 2 || tk.ContinuityStats().ReacquisitionsRefusedSmaller == 0 {
		t.Fatalf("the refused cluster should seed its own track: created=%d stats=%+v", tk.TracksCreated, tk.ContinuityStats())
	}

	// A partial view is smaller than the body and still the body.
	_, car = scene(guarded, ccCluster(11, 5, 1.6, 1.8))
	if car.LastSupport != SupportObserved {
		t.Fatal("guarded: a partial view of the car was refused")
	}
	// A cluster far larger than the belief is a merge or another object.
	tk, car = scene(guarded, ccCluster(10, 5, 12, 2.5))
	if car.LastSupport == SupportObserved || tk.ContinuityStats().ReacquisitionsRefusedLarger == 0 {
		t.Fatal("guarded: a bus-sized cluster reacquired a car")
	}
}

// The guard judges against a corroborated maximum, not the running mean of
// spans. Partial views drag the mean down, and against the mean a whole view
// of the same car would read as a merge on its return.
func TestReacquisitionGuardUsesACorroboratedExtent(t *testing.T) {
	guarded := continuityConfig(func(c *OcclusionContinuityConfig) { c.ReacquisitionGuard = true })
	tk := NewTracker(guarded)
	now := 1.0
	for i := 0; i < 13; i++ {
		length := float32(1.5) // mostly partial views
		if i < 3 {
			length = 4.5 // three whole ones
		}
		tk.Update([]WorldCluster{ccCluster(10, 5, length, 1.8)}, tdAt(now))
		now += 0.1
	}
	car := soleActive(t, tk)
	mean := car.believedLongExtent()
	if limit := mean*reacquisitionMaxExtentRatio + reacquisitionExtentSlackMetre; limit >= 4.5 {
		t.Fatalf("setup: the running mean %.2f m would admit the whole view anyway", mean)
	}
	tk.Update(nil, tdAt(now))
	tk.Update([]WorldCluster{ccCluster(10, 5, 4.5, 1.8)}, tdAt(now+0.1))
	if car.LastSupport != SupportObserved {
		t.Fatalf("a whole view of the car was refused on its return (mean %.2f m, guard belief %.2f m)",
			mean, reacquisitionBelief(car))
	}
	if off := NewTracker(DefaultTrackerConfig()); off.Config.OcclusionContinuity.ReacquisitionGuard {
		t.Fatal("the guard is on by default")
	}
}

// Ambiguous reacquisition stays unresolved rather than being guessed.
func TestReacquisitionGuardLeavesAmbiguityUnresolved(t *testing.T) {
	guarded := continuityConfig(func(c *OcclusionContinuityConfig) { c.ReacquisitionGuard = true })

	t.Run("two separate candidates for one coasting track", func(t *testing.T) {
		tk, ped, last := confirmedTrack(t, guarded, 10, 5, 0.6, 0.5)
		tk.Update(nil, tdAt(last+0.1))
		tk.Update([]WorldCluster{ccCluster(10, 5.6, 0.6, 0.5), ccCluster(10, 4.4, 0.6, 0.5)}, tdAt(last+0.2))
		if ped.LastSupport == SupportObserved {
			t.Fatal("the track guessed between two equidistant pedestrians")
		}
		if tk.ContinuityStats().ReacquisitionsAmbiguous == 0 || tk.TracksCreated != 3 {
			t.Fatalf("created=%d stats=%+v", tk.TracksCreated, tk.ContinuityStats())
		}
	})

	t.Run("two fragments of one body are not an identity question", func(t *testing.T) {
		tk, car, last := confirmedTrack(t, guarded, 10, 5, 4.5, 1.8)
		tk.Update(nil, tdAt(last+0.1))
		tk.Update([]WorldCluster{ccCluster(9, 5, 1.6, 1.8), ccCluster(11, 5, 1.6, 1.8)}, tdAt(last+0.2))
		if car.LastSupport != SupportObserved || tk.ContinuityStats().ReacquisitionsAmbiguous != 0 {
			t.Fatalf("a split car was treated as ambiguous: %s %+v", car.LastSupport, tk.ContinuityStats())
		}
	})

	t.Run("one cluster between two coasting tracks", func(t *testing.T) {
		tk := NewTracker(guarded)
		now := 1.0
		for i := 0; i < 8; i++ {
			tk.Update([]WorldCluster{ccCluster(10, 4.4, 0.6, 0.5), ccCluster(10, 5.6, 0.6, 0.5)}, tdAt(now))
			now += 0.1
		}
		tk.Update(nil, tdAt(now))
		tk.Update([]WorldCluster{ccCluster(10, 5, 0.6, 0.5)}, tdAt(now+0.1))
		for _, tr := range tk.Tracks {
			if tr.TrackState == TrackConfirmed && tr.LastSupport == SupportObserved {
				t.Fatalf("track %s claimed a cluster equidistant from two coasting tracks", tr.TrackID)
			}
		}
		if tk.TracksCreated != 3 {
			t.Fatalf("created=%d: the unresolved cluster should seed a new track", tk.TracksCreated)
		}
	})
}

// The non-finite guard records its reason.
func TestNonFiniteDeletionRecordsItsReason(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	track := &TrackedObject{TrackID: "nan", X: float32(math.NaN())}
	tk.predict(track, 0.1)
	if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryNonFinite || track.Existence != ExistenceExpired {
		t.Fatalf("state=%s reason=%s existence=%s", track.TrackState, track.ExpiryReason, track.Existence)
	}
	if got := tk.ContinuityStats().ExpiredByReason.NonFinite; got != 1 {
		t.Fatalf("NonFinite = %d", got)
	}
}

// A track the update deletes takes none of the observed bookkeeping: no hit,
// no miss reset, no coast reset, no observed support and no reacquisition.
func TestUpdateMatchedSkipsBookkeepingForDeletedTrack(t *testing.T) {
	tk, track, last := confirmedTrack(t, DefaultTrackerConfig(), 10, 5, 4.5, 1.8)
	tk.Update(nil, tdAt(last+0.1))
	if track.Misses == 0 || track.CoastAgeSecs == 0 || track.LastSupport != SupportCoasted {
		t.Fatalf("setup: misses=%d coast=%v support=%s", track.Misses, track.CoastAgeSecs, track.LastSupport)
	}
	hits, misses, coast := track.Hits, track.Misses, track.CoastAgeSecs
	lastObserved := track.LastObservedUnixNanos
	reacquired := tk.ContinuityStats().Reacquisitions

	track.X = float32(math.NaN())
	if tk.updateMatched(track, ccCluster(10, 5, 4.5, 1.8), tdAt(last+0.2).UnixNano()) {
		t.Fatal("updateMatched reported an observation for a track the update deleted")
	}
	if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryNonFinite {
		t.Fatalf("state=%s reason=%s", track.TrackState, track.ExpiryReason)
	}
	if track.Hits != hits || track.Misses != misses || track.CoastAgeSecs != coast {
		t.Fatalf("hits %d→%d misses %d→%d coast %v→%v", hits, track.Hits, misses, track.Misses, coast, track.CoastAgeSecs)
	}
	if track.LastObservedUnixNanos != lastObserved || track.LastSupport == SupportObserved {
		t.Fatalf("deleted track marked observed: last_observed %d→%d support=%s",
			lastObserved, track.LastObservedUnixNanos, track.LastSupport)
	}
	if got := tk.ContinuityStats().Reacquisitions; got != reacquired {
		t.Fatalf("reacquisitions %d→%d", reacquired, got)
	}
}

// Reset and the start of a scoring window both clear the continuity window.
func TestContinuityWindowResets(t *testing.T) {
	tk, _, _ := confirmedTrack(t, DefaultTrackerConfig(), 10, 5, 4.5, 1.8)
	if tk.ContinuityStats().TracksBorn != 1 {
		t.Fatal("setup recorded nothing")
	}
	tk.BeginTrackingBaseline()
	if got := tk.ContinuityStats(); got.TracksBorn != 0 || got.SupportInstants != (SupportCounts{}) {
		t.Fatalf("the scoring window did not start clean: %+v", got)
	}
	tk.Update([]WorldCluster{ccCluster(10, 5, 4.5, 1.8)}, tdAt(9))
	tk.Reset()
	if got := tk.ContinuityStats(); got.SupportInstants != (SupportCounts{}) {
		t.Fatalf("Reset kept %+v", got)
	}

	// The fragmentation proxy counts confirmations of tracks born in the
	// window only: a track born before it and promoted inside is in neither
	// count, so confirmed can never exceed born.
	tk = NewTracker(DefaultTrackerConfig())
	tk.Update([]WorldCluster{ccCluster(10, 5, 4.5, 1.8)}, tdAt(1))
	tk.BeginTrackingBaseline()
	for i := 1; i < 8; i++ {
		clusters := []WorldCluster{ccCluster(10, 5, 4.5, 1.8)}
		if i >= 2 {
			clusters = append(clusters, ccCluster(-10, 5, 4.5, 1.8))
		}
		tk.Update(clusters, tdAt(1+0.1*float64(i)))
	}
	if got := tk.ContinuityStats(); got.TracksBorn != 1 || got.TracksConfirmed != 1 {
		t.Fatalf("born %d, confirmed %d; want the in-window track alone in both", got.TracksBorn, got.TracksConfirmed)
	}
}
