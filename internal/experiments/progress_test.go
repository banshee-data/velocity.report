package experiments

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// progressSpec fans a replay out over two arms and two listed sites, scores
// each replay, then waits for a person to write it up.
const progressSpec = `---
id: abcd0002
title: Progress
status: approved
owner: t
created: 2026-10-13
asks: [docs/question.md]
report: docs/report.md
reclaim: any
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: b
    tuning: config/experiments/set.json
cases:
  corpus: kirk-labelled
  count: 2
  sites: [kirk0, kirk1]
steps:
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 10m }
  - id: card
    kind: track_scorecard
    over: [arms, cases]
    estimate: { each: 1m }
  - id: chart
    kind: chart
    estimate: { total: 4m }
  - id: write
    kind: manual:write
    estimate: { human: 2h }
---
`

func progressFixture(t *testing.T, edit func(string) string) (*Spec, *Ledger) {
	t.Helper()
	spec := progressSpec
	if edit != nil {
		spec = edit(spec)
	}
	root := tree(t, map[string]string{"data/experiments/abcd/abcd0002.md": spec})
	set := load(t, root)
	for _, e := range set.Validate(root) {
		t.Fatal(e)
	}
	return set.Get("abcd0002"), OpenLedger(filepath.Join(t.TempDir(), "state"))
}

var t0 = time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)

func apply(t *testing.T, l *Ledger, spec *Spec, id string, to Transition, worker string, at time.Duration, opts ...func(*Mark)) (UnitRecord, error) {
	t.Helper()
	u, _, err := spec.ParseUnit(id)
	if err != nil {
		t.Fatal(err)
	}
	m := Mark{Unit: u, To: to, Worker: worker, Now: t0.Add(at)}
	for _, o := range opts {
		o(&m)
	}
	return l.Apply(spec, m, nil)
}

func mustApply(t *testing.T, l *Ledger, spec *Spec, id string, to Transition, worker string, at time.Duration, opts ...func(*Mark)) UnitRecord {
	t.Helper()
	r, err := apply(t, l, spec, id, to, worker, at, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestUnitsFanOutOverArmsAndCases(t *testing.T) {
	spec, _ := progressFixture(t, nil)
	var ids []string
	for _, st := range spec.Steps {
		for _, u := range spec.Units(st) {
			ids = append(ids, u.ID())
		}
	}
	want := "replay/a0/kirk0 replay/a0/kirk1 replay/b/kirk0 replay/b/kirk1 card/a0/kirk0 card/a0/kirk1 card/b/kirk0 card/b/kirk1 chart write"
	if got := strings.Join(ids, " "); got != want {
		t.Fatalf("units\n got %s\nwant %s", got, want)
	}
	// Without sites, cases are numbered.
	spec, _ = progressFixture(t, func(s string) string { return strings.Replace(s, "  sites: [kirk0, kirk1]\n", "", 1) })
	if got := spec.Units(spec.Steps[0])[1].ID(); got != "replay/a0/case02" {
		t.Errorf("unnamed case: %s", got)
	}
	if _, _, err := spec.ParseUnit("replay/a0/case03"); err == nil {
		t.Error("ParseUnit accepted a case the spec does not have")
	}
	if _, _, err := spec.ParseUnit("nostep"); err == nil {
		t.Error("ParseUnit accepted a step the spec does not have")
	}
	if got := spec.UnitEstimate(spec.Steps[2]); got != 4*time.Minute {
		t.Errorf("a total step's one unit: %v", got)
	}
}

func TestAUnitWaitsOnlyForTheUnitsItReads(t *testing.T) {
	spec, l := progressFixture(t, nil)
	if _, err := apply(t, l, spec, "card/a0/kirk0", Start, "m", 0); err == nil || !strings.Contains(err.Error(), "waits for replay/a0/kirk0") {
		t.Fatalf("card before its replay: %v", err)
	}
	mustApply(t, l, spec, "replay/a0/kirk0", Start, "m", 0)
	r := mustApply(t, l, spec, "replay/a0/kirk0", Done, "m", 12*time.Minute, func(m *Mark) { m.Out = "/ev/a0" })
	if r.Wall() != 12*time.Minute || r.Out != "/ev/a0" || r.Attempt != 1 {
		t.Fatalf("done record: %+v", r)
	}
	// The scorecard of the same arm and case may start; the chart, which reads
	// every scorecard, may not.
	mustApply(t, l, spec, "card/a0/kirk0", Start, "m", 13*time.Minute)
	if _, err := apply(t, l, spec, "card/b/kirk0", Start, "m", 13*time.Minute); err == nil {
		t.Error("card/b/kirk0 started before replay/b/kirk0")
	}
	if _, err := apply(t, l, spec, "chart", Start, "m", 13*time.Minute); err == nil {
		t.Error("chart started before every scorecard")
	}
}

func TestStartDoneAndFailRules(t *testing.T) {
	spec, l := progressFixture(t, nil)
	if _, err := apply(t, l, spec, "replay/a0/kirk0", Done, "m", 0); err != nil {
		t.Fatalf("done without a start is a backfill: %v", err)
	}
	if _, err := apply(t, l, spec, "replay/a0/kirk0", Start, "m", 0); err == nil || !strings.Contains(err.Error(), "-force") {
		t.Errorf("restarting a done unit: %v", err)
	}
	if _, err := apply(t, l, spec, "replay/a0/kirk0", Done, "m", 0); err == nil {
		t.Error("done twice")
	}
	r := mustApply(t, l, spec, "replay/a0/kirk0", Start, "m", time.Minute, func(m *Mark) { m.Force = true; m.Note = "rerun" })
	if r.Attempt != 2 || !r.Forced || r.State != UnitRunning || r.Note != "rerun" {
		t.Errorf("forced rerun: %+v", r)
	}
	if _, err := apply(t, l, spec, "replay/b/kirk0", Fail, "m", 0); err == nil {
		t.Error("a pending unit failed")
	}
	mustApply(t, l, spec, "replay/b/kirk0", Start, "m", 0)
	if _, err := apply(t, l, spec, "replay/b/kirk0", Done, "other", time.Minute); err == nil {
		t.Error("another worker finished a unit it does not hold")
	}
	f := mustApply(t, l, spec, "replay/b/kirk0", Fail, "m", 3*time.Minute, func(m *Mark) { m.Note = "exit 1" })
	if f.State != UnitFailed || f.Wall() != 3*time.Minute || f.Note != "exit 1" {
		t.Errorf("failed record: %+v", f)
	}
	// A failed unit starts again as a new attempt.
	if r := mustApply(t, l, spec, "replay/b/kirk0", Start, "m", 4*time.Minute); r.Attempt != 2 || r.Note != "" {
		t.Errorf("retry after failure: %+v", r)
	}
	if _, err := apply(t, l, spec, "replay/b/kirk1", Transition("paused"), "m", 0); err == nil {
		t.Error("unknown transition accepted")
	}
}

func TestOnlyAnApprovedExperimentRuns(t *testing.T) {
	spec, l := progressFixture(t, func(s string) string { return strings.Replace(s, "status: approved", "status: proposed", 1) })
	if _, err := apply(t, l, spec, "replay/a0/kirk0", Start, "m", 0); err == nil || !strings.Contains(err.Error(), "approved") {
		t.Fatalf("proposed experiment ran: %v", err)
	}
	mustApply(t, l, spec, "replay/a0/kirk0", Start, "m", 0, func(m *Mark) { m.Force = true })
}

func TestReclaimFollowsTheExperimentPolicyAndTheWorker(t *testing.T) {
	for _, tc := range []struct {
		policy  string
		at      time.Duration
		reclaim bool
		ok      bool
		want    string
	}{
		{"any", 20 * time.Minute, true, false, "inside three times"},
		{"any", 31 * time.Minute, false, false, "does not reclaim"},
		{"any", 31 * time.Minute, true, true, ""},
		{"origin", 31 * time.Minute, true, false, "only m restarts it"},
		{"never", 31 * time.Minute, true, false, "operator"},
	} {
		spec, l := progressFixture(t, func(s string) string { return strings.Replace(s, "reclaim: any", "reclaim: "+tc.policy, 1) })
		mustApply(t, l, spec, "replay/a0/kirk0", Start, "m", 0)
		r, err := apply(t, l, spec, "replay/a0/kirk0", Start, "swan", tc.at, func(m *Mark) { m.Reclaim = tc.reclaim })
		if tc.ok {
			if err != nil || r.Worker != "swan" || r.Attempt != 2 {
				t.Errorf("%s: reclaim refused: %v %+v", tc.policy, err, r)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s at %v: got %v, want %q", tc.policy, tc.at, err, tc.want)
		}
		// The worker that started it may always restart it.
		if _, err := apply(t, l, spec, "replay/a0/kirk0", Start, "m", tc.at); err != nil {
			t.Errorf("%s: origin restart refused: %v", tc.policy, err)
		}
	}
}

func TestManualUnitsCloseOnTheirArtefact(t *testing.T) {
	spec, l := progressFixture(t, func(s string) string {
		return strings.Replace(s, "steps:\n", "steps:\n  - id: decide\n    kind: manual:decide\n    after: [\"-\"]\n    estimate: { human: 1h }\n", 1)
	})
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "DECISIONS.md"), []byte("| D-28 | eps |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	check := RepoArtefacts(root, spec)
	u, _, _ := spec.ParseUnit("decide")
	if _, err := l.Apply(spec, Mark{Unit: u, To: Done, Worker: "david", Now: t0}, check); err == nil || !strings.Contains(err.Error(), "-artefact") {
		t.Errorf("manual done without an artefact: %v", err)
	}
	for _, bad := range []string{"28", "D-2", "D-29"} {
		if _, err := l.Apply(spec, Mark{Unit: u, To: Done, Worker: "david", Artefact: bad, Now: t0}, check); err == nil {
			t.Errorf("decide closed on %q", bad)
		}
	}
	r, err := l.Apply(spec, Mark{Unit: u, To: Done, Worker: "david", Artefact: "D-28", Wall: 40 * time.Minute, Now: t0}, check)
	if err != nil || r.Artefact != "D-28" || r.Wall() != 40*time.Minute {
		t.Fatalf("decide: %v %+v", err, r)
	}
	if err := check("manual:write", "docs/other.md"); err == nil {
		t.Error("write closed on a file that is not the spec's report")
	}
	if err := check("manual:write", "docs/report.md"); err == nil {
		t.Error("write closed on a report that does not exist")
	}
	if err := check("manual:implement", "not-a-sha"); err == nil {
		t.Error("implement closed on something that is not a commit id")
	}
	if err := check("manual:implement", "0123456789abcdef"); err == nil {
		t.Error("implement closed on a commit outside the repository")
	}
	if err := check("manual:label", "anything"); err != nil {
		t.Errorf("labels are taken as recorded: %v", err)
	}
}

func TestImplementClosesOnACommitInTheRepository(t *testing.T) {
	root := filepath.Join("..", "..")
	head, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Skip("no git checkout:", err)
	}
	if err := RepoArtefacts(root, &Spec{})("manual:implement", strings.TrimSpace(string(head))); err != nil {
		t.Fatal(err)
	}
}

func TestProgressFoldsUnitsIntoStepsAndTimeLeft(t *testing.T) {
	spec, l := progressFixture(t, nil)
	recs, _ := l.Records(spec.ID)
	p := spec.Progress(recs, t0)
	// Nothing done: 10m on the longest replay chain (each unit's estimate,
	// four units in one step, so 40m), then 4m of scorecards, then the chart.
	if p.Units != 10 || p.Done != 0 || p.MachineLeft != 48*time.Minute || p.HumanLeft != 2*time.Hour || p.Ratio() != 0 {
		t.Fatalf("fresh: %+v", p)
	}
	if got := []string{p.Steps[0].State(), p.Steps[1].State(), p.Steps[3].State()}; strings.Join(got, ",") != "ready,blocked,blocked" {
		t.Errorf("fresh states %v", got)
	}

	// Two replays done at twice their estimate, one running past three times
	// its estimate, one failed.
	for i, id := range []string{"replay/a0/kirk0", "replay/a0/kirk1"} {
		at := time.Duration(i) * 20 * time.Minute
		mustApply(t, l, spec, id, Start, "m", at)
		mustApply(t, l, spec, id, Done, "m", at+20*time.Minute)
	}
	mustApply(t, l, spec, "replay/b/kirk0", Start, "m", 0)
	mustApply(t, l, spec, "replay/b/kirk1", Start, "m", 0)
	mustApply(t, l, spec, "replay/b/kirk1", Fail, "m", time.Minute)
	recs, _ = l.Records(spec.ID)
	p = spec.Progress(recs, t0.Add(40*time.Minute))
	rp := p.Steps[0]
	if rp.Done != 2 || rp.Running != 1 || rp.Stale != 1 || rp.Failed != 1 || rp.Wall != 40*time.Minute || rp.State() != "running" {
		t.Errorf("replay step: %+v %s", rp, rp.State())
	}
	if p.Ratio() != 2 || p.MachineLeft != 20*time.Minute+4*time.Minute+4*time.Minute {
		t.Errorf("ratio %.2f, machine left %v", p.Ratio(), p.MachineLeft)
	}
	// The scorecards of the done replays are ready, so the step is not blocked.
	if s := p.Steps[1]; s.Blocked != 2 || s.State() != "ready" {
		t.Errorf("card step: %+v %s", s, s.State())
	}

	mustApply(t, l, spec, "replay/b/kirk0", Fail, "m", 41*time.Minute)
	recs, _ = l.Records(spec.ID)
	if s := spec.Progress(recs, t0).Steps[0]; s.State() != "failed" {
		t.Errorf("a step with failures and nothing running: %s", s.State())
	}

	// Units the spec no longer has are reported, not hidden.
	spec2, _ := progressFixture(t, func(s string) string {
		return strings.Replace(s, "  - name: b\n    tuning: config/experiments/set.json\n", "", 1)
	})
	p = spec2.Progress(recs, t0)
	if strings.Join(p.Extra, ",") != "replay/b/kirk0,replay/b/kirk1" {
		t.Errorf("extra units: %v", p.Extra)
	}
}

func TestManualStepWaitingForAPerson(t *testing.T) {
	spec, l := progressFixture(t, func(s string) string {
		return strings.Replace(s, "    kind: manual:write\n", "    kind: manual:write\n    after: [\"-\"]\n", 1)
	})
	recs, _ := l.Records(spec.ID)
	if s := spec.Progress(recs, t0).Steps[3]; s.State() != "awaiting a person" {
		t.Errorf("manual step with nothing before it: %s", s.State())
	}
	mustApply(t, l, spec, "write", Start, "david", 0)
	recs, _ = l.Records(spec.ID)
	// A person's unit is stale only past three times the person estimate.
	if s := spec.Progress(recs, t0.Add(5*time.Hour)).Steps[3]; s.Stale != 0 || s.State() != "running" {
		t.Errorf("manual unit at 5h: %+v", s)
	}
}

func TestLedgerFilesAndDigest(t *testing.T) {
	spec, l := progressFixture(t, nil)
	mustApply(t, l, spec, "chart", Start, "m", 0, func(m *Mark) { m.Force = true; m.SpecDigest = "sha256:x" })
	p := filepath.Join(l.Root(), "abcd0002", "chart.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"spec_digest": "sha256:x"`) || !strings.Contains(string(raw), `"forced": true`) {
		t.Errorf("unit file:\n%s", raw)
	}
	// Temporary and foreign files are not units; a corrupt unit file is an error.
	_ = os.WriteFile(filepath.Join(l.Root(), "abcd0002", ".unit-123"), []byte("{"), 0o644)
	_ = os.WriteFile(filepath.Join(l.Root(), "abcd0002", "notes.txt"), []byte("x"), 0o644)
	if recs, err := l.Records(spec.ID); err != nil || len(recs) != 1 {
		t.Errorf("records: %v %v", recs, err)
	}
	_ = os.WriteFile(filepath.Join(l.Root(), "abcd0002", "write.json"), []byte("{"), 0o644)
	if _, err := l.Records(spec.ID); err == nil {
		t.Error("corrupt unit file read")
	}
	if recs, err := l.Records("nothing"); err != nil || len(recs) != 0 {
		t.Errorf("no state yet: %v %v", recs, err)
	}

	root := filepath.Join("..", "..")
	set := load(t, root)
	sp := set.Specs[0]
	d, err := SpecDigest(root, sp)
	if err != nil || !strings.HasPrefix(d, "sha256:") || len(d) != len("sha256:")+64 {
		t.Errorf("digest %q %v", d, err)
	}
	if _, err := SpecDigest(t.TempDir(), sp); err == nil {
		t.Error("digest of a missing file")
	}
}

func TestDefaultStateDir(t *testing.T) {
	t.Setenv(StateEnv, "/x/state")
	if DefaultStateDir() != "/x/state" {
		t.Error("env not honoured")
	}
	t.Setenv(StateEnv, "")
	if !strings.HasSuffix(DefaultStateDir(), filepath.Join("velocity-report", "experiments")) {
		t.Errorf("default %s", DefaultStateDir())
	}
}
