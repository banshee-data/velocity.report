package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
)

func laneServer(t *testing.T) *Server {
	t.Helper()
	testDB, cleanup := setupTestDBWrapped(t)
	t.Cleanup(cleanup)
	if _, err := testDB.Exec(`INSERT INTO lidar_sites (site_id, s2_l13_token, s2_l10_token, created_at_ns) VALUES ('808580f47', '808580f4', '80858', 1)`); err != nil {
		t.Fatal(err)
	}
	return &Server{db: testDB}
}

func laneBody(id, limit, from string) []byte {
	r := lanes.Request{
		RequestID: id, Writer: lanes.WriterPainter, SensorID: "s1", RecordID: "gms:rec",
		Pose: lanes.Pose{Rotation: [4]float64{1, 0, 0, 0}}, Author: "operator", GerardusVersion: "0.1",
		Features: []lanes.Feature{{
			FeatureID: "gms:e", RowID: "row-" + limit, Kind: lanes.KindTravelLane, XYZ: []float64{0, -2, 21, 50, -2, 21},
			Tags: map[string]string{"street": "KIRKHAM ST", "width_m": "3", "speed_limit": limit, "speed_limit_unit": "mph",
				"speed_limit_source": "sign", "speed_limit_effective_from": from},
		}},
	}
	b, _ := json.Marshal(r)
	return b
}

func laneCall(ws *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	ws.handleSiteByToken(rec, req)
	return rec
}

func TestSiteLanesPutGetHistoryAndAttribute(t *testing.T) {
	ws := laneServer(t)
	rec := laneCall(ws, http.MethodPut, "/api/lidar/sites/808580f47/lanes", laneBody("r1", "25", "2026-01-01"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	var put PutSiteLanesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &put)
	if put.SnapshotID == "" || put.Lanes != 1 || len(put.Directions) != 1 || put.Directions[0].Name != "Kirkham St eastbound" {
		t.Fatalf("put response %+v", put)
	}
	// The same request again is the same snapshot.
	if rec := laneCall(ws, http.MethodPut, "/api/lidar/sites/808580f47/lanes", laneBody("r1", "25", "2026-01-01")); rec.Code != http.StatusOK {
		t.Fatalf("repeat: %d", rec.Code)
	}
	if rec := laneCall(ws, http.MethodPut, "/api/lidar/sites/808580f47/lanes", laneBody("r1", "30", "2026-01-01")); rec.Code != http.StatusConflict {
		t.Fatalf("reused id: %d", rec.Code)
	}
	if rec := laneCall(ws, http.MethodPut, "/api/lidar/sites/808580f47/lanes", laneBody("r2", "30", "2026-08-01")); rec.Code != http.StatusCreated {
		t.Fatalf("second: %d", rec.Code)
	}

	rec = laneCall(ws, http.MethodGet, "/api/lidar/sites/808580f47/lanes", nil)
	var latest lanes.Snapshot
	_ = json.Unmarshal(rec.Body.Bytes(), &latest)
	if rec.Code != http.StatusOK || latest.Lanes[0].Limit.Value != 30 || latest.Supersedes != put.SnapshotID {
		t.Fatalf("latest: %d %+v", rec.Code, latest)
	}
	rec = laneCall(ws, http.MethodGet, "/api/lidar/sites/808580f47/lanes?at=2026-07-01", nil)
	var at SiteLanesAt
	_ = json.Unmarshal(rec.Body.Bytes(), &at)
	if rec.Code != http.StatusOK || len(at.Lanes) != 1 || at.Lanes[0].Lane.Limit.Value != 25 {
		t.Fatalf("at July: %d %s", rec.Code, rec.Body)
	}
	rec = laneCall(ws, http.MethodGet, "/api/lidar/sites/808580f47/lanes?history=true", nil)
	var history lanes.History
	_ = json.Unmarshal(rec.Body.Bytes(), &history)
	if rec.Code != http.StatusOK || len(history.Versions) != 2 || len(history.Snapshots) != 2 {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	rec = laneCall(ws, http.MethodGet, "/api/lidar/sites/808580f47/lanes/attribute?x=20&y=-2&heading_deg=0&at=2026-09-01", nil)
	var a lanes.Attribution
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if rec.Code != http.StatusOK || a.FeatureID != "gms:e" || a.Limit == nil || a.Limit.Value != 30 {
		t.Fatalf("attribute: %d %s", rec.Code, rec.Body)
	}
}

func TestSiteLanesRefusals(t *testing.T) {
	ws := laneServer(t)
	cases := []struct {
		method, path string
		body         []byte
		want         int
	}{
		{http.MethodPut, "/api/lidar/sites/nowhere/lanes", laneBody("r", "25", "2026-01-01"), http.StatusNotFound},
		{http.MethodPut, "/api/lidar/sites/808580f47/lanes", []byte(`{"request_id": "r", "surprise": 1}`), http.StatusBadRequest},
		{http.MethodPut, "/api/lidar/sites/808580f47/lanes", []byte(`{"request_id": "r"}`), http.StatusBadRequest},
		{http.MethodPut, "/api/lidar/sites/808580f47/lanes", append(laneBody("r2", "25", "2026-01-01"), []byte(` {"request_id": "r3"}`)...), http.StatusBadRequest},
		{http.MethodGet, "/api/lidar/sites/808580f47/lanes", nil, http.StatusNotFound},
		{http.MethodGet, "/api/lidar/sites/808580f47/lanes?at=yesterday", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/lidar/sites/808580f47/lanes/attribute?x=1", nil, http.StatusBadRequest},
		{http.MethodDelete, "/api/lidar/sites/808580f47/lanes", nil, http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/lidar/sites/808580f47/other", nil, http.StatusNotFound},
	}
	for _, c := range cases {
		if rec := laneCall(ws, c.method, c.path, c.body); rec.Code != c.want {
			t.Errorf("%s %s: %d, want %d (%s)", c.method, c.path, rec.Code, c.want, rec.Body)
		}
	}
	// Attribution with no snapshot says so rather than failing.
	rec := laneCall(ws, http.MethodGet, "/api/lidar/sites/808580f47/lanes/attribute?x=1&y=1&heading_deg=0", nil)
	var a lanes.Attribution
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if rec.Code != http.StatusOK || a.Reason != lanes.ReasonNoSnapshot {
		t.Fatalf("no snapshot: %d %s", rec.Code, rec.Body)
	}
}

// The body the macOS painter sends (LaneSnapshotRequest, sorted keys, every
// field present) is read with unknown fields refused.
func TestSiteLanesAcceptsThePaintersBody(t *testing.T) {
	ws := laneServer(t)
	body := []byte(`{"author":"op","features":[{"author":"op","closed":false,"edited_at":"2026-11-04T07:00:00.000Z",
	 "feature_id":"gms:x","kind":"travel_lane","row_id":"row-x","tags":{"confidence":"high","proposal":"gms:p1",
	 "speed_limit":"25","speed_limit_effective_from":"2026-09-02","speed_limit_jurisdiction":"US-CA","speed_limit_source":"statutory",
	 "speed_limit_unit":"mph","street":"COLUMBUS AVE","width_m":"3.2"},"xyz":[0,-2,21,40,-2,21]}],
	 "gerardus_address":"127.0.0.1:50061","gerardus_version":"0.1.0","pose":{"rotation_wxyz":[1,0,0,0],"translation_m":[-14.1,0,0]},
	 "record_id":"gms:accepted","request_id":"3F2504E0-4F89-11D3-9A0C-0305E82C3301","sensor_id":"hesai-01","writer":"painter"}`)
	rec := laneCall(ws, http.MethodPut, "/api/lidar/sites/808580f47/lanes", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("painter body: %d %s", rec.Code, rec.Body)
	}
	var put PutSiteLanesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &put)
	if put.Lanes != 1 || put.Directions[0].Name != "Columbus Ave eastbound" {
		t.Fatalf("answer %+v", put)
	}
}
