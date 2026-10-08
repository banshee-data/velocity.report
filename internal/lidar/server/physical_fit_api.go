package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// physicalFitRequest asks for an object's physical reference fitted to its
// reviewed returns (docs/plans/lidar-physical-fit-to-points-plan.md). The
// membership pin is the sidecar the client is looking at, by the digest of its
// bytes and optionally its revision: a fit against other membership would fill
// the draft from returns the operator has not seen.
type physicalFitRequest struct {
	Pack               string `json:"pack"`
	PackDigest         string `json:"pack_digest"`
	MembershipRevision int    `json:"membership_revision"`
	MembershipDigest   string `json:"membership_digest"`
	ObjectID           string `json:"object_id"`
	Samples            []int  `json:"samples"`
	Author             string `json:"author"`
	Session            string `json:"session"`
}

// handlePhysicalFit fits and returns proposals. It stores nothing: the client
// places them in its draft, and they are saved and reviewed like anything
// typed by hand.
func (ws *Server) handlePhysicalFit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writePhysicalError(w, http.StatusMethodNotAllowed, physicalCodeBadRequest, "this endpoint only accepts POST requests")
		return
	}
	var req physicalFitRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	switch {
	case strings.TrimSpace(req.Author) == "":
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "author is required")
		return
	case req.ObjectID == "":
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "object_id is required")
		return
	case req.MembershipDigest == "":
		ws.writePhysicalError(w, http.StatusBadRequest, physicalCodeBadRequest, "membership_digest is required")
		return
	}
	pack, _, ok := ws.resolvePhysicalPack(w, req.Pack, req.PackDigest)
	if !ok {
		return
	}
	sidecar, err := annotation.LoadSidecar(pack)
	if err != nil {
		ws.writePhysicalFailure(w, err)
		return
	}
	if sidecar.Digest() != req.MembershipDigest || (req.MembershipRevision != 0 && sidecar.Revision != req.MembershipRevision) {
		ws.writePhysicalFailure(w, annotation.ErrMembershipChanged)
		return
	}
	res, err := annotation.FitPhysicalObject(pack, sidecar, annotation.FitRequest{
		ObjectID: req.ObjectID, Samples: req.Samples, Author: req.Author, Session: req.Session,
	}, annotation.DefaultFitOptions())
	if err != nil {
		if errors.Is(err, annotation.ErrMembershipChanged) {
			ws.writePhysicalFailure(w, err)
			return
		}
		ws.writePhysicalError(w, http.StatusUnprocessableEntity, physicalCodeInvalid, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, res)
}
