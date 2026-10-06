package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/geoindex"
)

const (
	broadwayLat = 37.7987
	broadwayLng = -122.4073
)

// locatedCaseServer creates a server holding one replay case.
func locatedCaseServer(t *testing.T) (*Server, string) {
	t.Helper()
	ws := caseServer(t, 0, "file7.pcap")
	ws.db = mustCaseDB(t)

	rec := httptest.NewRecorder()
	ws.handleCreateScene(rec, httptest.NewRequest("POST", "/api/lidar/scenes",
		strings.NewReader(`{"sensor_id":"hesai-pandar40p","pcap_file":"file7.pcap","description":"broadway_columbus static-4"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	return ws, decodeCaseBody(t, rec)["replay_case_id"].(string)
}

func setLocation(t *testing.T, ws *Server, caseID, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	ws.handleSetCaseLocation(rec,
		httptest.NewRequest("POST", "/api/lidar/scenes/"+caseID+"/location", strings.NewReader(body)),
		caseID)
	return rec
}

func TestSetCaseLocationEndpointDerivesTheFamily(t *testing.T) {
	ws, caseID := locatedCaseServer(t)

	rec := setLocation(t, ws, caseID, `{"origin_lat":37.7987,"origin_lon":-122.4073,"geographic_source":"surveyed"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	loc := decodeCaseBody(t, rec)["location"].(map[string]any)

	if loc["geographic_status"] != "located" {
		t.Errorf("status = %v, want located", loc["geographic_status"])
	}
	if loc["geographic_source"] != "surveyed" {
		t.Errorf("source = %v, want surveyed", loc["geographic_source"])
	}
	// Tokens are the identity; displays are the derived presentation.
	for _, key := range []string{"s2_l10_token", "s2_l13_token", "s2_l16_token"} {
		token, _ := loc[key].(string)
		if token == "" {
			t.Errorf("%s is empty", key)
			continue
		}
		if _, err := geoindex.ParseToken(token); err != nil {
			t.Errorf("%s = %q is not canonical: %v", key, token, err)
		}
	}
	for _, tc := range []struct{ tokenKey, displayKey string }{
		{"s2_l10_token", "s2_l10_display"},
		{"s2_l13_token", "s2_l13_display"},
		{"s2_l16_token", "s2_l16_display"},
	} {
		token, _ := loc[tc.tokenKey].(string)
		display, _ := loc[tc.displayKey].(string)
		if display != geoindex.FamilyDisplay(token) {
			t.Errorf("%s = %q, want FamilyDisplay(%q)", tc.displayKey, display, token)
		}
	}
}

func TestSetCaseLocationEndpointIgnoresClientTokens(t *testing.T) {
	// The tokens are derived server-side with Parent. A family assembled by a
	// client could disagree with itself, which the guide treats as a hard
	// provenance error rather than something to reconcile.
	ws, caseID := locatedCaseServer(t)
	rec := setLocation(t, ws, caseID,
		`{"origin_lat":37.7987,"origin_lon":-122.4073,"s2_l10_token":"deadbeef","s2_l13_token":"nonsense"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	loc := decodeCaseBody(t, rec)["location"].(map[string]any)
	if loc["s2_l10_token"] == "deadbeef" {
		t.Error("a client-supplied token was stored")
	}
	if err := geoindex.Validate(geoindex.Tokens{
		Coarse:  loc["s2_l10_token"].(string),
		Fine:    loc["s2_l13_token"].(string),
		Precise: loc["s2_l16_token"].(string),
	}); err != nil {
		t.Errorf("derived tokens are not one family: %v", err)
	}
}

func TestSetCaseLocationEndpointRejectsBadInput(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"not JSON", `nonsense`, http.StatusBadRequest},
		{"past the pole", `{"origin_lat":91,"origin_lon":0}`, http.StatusBadRequest},
		{"null island", `{"origin_lat":0,"origin_lon":0}`, http.StatusBadRequest},
		{"unknown source", `{"origin_lat":37.8,"origin_lon":-122.4,"geographic_source":"vibes"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := setLocation(t, ws, caseID, tc.body); rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestSetCaseLocationEndpointOnAnUnknownCase(t *testing.T) {
	ws, _ := locatedCaseServer(t)
	rec := setLocation(t, ws, "case-nope", `{"origin_lat":37.8,"origin_lon":-122.4}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestClearCaseLocationEndpoint(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	if rec := setLocation(t, ws, caseID, `{"origin_lat":37.7987,"origin_lon":-122.4073}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d", rec.Code)
	}

	rec := httptest.NewRecorder()
	ws.handleSetCaseLocation(rec,
		httptest.NewRequest("DELETE", "/api/lidar/scenes/"+caseID+"/location", nil), caseID)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body.String())
	}
	if decodeCaseBody(t, rec)["geographic_status"] != "unavailable" {
		t.Error("clearing a location did not report it unavailable")
	}
}

func TestGetCaseReportsItsLocation(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	if rec := setLocation(t, ws, caseID, `{"origin_lat":37.7987,"origin_lon":-122.4073}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d", rec.Code)
	}

	rec := httptest.NewRecorder()
	ws.handleGetScene(rec, httptest.NewRequest("GET", "/api/lidar/scenes/"+caseID, nil), caseID)
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d: %s", rec.Code, rec.Body.String())
	}
	loc, ok := decodeCaseBody(t, rec)["location"].(map[string]any)
	if !ok {
		t.Fatal("reading a located case reported no location")
	}
	if loc["s2_l10_display"] == "" {
		t.Error("the case's family display was not derived on read")
	}
}

func TestSceneMapEndpointGroupsSites(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	if rec := setLocation(t, ws, caseID, `{"origin_lat":37.7987,"origin_lon":-122.4073}`); rec.Code != http.StatusOK {
		t.Fatalf("set = %d", rec.Code)
	}

	rec := httptest.NewRecorder()
	ws.handleSceneMap(rec, httptest.NewRequest("GET", "/api/lidar/scene-map", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scene map = %d: %s", rec.Code, rec.Body.String())
	}
	payload := decodeCaseBody(t, rec)
	if payload["area_count"] != float64(1) {
		t.Fatalf("area_count = %v, want 1", payload["area_count"])
	}
	if payload["site_count"] != float64(1) {
		t.Fatalf("site_count = %v, want 1", payload["site_count"])
	}
	// The levels are reported so a client need not hard-code them.
	if payload["coarse_level"] != float64(geoindex.LevelCoarse) {
		t.Errorf("coarse_level = %v, want %d", payload["coarse_level"], geoindex.LevelCoarse)
	}
	area := payload["areas"].([]any)[0].(map[string]any)
	if !strings.Contains(area["s2_l10_display"].(string), "-") {
		t.Errorf("area display %v is not a family display", area["s2_l10_display"])
	}
	sites := area["sites"].([]any)
	if len(sites) != 1 {
		t.Fatalf("area holds %d sites, want 1", len(sites))
	}
	site := sites[0].(map[string]any)
	if len(site["cases"].([]any)) != 1 {
		t.Error("the site does not link to its case")
	}
}

func TestSceneMapEndpointWithoutADatabase(t *testing.T) {
	ws := &Server{}
	rec := httptest.NewRecorder()
	ws.handleSceneMap(rec, httptest.NewRequest("GET", "/api/lidar/scene-map", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
