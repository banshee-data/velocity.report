package monitor

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

// setupSweepStoreWithRecord creates a test sweep store with one record inserted.
func setupSweepStoreWithRecord(t *testing.T) (*sql.DB, *sqlite.SweepStore, string) {
	t.Helper()
	db, store := setupTestSweepStoreForHandlers(t)

	// Add the newer columns that migration 000024 adds
	for _, col := range []string{
		"objective_name TEXT DEFAULT ''",
		"objective_version TEXT DEFAULT ''",
		"transform_pipeline_name TEXT DEFAULT ''",
		"transform_pipeline_version TEXT DEFAULT ''",
		"score_components_json TEXT",
		"recommendation_explanation_json TEXT",
		"label_provenance_summary_json TEXT",
	} {
		_, err := db.Exec("ALTER TABLE lidar_sweeps ADD COLUMN " + col)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			t.Fatalf("ALTER TABLE ADD COLUMN %s: %v", col, err)
		}
	}

	sweepID := "test-sweep-001"
	_, err := db.Exec(`INSERT INTO lidar_sweeps (sweep_id, sensor_id, mode, status, request, started_at)
		VALUES (?, 'sensor1', 'sweep', 'completed', '{}', ?)`,
		sweepID, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert sweep: %v", err)
	}
	return db, store, sweepID
}

func TestSweepHandlers_ListSweeps_Success(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store, sensorID: "sensor1"}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps", nil)
	w := httptest.NewRecorder()

	ws.handleListSweeps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 sweep, got %d", len(result))
	}
}

func TestSweepHandlers_ListSweeps_WithParams(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store, sensorID: "sensor1"}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps?sensor_id=sensor1&limit=5", nil)
	w := httptest.NewRecorder()

	ws.handleListSweeps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSweepHandlers_GetSweep_Success(t *testing.T) {
	db, store, sweepID := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps/"+sweepID, nil)
	w := httptest.NewRecorder()

	ws.handleGetSweep(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSweepHandlers_GetSweep_NotFound(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweeps/nonexistent-id", nil)
	w := httptest.NewRecorder()

	ws.handleGetSweep(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSweepHandlers_SweepExplain_StoreNotConfigured(t *testing.T) {
	ws := &WebServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/explain/test", nil)
	w := httptest.NewRecorder()

	ws.handleSweepExplain(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestSweepHandlers_SweepExplain_MissingSweepID(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/explain/", nil)
	w := httptest.NewRecorder()

	ws.handleSweepExplain(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSweepHandlers_SweepExplain_NotFound(t *testing.T) {
	db, store, _ := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/explain/nonexistent", nil)
	w := httptest.NewRecorder()

	ws.handleSweepExplain(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSweepHandlers_SweepExplain_Success(t *testing.T) {
	db, store, sweepID := setupSweepStoreWithRecord(t)
	defer db.Close()

	ws := &WebServer{sweepStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/sweep/explain/"+sweepID, nil)
	w := httptest.NewRecorder()

	ws.handleSweepExplain(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		SweepID string `json:"sweep_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SweepID != sweepID {
		t.Fatalf("expected sweep_id=%s, got %s", sweepID, resp.SweepID)
	}
}

func TestHINTHandlers_StatusSuccess(t *testing.T) {
	runner := &mockHINTRunner{state: map[string]string{"status": "idle"}}
	ws := &WebServer{hintRunner: runner}

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/hint/status", nil)
	w := httptest.NewRecorder()

	ws.handleHINTStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHINTHandlers_StatusNotConfigured(t *testing.T) {
	ws := &WebServer{}
	req := httptest.NewRequest(http.MethodGet, "/api/lidar/hint/status", nil)
	w := httptest.NewRecorder()

	ws.handleHINTStatus(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHINTHandlers_StopSuccess(t *testing.T) {
	runner := &mockHINTRunner{}
	ws := &WebServer{hintRunner: runner}

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/hint/stop", nil)
	w := httptest.NewRecorder()

	ws.handleHINTStop(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if runner.stopCalls != 1 {
		t.Fatalf("expected 1 stop call, got %d", runner.stopCalls)
	}
}

func TestHINTHandlers_StopNotConfigured(t *testing.T) {
	ws := &WebServer{}
	req := httptest.NewRequest(http.MethodPost, "/api/lidar/hint/stop", nil)
	w := httptest.NewRecorder()

	ws.handleHINTStop(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
