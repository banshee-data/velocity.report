// Package runner is the worker: given a job, it verifies the captures, runs
// the kind's tool, writes an immutable result bundle, and serves an API from
// which the bundle can be fetched.
//
// State is a directory tree, not a database. Every attempt is a directory
// under the work directory, with the job it was given, its state, its log and
// its bundle; the queue is the attempts that are still queued, oldest first.
// A directory is what survives a stopped process and a switched-off blade
// intact, is what an operator can read over SSH when nothing else answers,
// and is what the hub will one day be sent.
package runner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

// Record is what the store keeps of one attempt: the job as submitted and
// the attempt's state.
type Record struct {
	Job     jobs.JobRequest `json:"job"`
	Attempt jobs.Attempt    `json:"attempt"`
	// RunIdentity is derived once at submission and kept, so a list never
	// has to re-derive it and a changed contract cannot silently move it.
	RunIdentity       jobs.RunIdentity `json:"run_identity"`
	RunIdentityDigest jobs.Digest      `json:"run_identity_digest"`
	// Parent is the campaign attempt this is one config of, if any; Children
	// are the config attempts of a campaign.
	Parent   string   `json:"parent,omitempty"`
	Children []string `json:"children,omitempty"`
	// Label is the name a campaign gave this config, for listings.
	Label string `json:"label,omitempty"`
}

// Store is the attempt directory tree.
type Store struct {
	root string
	mu   sync.Mutex
}

const (
	attemptsDir = "attempts"
	recordFile  = "record.json"
	logFile     = "log.txt"
	bundleDir   = "bundle"
)

// Open opens or creates the work directory.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, attemptsDir), 0o755); err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	return &Store{root: root}, nil
}

// Root is the work directory.
func (s *Store) Root() string { return s.root }

// newID is a time-ordered id: a sortable timestamp so that listings and the
// queue read in submission order from the names alone, then random bits so
// two submissions in one microsecond do not collide.
func newID(prefix string, now time.Time) string {
	var r [4]byte
	_, _ = rand.Read(r[:])
	return fmt.Sprintf("%s_%s_%s", prefix, now.UTC().Format("20060102T150405.000000"), hex.EncodeToString(r[:]))
}

// Submit validates a request, derives its identity and queues one attempt.
func (s *Store) Submit(req jobs.JobRequest, now time.Time) (Record, error) {
	identity, digest, err := req.Identity()
	if err != nil {
		return Record{}, err
	}
	id := newID("att", now)
	rec := Record{
		Job: req, RunIdentity: identity, RunIdentityDigest: digest,
		Attempt: jobs.Attempt{
			AttemptID: id, JobID: id, Ordinal: 1, State: jobs.StateQueued, CreatedAt: now.UTC(),
		},
	}
	return rec, s.create(rec)
}

// SubmitChild queues one config of a campaign, under its parent.
func (s *Store) SubmitChild(req jobs.JobRequest, parent, label string, now time.Time) (Record, error) {
	rec, err := s.Submit(req, now)
	if err != nil {
		return Record{}, err
	}
	rec.Parent = parent
	rec.Label = label
	return rec, s.Save(rec)
}

func (s *Store) dir(id string) (string, error) {
	// An id is a file name this store made; anything else is not looked up.
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid attempt id %q", id)
	}
	return filepath.Join(s.root, attemptsDir, id), nil
}

func (s *Store) create(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.dir(rec.Attempt.AttemptID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("attempt %s already exists", rec.Attempt.AttemptID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, recordFile), rec)
}

// Save writes the record. The bundle directory beside it is never touched.
func (s *Store) Save(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.dir(rec.Attempt.AttemptID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("attempt %s: %w", rec.Attempt.AttemptID, err)
	}
	return writeJSON(filepath.Join(dir, recordFile), rec)
}

// Get reads one attempt.
func (s *Store) Get(id string) (Record, error) {
	dir, err := s.dir(id)
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := readJSON(filepath.Join(dir, recordFile), &rec); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Record{}, fmt.Errorf("attempt %s: %w", id, os.ErrNotExist)
		}
		return Record{}, err
	}
	return rec, nil
}

// List reads every attempt, oldest first.
func (s *Store) List() ([]Record, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, attemptsDir))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]Record, 0, len(names))
	for _, name := range names {
		rec, err := s.Get(name)
		if err != nil {
			// A half-written directory is reported, not fatal: one bad
			// attempt must not hide the rest.
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// NextQueued is the oldest attempt still waiting, or nil.
func (s *Store) NextQueued() (*Record, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Attempt.State == jobs.StateQueued && len(all[i].Children) == 0 {
			return &all[i], nil
		}
	}
	return nil, nil
}

// Transition moves an attempt's state through the contract's machine, by
// the given actor, and records when it started or finished.
func (s *Store) Transition(id string, to jobs.State, actor jobs.Actor, now time.Time) (Record, error) {
	rec, err := s.Get(id)
	if err != nil {
		return Record{}, err
	}
	if err := jobs.Transition(rec.Attempt.State, to, actor); err != nil {
		return Record{}, fmt.Errorf("attempt %s: %w", id, err)
	}
	rec.Attempt.State = to
	t := now.UTC()
	switch to {
	case jobs.StateRunning:
		rec.Attempt.StartedAt = &t
	}
	if to.Terminal() {
		rec.Attempt.FinishedAt = &t
		rec.Attempt.LeaseExpires = nil
	}
	return rec, s.Save(rec)
}

// RecoverAfterRestart marks every attempt that was leased, running or
// uploading when the process last stopped as lost: nothing is running it
// now, and its bundle directory is whatever it managed to write.
func (s *Store) RecoverAfterRestart(now time.Time) ([]string, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var lost []string
	for _, rec := range all {
		switch rec.Attempt.State {
		case jobs.StateLeased, jobs.StateRunning, jobs.StateUploading:
			rec.Attempt.State = jobs.StateLost
			t := now.UTC()
			rec.Attempt.FinishedAt = &t
			rec.Attempt.LeaseExpires = nil
			rec.Attempt.Failure = &jobs.Failure{
				Reason:    "the worker stopped while this attempt was in progress",
				LocalPath: s.BundlePath(rec.Attempt.AttemptID),
				Partial:   true,
			}
			if err := s.Save(rec); err != nil {
				return lost, err
			}
			lost = append(lost, rec.Attempt.AttemptID)
		}
	}
	return lost, nil
}

// BundlePath is where an attempt's bundle is written.
func (s *Store) BundlePath(id string) string {
	return filepath.Join(s.root, attemptsDir, id, bundleDir)
}

// LogPath is the attempt's log.
func (s *Store) LogPath(id string) string {
	return filepath.Join(s.root, attemptsDir, id, logFile)
}

// AppendLog writes a line to the attempt's log with a UTC timestamp.
func (s *Store) AppendLog(id string, now time.Time, format string, args ...any) {
	f, err := os.OpenFile(s.LogPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", now.UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// LogTail returns the last n lines of the attempt's log.
func (s *Store) LogTail(id string, n int) ([]string, error) {
	data, err := os.ReadFile(s.LogPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// ReadBundle reads an attempt's bundle manifest, if it has written one.
func (s *Store) ReadBundle(id string) (jobs.BundleManifest, error) {
	var b jobs.BundleManifest
	err := readJSON(filepath.Join(s.BundlePath(id), jobs.BundleManifestFile), &b)
	return b, err
}

func ensureDir(dir string) error { return os.MkdirAll(dir, 0o755) }

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
