package lidar

import (
	"testing"
	"time"
)

func TestTrackerInterface_Implementation(t *testing.T) {
	// Compile-time check that Tracker implements TrackerInterface
	var _ TrackerInterface = (*Tracker)(nil)

	// Runtime check that NewTracker returns an implementation
	config := DefaultTrackerConfig()
	tracker := NewTracker(config)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}

	// Verify it can be used as an interface
	var iface TrackerInterface = tracker
	if iface == nil {
		t.Fatal("expected non-nil interface")
	}
}

func TestTrackerInterface_Update(t *testing.T) {
	config := DefaultTrackerConfig()
	var tracker TrackerInterface = NewTracker(config)

	// Create a simple cluster
	clusters := []WorldCluster{
		{
			CentroidX:         5.0,
			CentroidY:         5.0,
			CentroidZ:         0.5,
			BoundingBoxLength: 2.0,
			BoundingBoxWidth:  1.5,
			BoundingBoxHeight: 1.0,
			PointsCount:       50,
			TSUnixNanos:       time.Now().UnixNano(),
		},
	}

	// Update tracker
	tracker.Update(clusters, time.Now())

	// Verify tracks were created
	tracks := tracker.GetActiveTracks()
	if len(tracks) == 0 {
		t.Error("expected at least one track after Update")
	}
}

func TestTrackerInterface_GetActiveTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	var tracker TrackerInterface = NewTracker(config)

	// Initially no tracks
	tracks := tracker.GetActiveTracks()
	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(tracks))
	}

	// Add a track
	cluster := WorldCluster{
		CentroidX:   10.0,
		CentroidY:   10.0,
		CentroidZ:   0.5,
		PointsCount: 50,
		TSUnixNanos: time.Now().UnixNano(),
	}
	tracker.Update([]WorldCluster{cluster}, time.Now())

	// Should have one active track
	tracks = tracker.GetActiveTracks()
	if len(tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(tracks))
	}

	if tracks[0].State != TrackTentative {
		t.Errorf("expected tentative track, got %s", tracks[0].State)
	}
}

func TestTrackerInterface_GetConfirmedTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2 // Lower threshold for test
	var tracker TrackerInterface = NewTracker(config)

	cluster := WorldCluster{
		CentroidX:   10.0,
		CentroidY:   10.0,
		CentroidZ:   0.5,
		PointsCount: 50,
	}

	// First update - tentative
	timestamp := time.Now()
	cluster.TSUnixNanos = timestamp.UnixNano()
	tracker.Update([]WorldCluster{cluster}, timestamp)

	confirmed := tracker.GetConfirmedTracks()
	if len(confirmed) != 0 {
		t.Errorf("expected 0 confirmed tracks, got %d", len(confirmed))
	}

	// Second update - should be confirmed now
	timestamp = timestamp.Add(100 * time.Millisecond)
	cluster.TSUnixNanos = timestamp.UnixNano()
	tracker.Update([]WorldCluster{cluster}, timestamp)

	confirmed = tracker.GetConfirmedTracks()
	if len(confirmed) != 1 {
		t.Errorf("expected 1 confirmed track, got %d", len(confirmed))
	}
}

func TestTrackerInterface_GetTrack(t *testing.T) {
	config := DefaultTrackerConfig()
	var tracker TrackerInterface = NewTracker(config)

	// Add a track
	cluster := WorldCluster{
		CentroidX:   10.0,
		CentroidY:   10.0,
		CentroidZ:   0.5,
		PointsCount: 50,
		TSUnixNanos: time.Now().UnixNano(),
	}
	tracker.Update([]WorldCluster{cluster}, time.Now())

	// Get the track ID
	tracks := tracker.GetActiveTracks()
	if len(tracks) == 0 {
		t.Fatal("expected at least one track")
	}
	trackID := tracks[0].TrackID

	// Retrieve by ID
	track := tracker.GetTrack(trackID)
	if track == nil {
		t.Fatal("expected to find track by ID")
	}
	if track.TrackID != trackID {
		t.Errorf("expected track ID %s, got %s", trackID, track.TrackID)
	}

	// Try to get non-existent track
	notFound := tracker.GetTrack("non-existent-id")
	if notFound != nil {
		t.Error("expected nil for non-existent track")
	}
}

func TestTrackerInterface_GetTrackCount(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2
	config.MaxMisses = 2
	config.MaxMissesConfirmed = 2 // Use same threshold so confirmed tracks delete quickly
	var tracker TrackerInterface = NewTracker(config)

	// Initially no tracks
	total, tentative, confirmed, deleted := tracker.GetTrackCount()
	if total != 0 || tentative != 0 || confirmed != 0 || deleted != 0 {
		t.Errorf("expected all counts to be 0, got: total=%d, tentative=%d, confirmed=%d, deleted=%d",
			total, tentative, confirmed, deleted)
	}

	// Add a track
	cluster := WorldCluster{
		CentroidX:   10.0,
		CentroidY:   10.0,
		CentroidZ:   0.5,
		PointsCount: 50,
	}
	timestamp := time.Now()
	cluster.TSUnixNanos = timestamp.UnixNano()
	tracker.Update([]WorldCluster{cluster}, timestamp)

	// Should have one tentative track
	total, tentative, confirmed, deleted = tracker.GetTrackCount()
	if total != 1 || tentative != 1 || confirmed != 0 || deleted != 0 {
		t.Errorf("expected total=1, tentative=1, confirmed=0, deleted=0, got: total=%d, tentative=%d, confirmed=%d, deleted=%d",
			total, tentative, confirmed, deleted)
	}

	// Confirm the track
	timestamp = timestamp.Add(100 * time.Millisecond)
	cluster.TSUnixNanos = timestamp.UnixNano()
	tracker.Update([]WorldCluster{cluster}, timestamp)

	total, tentative, confirmed, deleted = tracker.GetTrackCount()
	if total != 1 || tentative != 0 || confirmed != 1 || deleted != 0 {
		t.Errorf("expected total=1, tentative=0, confirmed=1, deleted=0, got: total=%d, tentative=%d, confirmed=%d, deleted=%d",
			total, tentative, confirmed, deleted)
	}

	// Delete the track by not providing observations
	timestamp = timestamp.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, timestamp)
	timestamp = timestamp.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, timestamp)

	total, tentative, confirmed, deleted = tracker.GetTrackCount()
	if total != 1 || tentative != 0 || confirmed != 0 || deleted != 1 {
		t.Errorf("expected total=1, tentative=0, confirmed=0, deleted=1, got: total=%d, tentative=%d, confirmed=%d, deleted=%d",
			total, tentative, confirmed, deleted)
	}
}

func TestTrackerInterface_GetAllTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	config.MaxMisses = 2
	var tracker TrackerInterface = NewTracker(config)

	// Add a track
	cluster := WorldCluster{
		CentroidX:   10.0,
		CentroidY:   10.0,
		CentroidZ:   0.5,
		PointsCount: 50,
	}
	timestamp := time.Now()
	cluster.TSUnixNanos = timestamp.UnixNano()
	tracker.Update([]WorldCluster{cluster}, timestamp)

	// Should have one track
	allTracks := tracker.GetAllTracks()
	if len(allTracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(allTracks))
	}

	// Delete the track
	timestamp = timestamp.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, timestamp)
	timestamp = timestamp.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, timestamp)

	// GetActiveTracks should return 0
	activeTracks := tracker.GetActiveTracks()
	if len(activeTracks) != 0 {
		t.Errorf("expected 0 active tracks, got %d", len(activeTracks))
	}

	// GetAllTracks should still return 1 (including deleted)
	allTracks = tracker.GetAllTracks()
	if len(allTracks) != 1 {
		t.Errorf("expected 1 track (including deleted), got %d", len(allTracks))
	}
	if allTracks[0].State != TrackDeleted {
		t.Errorf("expected deleted track, got state %s", allTracks[0].State)
	}
}
