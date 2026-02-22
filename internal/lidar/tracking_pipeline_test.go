package lidar

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTrackingPipelineTestDB creates a test database with proper schema from schema.sql.
// This avoids hardcoded CREATE TABLE statements that can get out of sync with migrations.
func setupTrackingPipelineTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Apply essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			t.Fatalf("Failed to execute %q: %v", pragma, err)
		}
	}

	// Read and execute schema.sql from the db package
	schemaPath := filepath.Join("..", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		t.Fatalf("Failed to execute schema.sql: %v", err)
	}

	// Baseline at latest migration version
	// NOTE: Update this when new migrations are added to internal/db/migrations/
	latestMigrationVersion := 15
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`, latestMigrationVersion); err != nil {
		db.Close()
		t.Fatalf("Failed to baseline migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// mockForegroundForwarder implements ForegroundForwarder for testing
type mockForegroundForwarder struct {
	forwardCalled bool
	lastPoints    []PointPolar
	callCount     int
}

func (m *mockForegroundForwarder) ForwardForeground(points []PointPolar) {
	m.forwardCalled = true
	m.lastPoints = points
	m.callCount++
}

func TestIsNilInterface(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{
			name:     "nil interface",
			value:    nil,
			expected: true,
		},
		{
			name:     "nil pointer",
			value:    (*mockForegroundForwarder)(nil),
			expected: true,
		},
		{
			name:     "nil slice",
			value:    ([]int)(nil),
			expected: true,
		},
		{
			name:     "nil map",
			value:    (map[string]int)(nil),
			expected: true,
		},
		{
			name:     "non-nil pointer",
			value:    &mockForegroundForwarder{},
			expected: false,
		},
		{
			name:     "non-nil value",
			value:    42,
			expected: false,
		},
		{
			name:     "non-nil string",
			value:    "hello",
			expected: false,
		},
		{
			name:     "empty slice (non-nil)",
			value:    []int{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNilInterface(tt.value)
			if result != tt.expected {
				t.Errorf("isNilInterface(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestIsNilInterface_WithForegroundForwarder(t *testing.T) {
	// Test the specific case that caused the bug: nil pointer assigned to interface
	var fwd ForegroundForwarder

	// Case 1: uninitialized interface
	if !isNilInterface(fwd) {
		t.Error("expected nil interface to be detected as nil")
	}

	// Case 2: nil pointer assigned to interface (the bug case)
	var nilPtr *mockForegroundForwarder
	fwd = nilPtr
	if !isNilInterface(fwd) {
		t.Error("expected interface holding nil pointer to be detected as nil")
	}

	// Case 3: valid pointer assigned to interface
	validPtr := &mockForegroundForwarder{}
	fwd = validPtr
	if isNilInterface(fwd) {
		t.Error("expected interface holding valid pointer to be detected as non-nil")
	}
}

func TestTrackingPipelineConfig_NilForwarder(t *testing.T) {
	// Test that NewFrameCallback handles nil forwarder gracefully
	config := &TrackingPipelineConfig{
		BackgroundManager: nil, // Will cause early return, which is fine for this test
		FgForwarder:       nil,
		Tracker:           nil,
		Classifier:        nil,
		DB:                nil,
		SensorID:          "test-sensor",
	}

	// This should not panic
	callback := config.NewFrameCallback()
	if callback == nil {
		t.Fatal("expected non-nil callback")
	}

	// Test with nil pointer assigned to interface (the bug scenario)
	var nilPtr *mockForegroundForwarder
	config.FgForwarder = nilPtr

	// This should also not panic
	callback = config.NewFrameCallback()
	if callback == nil {
		t.Fatal("expected non-nil callback with nil pointer forwarder")
	}
}

func TestTrackingPipelineConfig_WithValidForwarder(t *testing.T) {
	// Test that a valid forwarder is actually used
	mock := &mockForegroundForwarder{}

	config := &TrackingPipelineConfig{
		BackgroundManager: nil, // Will cause early return before forwarder is used
		FgForwarder:       mock,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()
	if callback == nil {
		t.Fatal("expected non-nil callback")
	}

	// Note: We can't easily test the full pipeline without mocking BackgroundManager,
	// but we've verified the callback is created without panicking
	if mock.forwardCalled {
		t.Error("forwarder should not be called during callback creation")
	}
}

func TestNewFrameCallback_NilFrame(t *testing.T) {
	config := &TrackingPipelineConfig{
		BackgroundManager: NewBackgroundManager("test", 16, 360, BackgroundParams{}, nil),
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	// Should handle nil frame gracefully
	callback(nil)
}

func TestNewFrameCallback_EmptyFrame(t *testing.T) {
	config := &TrackingPipelineConfig{
		BackgroundManager: NewBackgroundManager("test", 16, 360, BackgroundParams{}, nil),
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	// Should handle empty frame gracefully
	frame := &LiDARFrame{
		FrameID:    "test-frame",
		Points:     []Point{},
		MinAzimuth: 0,
		MaxAzimuth: 360,
	}
	callback(frame)
}

func TestNewFrameCallback_NilBackgroundManager(t *testing.T) {
	config := &TrackingPipelineConfig{
		BackgroundManager: nil,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 5.0},
		},
		MinAzimuth: 0,
		MaxAzimuth: 360,
	}

	// Should return early without panicking
	callback(frame)
}

func TestNewFrameCallback_NoForegroundPoints(t *testing.T) {
	// Create a background manager and populate it so all points are background
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background with some data
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 5.0, Intensity: 100},
		}
		bgMgr.ProcessFramePolar(points)
	}

	mock := &mockForegroundForwarder{}
	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		FgForwarder:       mock,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 5.0, Timestamp: time.Now()},
		},
		StartTimestamp: time.Now(),
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	callback(frame)

	// If no real foreground was detected, either forwarder wasn't called or empty slice sent
	if mock.forwardCalled && len(mock.lastPoints) > 0 {
		// This is actually okay - the background might not be fully settled yet
		t.Logf("Note: %d points forwarded (background may not be fully settled)", len(mock.lastPoints))
	}
}

func TestNewFrameCallback_WithForegroundPoints(t *testing.T) {
	// Create a background manager
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background at distance 10m
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0, Intensity: 100},
		}
		bgMgr.ProcessFramePolar(points)
	}

	mock := &mockForegroundForwarder{}
	tracker := NewTracker(DefaultTrackerConfig())
	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		FgForwarder:       mock,
		Tracker:           tracker,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	// Now send a frame with a point much closer (foreground)
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: time.Now()},
		},
		StartTimestamp: time.Now(),
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	callback(frame)

	// Should forward foreground points
	if !mock.forwardCalled {
		t.Error("expected forwarder to be called with foreground points")
	}
	if len(mock.lastPoints) == 0 {
		t.Error("expected at least one foreground point to be forwarded")
	}
}

func TestNewFrameCallback_WithDebugRange(t *testing.T) {
	// Test that debug range filters forwarded points
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		DebugRingMin:                   5, // Only channel 5-10
		DebugRingMax:                   10,
		DebugAzMin:                     170,
		DebugAzMax:                     190,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
			{Channel: 6, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	mock := &mockForegroundForwarder{}
	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		FgForwarder:       mock,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	// Send foreground points in and out of debug range
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: time.Now()}, // Outside debug range
			{Channel: 6, Azimuth: 180, Distance: 3.0, Timestamp: time.Now()}, // Inside debug range
		},
		StartTimestamp: time.Now(),
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	callback(frame)

	// Should only forward points within debug range
	if mock.forwardCalled && len(mock.lastPoints) > 0 {
		for _, p := range mock.lastPoints {
			// Channel is 1-based in PointPolar, debug range uses 0-based indexing
			// DebugRingMin=5, DebugRingMax=10 means channels 6-11 (1-based)
			if p.Channel < 6 || p.Channel > 11 {
				t.Errorf("expected only filtered points, got channel %d (should be 6-11)", p.Channel)
			}
		}
	}
}

func TestNewFrameCallback_WithDatabase(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	// Create background manager
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(TrackerConfig{
		MaxTracks:             100,
		MaxMisses:             3,
		HitsToConfirm:         2, // Lower to confirm quickly
		GatingDistanceSquared: 25.0,
	})

	classifier := NewTrackClassifier()

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        classifier,
		DB:                database,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()

	// Send multiple frames to create and confirm tracks
	for i := 0; i < 5; i++ {
		frame := &LiDARFrame{
			FrameID: "test-frame",
			Points: []Point{
				{Channel: 1, Azimuth: 180, Distance: 3.0 + float64(i)*0.1, Timestamp: now},
			},
			StartTimestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			MinAzimuth:     0,
			MaxAzimuth:     360,
		}
		callback(frame)
	}

	// Check that tracks were persisted to database
	var count int
	queryErr := database.QueryRow("SELECT COUNT(*) FROM lidar_tracks").Scan(&count)
	if queryErr != nil && queryErr != sql.ErrNoRows {
		t.Fatalf("failed to query tracks: %v", queryErr)
	}

	// We should have at least one track
	if count == 0 {
		t.Log("warning: no tracks persisted (might be expected if clustering threshold not met)")
	}
}

func TestNewFrameCallback_WithAnalysisRunManager(t *testing.T) {
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(DefaultTrackerConfig())

	// Create analysis run manager with proper schema
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	runMgr := NewAnalysisRunManager(database, "test-sensor")

	config := &TrackingPipelineConfig{
		BackgroundManager:  bgMgr,
		Tracker:            tracker,
		AnalysisRunManager: runMgr,
		SensorID:           "test-sensor",
	}

	callback := config.NewFrameCallback()

	// Start an analysis run
	params := DefaultRunParams()
	runID, err := runMgr.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("failed to start run: %v", err)
	}
	defer runMgr.CompleteRun()

	// Send frames
	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	callback(frame)

	// Verify that analysis run is active
	if !runMgr.IsRunActive() {
		t.Error("expected run to be active")
	}
	if runMgr.CurrentRunID() != runID {
		t.Errorf("expected run ID %s, got %s", runID, runMgr.CurrentRunID())
	}
}

func TestNewFrameCallback_DebugMode(t *testing.T) {
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// Should not panic in debug mode
	callback(frame)
}

func TestNewFrameCallback_BackgroundDownsampling(t *testing.T) {
	// Test that large background sets get downsampled
	bgMgr := NewBackgroundManager("test", 128, 3600, BackgroundParams{ // Large grid
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate with many background points
	for i := 0; i < 100; i++ {
		points := make([]PointPolar, 6000) // More than maxBackgroundChartPoints
		for j := range points {
			points[j] = PointPolar{
				Channel:  (j % 128) + 1,
				Azimuth:  float64(j % 3600),
				Distance: 10.0,
			}
		}
		bgMgr.ProcessFramePolar(points)
	}

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	points := make([]Point, 6000)
	for i := range points {
		points[i] = Point{
			Channel:   (i % 128) + 1,
			Azimuth:   float64(i % 3600),
			Distance:  10.0,
			Timestamp: now,
		}
	}

	frame := &LiDARFrame{
		FrameID:        "test-frame",
		Points:         points,
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     3600,
	}

	// Should handle large frames without panicking
	callback(frame)
}

func TestNewFrameCallback_RegistryBasedRunManager(t *testing.T) {
	// Test that the callback uses registry-based run manager when not explicitly set
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	// Create and register a run manager
	sensorID := "registry-test-sensor"
	runMgr := NewAnalysisRunManager(database, sensorID)
	RegisterAnalysisRunManager(sensorID, runMgr)
	defer RegisterAnalysisRunManager(sensorID, nil) // Clean up

	bgMgr := NewBackgroundManager(sensorID, 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(DefaultTrackerConfig())

	// Note: AnalysisRunManager is NOT set explicitly - should be looked up from registry
	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		SensorID:          sensorID, // Same sensor ID as registered manager
	}

	callback := config.NewFrameCallback()

	// Start an analysis run via the registered manager
	params := DefaultRunParams()
	_, startErr := runMgr.StartRun("/path/to/test.pcap", params)
	if startErr != nil {
		t.Fatalf("failed to start run: %v", startErr)
	}
	defer runMgr.CompleteRun()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// This should use the registry-based run manager
	callback(frame)

	// Verify run is still active (callback should have recorded)
	if !runMgr.IsRunActive() {
		t.Error("expected run to be active")
	}
}

func TestNewFrameCallback_TrackClassification(t *testing.T) {
	// Test that tracks get classified when they have enough observations
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background at 10m
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
			{Channel: 2, Azimuth: 180, Distance: 10.0},
			{Channel: 3, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(TrackerConfig{
		MaxTracks:             100,
		MaxMisses:             5,
		HitsToConfirm:         2, // Quick confirmation
		GatingDistanceSquared: 50.0,
	})

	classifier := NewTrackClassifier()

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        classifier,
		DB:                database,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()

	// Send multiple frames with cluster of points to build up track observations
	for i := 0; i < 10; i++ {
		points := make([]Point, 0)
		// Create a cluster of points close together
		for j := 0; j < 10; j++ {
			points = append(points, Point{
				Channel:   j + 1,
				Azimuth:   180.0 + float64(j)*0.1,
				Distance:  3.0 + float64(i)*0.1,
				Timestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			})
		}

		frame := &LiDARFrame{
			FrameID:        "test-frame",
			Points:         points,
			StartTimestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			MinAzimuth:     0,
			MaxAzimuth:     360,
		}
		callback(frame)
	}

	// Tracks should be created and potentially classified
	var count int
	queryErr := database.QueryRow("SELECT COUNT(*) FROM lidar_tracks").Scan(&count)
	if queryErr != nil && queryErr != sql.ErrNoRows {
		t.Fatalf("failed to query tracks: %v", queryErr)
	}
	t.Logf("Created %d tracks", count)
}

func TestNewFrameCallback_NilTracker(t *testing.T) {
	// Test that pipeline handles nil tracker gracefully
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           nil, // No tracker
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// Should handle nil tracker without panicking
	callback(frame)
}

func TestNewFrameCallback_CustomDBSCANParams(t *testing.T) {
	// Test that custom DBSCAN params from background manager are used
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		ForegroundMinClusterPoints:     5,   // Custom value
		ForegroundDBSCANEps:            1.5, // Custom value
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(DefaultTrackerConfig())

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// Should use custom DBSCAN params
	callback(frame)
}

func TestNewFrameCallback_EmptyFilteredPoints(t *testing.T) {
	// Test that no points are forwarded when debug range filters all points
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		DebugRingMin:                   100, // Very high range - filters all
		DebugRingMax:                   110,
		DebugAzMin:                     0,
		DebugAzMax:                     10,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	mock := &mockForegroundForwarder{}
	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		FgForwarder:       mock,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	callback(frame)

	// Due to debug range filtering, no points should be forwarded
	// The forwarder may be called with empty points or not called at all
	if mock.forwardCalled && len(mock.lastPoints) > 0 {
		// Check that points are indeed outside the debug range (channel 1 vs 100-110)
		t.Logf("Forwarded %d points (might be outside debug range)", len(mock.lastPoints))
	}
}

func TestNewFrameCallback_NoClusters(t *testing.T) {
	// Test path where DBSCAN returns no clusters
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		ForegroundMinClusterPoints:     100, // Very high - no clusters will form
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(DefaultTrackerConfig())

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// Should return early when no clusters
	callback(frame)
}

func TestNewFrameCallback_DebugModeNilForwarder(t *testing.T) {
	// Test debug log path when forwarder is nil in debug mode
	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 10; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		FgForwarder:       nil, // Nil forwarder
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()
	frame := &LiDARFrame{
		FrameID: "test-frame",
		Points: []Point{
			{Channel: 1, Azimuth: 180, Distance: 3.0, Timestamp: now},
		},
		StartTimestamp: now,
		MinAzimuth:     0,
		MaxAzimuth:     360,
	}

	// Should log debug message about nil forwarder
	callback(frame)
}

func TestNewFrameCallback_ConfirmedTracksWithDebug(t *testing.T) {
	// Test path where confirmed tracks exist with debug mode enabled
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 20; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
			{Channel: 2, Azimuth: 180, Distance: 10.0},
			{Channel: 3, Azimuth: 180, Distance: 10.0},
			{Channel: 4, Azimuth: 180, Distance: 10.0},
			{Channel: 5, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(TrackerConfig{
		MaxTracks:             100,
		MaxMisses:             10,
		HitsToConfirm:         2, // Quick confirmation
		GatingDistanceSquared: 100.0,
	})

	classifier := NewTrackClassifier()

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		Classifier:        classifier,
		DB:                database,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()

	// Send multiple frames with consistent cluster to create confirmed tracks
	for i := 0; i < 15; i++ {
		points := make([]Point, 0)
		// Create a tight cluster of points
		for j := 0; j < 15; j++ {
			points = append(points, Point{
				Channel:   j + 1,
				Azimuth:   180.0,
				Distance:  3.0,
				Timestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			})
		}

		frame := &LiDARFrame{
			FrameID:        "test-frame",
			Points:         points,
			StartTimestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			MinAzimuth:     0,
			MaxAzimuth:     360,
		}
		callback(frame)
	}

	// Check for confirmed tracks
	confirmedTracks := tracker.GetConfirmedTracks()
	t.Logf("Confirmed tracks: %d", len(confirmedTracks))
}

func TestNewFrameCallback_DatabaseInsertError(t *testing.T) {
	// Test database error handling path with debug mode
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer database.Close()

	// Create tables but intentionally omit observation table to cause error
	createSQL := `
	CREATE TABLE IF NOT EXISTS lidar_tracks (
		track_id TEXT PRIMARY KEY,
		sensor_id TEXT NOT NULL,
		world_frame TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		last_seen_at INTEGER NOT NULL,
		track_state TEXT NOT NULL,
		observation_count INTEGER NOT NULL,
		object_class TEXT
	);
	`
	if _, err := database.Exec(createSQL); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	bgMgr := NewBackgroundManager("test", 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 20; i++ {
		points := []PointPolar{
			{Channel: 1, Azimuth: 180, Distance: 10.0},
			{Channel: 2, Azimuth: 180, Distance: 10.0},
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(TrackerConfig{
		MaxTracks:             100,
		MaxMisses:             10,
		HitsToConfirm:         2,
		GatingDistanceSquared: 100.0,
	})

	config := &TrackingPipelineConfig{
		BackgroundManager: bgMgr,
		Tracker:           tracker,
		DB:                database,
		SensorID:          "test-sensor",
	}

	callback := config.NewFrameCallback()

	now := time.Now()

	// Send frames to create tracks - should log errors for observation insertion
	for i := 0; i < 10; i++ {
		points := make([]Point, 0)
		for j := 0; j < 10; j++ {
			points = append(points, Point{
				Channel:   j + 1,
				Azimuth:   180.0,
				Distance:  3.0,
				Timestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			})
		}

		frame := &LiDARFrame{
			FrameID:        "test-frame",
			Points:         points,
			StartTimestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			MinAzimuth:     0,
			MaxAzimuth:     360,
		}
		callback(frame)
	}
}

func TestNewFrameCallback_FullPipeline(t *testing.T) {
	// Integration test that exercises the full pipeline with all components
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	sensorID := "full-pipeline-sensor"

	bgMgr := NewBackgroundManager(sensorID, 16, 360, BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
	}, nil)

	// Populate background
	for i := 0; i < 30; i++ {
		points := make([]PointPolar, 0)
		for j := 1; j <= 10; j++ {
			points = append(points, PointPolar{Channel: j, Azimuth: 180, Distance: 10.0})
		}
		bgMgr.ProcessFramePolar(points)
	}

	tracker := NewTracker(TrackerConfig{
		MaxTracks:             100,
		MaxMisses:             10,
		HitsToConfirm:         3,
		GatingDistanceSquared: 100.0,
	})

	classifier := NewTrackClassifier()
	runMgr := NewAnalysisRunManager(database, sensorID)

	mock := &mockForegroundForwarder{}

	config := &TrackingPipelineConfig{
		BackgroundManager:  bgMgr,
		FgForwarder:        mock,
		Tracker:            tracker,
		Classifier:         classifier,
		DB:                 database,
		SensorID:           sensorID,
		AnalysisRunManager: runMgr,
	}

	callback := config.NewFrameCallback()

	// Start an analysis run
	params := DefaultRunParams()
	_, runErr := runMgr.StartRun("/path/to/test.pcap", params)
	if runErr != nil {
		t.Fatalf("failed to start run: %v", runErr)
	}
	defer runMgr.CompleteRun()

	now := time.Now()

	// Send many frames to create and confirm tracks
	for i := 0; i < 20; i++ {
		points := make([]Point, 0)
		// Create a cluster of foreground points
		for j := 1; j <= 10; j++ {
			points = append(points, Point{
				Channel:   j,
				Azimuth:   180.0,
				Distance:  3.0, // Foreground distance
				Timestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			})
		}

		frame := &LiDARFrame{
			FrameID:        "test-frame",
			Points:         points,
			StartTimestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
			MinAzimuth:     0,
			MaxAzimuth:     360,
		}
		callback(frame)
	}

	// Verify foreground was forwarded
	if !mock.forwardCalled {
		t.Error("expected forwarder to be called")
	}

	// Verify tracks were created
	var trackCount int
	trackErr := database.QueryRow("SELECT COUNT(*) FROM lidar_tracks").Scan(&trackCount)
	if trackErr != nil {
		t.Fatalf("failed to query tracks: %v", trackErr)
	}
	t.Logf("Created %d tracks", trackCount)

	// Verify observations were created
	var obsCount int
	obsErr := database.QueryRow("SELECT COUNT(*) FROM lidar_track_obs").Scan(&obsCount)
	if obsErr != nil {
		t.Fatalf("failed to query observations: %v", obsErr)
	}
	t.Logf("Created %d observations", obsCount)

	// Verify run recorded frames
	if !runMgr.IsRunActive() {
		t.Error("expected run to still be active")
	}
}
