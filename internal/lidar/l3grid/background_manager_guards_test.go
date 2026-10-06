package l3grid

import (
	"testing"
	"time"
)

// TestSetReplayModeTogglesTheFlag covers the replay-mode setter, including the
// nil receiver. Replay mode only lifts warm-up suppression of foreground; it is
// set from the replay path before a manager necessarily exists.
func TestSetReplayModeTogglesTheFlag(t *testing.T) {
	var nilMgr *BackgroundManager
	nilMgr.SetReplayMode(true) // must not panic

	bm := NewBackgroundManagerDI(t.Name(), 2, 4, BackgroundParams{}, nil)
	if bm.replayMode.Load() {
		t.Fatal("replay mode should be off by default")
	}

	bm.SetReplayMode(true)
	if !bm.replayMode.Load() {
		t.Error("SetReplayMode(true) did not set the flag")
	}

	bm.SetReplayMode(false)
	if bm.replayMode.Load() {
		t.Error("SetReplayMode(false) did not clear the flag")
	}
}

// TestGetEnableDiagnosticsReadsTheFlag covers the diagnostics getter and its
// nil-receiver guard. Status surfaces call it on a manager that may not exist
// yet for the sensor being queried.
func TestGetEnableDiagnosticsReadsTheFlag(t *testing.T) {
	var nilMgr *BackgroundManager
	if nilMgr.GetEnableDiagnostics() {
		t.Error("a nil manager should report diagnostics off")
	}

	bm := NewBackgroundManagerDI(t.Name(), 2, 4, BackgroundParams{}, nil)
	if bm.GetEnableDiagnostics() {
		t.Error("diagnostics should be off by default")
	}

	bm.enableDiagnostics.Store(true)
	if !bm.GetEnableDiagnostics() {
		t.Error("GetEnableDiagnostics did not observe the flag")
	}
}

// TestSettlingStatusClampsProgress covers both clamps. Progress is a ratio of
// elapsed time to a configured duration, so a warm-up duration already exceeded
// drives it past 1 — and a status bar reporting 140% helps nobody.
func TestSettlingStatusClampsProgress(t *testing.T) {
	bm := NewBackgroundManagerDI(t.Name(), 2, 4, BackgroundParams{
		WarmupMinFrames:     10,
		WarmupDurationNanos: int64(time.Millisecond),
	}, nil)

	// Started well over the warm-up duration ago, with frames still outstanding
	// so settling has not completed: the duration ratio exceeds 1.
	bm.StartTime = time.Now().Add(-time.Hour)
	bm.Grid.WarmupFramesRemaining = 10

	got := bm.SettlingStatus()

	if got.Complete {
		t.Error("settling should not report complete with frames outstanding")
	}
	if got.Progress < 0 || got.Progress > 1 {
		t.Errorf("Progress = %v, want it clamped into [0, 1]", got.Progress)
	}
	if got.Elapsed < time.Minute {
		t.Errorf("Elapsed = %v, want the full time since StartTime", got.Elapsed)
	}
}

// TestConstructorsWirePersistCallbackWhenStoreProvided covers the store arm of
// both constructors. Without a store the manager logs that persistence is off;
// with one it must install a callback that reaches Persist, including the
// default reason when the snapshot does not carry its own.
func TestConstructorsWirePersistCallbackWhenStoreProvided(t *testing.T) {
	tests := []struct {
		name string
		make func(store BgStore) *BackgroundManager
	}{
		{
			name: "NewBackgroundManager",
			make: func(store BgStore) *BackgroundManager {
				mgr := NewBackgroundManager("persist-cb-registered", 2, 4, BackgroundParams{}, store)
				t.Cleanup(func() { RegisterBackgroundManager("persist-cb-registered", nil) })
				return mgr
			},
		},
		{
			name: "NewBackgroundManagerDI",
			make: func(store BgStore) *BackgroundManager {
				return NewBackgroundManagerDI("persist-cb-di", 2, 4, BackgroundParams{}, store)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockPersistBgStore{}
			mgr := tc.make(store)
			if mgr == nil {
				t.Fatal("constructor returned nil")
			}
			if mgr.PersistCallback == nil {
				t.Fatal("a store was provided but no PersistCallback was installed")
			}

			// A snapshot carrying its own reason keeps it.
			if err := mgr.PersistCallback(&BgSnapshot{SnapshotReason: "settled"}); err != nil {
				t.Fatalf("PersistCallback with a reason: %v", err)
			}
			// A nil snapshot falls back to the default reason rather than panicking.
			if err := mgr.PersistCallback(nil); err != nil {
				t.Fatalf("PersistCallback with no snapshot: %v", err)
			}

			if len(store.snapshots) != 2 {
				t.Fatalf("store received %d snapshots, want 2", len(store.snapshots))
			}
			if got := store.snapshots[0].SnapshotReason; got != "settled" {
				t.Errorf("first snapshot reason = %q, want %q", got, "settled")
			}
			if got := store.snapshots[1].SnapshotReason; got != "manual" {
				t.Errorf("second snapshot reason = %q, want the %q default", got, "manual")
			}
		})
	}
}

// TestConstructorsRejectInvalidDimensions covers the guard both constructors
// share. A zero-dimension grid would index out of range on the first frame.
func TestConstructorsRejectInvalidDimensions(t *testing.T) {
	tests := []struct {
		name          string
		sensorID      string
		rings, azBins int
	}{
		{"empty sensor id", "", 2, 4},
		{"zero rings", "s", 0, 4},
		{"zero azimuth bins", "s", 2, 0},
		{"negative rings", "s", -1, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewBackgroundManagerDI(tc.sensorID, tc.rings, tc.azBins, BackgroundParams{}, nil); got != nil {
				t.Error("NewBackgroundManagerDI accepted invalid dimensions")
			}
		})
	}
}
