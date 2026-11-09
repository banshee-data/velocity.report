package l5tracks

import (
	"runtime"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// RobustBirthGate is default-off, and off it must leave the shipped tracker
// exactly as it was, with and without the robust likelihood cost it is built
// for. This test pins that: it drives associationScene and the synthetic
// passes through the real Update path under the robust cost and the options
// a birth rule sits beside (the birth gate on the body, the overlap end,
// tentative priority, the cascade), and hashes every cost the assignment saw
// and every tracked state and covariance on every frame. The golden digests
// were taken on the code before the option existed.
//
// They are pinned on amd64 only, for the reason solid_body_face_state_off_test
// gives.

// pieceScene is two cars and the pieces a birth rule is about: car A shedding
// a piece inside its own returns from frame 6 to 20, and car B coasting from
// frame 15 to 20 with a piece beside its prediction from frame 16 to 19.
func pieceScene(*testing.T) []syntheticPassFrame {
	start := time.Unix(1_700_000_000, 0)
	var frames []syntheticPassFrame
	for i := 0; i < 40; i++ {
		ax, bx := 10+0.5*float64(i), 30-0.4*float64(i)
		cs := []WorldCluster{carCluster(int64(10*i+1), ax, 8, 0)}
		if i >= 6 && i <= 20 {
			cs = append(cs, pieceCluster(int64(10*i+2), ax+1.2, 8))
		}
		if i < 15 || i > 20 {
			cs = append(cs, carCluster(int64(10*i+3), bx, -4, 0))
		}
		if i >= 16 && i <= 19 {
			cs = append(cs, pieceCluster(int64(10*i+4), bx+1.5, -3.2))
		}
		frames = append(frames, syntheticPassFrame{at: start.Add(time.Duration(i) * 100 * time.Millisecond), clusters: cs})
	}
	return frames
}

func robustBirthGateDigestScenarios() []associationDigestScenario {
	scene := scriptedScene
	pass := func(p func() l4perception.SyntheticPass) func(t *testing.T) []syntheticPassFrame {
		return func(t *testing.T) []syntheticPassFrame { return syntheticPassFrames(t, p()) }
	}
	robust := func(set func(*TrackerConfig)) func() TrackerConfig {
		return func() TrackerConfig {
			cfg := DefaultTrackerConfig()
			cfg.RobustLikelihoodAssociationCost = true
			if set != nil {
				set(&cfg)
			}
			return cfg
		}
	}
	return []associationDigestScenario{
		{"scene_default", scene, DefaultTrackerConfig, "b305f4eafd3428db"},
		{"scene_robust", scene, robust(nil), "6aa274350ae625d2"},
		{"scene_robust_birth_gate_body", scene, robust(func(cfg *TrackerConfig) {
			cfg.OverlapEndFrames = 3
			cfg.OverlapEndOnBody = true
			cfg.BirthGateOnBody = true
		}), "6aa274350ae625d2"},
		{"scene_robust_tentative_priority", scene, robust(func(cfg *TrackerConfig) {
			cfg.TentativePriority = true
		}), "7e0f124ba75476b4"},
		{"scene_robust_cascade_extent", scene, robust(func(cfg *TrackerConfig) {
			cfg.CascadedAssociation = true
			cfg.AssociationExtentCostWeight = 1
		}), "2ebfc275aa779ffd"},
		{"pieces_default", pieceScene, DefaultTrackerConfig, "7e0240b2f0ea184c"},
		{"pieces_robust", pieceScene, robust(nil), "c852d27be5fcd1bc"},
		{"pieces_robust_birth_gate_body", pieceScene, robust(func(cfg *TrackerConfig) {
			cfg.OverlapEndFrames = 1
			cfg.OverlapEndOnBody = true
			cfg.BirthGateOnBody = true
		}), "5920b07ae0797d17"},
		{"default_pass_robust", pass(l4perception.DefaultSyntheticPass), robust(nil), "66d522027782bc1a"},
		{"truck_near_edge_tracked_robust", pass(endOnTruckPass), func() TrackerConfig {
			cfg := nearEdgeTrackingConfig()
			cfg.RobustLikelihoodAssociationCost = true
			return cfg
		}, "99e2af4fc892d5fe"},
	}
}

func TestRobustBirthGateOffLeavesTheShippedArithmetic(t *testing.T) {
	for _, s := range robustBirthGateDigestScenarios() {
		got := associationFingerprint(s.config(), s.frames(t))
		t.Logf("%s: digest %s", s.name, got)
		if runtime.GOARCH != "amd64" {
			continue
		}
		if got != s.golden {
			t.Errorf("%s: digest %s, want %s", s.name, got, s.golden)
		}
	}
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
}
