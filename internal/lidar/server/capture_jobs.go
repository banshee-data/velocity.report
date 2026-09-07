package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// sessionMotionPassFunc runs the classification. It is a variable so the job
// plumbing can be exercised without libpcap, which the pass itself requires.
var sessionMotionPassFunc = sessionMotionPass

// captureJobStore adapts the sqlite capture store to the runner's narrow
// interface, which keeps capjobs free of any storage dependency.
type captureJobStore struct {
	store *sqlite.CaptureStore
}

func (a captureJobStore) ClaimNextJob() (capjobs.Job, error) {
	job, err := a.store.ClaimNextJob()
	if err != nil {
		// An empty queue arrives as a no-rows error. The runner has its own
		// name for that so it does not have to know about SQL.
		if errors.Is(err, sqlite.ErrNotFound) {
			return capjobs.Job{}, capjobs.ErrNoWork
		}
		return capjobs.Job{}, err
	}
	return capjobs.Job{
		JobID: job.JobID, Kind: job.Kind, SessionID: job.SessionID,
		RootID: job.RootID, State: job.State,
	}, nil
}

func (a captureJobStore) UpdateJobProgress(jobID string, current, total int64, detail string) error {
	return a.store.UpdateJobProgress(jobID, current, total, detail)
}

func (a captureJobStore) FinishJob(jobID, state, jobErr string) error {
	return a.store.FinishJob(jobID, state, jobErr)
}

func (a captureJobStore) GetJob(jobID string) (capjobs.Job, error) {
	job, err := a.store.GetJob(jobID)
	if err != nil {
		return capjobs.Job{}, err
	}
	return capjobs.Job{JobID: job.JobID, Kind: job.Kind, SessionID: job.SessionID,
		RootID: job.RootID, State: job.State}, nil
}

func (a captureJobStore) ReleaseRunningJobs() (int64, error) {
	return a.store.ReleaseRunningJobs()
}

// StartCaptureJobs begins draining the capture job queue. It is a no-op when
// the server has no database, since there is then nothing to queue work in.
func (ws *Server) StartCaptureJobs(ctx context.Context) error {
	store, err := ws.captureStore()
	if err != nil {
		return nil
	}
	ws.captureRunner = &capjobs.Runner{
		Store:   captureJobStore{store: store},
		Handler: ws.runCaptureJob,
		OnEvent: func(format string, args ...any) { diagf(format, args...) },
	}
	return ws.captureRunner.Start(ctx)
}

// runCaptureJob dispatches one queued job.
func (ws *Server) runCaptureJob(ctx context.Context, job capjobs.Job, report func(capjobs.Progress)) error {
	switch job.Kind {
	case capjobs.KindMotionPass:
		return ws.runMotionPass(ctx, job, report)
	default:
		return fmt.Errorf("unknown capture job kind %q", job.Kind)
	}
}

// runMotionPass classifies a whole session and stores its timeline.
//
// The pass runs over the session's captures joined into one stream, not file by
// file. The background model needs tens of seconds to settle, so a per-file
// pass reports that settling as motion at every file head and cannot see a
// static stretch that spans a boundary — which is the stretch a replay case
// most often wants.
func (ws *Server) runMotionPass(ctx context.Context, job capjobs.Job, report func(capjobs.Progress)) error {
	store, err := ws.captureStore()
	if err != nil {
		return err
	}
	if job.SessionID == "" {
		return errors.New("a motion pass needs a session")
	}

	files, err := store.SessionFiles(job.SessionID)
	if err != nil {
		return fmt.Errorf("loading session files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("session %s has no files", job.SessionID)
	}

	paths := make([]string, 0, len(files))
	for _, f := range files {
		resolved, err := ws.resolvePCAPPath(f.RelPath)
		if err != nil {
			// The index stores paths relative to a root, which may not be the
			// replay safe directory. Fall back to the root-relative join.
			resolved, err = ws.resolveCapturePath(f.RootID, f.RelPath)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", f.RelPath, err)
			}
		}
		paths = append(paths, resolved)
	}

	report(capjobs.Progress{Current: 0, Total: int64(len(paths)),
		Detail: fmt.Sprintf("joining %d captures", len(paths))})

	periods, err := sessionMotionPassFunc(ctx, paths, ws.udpPort, ws.snapshotTuningConfig(),
		func(current, total int64, detail string) {
			report(capjobs.Progress{Current: current, Total: total, Detail: detail})
		})
	if err != nil {
		return err
	}

	if err := store.ReplaceSessionPeriods(job.SessionID, periods); err != nil {
		return fmt.Errorf("storing the timeline: %w", err)
	}
	report(capjobs.Progress{Current: int64(len(paths)), Total: int64(len(paths)),
		Detail: fmt.Sprintf("%d periods", len(periods))})
	return nil
}

// resolveCapturePath joins a root-relative path to its configured root and
// refuses anything that escapes it.
func (ws *Server) resolveCapturePath(rootID, relPath string) (string, error) {
	store, err := ws.captureStore()
	if err != nil {
		return "", err
	}
	root, err := store.GetRoot(rootID)
	if err != nil {
		return "", fmt.Errorf("unknown capture root %s", rootID)
	}
	return safeCaptureJoin(root.Path, relPath)
}

// safeCaptureJoin resolves a root-relative path against its root and refuses
// anything that escapes it. The capture index stores paths relative to a root
// precisely so this check has something to check against.
func safeCaptureJoin(root, relPath string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid capture root: %w", err)
	}
	joined := filepath.Join(rootAbs, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(rootAbs, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("capture path escapes its root: %s", relPath)
	}
	return joined, nil
}

// handleCaptureMotionPass queues a motion pass over a session.
//
// POST /api/lidar/capture/motion-pass?session_id=…
func (ws *Server) handleCaptureMotionPass(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	sessions, err := store.ListSessions("")
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var found *sqlite.CaptureSession
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no such session")
		return
	}

	job, err := store.EnqueueJob(sqlite.JobKindMotionPass, sessionID, found.RootID,
		fmt.Sprintf("queued for %d captures", found.FileCount))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"job": job})
}

// handleCaptureJobs lists jobs, newest first.
//
// GET /api/lidar/capture/jobs?session_id=…&limit=…
func (ws *Server) handleCaptureJobs(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := store.ListJobs(r.URL.Query().Get("session_id"), limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"jobs": jobs, "count": len(jobs)})
}

// handleCaptureJobCancel cancels a queued or running job.
//
// POST /api/lidar/capture/jobs/cancel?job_id=…
func (ws *Server) handleCaptureJobCancel(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "job_id is required")
		return
	}
	if err := store.CancelJob(jobID); err != nil {
		if errors.Is(err, sqlite.ErrNotFound) {
			ws.writeJSONError(w, http.StatusNotFound, "no such job, or it has already finished")
			return
		}
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"job_id": jobID, "state": sqlite.JobCancelled})
}

// handleCapturePeriods returns a session's motion/static timeline.
//
// GET /api/lidar/capture/periods?session_id=…
func (ws *Server) handleCapturePeriods(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	periods, err := store.ListSessionPeriods(sessionID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var staticNs, motionNs int64
	for _, p := range periods {
		if p.Type == sqlite.PeriodStatic {
			staticNs += p.DurationNs
		} else {
			motionNs += p.DurationNs
		}
	}
	writeCaptureJSON(w, map[string]any{
		"session_id":     sessionID,
		"periods":        periods,
		"count":          len(periods),
		"static_seconds": time.Duration(staticNs).Seconds(),
		"motion_seconds": time.Duration(motionNs).Seconds(),
	})
}

// decodeJSONBody reads a JSON request body into v, reporting a client error.
func decodeJSONBody(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}
