package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

func TestHandleSiteConfigPeriods_CreateAndList(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:             "Period Site",
		Location:         "Test Location",
		CosineErrorAngle: 10.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	active, err := dbInst.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}
	active.IsActive = false
	end := 1000.0
	active.EffectiveEndUnix = &end
	if err := dbInst.UpdateSiteConfigPeriod(active); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	payload := db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1200,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   12.0,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/site_config_periods?site_id="+strconv.Itoa(site.ID), nil)
	w = httptest.NewRecorder()
	server.handleSiteConfigPeriods(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var periods []db.SiteConfigPeriod
	if err := json.NewDecoder(w.Body).Decode(&periods); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(periods) == 0 {
		t.Fatalf("Expected periods in response")
	}
}

func TestHandleSiteConfigPeriods_Overlap(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:             "Overlap Site",
		Location:         "Test Location",
		CosineErrorAngle: 10.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	active, err := dbInst.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}
	active.IsActive = false
	end := 1000.0
	active.EffectiveEndUnix = &end
	if err := dbInst.UpdateSiteConfigPeriod(active); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	first := db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1200,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           true,
		CosineErrorAngle:   12.0,
	}
	body, _ := json.Marshal(first)
	req := httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleSiteConfigPeriods(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	overlap := db.SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1500,
		EffectiveEndUnix:   floatPtr(2500),
		IsActive:           false,
		CosineErrorAngle:   15.0,
	}
	body, _ = json.Marshal(overlap)
	req = httptest.NewRequest(http.MethodPost, "/api/site_config_periods", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	server.handleSiteConfigPeriods(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTimeline(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:             "Timeline Site",
		Location:         "Test Location",
		CosineErrorAngle: 10.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}
	if err := dbInst.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	event := map[string]interface{}{
		"classifier":      "all",
		"start_time":      1000.0,
		"end_time":        1001.0,
		"delta_time_msec": 100,
		"max_speed_mps":   10.0,
		"min_speed_mps":   10.0,
		"speed_change":    0.0,
		"max_magnitude":   10,
		"avg_magnitude":   10,
		"total_frames":    1,
		"frames_per_mps":  1.0,
		"length_m":        1.0,
	}
	eventJSON, _ := json.Marshal(event)
	if err := dbInst.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("RecordRadarObject failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/timeline?site_id="+strconv.Itoa(site.ID), nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if _, ok := payload["data_range"]; !ok {
		t.Fatalf("Expected data_range in response")
	}
	if _, ok := payload["config_periods"]; !ok {
		t.Fatalf("Expected config_periods in response")
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
