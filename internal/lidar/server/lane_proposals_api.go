package server

// Lane proposals across the corpus (docs/plans/lidar-lane-field-batch-plan.md).
//
// Routes:
//   - GET  /api/lidar/clips?lanes=1 — every clip with its location, its
//     registration in Gerardus and its lane proposals (newest completed run,
//     newest job, and whether the newest is current)
//   - POST /api/lidar/clips/{clip_id}/lane-proposals — queue one job for a clip
//   - POST /api/lidar/clips/lane-proposals/rerun?force=1&dry_run=1 — queue one
//     job for every registered clip whose lanes are stale or absent
//   - POST /api/lidar/runs/{run_id}/lane-proposals?clip_id=…|dataset_id=… —
//     the same from a finished run's recording, without replaying
//
// The job replays the clip headlessly, builds its three lane fields, uploads
// them to Gerardus against the clip's accepted registration record, and asks
// Gerardus for the proposals once, so the derivation runs while nobody waits.
// Gerardus derives and grades; nothing here fits a lane (D-39). Only the
// fields' ids and the grade counts are kept (ADR-0011).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/gerardus"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/lanefieldrun"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/version"
)

// laneGerardus is what the job needs from Gerardus; tests substitute it.
type laneGerardus interface {
	ServerInfo(ctx context.Context) (gerardus.ServerInfo, error)
	Records(ctx context.Context, l16 string) ([]gerardus.Record, error)
	UploadLaneField(ctx context.Context, f l8analytics.LaneField) (gerardus.Upload, error)
	Proposals(ctx context.Context, l16 string) (gerardus.ProposalCounts, error)
}

// registrationTTL is how long a site's records are trusted before Gerardus is
// asked again. An operator accepting a pose in the visualiser shows up here
// within this long, or at once after a lane job at the site.
const registrationTTL = 3 * time.Minute

// gerardusCallTimeout bounds one call from a request handler; a slow or absent
// Gerardus makes a column read unknown rather than holding the page.
const gerardusCallTimeout = 5 * time.Second

type siteRecords struct {
	records []gerardus.Record
	err     error
	at      time.Time
}

// laneState is the server's Gerardus connection and its registration cache.
type laneState struct {
	mu      sync.Mutex
	address string
	client  laneGerardus
	dial    func(address string) (laneGerardus, error)
	sites   map[string]siteRecords
	now     func() time.Time
}

func (ws *Server) lanes() *laneState {
	ws.laneOnce.Do(func() {
		if ws.laneSt == nil {
			ws.laneSt = &laneState{}
		}
		st := ws.laneSt
		st.address = gerardus.Address(ws.gerardusAddress)
		if st.dial == nil {
			st.dial = func(address string) (laneGerardus, error) { return gerardus.Dial(address) }
		}
		if st.now == nil {
			st.now = time.Now
		}
		st.sites = map[string]siteRecords{}
	})
	return ws.laneSt
}

func (st *laneState) gerardus() (laneGerardus, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.client != nil {
		return st.client, nil
	}
	c, err := st.dial(st.address)
	if err != nil {
		return nil, err
	}
	st.client = c
	return c, nil
}

// records returns a site's registration records, from the cache when fresh.
func (st *laneState) records(ctx context.Context, l16 string, fresh bool) ([]gerardus.Record, error) {
	st.mu.Lock()
	cached, ok := st.sites[l16]
	st.mu.Unlock()
	if ok && !fresh && st.now().Sub(cached.at) < registrationTTL {
		return cached.records, cached.err
	}
	g, err := st.gerardus()
	if err != nil {
		return nil, err
	}
	recs, err := g.Records(ctx, l16)
	st.mu.Lock()
	st.sites[l16] = siteRecords{records: recs, err: err, at: st.now()}
	st.mu.Unlock()
	return recs, err
}

func (st *laneState) forget(l16 string) {
	st.mu.Lock()
	delete(st.sites, l16)
	st.mu.Unlock()
}

// clipRefs names a clip the way a registration record's capture_ref might:
// its captures' basenames and its dataset id.
func clipRefs(clip *sqlite.Clip, paths []string) []string {
	refs := make([]string, 0, len(paths)+1)
	for _, p := range paths {
		refs = append(refs, filepath.Base(p))
	}
	if clip.DatasetID != "" {
		refs = append(refs, clip.DatasetID)
	}
	return refs
}

// clipRegistration answers whether a clip is registered, and by which record.
func (ws *Server) clipRegistration(ctx context.Context, clip *sqlite.Clip, loc *sqlite.ClipLocation, paths []string, fresh bool) gerardus.Registration {
	st := ws.lanes()
	now := st.now()
	if loc == nil || loc.L16Token == "" {
		return gerardus.Registration{State: gerardus.ClipNotLocated, Reason: "the clip has no location", CheckedAt: now}
	}
	recs, err := st.records(ctx, loc.L16Token, fresh)
	if err != nil {
		return gerardus.Registration{State: gerardus.ClipUnknown, Reason: fmt.Sprintf("Gerardus at %s did not answer: %v", st.address, err), CheckedAt: now}
	}
	reg := gerardus.AcceptedRecord(recs, clip.SensorID, clipRefs(clip, paths))
	reg.CheckedAt = now
	return reg
}

// pipelineBuild is this binary's build as the lane field header spells it.
func pipelineBuild() string {
	if version.GitSHA != "" && version.GitSHA != "unknown" {
		return version.Version + "+" + version.GitSHA
	}
	return version.Version
}

// LaneStatus is a clip's lane proposals for the clips view.
type LaneStatus struct {
	Latest *sqlite.LaneProposalRun `json:"latest,omitempty"`
	Job    *sqlite.LaneProposalRun `json:"job,omitempty"`
	// Current says the newest completed run was built by this binary under
	// the server's current tuning; StaleReason says why not when it was not.
	Current     bool   `json:"current"`
	StaleReason string `json:"stale_reason,omitempty"`
}

func laneStatus(cl sqlite.ClipLanes, build, fingerprint string) LaneStatus {
	out := LaneStatus{Latest: cl.Latest, Job: cl.Job}
	switch {
	case cl.Latest == nil:
		out.StaleReason = "no lane proposals yet"
	case cl.Latest.PipelineBuild != build:
		out.StaleReason = fmt.Sprintf("built by %s, not %s", orUnknown(cl.Latest.PipelineBuild), build)
	case cl.Latest.TuningFingerprint != fingerprint:
		out.StaleReason = fmt.Sprintf("tuning %s, not %s", orUnknown(cl.Latest.TuningFingerprint), fingerprint)
	default:
		out.Current = true
	}
	return out
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// ClipWithLanes is a clip as the clips view lists it.
type ClipWithLanes struct {
	*sqlite.Clip
	Registration gerardus.Registration `json:"registration"`
	Lanes        LaneStatus            `json:"lanes"`
}

// listClipsWithLanes answers GET /api/lidar/clips?lanes=1.
func (ws *Server) listClipsWithLanes(w http.ResponseWriter, r *http.Request, clips []*sqlite.Clip) {
	store := sqlite.NewClipStore(ws.db)
	latest, err := sqlite.NewLaneProposalStore(ws.db).LatestByClip()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read lane proposals: %v", err))
		return
	}
	_, fingerprint := ws.activeTuning()
	build := pipelineBuild()
	ctx, cancel := context.WithTimeout(r.Context(), gerardusCallTimeout)
	defer cancel()
	out := make([]ClipWithLanes, 0, len(clips))
	for _, clip := range clips {
		loc, err := store.ClipLocationOf(clip.ClipID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read a clip location: %v", err))
			return
		}
		clip.Location = loc
		paths, err := store.ClipPaths(clip.ClipID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read a clip's captures: %v", err))
			return
		}
		out = append(out, ClipWithLanes{
			Clip: clip, Registration: ws.clipRegistration(ctx, clip, loc, paths, false),
			Lanes: laneStatus(latest[clip.ClipID], build, fingerprint),
		})
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{
		"clips": out, "count": len(out), "pipeline_build": build, "tuning_fingerprint": fingerprint,
		"gerardus": ws.lanes().address,
	})
}

// laneJobQueuedDetail is what a queued lane job shows until it reports.
const laneJobQueuedDetail = "queued: replay, lane fields, upload, derive"

// handleClipLaneProposals answers POST /api/lidar/clips/{clip_id}/lane-proposals.
func (ws *Server) handleClipLaneProposals(w http.ResponseWriter, r *http.Request, clipID string) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	store := sqlite.NewClipStore(ws.db)
	clip, err := store.GetClip(clipID)
	if errors.Is(err, sqlite.ErrNotFound) || (err == nil && clip == nil) {
		ws.writeJSONError(w, http.StatusNotFound, "clip not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	reg, err := ws.registrationOf(r.Context(), clip)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !reg.Placed() {
		status := http.StatusConflict
		if reg.State == gerardus.ClipUnknown {
			status = http.StatusServiceUnavailable
		}
		ws.writeJSON(w, status, map[string]any{"error": "the clip is not registered: " + reg.Reason, "registration": reg})
		return
	}
	job, created, err := sqlite.NewLaneProposalStore(ws.db).EnqueueLaneProposals(clipID, laneJobQueuedDetail)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusConflict
	}
	ws.writeJSON(w, status, map[string]any{"job": job, "created": created, "registration": reg})
}

func (ws *Server) registrationOf(ctx context.Context, clip *sqlite.Clip) (gerardus.Registration, error) {
	store := sqlite.NewClipStore(ws.db)
	loc, err := store.ClipLocationOf(clip.ClipID)
	if err != nil {
		return gerardus.Registration{}, err
	}
	paths, err := store.ClipPaths(clip.ClipID)
	if err != nil {
		return gerardus.Registration{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, gerardusCallTimeout)
	defer cancel()
	return ws.clipRegistration(cctx, clip, loc, paths, false), nil
}

// RerunClip is one clip's place in a re-run: queued, or skipped and why.
type RerunClip struct {
	ClipID      string `json:"clip_id"`
	Description string `json:"description,omitempty"`
	JobID       string `json:"job_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// RerunResponse is what POST …/clips/lane-proposals/rerun answers.
type RerunResponse struct {
	DryRun  bool        `json:"dry_run"`
	Force   bool        `json:"force"`
	Queued  []RerunClip `json:"queued"`
	Current []RerunClip `json:"current"`
	Active  []RerunClip `json:"active"`
	Skipped []RerunClip `json:"skipped"`
}

// handleRerunLaneProposals queues a job for every registered clip whose lanes
// are stale or absent, stale first; force queues current ones too. dry_run
// says what it would do and queues nothing.
func (ws *Server) handleRerunLaneProposals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	force, dry := truthy(q.Get("force")), truthy(q.Get("dry_run"))
	store := sqlite.NewClipStore(ws.db)
	clips, err := store.ListClips(q.Get("sensor_id"))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	lps := sqlite.NewLaneProposalStore(ws.db)
	latest, err := lps.LatestByClip()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, fingerprint := ws.activeTuning()
	build := pipelineBuild()
	out := RerunResponse{DryRun: dry, Force: force, Queued: []RerunClip{}, Current: []RerunClip{}, Active: []RerunClip{}, Skipped: []RerunClip{}}
	type candidate struct {
		clip  *sqlite.Clip
		order int64
	}
	var run []candidate
	ctx, cancel := context.WithTimeout(r.Context(), 4*gerardusCallTimeout)
	defer cancel()
	for _, clip := range clips {
		entry := RerunClip{ClipID: clip.ClipID, Description: clip.Description}
		loc, err := store.ClipLocationOf(clip.ClipID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		paths, err := store.ClipPaths(clip.ClipID)
		if err != nil {
			ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		reg := ws.clipRegistration(ctx, clip, loc, paths, false)
		if !reg.Placed() {
			entry.Reason = reg.State + ": " + reg.Reason
			out.Skipped = append(out.Skipped, entry)
			continue
		}
		cl := latest[clip.ClipID]
		if cl.Job != nil && cl.Job.Active() {
			entry.JobID, entry.Reason = cl.Job.JobID, "a job is already "+cl.Job.State
			out.Active = append(out.Active, entry)
			continue
		}
		status := laneStatus(cl, build, fingerprint)
		if status.Current && !force {
			entry.Reason = "current"
			out.Current = append(out.Current, entry)
			continue
		}
		// Never run first, then oldest first.
		order := int64(0)
		if cl.Latest != nil && cl.Latest.CompletedAtNs != nil {
			order = *cl.Latest.CompletedAtNs
		}
		run = append(run, candidate{clip, order})
	}
	sort.SliceStable(run, func(i, j int) bool { return run[i].order < run[j].order })
	for _, c := range run {
		entry := RerunClip{ClipID: c.clip.ClipID, Description: c.clip.Description}
		if !dry {
			job, _, err := lps.EnqueueLaneProposals(c.clip.ClipID, laneJobQueuedDetail)
			if err != nil {
				ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			entry.JobID = job.JobID
		}
		out.Queued = append(out.Queued, entry)
	}
	status := http.StatusAccepted
	if dry {
		status = http.StatusOK
	}
	ws.writeJSON(w, status, out)
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// laneJobOps are the parts of the job that touch captures and Gerardus.
type laneJobOps struct {
	// replay replays a clip's captures headlessly and returns its recording,
	// a cleanup that removes it
	// and the fingerprint of the tuning it ran.
	replay func(ctx context.Context, clip *sqlite.Clip, paths []string, report func(string)) (vrlog, fingerprint string, cleanup func(), err error)
	read   func(vrlog string) (lanefieldrun.Recording, error)
}

// laneJobSteps is how many steps a lane job reports: replay, field, three
// uploads, derive.
const laneJobSteps = 6

// runLaneProposalsWith runs one lane_proposals job.
func (ws *Server) runLaneProposalsWith(ctx context.Context, job capjobs.Job, report func(capjobs.Progress), ops laneJobOps) error {
	lps := sqlite.NewLaneProposalStore(ws.db)
	row, err := lps.ForJob(job.JobID)
	if err != nil {
		return fmt.Errorf("lane job has no clip: %w", err)
	}
	store := sqlite.NewClipStore(ws.db)
	clip, err := store.GetClip(row.ClipID)
	if err != nil || clip == nil {
		return fmt.Errorf("clip %s: %v", row.ClipID, err)
	}
	loc, err := store.ClipLocationOf(clip.ClipID)
	if err != nil {
		return err
	}
	paths, err := store.ClipPaths(clip.ClipID)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("clip has no captures")
	}
	step := func(n int64, detail string) {
		report(capjobs.Progress{Current: n, Total: laneJobSteps, Detail: detail})
	}
	step(0, "checking the registration in Gerardus")
	st := ws.lanes()
	g, err := st.gerardus()
	if err != nil {
		return err
	}
	info, err := g.ServerInfo(ctx)
	if err != nil {
		return err
	}
	if !info.SupportsLaneFields {
		return fmt.Errorf("Gerardus %s at %s takes no lane fields", info.Version, st.address)
	}
	reg := ws.clipRegistration(ctx, clip, loc, paths, true)
	if !reg.Placed() {
		return fmt.Errorf("the clip is not registered (%s): %s", reg.State, reg.Reason)
	}

	resolved := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := ws.resolvePCAPPath(p)
		if err != nil {
			return err
		}
		resolved = append(resolved, abs)
	}
	step(0, "replaying the clip")
	vrlog, fingerprint, cleanup, err := ops.replay(ctx, clip, resolved, func(d string) { step(0, d) })
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	step(1, "building the lane fields")
	recording, err := ops.read(vrlog)
	if err != nil {
		return fmt.Errorf("read the recording: %w", err)
	}
	meta := l8analytics.LaneFieldMeta{
		L16Token: loc.L16Token, SensorID: clip.SensorID, RecordID: reg.RecordID,
		CaptureRef: laneCaptureRef(clip, resolved), WindowStartNs: recording.WindowStartNs,
		WindowEndNs: recording.WindowEndNs, PipelineBuild: pipelineBuild(), TuningFingerprint: fingerprint,
	}
	done := &sqlite.LaneProposalRun{
		ProposalRunID: row.ProposalRunID, ClipID: clip.ClipID, JobID: job.JobID, L16Token: loc.L16Token,
		SensorID: clip.SensorID, RecordID: reg.RecordID, GerardusVersion: info.Version,
		WindowStartNs: recording.WindowStartNs, WindowEndNs: recording.WindowEndNs,
		PipelineBuild: meta.PipelineBuild, TuningFingerprint: fingerprint,
	}
	if err := ws.uploadAndDerive(ctx, g, recording.Tracks, meta, done, func(n int64, d string) { step(1+n, d) }); err != nil {
		return err
	}
	if _, err := lps.Complete(*done); err != nil {
		return err
	}
	st.forget(loc.L16Token)
	step(laneJobSteps, fmt.Sprintf("%d high, %d medium, %d to review", done.High, done.Medium, done.Outstanding))
	return nil
}

// uploadAndDerive builds the three parts, uploads each, asks for the
// proposals and fills done with what came back. step reports 1..5.
func (ws *Server) uploadAndDerive(ctx context.Context, g laneGerardus, tracks []l8analytics.LaneFieldTrack, meta l8analytics.LaneFieldMeta, done *sqlite.LaneProposalRun, step func(int64, string)) error {
	fields, err := l8analytics.BuildLaneFields(tracks, meta, l8analytics.DefaultLaneFieldOptions())
	if err != nil {
		return fmt.Errorf("build the lane fields: %w", err)
	}
	if len(fields) == 0 {
		return fmt.Errorf("no lane field was built")
	}
	done.TrackCount = int64(fields[0].Header.TrackCount)
	for i, f := range fields {
		step(int64(i), fmt.Sprintf("uploading %s (%d cells)", f.Header.WindowPart, f.CellCount()))
		up, err := g.UploadLaneField(ctx, f)
		if err != nil {
			return err
		}
		done.FieldIDs = append(done.FieldIDs, up.FieldID)
	}
	step(int64(len(fields)), "deriving proposals in Gerardus")
	counts, err := g.Proposals(ctx, meta.L16Token)
	if err != nil {
		return err
	}
	done.High, done.Medium, done.Adopted, done.Rejected, done.Outstanding = counts.High, counts.Medium, counts.Adopted, counts.Rejected, counts.Outstanding
	done.DeriveSecs = counts.DeriveSecs
	return nil
}

// laneCaptureRef names a clip's capture in a field's header: its clip id and
// its captures' basenames, so Gerardus keeps one field per clip and part and a
// re-run replaces rather than adds.
func laneCaptureRef(clip *sqlite.Clip, paths []string) string {
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	return "clip:" + clip.ClipID + ":" + strings.Join(names, "+")
}

// handleRunLaneProposals answers POST /api/lidar/runs/{run_id}/lane-proposals:
// the run's recorded fields uploaded and derived, against the clip named by
// clip_id or dataset_id. Synchronous: the scene publisher waits on it.
func (ws *Server) handleRunLaneProposals(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	store := sqlite.NewClipStore(ws.db)
	clip, err := ws.clipByIDOrDataset(store, q.Get("clip_id"), q.Get("dataset_id"))
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	runs := sqlite.NewRunStore(ws.db)
	run, err := runs.GetRun(runID)
	if errors.Is(err, sqlite.ErrNotFound) || (err == nil && run == nil) {
		ws.writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	reg, err := ws.registrationOf(r.Context(), clip)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !reg.Placed() {
		status := http.StatusConflict
		if reg.State == gerardus.ClipUnknown {
			status = http.StatusServiceUnavailable
		}
		ws.writeJSON(w, status, map[string]any{"error": "the clip is not registered: " + reg.Reason, "registration": reg})
		return
	}
	tracks, _, err := lanefieldrun.Tracks(runs, run, ws.resolveRunVRLOG)
	if err != nil {
		ws.writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf("the run's tracks: %v", err))
		return
	}
	st := ws.lanes()
	g, err := st.gerardus()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	info, err := g.ServerInfo(r.Context())
	if err != nil || !info.SupportsLaneFields {
		ws.writeJSONError(w, http.StatusServiceUnavailable, fmt.Sprintf("Gerardus at %s takes no lane fields (%v)", st.address, err))
		return
	}
	loc, _ := store.ClipLocationOf(clip.ClipID)
	meta := sqlite.LaneFieldMeta(run)
	meta.L16Token, meta.RecordID = loc.L16Token, reg.RecordID
	// The field's header keeps the run's own params hash, as the painter's
	// upload does; the row records the server's active tuning fingerprint,
	// which the scene publisher has just replayed the run under, so the stale
	// check compares like with like.
	_, active := ws.activeTuning()
	done := &sqlite.LaneProposalRun{
		ClipID: clip.ClipID, RunID: run.RunID, L16Token: loc.L16Token, SensorID: run.SensorID, RecordID: reg.RecordID,
		GerardusVersion: info.Version, WindowStartNs: meta.WindowStartNs, WindowEndNs: meta.WindowEndNs,
		PipelineBuild: meta.PipelineBuild, TuningFingerprint: active,
	}
	if err := ws.uploadAndDerive(r.Context(), g, tracks, meta, done, func(int64, string) {}); err != nil {
		ws.writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	saved, err := sqlite.NewLaneProposalStore(ws.db).Complete(*done)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st.forget(loc.L16Token)
	ws.writeJSON(w, http.StatusOK, saved)
}

func (ws *Server) clipByIDOrDataset(store *sqlite.ClipStore, clipID, datasetID string) (*sqlite.Clip, error) {
	if clipID != "" {
		clip, err := store.GetClip(clipID)
		if err != nil || clip == nil {
			return nil, fmt.Errorf("clip %s not found", clipID)
		}
		return clip, nil
	}
	if datasetID == "" {
		return nil, fmt.Errorf("name the clip with clip_id or dataset_id")
	}
	clips, err := store.ListClips("")
	if err != nil {
		return nil, err
	}
	for _, c := range clips {
		if c.DatasetID == datasetID {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no clip is marked for dataset %s", datasetID)
}

// writeLaneTuning writes the server's current tuning to dir for the job's
// replay and returns its path and fingerprint, so the row names the config
// the replay ran and the stale check compares like with like.
func (ws *Server) writeLaneTuning(dir string) (path, fingerprint string, err error) {
	cfg := ws.snapshotTuningConfig()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", "", err
	}
	path = filepath.Join(dir, "tuning.json")
	return path, cfg.Fingerprint(), os.WriteFile(path, b, 0o644)
}
