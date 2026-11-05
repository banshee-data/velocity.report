package l5tracks

import (
	"math"
	"testing"
)

func TestGapLifecycleUsesUnknownSupportAndNoObservations(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxMisses = 20
	cfg.MaxCoastSecsTentative = 0.5
	cfg.MeasurementTimePrediction = true
	tk := NewTracker(cfg)
	tk.Update([]WorldCluster{tdCluster(0, 10, 1.09)}, tdAt(1))
	track := soleActive(t, tk)
	track.VX = 5
	observed := track.LastObservedUnixNanos
	tk.UpdateGap(tdAt(1.2))
	if math.Abs(float64(track.X)-0.55) > 1e-5 || track.StateUnixNanos != tdAt(1.2).UnixNano() {
		t.Fatalf("gap did not predict from acquisition time to common frame time: x=%v time=%d", track.X, track.StateUnixNanos)
	}
	if track.LastSupport != SupportMissedUnknown || track.History[len(track.History)-1].Support != SupportMissedUnknown ||
		track.LastObservedUnixNanos != observed || track.ObservationCount != 1 {
		t.Fatal("a capture gap became an observation or an explained absence")
	}
	// Duplicate and backward capture times cannot predict backwards or make
	// the covariance non-finite. The following forward gap still expires.
	tk.UpdateGap(tdAt(1.2))
	tk.UpdateGap(tdAt(1.1))
	if math.Abs(float64(track.X)-0.55) > 1e-5 {
		t.Fatal("a repeated or backward gap moved the state")
	}
	tk.UpdateGap(tdAt(1.7))
	if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryCoastAge {
		t.Fatal("gap did not enforce the capture-time coast bound")
	}
	stats := tk.TimeDomainStats()
	if stats.DuplicateTimestamps != 1 || stats.BackwardTimestamps != 1 || stats.ExpiredByCoastAge != 1 {
		t.Fatalf("clock discontinuities or expiry unaccounted: %+v", stats)
	}
}
