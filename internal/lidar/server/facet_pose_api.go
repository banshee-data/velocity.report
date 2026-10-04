package server

import (
	"encoding/json"
	"errors"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"io"
	"net/http"
)

// Offline, read-only proposal. It cannot save a physical reference or track.
func (ws *Server) handleFacetPoseProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Use POST")
		return
	}
	var req annotation.FacetPoseRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPhysicalRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		ws.writeJSONError(w, http.StatusBadRequest, "Expected one JSON request")
		return
	}
	p, _, ok := ws.resolvePhysicalPack(w, req.Pack, req.PackDigest)
	if !ok {
		return
	}
	result, err := annotation.ProposeFacetPose(p, req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, annotation.ErrSidecarConflict) || errors.Is(err, annotation.ErrMembershipChanged) {
			status = http.StatusConflict
		}
		ws.writeJSONError(w, status, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, result)
}
