package l8analytics

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestFrozenRecoveryUsesSuppliedTable(t *testing.T) {
	cal := l5tracks.UniformNoiseCalibration(0.02)
	cal.ID = "synthetic-only-test-table"
	r, err := CheckFrozenRecovery(cal)
	if err != nil || r.Status != "within" || r.RecoveredFrames != 1 || r.CalibrationID != cal.ID || r.GateVerdict != "individual_row_only_no_promotion" {
		t.Fatalf("recovery %+v: %v", r, err)
	}
	cal.ID = ""
	if _, err := CheckFrozenRecovery(cal); err == nil {
		t.Fatal("accepted unnamed table")
	}
	if _, err := CheckFrozenRecovery(nil); err == nil {
		t.Fatal("accepted nil table")
	}
	cal = l5tracks.UniformNoiseCalibration(-1)
	if _, err := CheckFrozenRecovery(cal); err == nil {
		t.Fatal("accepted invalid coefficients")
	}
	// Reborn identities and failure to establish an identity cannot recover.
	for _, neverConfirmed := range []bool{false, true} {
		cfg := l5tracks.DefaultTrackerConfig()
		if neverConfirmed {
			cfg.HitsToConfirm = 1000
		} else {
			cfg.MaxMissesConfirmed = 1
		}
		failed := frozenRecovery(cfg, "synthetic-failure")
		if failed.RecoveredFrames != -1 || failed.Status != "outside" {
			t.Fatalf("credited missing original identity: %+v", failed)
		}
	}
}
