package server

import (
	"bytes"
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
	// Read one bounded JSON object, then strictly decode its declared version.
	// A v2 matches list cannot be ignored by a legacy single-facet decoder.
	var raw json.RawMessage
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPhysicalRequestBytes))
	if err := decoder.Decode(&raw); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		ws.writeJSONError(w, http.StatusBadRequest, "Expected one JSON request")
		return
	}
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req annotation.FacetPoseRequest
	var joint annotation.FacetJointPoseRequest
	var pack, digest string
	switch envelope.SchemaVersion {
	case 1:
		if err := decodeFacetPoseVersion(raw, &req); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		pack, digest = req.Pack, req.PackDigest
	case 2:
		if err := decodeFacetPoseVersion(raw, &joint); err != nil {
			ws.writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		pack, digest = joint.Pack, joint.PackDigest
	default:
		ws.writeJSONError(w, http.StatusBadRequest, "Unsupported facet pose request version")
		return
	}
	p, _, ok := ws.resolvePhysicalPack(w, pack, digest)
	if !ok {
		return
	}
	var result any
	var err error
	if envelope.SchemaVersion == 1 {
		result, err = annotation.ProposeFacetPose(p, req)
	} else {
		result, err = annotation.ProposeJointFacetPose(p, joint)
	}

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

func decodeFacetPoseVersion(raw json.RawMessage, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(into)
}
