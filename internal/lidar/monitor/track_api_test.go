package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

func TestTrackAPI_HandleActiveTracks_WithTracker(t *testing.T) {
	// Create a tracker with some test tracks
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Add a test cluster to create a track
	cluster := lidar.WorldCluster{
		SensorID:          "test-sensor",
		CentroidX:         10.0,
		CentroidY:         5.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
		HeightP95:         1.4,
		IntensityMean:     100,
	}

	// Update tracker multiple times to create and confirm a track
	ts := time.Now()
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update([]lidar.WorldCluster{cluster}, ts)
	}

	// Create API with tracker
	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	// Test active tracks endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response TracksListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Count == 0 {
		t.Error("expected at least one track")
	}

	if len(response.Tracks) != response.Count {
		t.Errorf("tracks count mismatch: count=%d, len=%d", response.Count, len(response.Tracks))
	}
}

func TestTrackAPI_HandleActiveTracks_NoTrackerOrDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/active", nil)
	w := httptest.NewRecorder()

	api.handleActiveTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleListTracks_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks", nil)
	w := httptest.NewRecorder()

	api.handleListTracks(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackByID_MissingID(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/", nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackByID_NotFound(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())
	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/nonexistent", nil)
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestTrackAPI_HandleTrackSummary(t *testing.T) {
	tracker := lidar.NewTracker(lidar.DefaultTrackerConfig())

	// Add multiple clusters to create tracks
	clusters := []lidar.WorldCluster{
		{SensorID: "test-sensor", CentroidX: 10.0, CentroidY: 5.0, BoundingBoxLength: 4.0, BoundingBoxHeight: 1.5},
		{SensorID: "test-sensor", CentroidX: 20.0, CentroidY: 15.0, BoundingBoxLength: 1.0, BoundingBoxHeight: 1.8},
	}

	// Update tracker to create tracks
	ts := time.Now()
	for i := 0; i < 5; i++ {
		for j := range clusters {
			clusters[j].CentroidX += 0.5
		}
		ts = ts.Add(100 * time.Millisecond)
		tracker.Update(clusters, ts)
	}

	api := NewTrackAPI(nil, "test-sensor")
	api.SetTracker(tracker)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/tracks/summary", nil)
	w := httptest.NewRecorder()

	api.handleTrackSummary(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response TrackSummaryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Overall.TotalTracks == 0 {
		t.Error("expected at least one track in summary")
	}
}

func TestTrackAPI_HandleListClusters_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/clusters", nil)
	w := httptest.NewRecorder()

	api.handleListClusters(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestTrackAPI_HandleUpdateTrack_NoDB(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	body := `{"object_class": "car"}`
	req := httptest.NewRequest(http.MethodPut, "/api/lidar/tracks/track_1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	api.handleTrackByID(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrackAPI_MethodNotAllowed(t *testing.T) {
	api := NewTrackAPI(nil, "test-sensor")

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list tracks POST", http.MethodPost, "/api/lidar/tracks"},
		{"active tracks POST", http.MethodPost, "/api/lidar/tracks/active"},
		{"summary POST", http.MethodPost, "/api/lidar/tracks/summary"},
		{"clusters POST", http.MethodPost, "/api/lidar/clusters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			switch {
			case tt.path == "/api/lidar/tracks":
				api.handleListTracks(w, req)
			case tt.path == "/api/lidar/tracks/active":
				api.handleActiveTracks(w, req)
			case tt.path == "/api/lidar/tracks/summary":
				api.handleTrackSummary(w, req)
			case tt.path == "/api/lidar/clusters":
				api.handleListClusters(w, req)
			}

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: expected status 405, got %d", tt.name, w.Code)
			}
		})
	}
}
