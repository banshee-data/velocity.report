package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanegeom"
	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// maxLaneSnapshotBytes bounds a snapshot request: a site's street use is a
// few hundred features.
const maxLaneSnapshotBytes = 8 << 20

// handleSiteLanes serves the lane snapshot
// (docs/plans/lidar-lane-snapshot-and-attribution-plan.md):
//
//	PUT /api/lidar/sites/{l16}/lanes                  write a snapshot
//	GET /api/lidar/sites/{l16}/lanes[?at=][&sensor_id=]  the latest, or the lanes in force at a time
//	GET /api/lidar/sites/{l16}/lanes?history=true     every lane version with its window
//	GET /api/lidar/sites/{l16}/lanes/attribute?x=&y=&heading_deg=&at=
func (ws *Server) handleSiteLanes(w http.ResponseWriter, r *http.Request, l16, sub string) {
	switch {
	case sub == "" && r.Method == http.MethodPut:
		ws.handlePutSiteLanes(w, r, l16)
	case sub == "" && r.Method == http.MethodGet:
		ws.handleGetSiteLanes(w, r, l16)
	case sub == "attribute" && r.Method == http.MethodGet:
		ws.handleAttributeSiteLanes(w, r, l16)
	case sub == "" || sub == "attribute":
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	default:
		ws.writeJSONError(w, http.StatusNotFound, "not found")
	}
}

// PutSiteLanesResponse is what a write answers: the snapshot's id, whether it
// repeated a stored request, and the directions derived.
type PutSiteLanesResponse struct {
	SnapshotID string            `json:"snapshot_id"`
	Repeated   bool              `json:"repeated"`
	Supersedes string            `json:"supersedes,omitempty"`
	Lanes      int               `json:"lanes"`
	Directions []lanes.Direction `json:"directions"`
}

func (ws *Server) handlePutSiteLanes(w http.ResponseWriter, r *http.Request, l16 string) {
	if _, err := sqlite.NewSiteStore(ws.db).ByToken(l16); err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			ws.writeJSONError(w, http.StatusNotFound, "no site at this token yet: locate a case there first")
			return
		}
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxLaneSnapshotBytes+1))
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "could not read the request body: "+err.Error())
		return
	}
	if len(body) > maxLaneSnapshotBytes {
		ws.writeJSONError(w, http.StatusRequestEntityTooLarge, "the request is too large")
		return
	}
	var req lanes.Request
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	// One object only: anything after it is refused, not ignored.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body: one object expected")
		return
	}
	snap, repeated, err := sqlite.NewLaneSnapshotStore(ws.db).Put(l16, req, time.Now())
	switch {
	case errors.Is(err, lanes.ErrInvalid):
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, sqlite.ErrLaneRequestConflict):
		ws.writeJSONError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusCreated
	if repeated {
		status = http.StatusOK
	}
	ws.writeJSON(w, status, PutSiteLanesResponse{
		SnapshotID: snap.SnapshotID, Repeated: repeated, Supersedes: snap.Supersedes,
		Lanes: len(snap.Lanes), Directions: snap.Directions,
	})
}

// SiteLanesAt is the lanes in force at a time.
type SiteLanesAt struct {
	At         time.Time         `json:"at"`
	Lanes      []lanes.Version   `json:"lanes"`
	Directions []lanes.Direction `json:"directions"`
	Snapshots  []string          `json:"snapshot_ids"`
}

func (ws *Server) handleGetSiteLanes(w http.ResponseWriter, r *http.Request, l16 string) {
	q := r.URL.Query()
	store := sqlite.NewLaneSnapshotStore(ws.db)
	sensor := q.Get("sensor_id")
	switch {
	case q.Get("history") == "true":
		h, err := store.History(l16, sensor)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if h.Empty() {
			ws.writeJSONError(w, http.StatusNotFound, "no lane snapshot for this site")
			return
		}
		ws.writeJSON(w, http.StatusOK, h)
	case q.Has("at"):
		at, err := parseLaneTime(q.Get("at"))
		if err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		h, err := store.History(l16, sensor)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if h.Empty() {
			ws.writeJSONError(w, http.StatusNotFound, "no lane snapshot for this site")
			return
		}
		out := SiteLanesAt{At: at, Lanes: h.At(at), Directions: h.DirectionsAt(at), Snapshots: h.Snapshots}
		if out.Lanes == nil {
			out.Lanes = []lanes.Version{}
		}
		ws.writeJSON(w, http.StatusOK, out)
	default:
		latest, err := store.Latest(l16, sensor)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if latest == nil {
			ws.writeJSONError(w, http.StatusNotFound, "no lane snapshot for this site")
			return
		}
		ws.writeJSON(w, http.StatusOK, latest)
	}
}

func (ws *Server) handleAttributeSiteLanes(w http.ResponseWriter, r *http.Request, l16 string) {
	q := r.URL.Query()
	x, errX := strconv.ParseFloat(q.Get("x"), 64)
	y, errY := strconv.ParseFloat(q.Get("y"), 64)
	headingDeg, errH := strconv.ParseFloat(q.Get("heading_deg"), 64)
	if errX != nil || errY != nil || errH != nil || math.IsNaN(x+y+headingDeg) || math.IsInf(x+y+headingDeg, 0) {
		ws.writeJSONError(w, http.StatusBadRequest, "x, y (sensor frame, metres) and heading_deg (counter-clockwise from the sensor's +X) are required numbers")
		return
	}
	at := time.Now().UTC()
	if q.Has("at") {
		var err error
		if at, err = parseLaneTime(q.Get("at")); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	h, err := sqlite.NewLaneSnapshotStore(ws.db).History(l16, q.Get("sensor_id"))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, h.Attribute(lanegeom.Point{X: x, Y: y}, headingDeg*math.Pi/180, at, q.Get("previous")))
}

// parseLaneTime reads RFC 3339 or a YYYY-MM-DD day (UTC midnight).
func parseLaneTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("at %q must be RFC 3339 or YYYY-MM-DD", s)
}
