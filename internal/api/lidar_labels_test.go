package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupLabelTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create lidar_tracks table (required for foreign key)
	_, err = db.Exec(`
		CREATE TABLE lidar_tracks (
			track_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			world_frame TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL,
			track_state TEXT NOT NULL,
			observation_count INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_tracks table: %v", err)
	}

	// Insert test track
	_, err = db.Exec(`
		INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, created_at, last_seen_at, track_state, observation_count)
		VALUES ('track-001', 'test-sensor', 'ENU', 1000000000, 2000000000, 'confirmed', 10)
	`)
	if err != nil {
		t.Fatalf("failed to insert test track: %v", err)
	}

	// Create lidar_labels table
	_, err = db.Exec(`
		CREATE TABLE lidar_labels (
			label_id TEXT PRIMARY KEY,
			track_id TEXT NOT NULL,
			class_label TEXT NOT NULL,
			start_timestamp_ns INTEGER NOT NULL,
			end_timestamp_ns INTEGER,
			confidence REAL,
			created_by TEXT,
			created_at_ns INTEGER NOT NULL,
			updated_at_ns INTEGER,
			notes TEXT,
			FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE
		);
		CREATE INDEX idx_lidar_labels_track ON lidar_labels(track_id);
		CREATE INDEX idx_lidar_labels_time ON lidar_labels(start_timestamp_ns, end_timestamp_ns);
		CREATE INDEX idx_lidar_labels_class ON lidar_labels(class_label);
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_labels table: %v", err)
	}

	return db
}

func TestLidarLabelAPI_CreateLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	label := LidarLabel{
		TrackID:          "track-001",
		ClassLabel:       "car",
		StartTimestampNs: 1500000000,
	}

	body, _ := json.Marshal(label)
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/labels", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var created LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.LabelID == "" {
		t.Error("expected label_id to be generated")
	}
	if created.TrackID != "track-001" {
		t.Errorf("expected track_id 'track-001', got '%s'", created.TrackID)
	}
	if created.ClassLabel != "car" {
		t.Errorf("expected class_label 'car', got '%s'", created.ClassLabel)
	}
}

func TestLidarLabelAPI_ListLabels(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	// Insert test labels
	_, err := db.Exec(`
		INSERT INTO lidar_labels (label_id, track_id, class_label, start_timestamp_ns, created_at_ns)
		VALUES 
			('label-001', 'track-001', 'car', 1000000000, 1000000000),
			('label-002', 'track-001', 'truck', 2000000000, 2000000000),
			('label-003', 'track-001', 'car', 3000000000, 3000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test labels: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	count, ok := response["count"].(float64)
	if !ok || int(count) != 3 {
		t.Errorf("expected count 3, got %v", response["count"])
	}
}

func TestLidarLabelAPI_GetLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	// Insert test label
	_, err := db.Exec(`
		INSERT INTO lidar_labels (label_id, track_id, class_label, start_timestamp_ns, created_at_ns)
		VALUES ('label-001', 'track-001', 'car', 1000000000, 1000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test label: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/label-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var label LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&label); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if label.LabelID != "label-001" {
		t.Errorf("expected label_id 'label-001', got '%s'", label.LabelID)
	}
}

func TestLidarLabelAPI_UpdateLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	// Insert test label
	_, err := db.Exec(`
		INSERT INTO lidar_labels (label_id, track_id, class_label, start_timestamp_ns, created_at_ns)
		VALUES ('label-001', 'track-001', 'car', 1000000000, 1000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test label: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	update := LidarLabel{
		ClassLabel: "truck",
	}

	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/labels/label-001", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var updated LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if updated.ClassLabel != "truck" {
		t.Errorf("expected class_label 'truck', got '%s'", updated.ClassLabel)
	}
	if updated.UpdatedAtNs == nil {
		t.Error("expected updated_at_ns to be set")
	}
}

func TestLidarLabelAPI_DeleteLabel(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	// Insert test label
	_, err := db.Exec(`
		INSERT INTO lidar_labels (label_id, track_id, class_label, start_timestamp_ns, created_at_ns)
		VALUES ('label-001', 'track-001', 'car', 1000000000, 1000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test label: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/lidar/labels/label-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Verify deletion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_labels WHERE label_id = ?", "label-001").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query database: %v", err)
	}
	if count != 0 {
		t.Errorf("expected label to be deleted, but found %d rows", count)
	}
}

func TestLidarLabelAPI_Export(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	// Insert test labels
	_, err := db.Exec(`
		INSERT INTO lidar_labels (label_id, track_id, class_label, start_timestamp_ns, created_at_ns)
		VALUES 
			('label-001', 'track-001', 'car', 1000000000, 1000000000),
			('label-002', 'track-001', 'truck', 2000000000, 2000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test labels: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels/export", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var labels []LidarLabel
	if err := json.NewDecoder(rec.Body).Decode(&labels); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(labels))
	}

	// Check Content-Disposition header
	contentDisposition := rec.Header().Get("Content-Disposition")
	if contentDisposition != "attachment; filename=lidar_labels_export.json" {
		t.Errorf("unexpected Content-Disposition: %s", contentDisposition)
	}
}

func TestLidarLabelAPI_FilterByTrackID(t *testing.T) {
	db := setupLabelTestDB(t)
	defer db.Close()

	// Insert another track
	_, err := db.Exec(`
		INSERT INTO lidar_tracks (track_id, sensor_id, world_frame, created_at, last_seen_at, track_state, observation_count)
		VALUES ('track-002', 'test-sensor', 'ENU', 1000000000, 2000000000, 'confirmed', 5)
	`)
	if err != nil {
		t.Fatalf("failed to insert test track: %v", err)
	}

	// Insert test labels
	_, err = db.Exec(`
		INSERT INTO lidar_labels (label_id, track_id, class_label, start_timestamp_ns, created_at_ns)
		VALUES 
			('label-001', 'track-001', 'car', 1000000000, 1000000000),
			('label-002', 'track-002', 'truck', 2000000000, 2000000000)
	`)
	if err != nil {
		t.Fatalf("failed to insert test labels: %v", err)
	}

	api := NewLidarLabelAPI(db)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/labels?track_id=track-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	count, ok := response["count"].(float64)
	if !ok || int(count) != 1 {
		t.Errorf("expected count 1, got %v", response["count"])
	}

	labels, ok := response["labels"].([]interface{})
	if !ok || len(labels) != 1 {
		t.Fatalf("expected 1 label in response")
	}

	firstLabel := labels[0].(map[string]interface{})
	if firstLabel["track_id"] != "track-001" {
		t.Errorf("expected track_id 'track-001', got '%v'", firstLabel["track_id"])
	}
}
