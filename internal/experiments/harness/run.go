package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/experiments"
	"golang.org/x/sys/unix"
)

// Options selects a local checkout and host. Empty Slug selects all ready specs.
type Options struct {
	Root        string
	Host        Host
	Slug        string
	DryRun      bool
	RetryFailed bool
	buildTools  func(context.Context, string, string, string) (string, string, error)
}

type Status struct {
	Updated time.Time         `json:"updated"`
	Commit  string            `json:"commit,omitempty"`
	State   string            `json:"state"`
	Skipped map[string]string `json:"skipped"`
	Runs    []string          `json:"runs,omitempty"`
}

type Run struct {
	PR             string                        `json:"pr,omitempty"`
	PRNumber       int                           `json:"pr_number,omitempty"`
	PRNode         string                        `json:"pr_node,omitempty"`
	PublishState   string                        `json:"publish_state,omitempty"`
	PublishError   string                        `json:"publish_error,omitempty"`
	RetryLimits    map[string]int                `json:"retry_limits,omitempty"`
	Slug           string                        `json:"slug"`
	ID             string                        `json:"id"`
	Commit         string                        `json:"commit"`
	SpecDigest     string                        `json:"spec_digest"`
	InputDigest    string                        `json:"input_digest"`
	BuildVersion   string                        `json:"build_version"`
	Host           string                        `json:"host"`
	Class          string                        `json:"class"`
	Started        time.Time                     `json:"started"`
	Updated        time.Time                     `json:"updated"`
	State          string                        `json:"state"`
	Arms           []experiments.Arm             `json:"arms"`
	Sources        map[string]string             `json:"sources"`
	Measures       map[string]map[string]float64 `json:"measures"`
	SummaryDigests map[string]string             `json:"summary_digests"`
}

// Tick runs one offline scheduling pass. A kernel lock is released even on
// SIGKILL; on the next pass interrupted units become distinct retry attempts.
func Tick(ctx context.Context, o Options) (status Status, err error) {
	status = Status{Updated: time.Now().UTC(), State: "checking", Skipped: map[string]string{}}
	root, e := filepath.Abs(o.Root)
	if e != nil {
		return status, e
	}
	o.Root = root
	if o.buildTools == nil {
		o.buildTools = build
	}
	h := o.Host
	for _, p := range []string{h.EvidenceRoot, h.StageDir} {
		if !outside(root, p) {
			return status, fmt.Errorf("raw state/output must be outside checkout: %s", p)
		}
		if e = os.MkdirAll(p, 0755); e != nil {
			return status, e
		}
	}
	state := filepath.Join(h.EvidenceRoot, "state")
	if e = os.MkdirAll(state, 0755); e != nil {
		return status, e
	}
	lock, e := os.OpenFile(filepath.Join(state, "tick.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return status, e
	}
	defer lock.Close()
	if e = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		if errors.Is(e, unix.EWOULDBLOCK) {
			status.State = "busy"
			return status, nil
		}
		return status, e
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	// The executor inherits this descriptor: if this process is SIGKILLed,
	// a surviving replay keeps the lock until it exits and cannot overlap a retry.
	ctx = context.WithValue(ctx, lockContextKey{}, lock)
	defer func() {
		status.Updated = time.Now().UTC()
		if err != nil {
			status.State = "error"
			status.Skipped["tick"] = err.Error()
		}
		if e := atomicJSON(filepath.Join(h.EvidenceRoot, "status", "heartbeat.json"), status); err == nil {
			err = e
		}
	}()
	sha, e := GitStatus(ctx, root)
	if e != nil {
		return status, e
	}
	if h.FetchOnTick && !o.DryRun {
		syncCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		syncErr := syncMain(syncCtx, root)
		cancel()
		if syncErr != nil {
			status.Skipped["sync"] = syncErr.Error()
		}
		sha, e = GitStatus(ctx, root)
		if e != nil {
			return status, e
		}
	}
	status.Commit = sha
	if e = atomicJSON(filepath.Join(h.EvidenceRoot, "status", "heartbeat.json"), status); e != nil {
		return status, e
	}
	if paused(root, state) {
		status.State = "paused"
		return status, nil
	}
	set, e := experiments.LoadCompatible(root, status.Skipped)
	if e != nil {
		return status, e
	}
	candidates := slices.Clone(set.Specs)
	// Longest downstream machine chain first, then oldest proposal, then slug.
	var downstream func(*experiments.Spec, map[string]bool) time.Duration
	downstream = func(sp *experiments.Spec, seen map[string]bool) time.Duration {
		if seen[sp.ID] {
			return 0
		}
		seen[sp.ID] = true
		defer delete(seen, sp.ID)
		own, _ := sp.Estimate()
		var tail time.Duration
		for _, next := range set.Specs {
			if !next.Open() {
				continue
			}
			if slices.Contains(next.DependsOn, sp.ID) || slices.Contains(next.NeedsRuns, sp.ID) {
				tail = max(tail, downstream(next, seen))
			}
		}
		return own + tail
	}
	priorities := map[string]time.Duration{}
	for _, sp := range candidates {
		priorities[sp.ID] = downstream(sp, map[string]bool{})
	}
	priority := func(sp *experiments.Spec) time.Duration { return priorities[sp.ID] }
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if priority(a) != priority(b) {
			return priority(a) > priority(b)
		}
		if a.Created != b.Created {
			return a.Created < b.Created
		}
		return a.ID < b.ID
	})
	found := o.Slug == ""
	for _, sp := range candidates {
		if o.Slug != "" && sp.ID != o.Slug {
			continue
		}
		found = true
		if sp.Status != experiments.StatusApproved {
			status.Skipped[sp.ID] = "status " + string(sp.Status)
			continue
		}
		if errs := set.ValidateSpec(root, sp); len(errs) > 0 {
			status.Skipped[sp.ID] = fmt.Sprint(errs)
			continue
		}
		if why := eligible(ctx, root, set, sp, h); why != "" {
			status.Skipped[sp.ID] = why
			continue
		}
		if o.DryRun {
			status.Runs = append(status.Runs, sp.ID)
			continue
		}
		if paused(root, state) {
			status.State = "paused"
			return status, nil
		}
		status.State = "running " + sp.ID
		if e = atomicJSON(filepath.Join(h.EvidenceRoot, "status", "heartbeat.json"), status); e != nil {
			return status, e
		}
		runID, why, e := runSpec(ctx, o, sp, sha)
		if e != nil {
			return status, e
		}
		if why != "" {
			status.Skipped[sp.ID] = why
		}
		if runID != "" {
			status.Runs = append(status.Runs, sp.ID+"/"+runID)
		}
		if ctx.Err() != nil {
			return status, ctx.Err()
		}
	}
	if !found {
		if why, ok := status.Skipped[o.Slug]; ok {
			return status, fmt.Errorf("%s: %s", o.Slug, why)
		}
		return status, fmt.Errorf("unknown experiment %s", o.Slug)
	}
	status.State = "idle"
	if o.DryRun {
		status.State = "dry-run"
	}
	return status, nil
}

func paused(root, state string) bool {
	if _, e := os.Stat(filepath.Join(state, "PAUSE")); e == nil {
		return true
	}
	var c struct {
		Paused bool `json:"paused"`
	}
	p := filepath.Join(root, "config", "harness.json")
	if e := readJSON(p, &c); e != nil && !os.IsNotExist(e) {
		return true
	}
	return c.Paused
}

func eligible(ctx context.Context, root string, set *experiments.Set, sp *experiments.Spec, h Host) string {
	for _, d := range sp.DependsOn {
		dep := set.Get(d)
		if dep == nil || dep.Status != experiments.StatusAnswered {
			return "waiting on answer " + d
		}
	}
	for _, d := range sp.NeedsRuns {
		if d == sp.ID || set.Get(d) == nil || !hasRunNote(root, d) {
			return "waiting on checked-in successful run note " + d
		}
	}
	if sp.Requires.Cores > h.Cores {
		return "insufficient cores"
	}
	for _, c := range sp.Requires.Capabilities {
		if !slices.Contains(h.Capabilities, c) {
			return "missing capability " + c
		}
	}
	machine := false
	for _, st := range sp.Steps {
		if experiments.ClassifyKind(st.Kind) == experiments.KindManual {
			if st.Closed != "" {
				if e := experiments.CheckClosure(ctx, root, sp, st); e != nil {
					return e.Error()
				}
			}
			continue
		}
		machine = true
		if !supported(st.Kind) {
			return "no offline executor for " + st.Kind
		}
		if _, e := params(st); e != nil {
			return e.Error()
		}
	}
	if !machine {
		return "no machine steps"
	}
	var active Run
	if e := readJSON(filepath.Join(h.EvidenceRoot, "state", sp.ID, "active.json"), &active); os.IsNotExist(e) {
		simulated := map[string]experiments.UnitRecord{}
		ready := false
		for _, st := range sp.Steps {
			for _, u := range sp.Units(st) {
				if sp.BlockedBy(u, simulated) != "" {
					continue
				}
				if experiments.ClassifyKind(st.Kind) == experiments.KindManual && st.Closed == "" {
					continue
				}
				simulated[u.ID()] = experiments.UnitRecord{Unit: u, State: experiments.UnitDone}
				if experiments.ClassifyKind(st.Kind) != experiments.KindManual {
					ready = true
				}
			}
		}
		if !ready {
			return "waiting for a checked-in manual closure"
		}
	}

	return ""
}

func runSpec(ctx context.Context, o Options, sp *experiments.Spec, sha string) (string, string, error) {
	h := o.Host
	raw, e := os.ReadFile(filepath.Join(o.Root, sp.Path))
	if e != nil {
		return "", "", e
	}
	specDigest := digest(raw)
	inputs := []string{specDigest}
	for _, a := range sp.Arms {
		b, e := os.ReadFile(filepath.Join(o.Root, a.Tuning))
		if e != nil {
			return "", "", e
		}
		inputs = append(inputs, digest(b))
	}
	corpora := map[string]Corpus{}
	references := map[string]Reference{}
	for _, st := range sp.Steps {
		if st.Kind == "state_estimation_baseline" || st.Kind == "box_eval" {
			if c, ok := h.Corpora[sp.CasesName()]; ok {
				corpora[sp.CasesName()] = c
			}
		}
	}
	for key, ref := range h.References {
		if strings.HasPrefix(key, sp.ID+"/") {
			references[key] = ref
		}
	}
	// Only input bindings affect the execution identity, not connectivity,
	// retry policy, disk floor or daily budget changes used during recovery.
	identity := struct {
		Worker, Class, CaptureRoot, CaptureSubdir string
		Corpora                                   map[string]Corpus
		References                                map[string]Reference
	}{h.Worker, h.Class, h.CaptureRoot, h.CaptureSubdir, corpora, references}

	hb, e := jsonBytes(identity)
	if e != nil {
		return "", "", e
	}
	inputs = append(inputs, digest(hb))
	for _, c := range corpora {
		for _, p := range []string{c.Corpus, c.Index} {
			if !filepath.IsAbs(p) {
				b, e := os.ReadFile(filepath.Join(o.Root, p))
				if e != nil {
					return "", "", e
				}
				inputs = append(inputs, p+digest(b))
			} else {
				return "", "", fmt.Errorf("corpus/index must be checked-in paths: %s", p)
			}
		}
	}
	sort.Strings(inputs)
	inputDigest := digest([]byte(fmt.Sprint(inputs)))
	base := filepath.Join(h.EvidenceRoot, "state", sp.ID)
	active := filepath.Join(base, "active.json")
	var r Run
	e = readJSON(active, &r)
	if e != nil && !os.IsNotExist(e) {
		return "", "", e
	}
	if r.ID != "" && r.InputDigest != inputDigest {
		r.State = "superseded"
		r.Updated = time.Now().UTC()
		if e = atomicJSON(filepath.Join(base, r.ID, "run.json"), r); e != nil {
			return "", "", e
		}
		oldRaw, err := git(ctx, o.Root, "show", r.Commit+":"+sp.Path)
		if err != nil {
			return "", "", err
		}
		oldSpec, err := experiments.Parse([]byte(oldRaw), sp.Path)
		if err != nil {
			return "", "", err
		}
		oldSpec.Path = sp.Path
		if e = writeNote(h, &r, experiments.OpenLedger(filepath.Join(base, r.ID, "units")), oldSpec); e != nil {
			return "", "", e
		}
		r = Run{}
	}
	if r.ID == "" {
		version, e := buildVersion(o.Root)
		if e != nil {
			return "", "", e
		}
		r = Run{Slug: sp.ID, ID: time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + sha[:7], Commit: sha, SpecDigest: specDigest, InputDigest: inputDigest, BuildVersion: version, Host: h.Worker, Class: h.Class, Started: time.Now().UTC(), Arms: sp.Arms, Sources: map[string]string{}, Measures: map[string]map[string]float64{}, SummaryDigests: map[string]string{}}
	}
	if r.RetryLimits == nil {
		r.RetryLimits = map[string]int{}
	}
	dir := filepath.Join(base, r.ID)
	ledger := experiments.OpenLedger(filepath.Join(dir, "units"))
	// Publication fields never change the measurement note's timestamp. A
	// terminal run stays byte-identical while CI checks its previous push.
	stable := func() []byte {
		copy := r
		copy.Updated = time.Time{}
		copy.PR = ""
		copy.PRNumber = 0
		copy.PRNode = ""
		copy.PublishState = ""
		copy.PublishError = ""
		b, _ := jsonBytes(copy)
		return b
	}
	previous := stable()
	save := func() error {
		current := stable()
		if r.Updated.IsZero() || !bytes.Equal(previous, current) {
			r.Updated = time.Now().UTC()
			previous = current
		}
		if e := atomicJSON(filepath.Join(dir, "run.json"), r); e != nil {
			return e
		}
		if e := atomicJSON(active, r); e != nil {
			return e
		}
		if e := atomicJSON(filepath.Join(h.EvidenceRoot, "status", "last-run.json"), r); e != nil {
			return e
		}
		return writeNote(h, &r, ledger, sp)
	}
	publishProgress := func() error {
		if e := save(); e != nil {
			return e
		}
		if h.Publish != nil {
			publishCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			pe := publish(publishCtx, h, &r)
			cancel()
			r.PublishError = ""
			if pe != nil {
				r.PublishError = pe.Error()
			}
			return save()
		}
		return nil
	}

	var src, bin string
	recs, e := ledger.Records(sp.ID)
	if e != nil {
		return r.ID, "", e
	}
	// Revalidate every completed unit before any network publication. A saved
	// complete state alone is not evidence that a resumed run can be merged.
	for _, st := range sp.Steps {
		if experiments.ClassifyKind(st.Kind) == experiments.KindManual {
			continue
		}
		for _, u := range sp.Units(st) {
			cur := recs[u.ID()]
			if cur.State != experiments.UnitDone {
				if r.State == "complete" {
					r.State = "waiting"
				}
				continue
			}
			var receipt Receipt
			if e = readJSON(filepath.Join(cur.Out, "completion.json"), &receipt); e != nil {
				return r.ID, "", fmt.Errorf("done unit %s has no valid completion receipt: %w", u.ID(), e)
			}
			b, err := os.ReadFile(filepath.Join(cur.Out, "tool-summary.json"))
			if err != nil || receipt.SpecDigest != r.SpecDigest || digest(b) != receipt.Digest {
				return r.ID, "", fmt.Errorf("done unit %s has changed evidence; preserve it and investigate", u.ID())
			}
			r.Measures[u.ID()] = receipt.Measures
			r.SummaryDigests[u.ID()] = receipt.Digest
		}
	}
	if e = publishProgress(); e != nil {
		return r.ID, "", e
	}
	for _, st := range sp.Steps {
		for _, u := range sp.Units(st) {
			cur := recs[u.ID()]
			if cur.State == experiments.UnitDone {
				continue
			}
			if experiments.ClassifyKind(st.Kind) == experiments.KindManual {
				if st.Closed != "" {
					v, e := ledger.Apply(sp, experiments.Mark{Unit: u, To: experiments.Done, Worker: h.Worker, Class: h.Class, Artefact: st.Closed, Now: time.Now().UTC(), SpecDigest: r.SpecDigest}, func(_, _ string) error { return experiments.CheckClosure(ctx, o.Root, sp, st) })
					if e != nil {
						continue
					}
					recs[u.ID()] = v
				}
				continue
			}
			if sp.BlockedBy(u, recs) != "" {
				continue
			}
			if cur.State == experiments.UnitRunning {
				if sp.Reclaim == experiments.ReclaimNever || !h.Reclaim || cur.Worker != h.Worker && (sp.Reclaim == experiments.ReclaimOrigin || !sp.Stale(st, cur, time.Now())) {
					continue
				}
				// The exclusive host lock proves our former process has exited.
				v, e := ledger.Apply(sp, experiments.Mark{Unit: u, To: experiments.Fail, Worker: cur.Worker, Note: "interrupted attempt recovered under exclusive host lock", Now: time.Now().UTC(), SpecDigest: r.SpecDigest}, nil)
				if e != nil {
					return r.ID, "", e
				}
				recs[u.ID()] = v
				if e = archiveAttempt(dir, v); e != nil {
					return r.ID, "", e
				}
			}
			prior := recs[u.ID()]
			if prior.State == experiments.UnitFailed && prior.Out != "" && st.Kind == "state_estimation_baseline" {
				stage := filepath.Join(h.StageDir, r.ID, filepath.FromSlash(u.ID()), filepath.Base(prior.Out))
				if e = archiveStage(stage, filepath.Join(prior.Out, "staging")); e != nil {
					r.State = "staging archive wait"
					if se := save(); se != nil {
						return r.ID, "", se
					}
					return r.ID, e.Error(), nil
				}
			}
			limit := r.RetryLimits[u.ID()]
			if limit == 0 {
				limit = 3
			}
			if o.RetryFailed && recs[u.ID()].State == experiments.UnitFailed && recs[u.ID()].Attempt >= limit {
				limit = recs[u.ID()].Attempt + 3
				r.RetryLimits[u.ID()] = limit
				if e = save(); e != nil {
					return r.ID, "", e
				}
			}
			for recs[u.ID()].Attempt < limit && recs[u.ID()].State != experiments.UnitDone {
				if ctx.Err() != nil {
					r.State = "interrupted"
					if e = save(); e != nil {
						return r.ID, "", e
					}
					return r.ID, "interrupted", nil
				}
				if paused(o.Root, filepath.Join(h.EvidenceRoot, "state")) {
					r.State = "paused"
					e = save()
					return r.ID, "paused", e
				}
				current, e := GitStatus(ctx, o.Root)
				if e != nil || current != sha {
					r.State = "checkout changed"
					if se := save(); se != nil {
						return r.ID, "", se
					}
					return r.ID, "checkout changed; resume after a clean main check-in", nil
				}
				prior := recs[u.ID()]
				if prior.State == experiments.UnitFailed && prior.Out != "" && st.Kind == "state_estimation_baseline" {
					stage := filepath.Join(h.StageDir, r.ID, filepath.FromSlash(u.ID()), filepath.Base(prior.Out))
					if e = archiveStage(stage, filepath.Join(prior.Out, "staging")); e != nil {
						r.State = "staging archive wait"
						if se := save(); se != nil {
							return r.ID, "", se
						}
						return r.ID, e.Error(), nil
					}
				}
				if e = diskGate(h, sp.Requires.DiskGB); e != nil {
					r.State = "disk wait"
					if se := save(); se != nil {
						return r.ID, "", se
					}
					return r.ID, e.Error(), nil
				}
				if e = budget(h); e != nil {
					r.State = "budget wait"
					if se := save(); se != nil {
						return r.ID, "", se
					}
					return r.ID, e.Error(), nil
				}
				if bin == "" {
					buildCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
					src, bin, e = o.buildTools(buildCtx, o.Root, h.EvidenceRoot, r.Commit)
					cancel()
					if e != nil {
						return r.ID, "", e
					}
				}
				attempt := recs[u.ID()].Attempt + 1
				out := filepath.Join(h.EvidenceRoot, "runs", sp.ID, r.ID, filepath.FromSlash(u.ID()), fmt.Sprintf("attempt-%d", attempt))
				if e = os.MkdirAll(out, 0755); e != nil {
					return r.ID, "", e
				}
				v, e := ledger.Apply(sp, experiments.Mark{Unit: u, To: experiments.Start, Worker: h.Worker, Class: h.Class, Reclaim: h.Reclaim, Out: out, Timeout: 3 * sp.UnitEstimate(st), Now: time.Now().UTC(), SpecDigest: r.SpecDigest}, nil)
				if e != nil {
					return r.ID, "", e
				}
				recs[u.ID()] = v
				if e = archiveAttempt(dir, v); e != nil {
					return r.ID, "", e
				}
				r.State = "running"
				if e = save(); e != nil {
					return r.ID, "", e
				}

				timeout := 3 * sp.UnitEstimate(st)
				if timeout <= 0 {
					return r.ID, "", fmt.Errorf("unit %s has no positive timeout estimate", u.ID())
				}
				unitCtx, cancel := context.WithTimeout(ctx, timeout)
				measures, summary, execErr := execute(unitCtx, src, bin, h, sp, st, u, out, recs, &r, func() error { return save() })
				cancel()
				if execErr == nil {
					if e = atomicFile(filepath.Join(out, "tool-summary.json"), summary); e != nil {
						execErr = e
					}
					if execErr == nil {
						execErr = atomicJSON(filepath.Join(out, "completion.json"), Receipt{SpecDigest: r.SpecDigest, Digest: digest(summary), Measures: measures})
					}
				}
				finish := experiments.Done
				note := ""
				if execErr != nil {
					finish = experiments.Fail
					note = execErr.Error()
				}
				v, e = ledger.Apply(sp, experiments.Mark{Unit: u, To: finish, Worker: h.Worker, Class: h.Class, Out: out, Note: note, Now: time.Now().UTC(), SpecDigest: r.SpecDigest}, nil)
				if e != nil {
					return r.ID, "", e
				}
				recs[u.ID()] = v
				if e = archiveAttempt(dir, v); e != nil {
					return r.ID, "", e
				}
				if execErr == nil {
					r.Measures[u.ID()] = measures
					r.SummaryDigests[u.ID()] = digest(summary)
				}
				r.State = "waiting"
				if e = save(); e != nil {
					return r.ID, "", e
				}
			}
		}
		if e = publishProgress(); e != nil {
			return r.ID, "", e
		}
	}

	r.State = "awaiting manual work"
	complete := true
	failed := false
	for _, st := range sp.Steps {
		if experiments.ClassifyKind(st.Kind) == experiments.KindManual {
			continue
		}
		for _, u := range sp.Units(st) {
			rec := recs[u.ID()]
			complete = complete && rec.State == experiments.UnitDone
			failed = failed || rec.State == experiments.UnitFailed
		}
	}
	if failed {
		r.State = "failed"
	} else if complete {
		r.State = "complete"
	}
	return r.ID, r.State, publishProgress()
}

func diskGate(h Host, need float64) error {
	for _, p := range []string{h.StageDir, h.EvidenceRoot} {
		var s unix.Statfs_t
		if e := unix.Statfs(p, &s); e != nil {
			return e
		}
		free := float64(s.Bavail) * float64(s.Bsize) / 1e9
		if free < h.DiskFloorGB+need {
			return fmt.Errorf("disk wait at %s: %.2f GB free; need %.2f GB above %.2f GB floor", p, free, need, h.DiskFloorGB)
		}
	}
	return nil
}

func budget(h Host) error {
	// Include unfinished attempts up to their timeout, so restarting cannot reset
	// today's budget. Units spanning midnight charge only today's interval.
	start := time.Now().UTC().Truncate(24 * time.Hour)
	var seconds float64
	err := filepath.WalkDir(filepath.Join(h.EvidenceRoot, "state"), func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() || !strings.HasSuffix(p, ".attempt.json") {
			return nil
		}
		var r experiments.UnitRecord
		if readJSON(p, &r) != nil || r.Attempt == 0 || r.Started.IsZero() {
			return nil
		}
		end := r.Finished
		if end.IsZero() {
			end = time.Now().UTC()
			if !r.Deadline.IsZero() && r.Deadline.Before(end) {
				end = r.Deadline
			}
		}
		if end.After(start) {
			seconds += end.Sub(maxTime(start, r.Started)).Seconds()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if seconds >= h.DailyHours*3600 {
		return fmt.Errorf("daily machine-hour budget reached")
	}
	return nil
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func archiveAttempt(dir string, r experiments.UnitRecord) error {
	return atomicJSON(filepath.Join(dir, "history", filepath.FromSlash(r.ID()), fmt.Sprintf("%d.attempt.json", r.Attempt)), r)
}

type Receipt struct {
	SpecDigest string             `json:"spec_digest"`
	Digest     string             `json:"digest"`
	Measures   map[string]float64 `json:"measures"`
}

type lockContextKey struct{}
