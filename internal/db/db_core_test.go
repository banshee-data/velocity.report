package db

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// TestRecordRadarObject tests recording radar objects
func TestRecordRadarObject(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      float64(time.Now().Unix()),
		"end_time":        float64(time.Now().Unix() + 5),
		"delta_time_msec": 5000,
		"max_speed_mps":   15.0,
		"min_speed_mps":   10.0,
		"speed_change":    5.0,
		"max_magnitude":   100,
		"avg_magnitude":   80,
		"total_frames":    50,
		"frames_per_mps":  3.33,
		"length_m":        4.5,
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	err = db.RecordRadarObject(string(eventJSON))
	if err != nil {
		t.Fatalf("RecordRadarObject failed: %v", err)
	}

	// Verify it was inserted
	var count int
	err = db.DB.QueryRow("SELECT COUNT(*) FROM radar_objects").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count radar_objects: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 radar object, got %d", count)
	}
}

// TestRecordRadarObject_InvalidJSON tests error handling
func TestRecordRadarObject_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create a map with a channel which can't be marshaled to JSON
	invalidEvent := map[string]interface{}{
		"classifier": 123, // Invalid type
	}

	invalidJSON, err := json.Marshal(invalidEvent)
	if err != nil {
		t.Fatalf("Failed to marshal invalid event: %v", err)
	}

	err = db.RecordRadarObject(string(invalidJSON))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

// TestRecordRawData tests recording raw radar data
func TestRecordRawData(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	event := map[string]interface{}{
		"uptime":    12345.67,
		"magnitude": 75,
		"speed":     12.5,
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	err = db.RecordRawData(string(eventJSON))
	if err != nil {
		t.Fatalf("RecordRawData failed: %v", err)
	}

	// Verify it was inserted
	var count int
	err = db.DB.QueryRow("SELECT COUNT(*) FROM radar_data").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count radar_data: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 radar data record, got %d", count)
	}
}

// TestEvents tests retrieving all events
func TestEvents(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert some test data
	event1 := map[string]interface{}{
		"uptime":    100.0,
		"magnitude": 50,
		"speed":     10.0,
	}
	event2 := map[string]interface{}{
		"uptime":    200.0,
		"magnitude": 60,
		"speed":     15.0,
	}

	event1JSON, err := json.Marshal(event1)
	if err != nil {
		t.Fatalf("Failed to marshal event1: %v", err)
	}
	event2JSON, err := json.Marshal(event2)
	if err != nil {
		t.Fatalf("Failed to marshal event2: %v", err)
	}

	if err := db.RecordRawData(string(event1JSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}
	if err := db.RecordRawData(string(event2JSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events() failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

// TestEvents_Empty tests retrieving events when none exist
func TestEvents_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events() failed: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

// TestEventToAPI tests the Event to EventAPI conversion
func TestEventToAPI(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		hasSpeed bool
	}{
		{
			name: "with speed",
			event: Event{
				Speed: sql.NullFloat64{Float64: 10.5, Valid: true},
			},
			hasSpeed: true,
		},
		{
			name: "without speed",
			event: Event{
				Speed: sql.NullFloat64{Valid: false},
			},
			hasSpeed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := EventToAPI(tt.event)

			if tt.hasSpeed {
				if api.Speed == nil {
					t.Error("Expected Speed to be non-nil")
				} else if *api.Speed != tt.event.Speed.Float64 {
					t.Errorf("Expected speed %f, got %f", tt.event.Speed.Float64, *api.Speed)
				}
			} else {
				if api.Speed != nil {
					t.Error("Expected Speed to be nil")
				}
			}
		})
	}
}

// TestEventString tests the Event.String() method
func TestEventString(t *testing.T) {
	event := Event{
		Speed: sql.NullFloat64{Float64: 10.5, Valid: true},
	}

	str := event.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Should contain the speed value
	// This is a basic check - the actual format may vary
	if len(str) < 5 {
		t.Error("String representation seems too short")
	}
}

// TestRadarObjects tests retrieving radar objects
func TestRadarObjects(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert a radar object first
	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      1000.0,
		"end_time":        1005.0,
		"delta_time_msec": 5000,
		"max_speed_mps":   15.0,
		"min_speed_mps":   10.0,
		"speed_change":    5.0,
		"max_magnitude":   100,
		"avg_magnitude":   80,
		"total_frames":    50,
		"frames_per_mps":  3.33,
		"length_m":        4.5,
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	if err := db.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	objects, err := db.RadarObjects()
	if err != nil {
		t.Fatalf("RadarObjects() failed: %v", err)
	}

	if len(objects) != 1 {
		t.Errorf("Expected 1 radar object, got %d", len(objects))
	}

	if len(objects) > 0 {
		if objects[0].Classifier != "vehicle" {
			t.Errorf("Expected classifier 'vehicle', got '%s'", objects[0].Classifier)
		}
	}
}

// TestRadarObjectString tests the RadarObject.String() method
func TestRadarObjectString(t *testing.T) {
	obj := RadarObject{
		Classifier: "vehicle",
		MaxSpeed:   15.0,
	}

	str := obj.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Should contain some representation of the data
	if len(str) < 10 {
		t.Error("String representation seems too short")
	}
}

// TestRadarObjectRollupRange_WithFilters tests the rollup with various filters
func TestRadarObjectRollupRange_WithFilters(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert test radar objects with different speeds
	now := time.Now().Unix()

	events := []map[string]interface{}{
		{
			"classifier":      "vehicle",
			"start_time":      float64(now),
			"end_time":        float64(now + 5),
			"delta_time_msec": 5000,
			"max_speed_mps":   5.0,
			"min_speed_mps":   4.0,
			"speed_change":    1.0,
			"max_magnitude":   50,
			"avg_magnitude":   40,
			"total_frames":    25,
			"frames_per_mps":  5.0,
			"length_m":        3.0,
		},
		{
			"classifier":      "vehicle",
			"start_time":      float64(now + 10),
			"end_time":        float64(now + 15),
			"delta_time_msec": 5000,
			"max_speed_mps":   15.0,
			"min_speed_mps":   14.0,
			"speed_change":    1.0,
			"max_magnitude":   100,
			"avg_magnitude":   80,
			"total_frames":    50,
			"frames_per_mps":  3.33,
			"length_m":        4.5,
		},
	}

	for _, event := range events {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRadarObject(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	tests := []struct {
		name          string
		minSpeed      float64
		expectedCount int64
	}{
		{"no filter", 0.0, 2},
		{"filter slow vehicles", 10.0, 1},
		{"filter all", 20.0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.RadarObjectRollupRange(
				now-100, now+100, 3600, // 1 hour buckets
				tt.minSpeed,
				"radar_objects",
				"",
				0, 0,
				0,
			)
			if err != nil {
				t.Fatalf("RadarObjectRollupRange failed: %v", err)
			}

			totalCount := int64(0)
			for _, metric := range result.Metrics {
				totalCount += metric.Count
			}

			if totalCount != tt.expectedCount {
				t.Errorf("Expected count %d, got %d", tt.expectedCount, totalCount)
			}
		})
	}
}
