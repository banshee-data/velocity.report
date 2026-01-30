package lidar

import (
	"fmt"
	"sync"
	"testing"
)

// TestSidecarState_GetActiveTrackCount tests the thread-safe retrieval of active track count
func TestSidecarState_GetActiveTrackCount(t *testing.T) {
	state := &SidecarState{
		Tracks:       make(map[string]*Track),
		ActiveTracks: 5,
	}

	count := state.GetActiveTrackCount()
	if count != 5 {
		t.Errorf("GetActiveTrackCount() = %d, want 5", count)
	}

	// Test with zero tracks
	state.ActiveTracks = 0
	count = state.GetActiveTrackCount()
	if count != 0 {
		t.Errorf("GetActiveTrackCount() = %d, want 0", count)
	}
}

// TestSidecarState_GetTrack tests retrieving a track by ID
func TestSidecarState_GetTrack(t *testing.T) {
	state := &SidecarState{
		Tracks: make(map[string]*Track),
	}

	// Add a test track
	testTrack := &Track{
		TrackID:  "track-001",
		SensorID: "sensor-01",
		State: TrackState2D{
			X: 10.0,
			Y: 20.0,
		},
	}
	state.Tracks[testTrack.TrackID] = testTrack

	// Test successful retrieval
	track, exists := state.GetTrack("track-001")
	if !exists {
		t.Error("GetTrack() exists = false, want true")
	}
	if track == nil {
		t.Fatal("GetTrack() returned nil track")
	}
	if track.TrackID != "track-001" {
		t.Errorf("GetTrack() TrackID = %s, want track-001", track.TrackID)
	}
	if track.State.X != 10.0 || track.State.Y != 20.0 {
		t.Errorf("GetTrack() State = (%f, %f), want (10.0, 20.0)", track.State.X, track.State.Y)
	}

	// Test non-existent track
	_, exists = state.GetTrack("nonexistent")
	if exists {
		t.Error("GetTrack('nonexistent') exists = true, want false")
	}
}

// TestSidecarState_AddTrack tests adding a track to the state
func TestSidecarState_AddTrack(t *testing.T) {
	state := &SidecarState{
		Tracks:       make(map[string]*Track),
		ActiveTracks: 0,
		TrackCount:   0,
	}

	// Add first track
	track1 := &Track{
		TrackID:  "track-001",
		SensorID: "sensor-01",
	}
	state.AddTrack(track1)

	if state.ActiveTracks != 1 {
		t.Errorf("ActiveTracks = %d, want 1", state.ActiveTracks)
	}
	if state.TrackCount != 1 {
		t.Errorf("TrackCount = %d, want 1", state.TrackCount)
	}
	if _, exists := state.Tracks["track-001"]; !exists {
		t.Error("Track was not added to Tracks map")
	}

	// Add second track
	track2 := &Track{
		TrackID:  "track-002",
		SensorID: "sensor-02",
	}
	state.AddTrack(track2)

	if state.ActiveTracks != 2 {
		t.Errorf("ActiveTracks = %d, want 2", state.ActiveTracks)
	}
	if state.TrackCount != 2 {
		t.Errorf("TrackCount = %d, want 2", state.TrackCount)
	}

	// Verify both tracks exist
	if len(state.Tracks) != 2 {
		t.Errorf("len(Tracks) = %d, want 2", len(state.Tracks))
	}
}

// TestSidecarState_RemoveTrack tests removing a track from the state
func TestSidecarState_RemoveTrack(t *testing.T) {
	state := &SidecarState{
		Tracks: make(map[string]*Track),
	}

	// Add tracks
	track1 := &Track{TrackID: "track-001"}
	track2 := &Track{TrackID: "track-002"}
	state.AddTrack(track1)
	state.AddTrack(track2)

	if state.ActiveTracks != 2 {
		t.Fatalf("Setup failed: ActiveTracks = %d, want 2", state.ActiveTracks)
	}

	// Remove first track
	state.RemoveTrack("track-001")

	if state.ActiveTracks != 1 {
		t.Errorf("After remove: ActiveTracks = %d, want 1", state.ActiveTracks)
	}
	if _, exists := state.Tracks["track-001"]; exists {
		t.Error("track-001 still exists after removal")
	}
	if _, exists := state.Tracks["track-002"]; !exists {
		t.Error("track-002 was removed unexpectedly")
	}

	// Remove second track
	state.RemoveTrack("track-002")

	if state.ActiveTracks != 0 {
		t.Errorf("After remove all: ActiveTracks = %d, want 0", state.ActiveTracks)
	}
	if len(state.Tracks) != 0 {
		t.Errorf("len(Tracks) = %d, want 0", len(state.Tracks))
	}

	// Remove non-existent track (should not panic)
	state.RemoveTrack("nonexistent")
	if state.ActiveTracks != 0 {
		t.Errorf("ActiveTracks changed after removing nonexistent track: %d", state.ActiveTracks)
	}
}

// TestSidecarState_ConcurrentOperations tests thread-safe concurrent operations
func TestSidecarState_ConcurrentOperations(t *testing.T) {
	state := &SidecarState{
		Tracks: make(map[string]*Track),
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	tracksPerGoroutine := 5

	// Concurrent adds
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < tracksPerGoroutine; i++ {
				trackID := formatTrackID(goroutineID, i)
				track := &Track{
					TrackID:  trackID,
					SensorID: "sensor-test",
				}
				state.AddTrack(track)
			}
		}(g)
	}

	wg.Wait()

	expectedTotal := int64(numGoroutines * tracksPerGoroutine)
	if state.GetActiveTrackCount() != expectedTotal {
		t.Errorf("After concurrent adds: ActiveTracks = %d, want %d",
			state.GetActiveTrackCount(), expectedTotal)
	}

	// Concurrent reads and removes
	for g := 0; g < numGoroutines; g++ {
		wg.Add(2)

		// Reader goroutine
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < tracksPerGoroutine; i++ {
				trackID := formatTrackID(goroutineID, i)
				_, _ = state.GetTrack(trackID)
				_ = state.GetActiveTrackCount()
			}
		}(g)

		// Remover goroutine
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < tracksPerGoroutine; i++ {
				trackID := formatTrackID(goroutineID, i)
				state.RemoveTrack(trackID)
			}
		}(g)
	}

	wg.Wait()

	// All tracks should be removed
	if state.GetActiveTrackCount() != 0 {
		t.Errorf("After concurrent removes: ActiveTracks = %d, want 0",
			state.GetActiveTrackCount())
	}
}

// Helper function to format track IDs for concurrent tests
func formatTrackID(goroutineID, trackNum int) string {
	// Use fmt.Sprintf for robust formatting
	return fmt.Sprintf("g%02d-t%d", goroutineID, trackNum)
}

// TestNewPerformanceEvent tests creation of performance events
func TestNewPerformanceEvent(t *testing.T) {
	sensorID := "sensor-test"
	metricName := "frame_processing_time_ms"
	metricValue := 42.5

	event := NewPerformanceEvent(&sensorID, metricName, metricValue)

	if event == nil {
		t.Fatal("NewPerformanceEvent() returned nil")
	}
	if event.Level != "info" {
		t.Errorf("Level = %s, want info", event.Level)
	}
	if event.Message != "Performance metric recorded" {
		t.Errorf("Message = %s, want 'Performance metric recorded'", event.Message)
	}
	if event.EventType != "performance" {
		t.Errorf("EventType = %s, want performance", event.EventType)
	}
	if event.SensorID == nil || *event.SensorID != sensorID {
		t.Errorf("SensorID = %v, want %s", event.SensorID, sensorID)
	}

	// Check context
	if event.Context == nil {
		t.Fatal("Context is nil")
	}
	if event.Context["metric_name"] != metricName {
		t.Errorf("Context[metric_name] = %v, want %s", event.Context["metric_name"], metricName)
	}
	if event.Context["metric_value"] != metricValue {
		t.Errorf("Context[metric_value] = %v, want %f", event.Context["metric_value"], metricValue)
	}

	// Test with nil sensor ID
	eventNil := NewPerformanceEvent(nil, "test_metric", 100.0)
	if eventNil.SensorID != nil {
		t.Error("Expected nil SensorID")
	}
}

// TestNewTrackInitiateEvent tests creation of track initiate events
func TestNewTrackInitiateEvent(t *testing.T) {
	trackID := "track-xyz"
	sensorID := "sensor-abc"
	initialPos := [2]float32{15.5, 25.3}

	event := NewTrackInitiateEvent(trackID, sensorID, initialPos)

	if event == nil {
		t.Fatal("NewTrackInitiateEvent() returned nil")
	}
	if event.Level != "info" {
		t.Errorf("Level = %s, want info", event.Level)
	}
	if event.Message != "New track initiated" {
		t.Errorf("Message = %s, want 'New track initiated'", event.Message)
	}
	if event.EventType != "track_initiate" {
		t.Errorf("EventType = %s, want track_initiate", event.EventType)
	}
	if event.SensorID == nil || *event.SensorID != sensorID {
		t.Errorf("SensorID = %v, want %s", event.SensorID, sensorID)
	}

	// Check context
	if event.Context == nil {
		t.Fatal("Context is nil")
	}
	if event.Context["track_id"] != trackID {
		t.Errorf("Context[track_id] = %v, want %s", event.Context["track_id"], trackID)
	}

	posMap, ok := event.Context["initial_position"].(map[string]float32)
	if !ok {
		t.Fatalf("Context[initial_position] has wrong type: %T", event.Context["initial_position"])
	}
	if posMap["x"] != initialPos[0] {
		t.Errorf("initial_position.x = %f, want %f", posMap["x"], initialPos[0])
	}
	if posMap["y"] != initialPos[1] {
		t.Errorf("initial_position.y = %f, want %f", posMap["y"], initialPos[1])
	}
}

// TestNewTrackTerminateEvent tests creation of track terminate events
func TestNewTrackTerminateEvent(t *testing.T) {
	trackID := "track-end"
	sensorID := "sensor-end"
	finalStats := map[string]interface{}{
		"total_observations": 150,
		"avg_speed_mps":      12.5,
		"duration_seconds":   30.0,
	}

	event := NewTrackTerminateEvent(trackID, sensorID, finalStats)

	if event == nil {
		t.Fatal("NewTrackTerminateEvent() returned nil")
	}
	if event.Level != "info" {
		t.Errorf("Level = %s, want info", event.Level)
	}
	if event.Message != "Track terminated" {
		t.Errorf("Message = %s, want 'Track terminated'", event.Message)
	}
	if event.EventType != "track_terminate" {
		t.Errorf("EventType = %s, want track_terminate", event.EventType)
	}
	if event.SensorID == nil || *event.SensorID != sensorID {
		t.Errorf("SensorID = %v, want %s", event.SensorID, sensorID)
	}

	// Check context
	if event.Context == nil {
		t.Fatal("Context is nil")
	}
	if event.Context["track_id"] != trackID {
		t.Errorf("Context[track_id] = %v, want %s", event.Context["track_id"], trackID)
	}

	statsMap, ok := event.Context["final_stats"].(map[string]interface{})
	if !ok {
		t.Fatalf("Context[final_stats] has wrong type: %T", event.Context["final_stats"])
	}
	if statsMap["total_observations"] != 150 {
		t.Errorf("final_stats.total_observations = %v, want 150", statsMap["total_observations"])
	}
	if statsMap["avg_speed_mps"] != 12.5 {
		t.Errorf("final_stats.avg_speed_mps = %v, want 12.5", statsMap["avg_speed_mps"])
	}

	// Test with empty stats
	eventEmpty := NewTrackTerminateEvent("track-2", "sensor-2", map[string]interface{}{})
	if eventEmpty.Context["final_stats"] == nil {
		t.Error("Expected empty map, got nil")
	}
}
