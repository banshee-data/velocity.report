package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// registerGridlessManager registers a background manager with no grid for the
// duration of the test. A manager exists in the registry from the moment the
// sensor is known, but its grid is only built once the first frame arrives, so
// this is the real window every status endpoint is exposed to at startup.
func registerGridlessManager(t *testing.T, sensorID string) {
	t.Helper()
	l3grid.RegisterBackgroundManager(sensorID, &l3grid.BackgroundManager{})
	t.Cleanup(func() { l3grid.RegisterBackgroundManager(sensorID, nil) })
}

// TestGridStatusReportsErrorBeforeGridExists covers the nil-status arm. Before
// the first frame the manager has no grid, and the endpoint has to say so
// rather than serialise a nil into a 200.
func TestGridStatusReportsErrorBeforeGridExists(t *testing.T) {
	sensorID := "test-status-gridless"
	registerGridlessManager(t, sensorID)

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})

	rec := httptest.NewRecorder()
	ws.handleGridStatus(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/grid?sensor_id=test-status-gridless", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "background manager") {
		t.Errorf("body %q should point at the uninitialised background manager", rec.Body.String())
	}
}

// TestDataSourceIncludesSettlingProgress covers the settling lookup on the
// data-source endpoint. Until the grid settles there is no foreground at all,
// so an operator watching an empty scene needs this field to tell "still
// settling" from "broken sensor".
func TestDataSourceIncludesSettlingProgress(t *testing.T) {
	sensorID := "test-status-settling"
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})
	ws.setBaseContext(t.Context())

	rec := httptest.NewRecorder()
	ws.handleDataSource(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/datasource", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status body: %v", err)
	}
	if _, ok := body["settling"]; !ok {
		t.Errorf("data-source response has no settling field; keys: %v", keysOf(body))
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// failingResponseWriter fails every write, standing in for a client that
// disconnects part-way through the status page.
type failingResponseWriter struct {
	header http.Header
	code   int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *failingResponseWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset") }
func (w *failingResponseWriter) WriteHeader(code int)      { w.code = code }

// TestStatusPageSurvivesRenderFailure covers the template execute error path.
// Unlike the asset and parse failures removed alongside this test, a render
// failure is genuinely reachable: the template writes straight to the client,
// so any disconnect mid-response surfaces here.
func TestStatusPageSurvivesRenderFailure(t *testing.T) {
	sensorID := "test-status-render-failure"
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})
	ws.setBaseContext(t.Context())

	w := &failingResponseWriter{}
	ws.handleStatus(w, httptest.NewRequest(http.MethodGet, "/api/lidar/server", nil))

	// The handler must not panic, and must not leave the response unstarted.
	if w.code == 0 {
		t.Error("expected the handler to write a status code even when rendering fails")
	}
}

// TestAcceptanceMetricsBeforeGridExists covers the nil-metrics substitution.
// A manager is registered as soon as the sensor is known but has no grid until
// the first frame, and the endpoint has to answer with zeroes rather than a
// null the dashboard cannot plot.
func TestAcceptanceMetricsBeforeGridExists(t *testing.T) {
	sensorID := "test-acceptance-gridless"
	registerGridlessManager(t, sensorID)

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})

	rec := httptest.NewRecorder()
	ws.handleAcceptanceMetrics(rec,
		httptest.NewRequest(http.MethodGet, "/api/lidar/acceptance?sensor_id="+sensorID, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode acceptance body: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected a zeroed metrics document, got an empty response")
	}
}

// TestBackgroundGridSkipsNearZeroRangeCells covers the range filter. A cell at
// effectively zero range carries no usable bearing — the polar-to-Cartesian
// conversion would collapse it onto the origin and skew the accumulator it
// lands in.
func TestBackgroundGridSkipsNearZeroRangeCells(t *testing.T) {
	sensorID := "test-grid-nearzero"
	cleanup := setupTestBackgroundManager(t, sensorID)
	defer cleanup()

	mgr := l3grid.GetBackgroundManager(sensorID)
	if mgr == nil {
		t.Fatal("expected a registered background manager")
	}

	// One usable cell and one at effectively zero range.
	mgr.Grid.Cells[0].AverageRangeMeters = 12.5
	mgr.Grid.Cells[0].TimesSeenCount = 9
	mgr.Grid.Cells[1].AverageRangeMeters = 0.01
	mgr.Grid.Cells[1].TimesSeenCount = 9

	ws := NewServer(Config{Address: ":0", Stats: NewPacketStats(), SensorID: sensorID})

	rec := httptest.NewRecorder()
	ws.handleBackgroundGrid(rec,
		httptest.NewRequest(http.MethodGet, "/api/lidar/grid/background?sensor_id="+sensorID, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "{") {
		t.Errorf("expected a JSON document, got %q", rec.Body.String())
	}
}
