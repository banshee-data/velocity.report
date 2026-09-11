package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// locatedSiteToken creates a located case and returns its L16 token — the
// site created alongside it by SetCaseLocation.
func locatedSiteToken(t *testing.T, ws *Server, caseID string) string {
	t.Helper()
	rec := setLocation(t, ws, caseID, `{"origin_lat":37.7987,"origin_lon":-122.4073}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set location = %d: %s", rec.Code, rec.Body.String())
	}
	loc := decodeCaseBody(t, rec)["location"].(map[string]any)
	return loc["s2_l16_token"].(string)
}

func TestSetSiteCanonicalPoseEndpoint(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	token := locatedSiteToken(t, ws, caseID)

	rec := httptest.NewRecorder()
	ws.handleSiteByToken(rec, httptest.NewRequest("PUT", "/api/lidar/sites/"+token,
		strings.NewReader(`{"canonical_lat":37.79875,"canonical_lon":-122.40735,"canonical_source":"surveyed","label":"Broadway & Columbus"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	site := decodeCaseBody(t, rec)
	if site["canonical_lat"] != 37.79875 {
		t.Errorf("canonical_lat = %v, want 37.79875", site["canonical_lat"])
	}
	if site["canonical_source"] != "surveyed" {
		t.Errorf("canonical_source = %v, want surveyed", site["canonical_source"])
	}
	if site["label"] != "Broadway & Columbus" {
		t.Errorf("label = %v, want the given name", site["label"])
	}
	if site["s2_l16_token"] != token {
		t.Errorf("s2_l16_token = %v, want %q", site["s2_l16_token"], token)
	}
}

func TestSetSiteCanonicalPoseEndpointOnAnUnvisitedSite(t *testing.T) {
	ws, _ := locatedCaseServer(t)
	rec := httptest.NewRecorder()
	ws.handleSiteByToken(rec, httptest.NewRequest("PUT", "/api/lidar/sites/deadbeef",
		strings.NewReader(`{"canonical_lat":37.8,"canonical_lon":-122.4,"canonical_source":"operator"}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestSetSiteCanonicalPoseEndpointRejectsBadInput(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	token := locatedSiteToken(t, ws, caseID)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"not JSON", `nonsense`},
		{"unknown source", `{"canonical_lat":37.8,"canonical_lon":-122.4,"canonical_source":"fix"}`},
		{"missing source", `{"canonical_lat":37.8,"canonical_lon":-122.4}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ws.handleSiteByToken(rec, httptest.NewRequest("PUT", "/api/lidar/sites/"+token,
				strings.NewReader(tc.body)))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleSiteByTokenRequiresAToken(t *testing.T) {
	ws, _ := locatedCaseServer(t)
	rec := httptest.NewRecorder()
	ws.handleSiteByToken(rec, httptest.NewRequest("PUT", "/api/lidar/sites/", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleSiteByTokenRejectsOtherMethods(t *testing.T) {
	ws, caseID := locatedCaseServer(t)
	token := locatedSiteToken(t, ws, caseID)
	rec := httptest.NewRecorder()
	ws.handleSiteByToken(rec, httptest.NewRequest("GET", "/api/lidar/sites/"+token, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleSiteByTokenWithoutADatabase(t *testing.T) {
	ws := &Server{}
	rec := httptest.NewRecorder()
	ws.handleSiteByToken(rec, httptest.NewRequest("PUT", "/api/lidar/sites/deadbeef", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
