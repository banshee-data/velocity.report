package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

func TestShowRadarObjectStats_CosineCorrection(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	site := &db.Site{
		Name:             "Cosine Site",
		Location:         "Test Location",
		CosineErrorAngle: 60.0,
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
	active.CosineErrorAngle = 60.0
	if err := dbInst.UpdateSiteConfigPeriod(active); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	event := map[string]interface{}{
		"classifier":      "all",
		"start_time":      float64(active.EffectiveStartUnix + 1),
		"end_time":        float64(active.EffectiveStartUnix + 2),
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

	startValue := int64(active.EffectiveStartUnix + 1)
	endValue := int64(active.EffectiveStartUnix + 10)
	start := strconv.FormatInt(startValue, 10)
	end := strconv.FormatInt(endValue, 10)
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/radar_stats?start="+start+"&end="+end+"&group=all&site_id="+strconv.Itoa(site.ID),
		nil,
	)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	metrics, ok := response["metrics"].([]interface{})
	if !ok || len(metrics) == 0 {
		t.Fatalf("Expected metrics data")
	}
	metric := metrics[0].(map[string]interface{})
	corrected := metric["MaxSpeed"].(float64)
	expected := 10.0 / math.Cos(math.Radians(60.0))
	if math.Abs(corrected-expected) > 0.01 {
		t.Fatalf("Expected corrected max speed %.2f, got %.2f", expected, corrected)
	}

	correction, ok := response["cosine_correction"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected cosine_correction in response")
	}
	angles := correction["angles"].([]interface{})
	if len(angles) != 1 {
		t.Fatalf("Expected one angle, got %d", len(angles))
	}
}
