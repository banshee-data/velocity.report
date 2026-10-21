package server

import (
	"fmt"
	"html"
	"net/http"
)

// The two :8081 dashboards that still have no Svelte replacement. Both are
// static pages that fetch JSON from this server; neither uses go-echarts. They
// leave with status.html once the Svelte sweep and regions pages ship.

// handleSweepDashboard renders the parameter sweep dashboard.
func (ws *Server) handleSweepDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}
	// Use html.EscapeString because the value is interpolated into an HTML
	// attribute (meta tag) in sweep_dashboard.html: the JS now reads
	// sensorId from the DOM instead of a string literal.
	safeSensorID := html.EscapeString(sensorID)

	doc := fmt.Sprintf(sweepDashboardHTML, safeSensorID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(doc))
}

// handleBackgroundRegionsDashboard renders an HTML visualization of the background regions
func (ws *Server) handleBackgroundRegionsDashboard(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		sensorID = ws.sensorID
	}

	escapedSensorID := html.EscapeString(sensorID)

	// Use escapedSensorID for both instances (title and meta tag)
	// The template has been updated to use %[1]s in both places and handle decoding via DOM
	doc := fmt.Sprintf(regionsDashboardHTML, escapedSensorID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(doc))
}
