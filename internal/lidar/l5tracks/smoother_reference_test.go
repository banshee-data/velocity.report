package l5tracks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The smoother and a change of reference (near-edge plan S2.4). What carries
// the weight: a chain ends where the stated reference changes, so no state is
// revised by evidence about another point; the break is recorded as
// reference_changed, apart from the numerical barriers; an observation the
// filter did not apply is not evidence; and the tracked body's reading rides
// with each step and state, only when the body's state is the filter's.

// referenceStep is manualStep stating a reference.
func referenceStep(secs float64, x float32, linked bool, reference ReferencePoint) FilterStep {
	s := manualStep(1, secs, x, 1, true, linked)
	s.Reference = reference
	return s
}

func feedSteps(s *FixedLagSmoother, steps ...FilterStep) []SmoothedState {
	var out []SmoothedState
	for _, st := range steps {
		out = append(out, s.Observe(FilterFrame{FrameUnixNanos: st.FrameUnixNanos, Steps: []FilterStep{st}})...)
	}
	return append(out, s.Flush()...)
}

func TestSmootherEndsAChainAtAReferenceChange(t *testing.T) {
	steps := []FilterStep{
		referenceStep(1.0, 0.0, false, ReferenceClusterMedoid),
		referenceStep(1.1, 0.1, true, ReferenceClusterMedoid),
		// The first fix: translated half a body, now the centre.
		referenceStep(1.2, 1.2, true, ReferenceBodyCentre),
		referenceStep(1.3, 1.3, true, ReferenceBodyCentre),
		referenceStep(1.4, 1.4, true, ReferenceBodyCentre),
	}
	for _, lag := range []SmootherLag{LagFrames(3), LagSeconds(0.5), LagTrackEnd()} {
		s, err := NewFixedLagSmoother(SmootherConfig{Lag: lag})
		if err != nil {
			t.Fatal(err)
		}
		out := feedSteps(s, steps...)
		if len(out) != len(steps) {
			t.Fatalf("%s: released %d of %d states", lag, len(out), len(steps))
		}
		for i, st := range out[:2] {
			if st.Release != ReleaseReferenceChanged || !st.LookaheadTruncated || st.Stage != RefinementFixedLag ||
				st.Reference != ReferenceClusterMedoid || st.ReleasedAtUnixNanos != steps[2].FrameUnixNanos {
				t.Fatalf("%s: medoid state %d released %+v", lag, i, st)
			}
			for _, e := range st.Revision.Evidence {
				if e.FrameUnixNanos >= steps[2].FrameUnixNanos {
					t.Fatalf("%s: medoid state %d took evidence from the body centre at %d", lag, i, e.FrameUnixNanos)
				}
			}
		}
		// The last medoid state ends its chain: nothing after it entered, so
		// it is its own posterior.
		if last := out[1]; last.Smoothed.X != last.Online.X || last.Smoothed.VX != last.Online.VX ||
			last.Revision.PositionMetres != 0 || len(last.Revision.Evidence) != 0 {
			t.Fatalf("%s: the state before the change was revised across it: %+v", lag, last)
		}
		if len(out[0].Revision.Evidence) != 1 {
			t.Fatalf("%s: the first state lists %d evidence entries, want its chain's one", lag, len(out[0].Revision.Evidence))
		}
		for _, st := range out[2:] {
			if st.Reference != ReferenceBodyCentre || st.Release == ReleaseReferenceChanged {
				t.Fatalf("%s: body-centre state %+v", lag, st)
			}
		}
		stats := s.Stats()
		if stats.ReferenceChanges != 1 || stats.ReleasedAtReferenceChange != 2 || stats.Barriers != 0 ||
			stats.Released != stats.Steps || stats.HeldSteps != 0 {
			t.Fatalf("%s: stats %+v", lag, stats)
		}
	}
}

// An unlinked step is a missed chain end whatever it refers to: it stays a
// barrier, so the two records never count one break twice.
func TestAnUnlinkedReReferenceIsABarrier(t *testing.T) {
	s, _ := NewFixedLagSmoother(SmootherConfig{Lag: LagFrames(3)})
	out := feedSteps(s,
		referenceStep(1.0, 0.0, false, ReferenceClusterMedoid),
		referenceStep(1.1, 1.1, false, ReferenceBodyCentre),
	)
	stats := s.Stats()
	if stats.Barriers != 1 || stats.ReferenceChanges != 0 || out[0].Release != ReleaseBarrier {
		t.Fatalf("stats %+v, first release %s", stats, out[0].Release)
	}
}

// A report from a replay whose tracks never re-reference is the one written
// before reference changes were counted: the counters are omitted at zero.
func TestSmootherStatsOmitReferenceChangesThatDidNotHappen(t *testing.T) {
	payload, err := json.Marshal(SmootherStats{Steps: 3, Released: 3})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "reference_change") {
		t.Fatalf("zero reference changes were written: %s", payload)
	}
	payload, _ = json.Marshal(SmootherStats{ReferenceChanges: 1, ReleasedAtReferenceChange: 2})
	if !strings.Contains(string(payload), `"reference_changes":1`) || !strings.Contains(string(payload), `"released_at_reference_change":2`) {
		t.Fatalf("reference changes were not written: %s", payload)
	}
}

// An observation whose update the filter did not apply moved nothing, so it
// is not evidence for an earlier state, and a revision it is the only later
// step of stays zero without being flagged.
func TestAnObservationNotAppliedIsNotEvidence(t *testing.T) {
	s, _ := NewFixedLagSmoother(SmootherConfig{Lag: LagFrames(2)})
	first := referenceStep(1.0, 0, false, ReferenceBodyCentre)
	faceless := referenceStep(1.1, 0.1, true, ReferenceBodyCentre)
	faceless.Prior = faceless.Posterior
	faceless.Observation.Disposition, faceless.Observation.Reason = ResidualNotApplied, "no_usable_face"
	out := feedSteps(s, first, faceless)
	if len(out) != 2 {
		t.Fatalf("released %d states", len(out))
	}
	if st := out[0]; len(st.Revision.Evidence) != 0 || st.Revision.PositionMetres != 0 || st.RevisionWithoutEvidence {
		t.Fatalf("a faceless frame entered the earlier state: %+v", st.Revision)
	}
	if out[1].Observation.Applied() || !out[1].Observed {
		t.Fatalf("the faceless step is associated and not applied: %+v", out[1].Observation)
	}
	if !first.Observation.Applied() {
		t.Fatal("an accepted update reads as not applied")
	}
}

// runNearEdgeSmootherScene runs the synthetic pass under cfg with smoothers
// attached from the first frame. The frames from faceless, MaxMisses of them,
// lose their member points, so a body-centre track lapses there and
// re-references once faces return; negative faceless leaves every frame.
func runNearEdgeSmootherScene(t *testing.T, cfg TrackerConfig, faceless int, lags ...SmootherLag) (*Tracker, *smootherArms) {
	t.Helper()
	tracker := NewTracker(cfg)
	arms := newSmootherArms(t, lags...)
	tracker.SetFilterStepObserver(arms)
	for i, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		if faceless >= 0 && i >= faceless && i < faceless+cfg.MaxMisses {
			for c := range f.clusters {
				f.clusters[c].RetainedPoints = nil
			}
		}
		tracker.Update(f.clusters, f.at)
	}
	arms.flush()
	return tracker, arms
}

// stepReferenceChanges counts, over the recorded steps, the linked steps
// that state another reference than their track's previous step.
func stepReferenceChanges(frames []FilterFrame) (total int, perTrack map[int64]int) {
	perTrack = map[int64]int{}
	last := map[int64]ReferencePoint{}
	for _, f := range frames {
		for _, s := range f.Steps {
			if previous, ok := last[s.CreationSequence]; ok && s.Linked && previous != s.Reference {
				total++
				perTrack[s.CreationSequence]++
			}
			last[s.CreationSequence] = s.Reference
		}
		for _, e := range f.Ended {
			delete(last, e.CreationSequence)
		}
	}
	return total, perTrack
}

func TestTheSmootherNeverCrossesANearEdgeReferenceChange(t *testing.T) {
	cfg := nearEdgeTrackingConfig()
	_, arms := runNearEdgeSmootherScene(t, cfg, 14, LagFrames(3), LagSeconds(0.5), LagTrackEnd())

	type stepKey struct{ seq, frame int64 }
	steps := map[stepKey]FilterStep{}
	notApplied := 0
	for _, f := range arms.frames {
		for _, s := range f.Steps {
			steps[stepKey{s.CreationSequence, s.FrameUnixNanos}] = s
			if s.SolidBody == nil {
				t.Fatalf("track %d at %d: a tracked body's step carries no reading", s.CreationSequence, s.FrameUnixNanos)
			}
			// The reading is the filter's state at the end of the frame, at
			// the reference the step states.
			if b := s.SolidBody; b.Estimate.Reference != s.Reference || b.Estimate.X != s.Posterior.X ||
				b.Estimate.Y != s.Posterior.Y || b.VX != s.Posterior.VX || b.Covariance != s.Posterior.P {
				t.Fatalf("track %d at %d: reading %s (%v, %v) is not the step's %s (%v, %v)", s.CreationSequence, s.FrameUnixNanos,
					b.Estimate.Reference, b.Estimate.X, b.Estimate.Y, s.Reference, s.Posterior.X, s.Posterior.Y)
			}
			if s.Observed && !s.Observation.Applied() {
				notApplied++
			}
		}
	}
	changes, perTrack := stepReferenceChanges(arms.frames)
	busiest := 0
	for _, n := range perTrack {
		busiest = max(busiest, n)
	}
	t.Logf("%d reference changes (at most %d on one track), %d associated observations not applied", changes, busiest, notApplied)
	// The vehicle's track re-references at its first fix, lapses in the
	// faceless run and re-references when faces return.
	if busiest < 3 || notApplied == 0 {
		t.Fatalf("the scene re-referenced at most %d times on one track, with %d unapplied observations; want a fix, a lapse and a fix",
			busiest, notApplied)
	}

	for i, released := range arms.released {
		lag := arms.smoothers[i].Config().Lag
		stats := arms.smoothers[i].Stats()
		if stats.ReferenceChanges != int64(changes) || stats.Released != stats.Steps || stats.HeldSteps != 0 {
			t.Fatalf("%s: stats %+v, %d reference changes in the record", lag, stats, changes)
		}
		for seq, states := range byTrack(released) {
			for k, st := range states {
				if st.SolidBody == nil || st.SolidBody.Estimate.Reference != st.Reference || st.SolidBody.Estimate.X != st.Online.X {
					t.Fatalf("%s: track %d at %d carries reading %+v", lag, seq, st.FrameUnixNanos, st.SolidBody)
				}
				for _, e := range st.Revision.Evidence {
					later := steps[stepKey{seq, e.FrameUnixNanos}]
					if later.Reference != st.Reference {
						t.Fatalf("%s: track %d's %s state at %d took evidence about the %s at %d",
							lag, seq, st.Reference, st.FrameUnixNanos, later.Reference, e.FrameUnixNanos)
					}
					if !later.Observation.Applied() {
						t.Fatalf("%s: track %d's state at %d lists an unapplied observation at %d as evidence", lag, seq, st.FrameUnixNanos, e.FrameUnixNanos)
					}
				}
				if st.RevisionWithoutEvidence {
					t.Fatalf("%s: track %d at %d revised without evidence", lag, seq, st.FrameUnixNanos)
				}
				if k+1 < len(states) && states[k+1].Reference != st.Reference {
					if st.Release != ReleaseReferenceChanged || st.Stage != RefinementFixedLag || st.Smoothed.X != st.Online.X || st.Smoothed.Y != st.Online.Y {
						t.Fatalf("%s: track %d's last %s state before the change at %d: %+v", lag, seq, st.Reference, states[k+1].FrameUnixNanos, st)
					}
				}
			}
		}
	}
}

// Without NearEdgeTracking the record and the smoother are what they were:
// no reading rides with a step (a shadow body runs its own filter), every
// observation was applied, and no chain is broken by a reference change.
func TestWithoutNearEdgeTrackingNoStepCarriesABody(t *testing.T) {
	for name, cfg := range map[string]TrackerConfig{"default": DefaultTrackerConfig(), "shadow": solidBodyConfig()} {
		_, arms := runNearEdgeSmootherScene(t, cfg, -1, LagFrames(3))
		steps := 0
		for _, f := range arms.frames {
			for _, s := range f.Steps {
				steps++
				if s.SolidBody != nil || !s.Observation.Applied() || s.Reference != ReferenceClusterMedoid {
					t.Fatalf("%s: step %+v", name, s)
				}
			}
		}
		if steps == 0 {
			t.Fatalf("%s: no steps recorded", name)
		}
		if st := arms.smoothers[0].Stats(); st.ReferenceChanges != 0 || st.ReleasedAtReferenceChange != 0 {
			t.Fatalf("%s: stats %+v", name, st)
		}
		for _, st := range arms.released[0] {
			if st.SolidBody != nil || st.Release == ReleaseReferenceChanged {
				t.Fatalf("%s: released %+v", name, st)
			}
		}
	}
}
