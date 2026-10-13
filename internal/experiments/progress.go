package experiments

// Progress below the spec: where an experiment is in its execution.
//
// A step fans out into units, one per arm and case it runs over; a step
// without over is a single unit. The unit is the chunk of work a worker or a
// person starts and finishes. A step is done when all its units are. A unit
// may start once the units of its predecessor steps that it reads are done:
// the same arm and case where the predecessor fans out over them, all of the
// predecessor where it does not. So a scorecard of one arm on one case can
// run as soon as that replay finishes, without waiting for the other
// replays.
//
// Run state is not authoring state, so it never goes to Git. Until the hub
// exists it is a directory outside the checkout with one JSON file per unit,
// as the job runner keeps one directory per attempt: a file survives a
// stopped process, an operator can read it over SSH, and two lanes writing
// different units never contend. The hub's steps table replaces it.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// StateEnv names the state directory when -state is not given.
const StateEnv = "VELOCITY_EXPERIMENT_STATE"

// DefaultStateDir is $VELOCITY_EXPERIMENT_STATE, or
// ~/.local/state/velocity-report/experiments.
func DefaultStateDir() string {
	if d := os.Getenv(StateEnv); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".experiment-state")
	}
	return filepath.Join(home, ".local", "state", "velocity-report", "experiments")
}

// Unit is one chunk of a step: the step, and the arm and case it runs on
// where the step fans out over them.
type Unit struct {
	Step string `json:"step"`
	Arm  string `json:"arm,omitempty"`
	Case string `json:"case,omitempty"`
}

// ID names the unit: the step, then the arm, then the case, joined by "/"
// (replay/cmr/lombard-laguna). It is also the unit's file under the state
// directory.
func (u Unit) ID() string {
	parts := []string{u.Step}
	if u.Arm != "" {
		parts = append(parts, u.Arm)
	}
	if u.Case != "" {
		parts = append(parts, u.Case)
	}
	return strings.Join(parts, "/")
}

// CaseIDs names the experiment's cases: its sites when listed, otherwise
// case01, case02 and so on to the count.
func (spec *Spec) CaseIDs() []string {
	if spec.Cases == nil {
		return nil
	}
	if len(spec.Cases.Sites) > 0 {
		return spec.Cases.Sites
	}
	ids := make([]string, spec.Cases.Count)
	for i := range ids {
		ids[i] = fmt.Sprintf("case%02d", i+1)
	}
	return ids
}

// Units are a step's units, arm by arm.
func (spec *Spec) Units(st Step) []Unit {
	arms, cases := []string{""}, []string{""}
	if slices.Contains(st.Over, "arms") && len(spec.Arms) > 0 {
		arms = arms[:0]
		for _, a := range spec.Arms {
			arms = append(arms, a.Name)
		}
	}
	if slices.Contains(st.Over, "cases") {
		if ids := spec.CaseIDs(); len(ids) > 0 {
			cases = ids
		}
	}
	var out []Unit
	for _, a := range arms {
		for _, c := range cases {
			out = append(out, Unit{Step: st.ID, Arm: a, Case: c})
		}
	}
	return out
}

// ParseUnit finds the unit an id names.
func (spec *Spec) ParseUnit(id string) (Unit, Step, error) {
	step, _, _ := strings.Cut(id, "/")
	i := spec.stepIndex(step)
	if i < 0 {
		return Unit{}, Step{}, fmt.Errorf("%s has no step %q", spec.ID, step)
	}
	st := spec.Steps[i]
	var names []string
	for _, u := range spec.Units(st) {
		if u.ID() == id {
			return u, st, nil
		}
		names = append(names, u.ID())
	}
	return Unit{}, Step{}, fmt.Errorf("%s has no unit %q; step %s has %s", spec.ID, id, step, strings.Join(names, ", "))
}

func (spec *Spec) stepIndex(id string) int {
	for i, st := range spec.Steps {
		if st.ID == id {
			return i
		}
	}
	return -1
}

// UnitEstimate is one unit's estimate: the step's each, its total shared
// over its units, or a manual step's person time.
func (spec *Spec) UnitEstimate(st Step) time.Duration {
	switch {
	case st.Estimate.Human > 0:
		return time.Duration(st.Estimate.Human)
	case st.Estimate.Each > 0:
		return time.Duration(st.Estimate.Each)
	}
	return time.Duration(st.Estimate.Total) / time.Duration(spec.fanout(st))
}

// UnitState is where a unit is. A unit with no file is pending.
type UnitState string

const (
	UnitPending UnitState = "pending"
	UnitRunning UnitState = "running"
	UnitDone    UnitState = "done"
	UnitFailed  UnitState = "failed"
)

// UnitRecord is the state file of one unit: its latest attempt.
type UnitRecord struct {
	Unit
	State   UnitState `json:"state"`
	Attempt int       `json:"attempt"`
	// Worker is the host that holds or held the unit; Class is its worker
	// class, for the estimate book.
	Worker   string    `json:"worker,omitempty"`
	Class    string    `json:"class,omitempty"`
	Started  time.Time `json:"started,omitzero"`
	Finished time.Time `json:"finished,omitzero"`
	// WallSeconds is the attempt's wall time: finish less start, or what
	// the worker reported for a unit marked done without a start.
	WallSeconds float64 `json:"wall_seconds,omitempty"`
	// Out is where the unit's output is: a local evidence directory or a
	// bundle. It is a pointer for the person writing the report, never a
	// path in Git.
	Out      string `json:"out,omitempty"`
	Artefact string `json:"artefact,omitempty"`
	Note     string `json:"note,omitempty"`
	// Forced is set when an operator overrode a refusal.
	Forced     bool   `json:"forced,omitempty"`
	SpecDigest string `json:"spec_digest,omitempty"`
}

// Wall is the attempt's wall time.
func (r UnitRecord) Wall() time.Duration {
	return time.Duration(r.WallSeconds * float64(time.Second))
}

// SpecDigest is the SHA-256 of a spec's file, recorded with each mark so a
// unit says which version of the spec it ran under.
func SpecDigest(root string, spec *Spec) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(spec.Path)))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Ledger is a state directory: <root>/<slug>/<unit id>.json.
type Ledger struct{ root string }

// OpenLedger opens a state directory; it is created on the first mark.
func OpenLedger(root string) *Ledger { return &Ledger{root: root} }

// Root is the state directory.
func (l *Ledger) Root() string { return l.root }

func (l *Ledger) path(slug string, u Unit) string {
	return filepath.Join(l.root, slug, filepath.FromSlash(u.ID())+".json")
}

// Records reads one experiment's unit files, keyed by unit id. Files for
// units the spec no longer has are returned too, so that a changed spec shows
// its leftovers rather than hiding them.
func (l *Ledger) Records(slug string) (map[string]UnitRecord, error) {
	out := map[string]UnitRecord{}
	dir := filepath.Join(l.root, slug)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") || !strings.HasSuffix(p, ".json") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var r UnitRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		out[r.ID()] = r
		return nil
	})
	return out, err
}

// write replaces a unit's file whole, through a rename, so a reader never
// sees half of it.
func (l *Ledger) write(slug string, r UnitRecord) error {
	p := l.path(slug, r.Unit)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".unit-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Transition is what a worker or a person reports about a unit.
type Transition string

const (
	Start Transition = "start"
	Done  Transition = "done"
	Fail  Transition = "failed"
)

// Mark is one report about one unit.
type Mark struct {
	Unit   Unit
	To     Transition
	Worker string
	Class  string
	// Reclaim is the worker's reclaim_orphans setting: it may take over a
	// unit another worker left running past three times its estimate, where
	// the experiment's policy allows.
	Reclaim bool
	// Force is an operator's override of every refusal.
	Force bool
	Out   string
	// Artefact closes a manual unit: what ManualKinds says the kind closes on.
	Artefact string
	Note     string
	// Wall is the wall time of a unit marked done without a start.
	Wall       time.Duration
	SpecDigest string
	Now        time.Time
}

// ArtefactCheck says whether a manual unit's artefact is real.
type ArtefactCheck func(kind, artefact string) error

// Apply records a mark, refusing what the queue would refuse: any mark on
// an experiment that is not approved, starting a unit whose inputs are not
// done, starting one that is done, taking one
// that another worker holds unless the experiment's reclaim policy and this
// worker's setting allow it, finishing one another worker holds, and
// closing a manual unit without its artefact. Force overrides all of them
// except the artefact.
func (l *Ledger) Apply(spec *Spec, m Mark, check ArtefactCheck) (UnitRecord, error) {
	u, st, err := spec.ParseUnit(m.Unit.ID())
	if err != nil {
		return UnitRecord{}, err
	}
	recs, err := l.Records(spec.ID)
	if err != nil {
		return UnitRecord{}, err
	}
	if spec.Status != StatusApproved && !m.Force {
		return UnitRecord{}, fmt.Errorf("%s is %s: only an approved experiment runs (approve it in its spec)", spec.ID, spec.Status)
	}
	cur, have := recs[u.ID()]
	if !have {
		cur = UnitRecord{Unit: u, State: UnitPending}
	}
	next := cur
	next.Unit = u
	next.Forced = m.Force
	next.SpecDigest = m.SpecDigest
	if m.Note != "" {
		next.Note = m.Note
	}
	if m.Out != "" {
		next.Out = m.Out
	}
	fresh := func() {
		next.Attempt = cur.Attempt + 1
		next.Worker, next.Class = m.Worker, m.Class
		next.Started, next.Finished, next.WallSeconds = time.Time{}, time.Time{}, 0
		next.Artefact = ""
		next.Out, next.Note = m.Out, m.Note
	}
	switch m.To {
	case Start:
		if !m.Force {
			if wait := spec.blockedBy(u, recs); wait != "" {
				return cur, fmt.Errorf("%s waits for %s", u.ID(), wait)
			}
			switch {
			case cur.State == UnitDone:
				return cur, fmt.Errorf("%s is done (attempt %d on %s); -force runs it again", u.ID(), cur.Attempt, cur.Worker)
			case cur.State == UnitRunning && cur.Worker != m.Worker:
				if err := spec.mayReclaim(st, cur, m); err != nil {
					return cur, err
				}
			}
		}
		fresh()
		next.State = UnitRunning
		next.Started = m.Now
	case Done:
		if ClassifyKind(st.Kind) == KindManual {
			if strings.TrimSpace(m.Artefact) == "" {
				return cur, fmt.Errorf("%s is %s, which closes on %s: give -artefact", u.ID(), st.Kind, ManualKinds[st.Kind])
			}
			if check != nil {
				if err := check(st.Kind, m.Artefact); err != nil {
					return cur, err
				}
			}
		}
		if !m.Force {
			switch cur.State {
			case UnitDone:
				return cur, fmt.Errorf("%s is already done", u.ID())
			case UnitRunning:
				if cur.Worker != m.Worker {
					return cur, fmt.Errorf("%s is running on %s, not %s", u.ID(), cur.Worker, m.Worker)
				}
			default:
				if wait := spec.blockedBy(u, recs); wait != "" {
					return cur, fmt.Errorf("%s waits for %s", u.ID(), wait)
				}
			}
		}
		if cur.State == UnitRunning {
			next.WallSeconds = m.Now.Sub(cur.Started).Seconds()
		} else {
			fresh()
			next.WallSeconds = m.Wall.Seconds()
		}
		next.State = UnitDone
		next.Finished = m.Now
		next.Artefact = m.Artefact
	case Fail:
		if !m.Force && cur.State != UnitRunning {
			return cur, fmt.Errorf("%s is %s: only a running unit fails", u.ID(), cur.State)
		}
		if cur.State == UnitRunning {
			next.WallSeconds = m.Now.Sub(cur.Started).Seconds()
		}
		next.State = UnitFailed
		next.Finished = m.Now
	default:
		return cur, fmt.Errorf("mark %q: want start, done or failed", m.To)
	}
	if err := l.write(spec.ID, next); err != nil {
		return cur, err
	}
	return next, nil
}

// blockedBy names the first unit of a predecessor step that u reads and
// that is not done, or "" when none is.
func (spec *Spec) blockedBy(u Unit, recs map[string]UnitRecord) string {
	i := spec.stepIndex(u.Step)
	if i < 0 {
		return ""
	}
	for _, j := range spec.predecessors(i) {
		for _, p := range spec.Units(spec.Steps[j]) {
			reads := (p.Arm == "" || u.Arm == "" || p.Arm == u.Arm) && (p.Case == "" || u.Case == "" || p.Case == u.Case)
			if reads && recs[p.ID()].State != UnitDone {
				return p.ID()
			}
		}
	}
	return ""
}

// Stale reports a running unit past three times its estimate: the orphan
// the plan's sweeper would mark lost.
func (spec *Spec) Stale(st Step, r UnitRecord, now time.Time) bool {
	est := spec.UnitEstimate(st)
	return r.State == UnitRunning && est > 0 && now.Sub(r.Started) > 3*est
}

// mayReclaim decides whether m's worker may take a unit another worker
// holds, by the experiment's reclaim policy and the worker's own setting.
func (spec *Spec) mayReclaim(st Step, cur UnitRecord, m Mark) error {
	held := fmt.Sprintf("%s is running on %s since %s", cur.ID(), cur.Worker, cur.Started.UTC().Format(time.RFC3339))
	switch spec.Reclaim {
	case ReclaimNever:
		return fmt.Errorf("%s, and %s's reclaim policy is never: the operator resolves it (-force)", held, spec.ID)
	case ReclaimOrigin:
		return fmt.Errorf("%s, and %s's reclaim policy is origin: only %s restarts it", held, spec.ID, cur.Worker)
	}
	if !spec.Stale(st, cur, m.Now) {
		return fmt.Errorf("%s, inside three times its estimate", held)
	}
	if !m.Reclaim {
		return fmt.Errorf("%s and is an orphan, but this worker does not reclaim orphans (-reclaim)", held)
	}
	return nil
}

// StepProgress is one step's units by state.
type StepProgress struct {
	Step    Step
	Units   int
	Done    int
	Running int
	Failed  int
	// Stale running units are past three times their estimate.
	Stale int
	// Blocked pending units wait for a predecessor's units.
	Blocked int
	// Wall is the done units' wall time, summed; Left is the estimate of the
	// units not done.
	Wall time.Duration
	Left time.Duration
}

// State names the step's state as the plan's steps table does.
func (p StepProgress) State() string {
	switch {
	case p.Done == p.Units:
		return "done"
	case p.Running > 0:
		return "running"
	case p.Failed > 0:
		return "failed"
	case p.Blocked == p.Units-p.Done:
		return "blocked"
	case ClassifyKind(p.Step.Kind) == KindManual:
		return "awaiting a person"
	}
	return "ready"
}

// Progress is an experiment's units by state, and the time it has left.
type Progress struct {
	Steps []StepProgress
	Units int
	Done  int
	// MachineLeft is the estimate of the machine units not done, along the
	// longest chain of steps; HumanLeft the person time of the manual units
	// not done.
	MachineLeft time.Duration
	HumanLeft   time.Duration
	// MachineWall and MachineEstimate are the done machine units' wall time
	// and their estimate, for Ratio.
	MachineWall     time.Duration
	MachineEstimate time.Duration
	// Extra are unit files the spec no longer has.
	Extra []string
}

// Ratio is how the done machine units ran against their estimate (1.5 is
// half as long again), or 0 when none has a wall time.
func (p Progress) Ratio() float64 {
	if p.MachineEstimate <= 0 || p.MachineWall <= 0 {
		return 0
	}
	return float64(p.MachineWall) / float64(p.MachineEstimate)
}

// Progress folds an experiment's unit records into its steps.
func (spec *Spec) Progress(recs map[string]UnitRecord, now time.Time) Progress {
	var p Progress
	known := map[string]bool{}
	finish := make([]time.Duration, len(spec.Steps))
	for i, st := range spec.Steps {
		sp := StepProgress{Step: st}
		est := spec.UnitEstimate(st)
		manual := ClassifyKind(st.Kind) == KindManual
		for _, u := range spec.Units(st) {
			known[u.ID()] = true
			sp.Units++
			r, ok := recs[u.ID()]
			switch {
			case ok && r.State == UnitDone:
				sp.Done++
				sp.Wall += r.Wall()
				if !manual && r.WallSeconds > 0 {
					p.MachineWall += r.Wall()
					p.MachineEstimate += est
				}
				continue
			case ok && r.State == UnitRunning:
				sp.Running++
				if spec.Stale(st, r, now) {
					sp.Stale++
				}
			case ok && r.State == UnitFailed:
				sp.Failed++
			default:
				if spec.blockedBy(u, recs) != "" {
					sp.Blocked++
				}
			}
			sp.Left += est
		}
		var start time.Duration
		for _, j := range spec.predecessors(i) {
			start = max(start, finish[j])
		}
		finish[i] = start
		if manual {
			p.HumanLeft += sp.Left
		} else {
			finish[i] += sp.Left
		}
		p.MachineLeft = max(p.MachineLeft, finish[i])
		p.Units += sp.Units
		p.Done += sp.Done
		p.Steps = append(p.Steps, sp)
	}
	for id := range recs {
		if !known[id] {
			p.Extra = append(p.Extra, id)
		}
	}
	sort.Strings(p.Extra)
	return p
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// RepoArtefacts checks manual artefacts against a checkout: a commit in the
// repository for manual:implement, a decision present in docs/DECISIONS.md
// for manual:decide, and the spec's report on disk for manual:write. The
// other manual kinds close on things this host cannot see yet (a capture, a
// pack digest, a frozen split) and are taken as recorded until the hub
// checks them.
func RepoArtefacts(root string, spec *Spec) ArtefactCheck {
	return func(kind, a string) error {
		switch kind {
		case "manual:implement":
			if !commitPattern.MatchString(a) {
				return fmt.Errorf("manual:implement closes on a commit: %q is not a commit id", a)
			}
			if err := exec.Command("git", "-C", root, "cat-file", "-e", a+"^{commit}").Run(); err != nil {
				return fmt.Errorf("commit %s is not in the repository at %s", a, root)
			}
		case "manual:decide":
			if !decisionPattern.MatchString(a) {
				return fmt.Errorf("manual:decide closes on a decision id such as D-28, not %q", a)
			}
			raw, err := os.ReadFile(filepath.Join(root, "docs", "DECISIONS.md"))
			if err != nil {
				return err
			}
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(a) + `\b`).Match(raw) {
				return fmt.Errorf("%s is not in docs/DECISIONS.md", a)
			}
		case "manual:write":
			if spec.Report != "" && a != spec.Report {
				return fmt.Errorf("manual:write closes on the spec's report, %s, not %s", spec.Report, a)
			}
			if !exists(root, a) {
				return fmt.Errorf("report %s does not exist", a)
			}
		}
		return nil
	}
}
