package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

// handleFeatureAnnotations uses the shared annotation protobuf in both
// directions. The pack handle has the same confined root as physical edits.
func (ws *Server) handleFeatureAnnotations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "Use GET or POST")
		return
	}
	p, _, ok := ws.resolvePhysicalPack(w, r.URL.Query().Get("pack"), r.URL.Query().Get("pack_digest"))
	if !ok {
		return
	}
	var state *pb.FeatureState
	var err error
	var revision uint64
	if values, present := r.URL.Query()["revision"]; present {
		if r.Method != http.MethodGet || len(values) != 1 {
			ws.writeJSONError(w, http.StatusBadRequest, "Revision is a single read-only parameter")
			return
		}
		revision, err = strconv.ParseUint(values[0], 10, 31)
		if err != nil || revision == 0 {
			ws.writeJSONError(w, http.StatusBadRequest, "Invalid feature revision")
			return
		}
	}
	if r.Method == http.MethodGet {
		if revision != 0 {
			state, err = annotation.LoadFeatureRevision(p, revision)
		} else {
			state, err = annotation.LoadFeatures(p)
		}
	} else {
		var b []byte
		b, err = io.ReadAll(http.MaxBytesReader(w, r.Body, maxPhysicalRequestBytes))
		if err == nil {
			edit := new(pb.FeatureEdit)
			err = proto.Unmarshal(b, edit)
			if err == nil && len(edit.ProtoReflect().GetUnknown()) != 0 {
				ws.writeJSONError(w, http.StatusBadRequest, "Unsupported feature edit fields")
				return
			}
			if err == nil {
				state, err = annotation.SaveFeatures(p, edit)
			}
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, annotation.ErrSidecarConflict) || errors.Is(err, annotation.ErrMembershipChanged) || errors.Is(err, annotation.ErrSidecarBusy) {
			status = http.StatusConflict
		}
		ws.writeJSONError(w, status, err.Error())
		return
	}
	ws.writeFeatureState(w, state)
}

func (ws *Server) writeFeatureState(w http.ResponseWriter, state *pb.FeatureState) {
	b, err := proto.Marshal(state)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Write(b)
}
