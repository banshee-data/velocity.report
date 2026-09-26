package api

// GET /api/scenes/<scene-id>/headway serves a scene's following encounters:
// the headway distribution of one analysis version and a summary row per
// encounter, per Section 10.4 of docs/plans/lidar-behaviour-analytics-plan.md.
// Every name in the response is a behaviour registry id, a suppression token
// or a structural name that l8behaviour.AuditSurfaceJSON accepts.
//
// Scene to source. A scene records its capture window, not the analysis
// source its encounters were stored under (the source id is a digest the
// scene cannot reproduce), so the source is derived: the sources with
// following encounters fully inside the scene's capture window. One source is
// used as found; several (one capture extracted under two tunings, say) are
// listed and never merged, and the caller names one with source_id.
//
// Version. The default is the most recently written version at stage final.
// stage selects another stage, and estimator_id, obs_model_id, method_id and
// param_hash together select one exact version. When none matches, the
// response says so and lists the versions that exist, rather than falling
// back to a less final stage.
//
// Status. Every response is provisional: nothing is promoted until the field
// promotion gates pass, and output from the analytic fixture estimator is a
// synthetic oracle.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Headway availability: why a response does or does not carry a
// distribution. Each is a structural token, not a finding about traffic.
const (
	headwayAvailable         = "available"
	headwayNoCaptureWindow   = "no_capture_window"
	headwayNoEncounters      = "no_encounters"
	headwaySourceAmbiguous   = "source_ambiguous"
	headwayNoMatchingVersion = "no_matching_version"
)

// sceneHeadwayResponse is the body of GET /api/scenes/<id>/headway.
type sceneHeadwayResponse struct {
	SceneID      string                    `json:"scene_id"`
	Status       l8behaviour.SurfaceStatus `json:"status"`
	Availability string                    `json:"availability"`
	// StartUnixNanos and EndUnixNanos are the scene's capture window; served
	// encounter rows stay wholly inside it.
	StartUnixNanos *int64 `json:"start_unix_nanos,omitempty"`
	EndUnixNanos   *int64 `json:"end_unix_nanos,omitempty"`
	// SourceID is the source the distribution is read from; Sources lists
	// every source overlapping the window.
	SourceID string          `json:"source_id,omitempty"`
	Sources  []headwaySource `json:"sources"`
	// Version is the version served; Versions lists every version of the
	// source in the window, newest first.
	Version      *l8behaviour.InteractionVersion    `json:"version,omitempty"`
	Versions     []headwayVersion                   `json:"versions"`
	Distribution *l8behaviour.FollowingDistribution `json:"distribution,omitempty"`
	Encounters   []l8behaviour.EncounterSummary     `json:"encounters"`
}

type headwaySource struct {
	SourceID       string `json:"source_id"`
	Events         int    `json:"events"`
	FirstUnixNanos int64  `json:"first_unix_nanos"`
	LastUnixNanos  int64  `json:"last_unix_nanos"`
}

type headwayVersion struct {
	Version l8behaviour.InteractionVersion `json:"version"`
	Events  int                            `json:"events"`
	Status  l8behaviour.SurfaceStatus      `json:"status"`
}

// headwaySelection is what the caller asked for.
type headwaySelection struct {
	SourceID string
	Stage    l8behaviour.EstimateStage
	// Exact is set when all four version axes were given.
	Exact *l8behaviour.InteractionVersion
}

// parseHeadwaySelection reads source_id, stage and the four version axes. It
// returns a message the caller can act on for anything malformed.
func parseHeadwaySelection(q url.Values) (headwaySelection, string) {
	sel := headwaySelection{SourceID: q.Get("source_id"), Stage: l8behaviour.StageFinal}
	if raw := q.Get("stage"); raw != "" {
		stage, err := l8behaviour.ParseEstimateStage(raw)
		if err != nil {
			return headwaySelection{}, "Invalid 'stage'; must be one of online, fixed_lag, final"
		}
		sel.Stage = stage
	}
	v := l8behaviour.InteractionVersion{
		EstimateStage: sel.Stage, EstimatorID: q.Get("estimator_id"), ObsModelID: q.Get("obs_model_id"),
		MethodID: q.Get("method_id"), ParamHash: q.Get("param_hash"),
	}
	given := 0
	for _, axis := range []string{v.EstimatorID, v.ObsModelID, v.MethodID, v.ParamHash} {
		if axis != "" {
			given++
		}
	}
	switch given {
	case 0:
	case 4:
		sel.Exact = &v
	default:
		return headwaySelection{}, "Give all of 'estimator_id', 'obs_model_id', 'method_id' and 'param_hash' to select a version, or none"
	}
	return sel, ""
}

// errHeadwaySelection marks a selection the scene cannot satisfy: a client
// error, reported with its message.
var errHeadwaySelection = errors.New("headway selection")

// resolveSceneHeadway finds a scene's source and version and reads its
// distribution. An error wrapping errHeadwaySelection is the caller's to
// fix; any other error is the server's.
func (s *Server) resolveSceneHeadway(scene *db.Scene, sel headwaySelection) (sceneHeadwayResponse, error) {
	resp := sceneHeadwayResponse{
		SceneID: scene.SceneID, Status: l8behaviour.StatusProvisional,
		Sources: []headwaySource{}, Versions: []headwayVersion{}, Encounters: []l8behaviour.EncounterSummary{},
	}
	if scene.CapturedStartNs == nil || scene.CapturedEndNs == nil || *scene.CapturedStartNs <= 0 {
		resp.Availability = headwayNoCaptureWindow
		return resp, nil
	}
	start, end := *scene.CapturedStartNs, *scene.CapturedEndNs
	resp.StartUnixNanos, resp.EndUnixNanos = &start, &end

	store := sqlite.NewInteractionStore(s.db)
	sources, err := store.SourcesOverlapping(start, end)
	if err != nil {
		return resp, err
	}
	for _, src := range sources {
		resp.Sources = append(resp.Sources, headwaySource{
			SourceID: src.SourceID, Events: src.Events, FirstUnixNanos: src.FirstUnixNanos, LastUnixNanos: src.LastUnixNanos,
		})
	}
	switch {
	case sel.SourceID != "":
		found := false
		for _, src := range sources {
			found = found || src.SourceID == sel.SourceID
		}
		if !found {
			return resp, fmt.Errorf("%w: source %q has no following encounters in this scene's capture window",
				errHeadwaySelection, sel.SourceID)
		}
		resp.SourceID = sel.SourceID
	case len(sources) == 0:
		resp.Availability = headwayNoEncounters
		return resp, nil
	case len(sources) == 1:
		resp.SourceID = sources[0].SourceID
	default:
		resp.Availability = headwaySourceAmbiguous
		return resp, nil
	}

	versions, err := store.VersionsOverlapping(resp.SourceID, start, end)
	if err != nil {
		return resp, err
	}
	var chosen *l8behaviour.InteractionVersion
	for _, v := range versions {
		resp.Versions = append(resp.Versions, headwayVersion{
			Version: v.Version, Events: v.Events, Status: l8behaviour.StatusOf(v.Version),
		})
		matches := v.Version.EstimateStage == sel.Stage
		if sel.Exact != nil {
			matches = v.Version == *sel.Exact
		}
		if matches && chosen == nil {
			chosen = &v.Version
		}
	}
	if chosen == nil {
		resp.Availability = headwayNoMatchingVersion
		return resp, nil
	}
	resp.Version, resp.Status = chosen, l8behaviour.StatusOf(*chosen)

	interactions, err := store.ListInteractionsOverlapping(resp.SourceID, *chosen, start, end)
	if err != nil {
		return resp, err
	}
	d, err := l8behaviour.AggregateFollowing(interactions)
	if errors.Is(err, l8behaviour.ErrNothingToAggregate) {
		// The version was removed between the two reads.
		resp.Availability = headwayNoMatchingVersion
		return resp, nil
	}
	if err != nil {
		return resp, err
	}
	resp.Availability, resp.Distribution = headwayAvailable, &d
	for _, fi := range interactions {
		resp.Encounters = append(resp.Encounters, fi.Event.Summary())
	}
	return resp, nil
}

// loadSceneHeadway reads the scene and resolves its headway, writing any
// error response itself; ok is false when it did.
func (s *Server) loadSceneHeadway(w http.ResponseWriter, r *http.Request, sceneID string, q url.Values) (sceneHeadwayResponse, bool) {
	sel, problem := parseHeadwaySelection(q)
	if problem != "" {
		s.writeJSONError(w, http.StatusBadRequest, problem)
		return sceneHeadwayResponse{}, false
	}
	scene, err := s.db.GetScene(r.Context(), sceneID)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve scene: %v", err))
		return sceneHeadwayResponse{}, false
	}
	if scene == nil {
		s.writeJSONError(w, http.StatusNotFound, "Scene not found")
		return sceneHeadwayResponse{}, false
	}
	resp, err := s.resolveSceneHeadway(scene, sel)
	if errors.Is(err, errHeadwaySelection) {
		s.writeJSONError(w, http.StatusBadRequest, err.Error())
		return sceneHeadwayResponse{}, false
	}
	if err != nil {
		log.Printf("Scene %s headway read error: %v", sceneID, err)
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read headway: %v", err))
		return sceneHeadwayResponse{}, false
	}
	return resp, true
}

func (s *Server) getSceneHeadway(w http.ResponseWriter, r *http.Request, sceneID string) {
	resp, ok := s.loadSceneHeadway(w, r, sceneID, r.URL.Query())
	if !ok {
		return
	}
	// Encode before writing: a vocabulary token that refuses to serialise
	// must become an error response, not half a body.
	body, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Scene %s headway encode error: %v", sceneID, err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode headway")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(append(body, '\n')); err != nil {
		log.Printf("Failed to write headway response: %v", err)
	}
}
