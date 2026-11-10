package l5tracks

import (
	"runtime"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// NearEdgeCommonGate is default-off, and off it must leave the shipped
// arithmetic exactly as it was: the tracked near-edge filter (A2 and A1), the
// face-state tracked filter, and the default tracker without a solid body.
// This test pins it with the solid-body fingerprint (every bit the tracked
// filter and the solid body publish on every frame). The golden digests were
// taken on the code before the option existed, and are pinned on amd64 only,
// for the reason solid_body_face_state_off_test.go gives.

func nearEdgeCommonGateOffScenarios() []solidBodyFingerprintScenario {
	return []solidBodyFingerprintScenario{
		{
			name:   "default_through_truck",
			pass:   throughTruckPass,
			config: DefaultTrackerConfig,
			golden: "585366e990c09e1a",
		},
		{
			name:   "tracked_default_pass",
			pass:   l4perception.DefaultSyntheticPass,
			config: nearEdgeTrackingConfig,
			golden: "ef9710a8bd964624",
		},
		{
			name: "tracked_face_state_through_truck",
			pass: throughTruckPass,
			config: func() TrackerConfig {
				cfg := faceStateConfig()
				cfg.NearEdgeTracking = true
				return cfg
			},
			golden: "e02c5eac99f656b2",
		},
		{
			name: "tracked_a1_end_on_truck",
			pass: endOnTruckPass,
			config: func() TrackerConfig {
				cfg := nearEdgeTrackingConfig()
				cfg.NearEdgeMedoidGate = true
				return cfg
			},
			golden: "9c74637045061d14",
		},
	}
}

func TestNearEdgeCommonGateOffLeavesTheShippedArithmetic(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	for _, s := range nearEdgeCommonGateOffScenarios() {
		got := solidBodyFingerprint(t, s.config(), syntheticPassFrames(t, s.pass()))
		if got != s.golden {
			t.Errorf("%s: digest %s, want %s", s.name, got, s.golden)
		}
	}
}
