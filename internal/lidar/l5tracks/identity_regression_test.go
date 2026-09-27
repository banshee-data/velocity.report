package l5tracks_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// K4 as a scene, scored by the per-frame evaluator (gap analysis K4, S1): a
// car hidden behind an occluder emerges level with a pedestrian or cyclist
// on the kerb beside it, close enough that DBSCAN makes one cluster of the
// two. The only track in reach is the small road user's, so on position
// alone it takes the merge, and one track carries clusters of both bodies:
// for the pedestrian the two tracks go on trading the car between them.
// ClassIdentity refuses a track L6 has labelled a small road user a cluster
// outside that label's envelope, so the merge seeds the car's own track
// instead.

// emergingCarScene is the scene for one small road user, walking or riding
// along the kerb at speed m/s while the car passes at 8 m/s.
func emergingCarScene(label string, obs func(string, float32, float32) sceneObs,
	at func(string, float32, float32) sceneTruth, speed float32) scriptedScene {
	sc := scriptedScene{labels: map[string]string{"small": label, "car": "car"}}
	for f := 0; f < 60; f++ {
		elapsed := float32(f) * float32(identityFramePeriod.Seconds())
		sx, sy := 2+speed*elapsed, float32(1.9)
		cx, cy := -20+8*elapsed, float32(0)
		small, car := obs("small", sx, sy), carObs("car", cx, cy)
		frame := scriptedFrame{truth: []sceneTruth{at("small", sx, sy)}}
		switch {
		case cx < sx-3: // behind the occluder
			frame.clusters = []sceneObs{small}
		case clearance(small, car) < dbscanEps:
			frame.clusters = []sceneObs{mergedObs("car", car, small)}
		default:
			frame.clusters = []sceneObs{small, car}
		}
		if cx >= sx-3 {
			frame.truth = append(frame.truth, carAt("car", cx, cy))
		}
		sc.frames = append(sc.frames, frame)
	}
	return sc
}

func TestClassIdentityKeepsAMergeFromCarryingASmallRoadUsersIdentity(t *testing.T) {
	for _, tc := range []struct {
		label string
		obs   func(string, float32, float32) sceneObs
		at    func(string, float32, float32) sceneTruth
		speed float32
	}{
		{"pedestrian", pedestrianObs, pedestrianAt, 1.4},
		{"cyclist", cyclistObs, cyclistAt, 4},
	} {
		t.Run(tc.label, func(t *testing.T) {
			sc := emergingCarScene(tc.label, tc.obs, tc.at, tc.speed)

			off := runScripted(t, l5tracks.DefaultTrackerConfig(), sc)
			t.Logf("off: %s", off.summary())
			if len(off.shared) == 0 {
				t.Fatalf("scene precondition: position alone kept the identities apart (%s); "+
					"the scene no longer exercises K4", off.summary())
			}

			cfg := l5tracks.DefaultTrackerConfig()
			cfg.ClassIdentity = true
			on := runScripted(t, cfg, sc)
			t.Logf("on:  %s refusals=%d", on.summary(), on.stats.ClassIdentityRefusals)
			if len(on.shared) != 0 || on.metrics.IDSwitches != 0 || on.metrics.Fragmentations != 0 {
				t.Fatalf("with ClassIdentity a track still took clusters from both bodies: %s", on.summary())
			}
			for body, takers := range on.takers {
				if len(takers) != 1 {
					t.Errorf("%s was taken by %d tracks %v, want one", body, len(takers), takers)
				}
			}
			if on.stats.ClassIdentityRefusals == 0 {
				t.Error("no ClassIdentity refusal was counted")
			}
			if on.identity.IDF1 <= off.identity.IDF1 {
				t.Errorf("IDF1 %.3f with ClassIdentity, %.3f without; want it higher", on.identity.IDF1, off.identity.IDF1)
			}
		})
	}
}
