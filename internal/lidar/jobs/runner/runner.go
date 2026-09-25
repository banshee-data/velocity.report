package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

// Runner takes queued attempts one at a time through the contract's states,
// running each kind's executor and writing its bundle. It is both the worker
// and, for now, its own hub: there is no other authority to lease from, so it
// plays both actors and keeps to the transitions each is allowed.
type Runner struct {
	Store    *Store
	Captures *Captures
	Tools    ToolStager
	Profile  jobs.WorkerProfile
	// Executors defaults to the package table.
	Executors map[string]Executor
	// PollInterval is how long to wait after finding nothing queued.
	PollInterval time.Duration
	// Now is the clock; tests set it.
	Now func() time.Time
	// OnEvent receives a line per transition, for the process log.
	OnEvent func(format string, args ...any)

	mu      sync.Mutex
	current string
	cancel  context.CancelFunc
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) executors() map[string]Executor {
	if r.Executors != nil {
		return r.Executors
	}
	return Executors
}

func (r *Runner) event(format string, args ...any) {
	if r.OnEvent != nil {
		r.OnEvent(format, args...)
	}
}

// Available reports whether this build can run a kind, and why not if not.
func (r *Runner) Available(kind string) error {
	if _, ok := jobs.Kinds[kind]; !ok {
		return fmt.Errorf("unknown kind %q", kind)
	}
	if _, ok := r.executors()[kind]; !ok {
		return fmt.Errorf("this worker build cannot run %s (built without the pcap tag, or the kind has no executor)", kind)
	}
	return nil
}

// Current is the attempt being run, or "".
func (r *Runner) Current() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current
}

// Cancel stops the attempt being run, if it is the one named. The attempt
// becomes cancelled by the operator; its partial bundle stays.
func (r *Runner) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == id && r.cancel != nil {
		r.cancel()
		return true
	}
	return false
}

// Run drains the queue until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	if lost, err := r.Store.RecoverAfterRestart(r.now()); err != nil {
		return err
	} else if len(lost) > 0 {
		r.event("marked %d attempt(s) lost that were in progress when the worker last stopped", len(lost))
		// A lost child's campaign parent was never told: finishParent
		// normally runs from RunOne on every child transition, and
		// RecoverAfterRestart bypasses RunOne entirely by construction, since
		// nothing was running to bypass. Give each affected parent the same
		// chance to finalise it would have gotten from a live transition;
		// finishParent already no-ops on a parent that is not found, already
		// terminal, or still waiting on another child.
		for _, id := range lost {
			if rec, err := r.Store.Get(id); err == nil && rec.Parent != "" {
				r.finishParent(rec)
			}
		}
	}
	interval := r.PollInterval
	if interval == 0 {
		interval = 2 * time.Second
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		next, err := r.Store.NextQueued()
		if err != nil {
			return err
		}
		if next == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
			continue
		}
		r.RunOne(ctx, *next)
	}
}

// RunOne takes one attempt from queued to a terminal state.
func (r *Runner) RunOne(ctx context.Context, rec Record) {
	id := rec.Attempt.AttemptID
	log := func(format string, args ...any) { r.Store.AppendLog(id, r.now(), format, args...) }

	// Lease. The runner is the hub here; the lease is what a later hub will
	// grant, and its expiry is what a later hub will watch.
	if _, err := r.lease(id); err != nil {
		r.event("%s: %v", id, err)
		return
	}
	attemptCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.current, r.cancel = id, cancel
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		r.current, r.cancel = "", nil
		r.mu.Unlock()
	}()

	fail := func(reason string, partial bool) {
		log("failed: %s", reason)
		rec, err := r.Store.Get(id)
		if err != nil {
			return
		}
		to := jobs.StateFailed
		if attemptCtx.Err() != nil && ctx.Err() == nil {
			to = jobs.StateCancelled
		}
		actor := jobs.ActorWorker
		if to == jobs.StateCancelled {
			actor = jobs.ActorOperator
		}
		if _, err := r.Store.Transition(id, to, actor, r.now()); err != nil {
			r.event("%s: %v", id, err)
			return
		}
		rec, _ = r.Store.Get(id)
		rec.Attempt.Failure = &jobs.Failure{Reason: reason, LocalPath: r.Store.BundlePath(id), Partial: partial}
		_ = r.Store.Save(rec)
		r.event("%s %s: %s", id, to, reason)
		r.finishParent(rec)
	}

	if err := r.Available(rec.Job.Kind); err != nil {
		fail(err.Error(), false)
		return
	}
	captures, err := r.Captures.Resolve(rec.Job.CaptureManifest, log)
	if err != nil {
		fail("captures: "+err.Error(), false)
		return
	}
	if _, err := r.Store.Transition(id, jobs.StateRunning, jobs.ActorWorker, r.now()); err != nil {
		r.event("%s: %v", id, err)
		return
	}
	r.event("%s running %s", id, rec.Job.Kind)
	bundleDir := r.Store.BundlePath(id)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		fail(err.Error(), false)
		return
	}
	started := r.now()
	env := Env{
		BundleDir: bundleDir, Captures: captures, CaptureRoot: r.Captures.Root(), Tools: r.Tools, Log: log,
		Report: func(p jobs.Progress) {
			if rec, err := r.Store.Get(id); err == nil {
				rec.Attempt.Progress = p
				_ = r.Store.Save(rec)
			}
		},
	}
	summary, runErr := r.executors()[rec.Job.Kind].Run(attemptCtx, rec.Job, env)
	finished := r.now()
	if runErr != nil {
		// What was written stays, as evidence, with a manifest that says
		// it is partial.
		_, _ = r.writeBundle(rec, started, finished, "partial", nil)
		fail(runErr.Error(), true)
		return
	}

	// Uploading and verifying are, without a hub, finalising the bundle
	// and checking it against the identity the job was queued with.
	if _, err := r.Store.Transition(id, jobs.StateUploading, jobs.ActorWorker, r.now()); err != nil {
		r.event("%s: %v", id, err)
		return
	}
	manifest, err := r.writeBundle(rec, started, finished, "completed", summary)
	if err != nil {
		fail("write bundle: "+err.Error(), true)
		return
	}
	if _, err := r.Store.Transition(id, jobs.StateVerifying, jobs.ActorHub, r.now()); err != nil {
		r.event("%s: %v", id, err)
		return
	}
	digest, err := r.verifyBundle(rec, manifest)
	if err != nil {
		fail("verify bundle: "+err.Error(), true)
		return
	}
	done, err := r.Store.Transition(id, jobs.StateAccepted, jobs.ActorHub, r.now())
	if err != nil {
		r.event("%s: %v", id, err)
		return
	}
	done.Attempt.BundleDigest = digest
	_ = r.Store.Save(done)
	log("accepted: bundle %s", digest.Short())
	r.event("%s accepted (%s)", id, digest.Short())
	r.finishParent(done)
}

func (r *Runner) lease(id string) (Record, error) {
	rec, err := r.Store.Transition(id, jobs.StateLeased, jobs.ActorHub, r.now())
	if err != nil {
		return rec, err
	}
	expires := r.now().Add(jobs.LeaseDuration)
	rec.Attempt.LeaseExpires = &expires
	rec.Attempt.WorkerID = r.Profile.WorkerID
	return rec, r.Store.Save(rec)
}

// writeBundle lists every file under the bundle directory with its digest
// and writes the manifest beside them.
func (r *Runner) writeBundle(rec Record, started, finished time.Time, outcome string, summary json.RawMessage) (jobs.BundleManifest, error) {
	dir := r.Store.BundlePath(rec.Attempt.AttemptID)
	var files []jobs.BundleFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == jobs.BundleManifestFile {
			return nil
		}
		digest, n, err := jobs.DigestFile(path)
		if err != nil {
			return err
		}
		files = append(files, jobs.BundleFile{Path: rel, Bytes: n, SHA256: digest})
		return nil
	})
	if err != nil {
		return jobs.BundleManifest{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	m := jobs.BundleManifest{
		OutputSchema: jobs.OutputSchemaVersion, Identity: rec.RunIdentity, IdentityDigest: rec.RunIdentityDigest,
		JobID: rec.Attempt.JobID, AttemptID: rec.Attempt.AttemptID, Worker: r.Profile,
		StartedUTC: started.UTC().Format(time.RFC3339), FinishedUTC: finished.UTC().Format(time.RFC3339),
		Outcome: outcome, Summary: summary, Files: files,
	}
	if err := m.Validate(); err != nil {
		return m, err
	}
	return m, writeJSON(filepath.Join(dir, jobs.BundleManifestFile), m)
}

// verifyBundle is what a hub will do on ingest: read the manifest back from
// disk, check it against the identity the job was queued with, and check
// every listed file's bytes.
func (r *Runner) verifyBundle(rec Record, written jobs.BundleManifest) (jobs.Digest, error) {
	m, err := r.Store.ReadBundle(rec.Attempt.AttemptID)
	if err != nil {
		return "", err
	}
	if m.IdentityDigest != rec.RunIdentityDigest {
		return "", fmt.Errorf("bundle identity %s is not the job's %s", m.IdentityDigest.Short(), rec.RunIdentityDigest.Short())
	}
	if m.Outcome != "completed" {
		return "", fmt.Errorf("bundle outcome is %s", m.Outcome)
	}
	dir := r.Store.BundlePath(rec.Attempt.AttemptID)
	for _, f := range m.Files {
		digest, n, err := jobs.DigestFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil {
			return "", err
		}
		if digest != f.SHA256 || n != f.Bytes {
			return "", fmt.Errorf("%s does not match its manifest entry", f.Path)
		}
	}
	digest, err := m.Digest()
	if err != nil {
		return "", err
	}
	// written is the manifest this worker itself produced moments ago, still
	// in memory; m is a fresh read of what is now actually on disk. They
	// should be identical, and a mismatch means the bundle changed, or was
	// corrupted, between the write and this verification.
	writtenDigest, err := written.Digest()
	if err != nil {
		return "", fmt.Errorf("the manifest this worker wrote: %w", err)
	}
	if digest != writtenDigest {
		return "", fmt.Errorf("bundle on disk (%s) does not match what this worker wrote (%s)", digest.Short(), writtenDigest.Short())
	}
	return digest, nil
}

// DefaultProfile describes this host.
func DefaultProfile(workerID, version, gitSHA string) jobs.WorkerProfile {
	host, _ := os.Hostname()
	return jobs.WorkerProfile{
		WorkerID: workerID, Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(), Version: version, GitSHA: gitSHA,
	}
}

// ErrNotFound is returned by lookups of an attempt that does not exist.
var ErrNotFound = errors.New("not found")

// IsNotFound reports whether err is a missing attempt.
func IsNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) || (err != nil && strings.Contains(err.Error(), "invalid attempt id"))
}
