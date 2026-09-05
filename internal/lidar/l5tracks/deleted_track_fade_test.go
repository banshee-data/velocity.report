package l5tracks

import (
	"testing"
	"time"
)

// deletedTrack builds a confirmed-then-deleted track that ended deadFor ago.
func deletedTrack(id string, nowNanos int64, deadFor time.Duration) *TrackedObject {
	tr := &TrackedObject{
		TrackID: id,
		TrackMeasurement: TrackMeasurement{
			TrackState:   TrackDeleted,
			EndUnixNanos: nowNanos - int64(deadFor),
		},
	}
	tr.ObservationCount = 30
	return tr
}

// The render fade and the re-association grace period are different concerns
// with different timescales. Publishing over the grace period is what made
// 45.8 per cent of track-frames in run baf20f02 frozen ghosts.
func TestRenderFadeIsShorterThanGracePeriod(t *testing.T) {
	cfg := DefaultTrackerConfig()
	if cfg.DeletedTrackRenderFade <= 0 {
		t.Fatal("DeletedTrackRenderFade is unset in the shipped defaults")
	}
	if cfg.DeletedTrackRenderFade >= cfg.DeletedTrackGracePeriod {
		t.Fatalf("render fade %v is not shorter than the grace period %v, so ghosts persist",
			cfg.DeletedTrackRenderFade, cfg.DeletedTrackGracePeriod)
	}
}

func TestRecentlyDeletedTracksUsesRenderFadeWindow(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.DeletedTrackRenderFade = 500 * time.Millisecond
	cfg.DeletedTrackGracePeriod = 5 * time.Second
	tk := NewTracker(cfg)

	now := time.Now().UnixNano()
	tk.Tracks["fresh"] = deletedTrack("fresh", now, 100*time.Millisecond)
	// Inside the grace period but well past the fade: this is the ghost.
	tk.Tracks["ghost"] = deletedTrack("ghost", now, 3*time.Second)

	got := tk.GetRecentlyDeletedTracks(now)
	if len(got) != 1 {
		t.Fatalf("published %d deleted tracks, want 1", len(got))
	}
	if got[0].TrackID != "fresh" {
		t.Fatalf("published %q, want \"fresh\"", got[0].TrackID)
	}
}

// The grace period must keep its own job: re-association still needs the track
// in the map long after it stops being drawn.
func TestGhostStaysAvailableForReassociation(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.DeletedTrackRenderFade = 500 * time.Millisecond
	cfg.DeletedTrackGracePeriod = 5 * time.Second
	tk := NewTracker(cfg)

	now := time.Now().UnixNano()
	tk.Tracks["ghost"] = deletedTrack("ghost", now, 3*time.Second)

	if len(tk.GetRecentlyDeletedTracks(now)) != 0 {
		t.Fatal("ghost is still being published")
	}
	if _, ok := tk.Tracks["ghost"]; !ok {
		t.Fatal("ghost was dropped from the tracker: re-association can no longer find it")
	}
	if tk.GetDeletedTrackGracePeriod() != 5*time.Second {
		t.Fatal("grace period was changed by the fade decoupling")
	}
}

func TestZeroRenderFadePublishesNothing(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.DeletedTrackRenderFade = 0
	tk := NewTracker(cfg)

	now := time.Now().UnixNano()
	tk.Tracks["fresh"] = deletedTrack("fresh", now, 10*time.Millisecond)

	if got := tk.GetRecentlyDeletedTracks(now); len(got) != 0 {
		t.Fatalf("published %d tracks with the fade disabled, want 0", len(got))
	}
}

func TestGetDeletedTrackRenderFadeAccessor(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.DeletedTrackRenderFade = 750 * time.Millisecond
	tk := NewTracker(cfg)

	if got := tk.GetDeletedTrackRenderFade(); got != 750*time.Millisecond {
		t.Fatalf("accessor returned %v, want 750ms", got)
	}
}
