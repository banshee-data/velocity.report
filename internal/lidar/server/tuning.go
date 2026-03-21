package server

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// handleTuningParams is the unified LIDAR configuration endpoint for all
// tuning parameters including background subtraction, frame builder, and tracker configuration.
//
// Query params: sensor_id (required), source (optional: "default"), format (optional: "compact")
//
// GET: Returns the full nested TuningConfig (L1/L3/L4/L5 sections) as indented JSON.
//   - source=default — returns the built-in default config rather than the live runtime config.
//   - format=compact — returns a single-line JSON response (default is indented).
//
// POST: Accepts partial or full JSON updates using nested objects (or legacy dot-path keys).
//
//	All fields are optional; only runtime-editable fields are applied,
//	non-editable fields are silently ignored.
func (ws *Server) handleTuningParams(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	// source=default returns the built-in config and does not require a
	// running sensor pipeline, so look up the background manager only
	// when we actually need runtime state.
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("source") == "default" {
			resp := cfgpkg.MustLoadDefaultConfig()
			w.Header().Set("Content-Type", "application/json")
			enc := json.NewEncoder(w)
			if r.URL.Query().Get("format") != "compact" {
				enc.SetIndent("", "  ")
			}
			if err := enc.Encode(resp); err != nil {
				opsf("failed to encode response: %v", err)
			}
			return
		}

		bm := l3grid.GetBackgroundManager(sensorID)
		if bm == nil || bm.Grid == nil {
			ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
			return
		}
		resp := ws.runtimeTuningConfig(bm)

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		if r.URL.Query().Get("format") != "compact" {
			enc.SetIndent("", "  ")
		}
		if err := enc.Encode(resp); err != nil {
			opsf("failed to encode response: %v", err)
		}
		return
	case http.MethodPost:
		bm := l3grid.GetBackgroundManager(sensorID)
		if bm == nil || bm.Grid == nil {
			ws.writeJSONError(w, http.StatusNotFound, "no background manager for sensor")
			return
		}

		var body map[string]interface{}
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if r.FormValue("config_json") != "" || mediaType == "application/x-www-form-urlencoded" {
			configJSON := r.FormValue("config_json")
			if configJSON == "" {
				ws.writeJSONError(w, http.StatusBadRequest, "missing config_json form value")
				return
			}
			if err := json.Unmarshal([]byte(configJSON), &body); err != nil {
				ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON in config_json: "+err.Error())
				return
			}
		} else {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
		}
		patch, err := normaliseTuningPatch(body)
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(patch) == 0 {
			ws.writeJSONError(w, http.StatusBadRequest, "empty tuning patch")
			return
		}
		if err := applyRuntimeTuningPatch(ws, bm, patch); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp := ws.runtimeTuningConfig(bm)

		// If this was a form submission, redirect back to status page
		if r.FormValue("config_json") != "" {
			http.Redirect(w, r, fmt.Sprintf("/lidar/server?sensor_id=%s", sensorID), http.StatusSeeOther)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(resp)
		return
	default:
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}
