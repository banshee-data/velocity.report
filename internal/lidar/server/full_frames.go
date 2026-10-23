package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// FullFramesControl switches the visualiser stream's full-frames mode: while
// it is on, every frame carries its whole classified cloud (0 background,
// 1 foreground) instead of split streaming's foreground-only points. The
// visualiser's frame adapter implements it; the publisher still records the
// foreground-only form, so a VRLOG does not grow.
type FullFramesControl interface {
	SetFullFrames(on bool)
	FullFrames() bool
}

// errFullFramesUnavailable is the refusal for a server with no visualiser
// stream to switch.
var errFullFramesUnavailable = errors.New("full frames are not available: the server was started without the visualiser gRPC stream")

// errFullFramesNotReplaying is the refusal outside a running PCAP replay.
var errFullFramesNotReplaying = errors.New("full frames are only for a running PCAP replay: start one with POST /api/lidar/pcap/start (a VRLOG replay holds foreground points only, and live input never gets full frames)")

// setReplayFullFrames turns full frames on or off for the PCAP replay that is
// running now. It refuses outside one, so the mode can only ever describe a
// PCAP replay's frames.
//
// fullFramesMu orders this against endReplayFullFrames. The replay goroutine
// releases the replay slot before it clears the mode, so a request that holds
// the lock either sees the replay still running (and its setting is cleared
// after) or sees it gone and is refused: the mode cannot outlive the replay.
func (ws *Server) setReplayFullFrames(on bool) error {
	if ws.fullFrames == nil {
		return errFullFramesUnavailable
	}
	ws.fullFramesMu.Lock()
	defer ws.fullFramesMu.Unlock()
	if !ws.PipelineState().PCAPInProgress() {
		return errFullFramesNotReplaying
	}
	ws.fullFrames.SetFullFrames(on)
	return nil
}

// endReplayFullFrames restores split streaming. Called on every way out of a
// PCAP replay: the replay goroutine's exit (end of file, stop, a failure),
// returning to live, resuming live and parking a finished replay.
func (ws *Server) endReplayFullFrames() {
	if ws.fullFrames == nil {
		return
	}
	ws.fullFramesMu.Lock()
	defer ws.fullFramesMu.Unlock()
	ws.fullFrames.SetFullFrames(false)
}

// fullFramesActive reports whether frames carry their whole cloud now.
func (ws *Server) fullFramesActive() bool {
	return ws.fullFrames != nil && ws.fullFrames.FullFrames()
}

// handleReplayFullFrames switches full frames on or off during a PCAP replay.
// POST /api/lidar/replay/full_frames
// Body: {"enabled": true}
//
// 409 unless a PCAP replay is running, 501 when the server has no visualiser
// stream. The mode ends with the replay whatever the client does.
func (ws *Server) handleReplayFullFrames(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID != "" && sensorID != ws.sensorID {
		ws.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("sensor '%s' is not recognised: check the sensor_id matches the configured sensor", sensorID))
		return
	}

	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			ws.writeJSONError(w, http.StatusBadRequest, "request body is missing: send {\"enabled\": true} or {\"enabled\": false}")
			return
		}
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("the request body is not valid JSON: %v", err))
		return
	}
	if body.Enabled == nil {
		ws.writeJSONError(w, http.StatusBadRequest, "enabled is required: send {\"enabled\": true} or {\"enabled\": false}")
		return
	}

	if err := ws.setReplayFullFrames(*body.Enabled); err != nil {
		status := http.StatusConflict
		if errors.Is(err, errFullFramesUnavailable) {
			status = http.StatusNotImplemented
		}
		ws.writeJSONError(w, status, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"full_frames":    ws.fullFramesActive(),
		"current_source": ws.PipelineState().DataSourceWire(),
	})
}
