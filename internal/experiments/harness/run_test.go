package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/experiments"
	"golang.org/x/sys/unix"
)

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(body), 0755); e != nil {
		t.Fatal(e)
	}
}
func gitTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	s, e := git(context.Background(), root, args...)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func fixture(t *testing.T) (Options, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	os.MkdirAll(root, 0755)
	gitTest(t, root, "init", "-b", "main")
	gitTest(t, root, "config", "user.email", "test@example.invalid")
	gitTest(t, root, "config", "user.name", "Test")
	defaults, e := os.ReadFile("../../../config/tuning.defaults.json")
	if e != nil {
		t.Fatal(e)
	}
	put(t, root, "config/tuning.defaults.json", string(defaults))
	fp, e := experiments.TuningFingerprint(root, "config/tuning.defaults.json")
	if e != nil {
		t.Fatal(e)
	}
	put(t, root, "VERSION", "0.5.2\n")
	put(t, root, "docs/question.md", "# Question\n")
	put(t, root, "data/experiments/host/README.md", "---\narea: host\ntitle: Test\nowner: test\nlayers: all\n---\n")
	spec := fmt.Sprintf(`---
id: host0001
title: Offline fixture
status: approved
owner: test
created: 2026-11-09
asks: [docs/question.md]
arms:
 - name: a0
   tuning: config/tuning.defaults.json
   fingerprint: %s
cases: {corpus: kirk0, count: 1, sites: [kirk0]}
steps:
 - id: replay
   kind: state_estimation_baseline
   over: [arms, cases]
   estimate: {each: 1s, worker: arrow}
---
# Rule
`, fp)
	put(t, root, "data/experiments/host/host0001.md", spec)
	put(t, root, "corpus.json", `{"cases":[{"id":"kirk0","expected_capture_count":1}]}`)
	put(t, root, "index.json", `[{"id":"kirk0","captures":["kirk0.pcapng"]}]`)
	gitTest(t, root, "add", ".")
	gitTest(t, root, "commit", "-m", "fixture")
	h := Host{Worker: "arrow-worker", Class: "arrow", Cores: 8, EvidenceRoot: filepath.Join(base, "evidence"), StageDir: filepath.Join(base, "stage"), CaptureRoot: filepath.Join(base, "captures"), DailyHours: 24, Reclaim: true, Corpora: map[string]Corpus{"kirk0": {Corpus: "corpus.json", Index: "index.json"}}}
	put(t, h.CaptureRoot, "kirk0.pcapng", "fixture capture")
	bin := filepath.Join(base, "bin")
	put(t, bin, "lidar-state-estimation-baseline", `#!/bin/sh
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-out" ]; then shift; out="$1"; fi
 shift
done
mkdir -p "$out"
printf '%s\n' '{"cases":[{"first_run_frames":20,"duration_seconds":2,"continuity":{"tracks_confirmed":3}}]}' > "$out/phase0-summary.json"
`)
	o := Options{Root: root, Host: h, buildTools: func(context.Context, string, string, string) (string, string, error) { return root, bin, nil }}
	return o, bin
}
func activeRun(t *testing.T, o Options) Run {
	t.Helper()
	var r Run
	if e := readJSON(filepath.Join(o.Host.EvidenceRoot, "state/host0001/active.json"), &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func ledgerFor(o Options, r Run) *experiments.Ledger {
	return experiments.OpenLedger(filepath.Join(o.Host.EvidenceRoot, "state", r.Slug, r.ID, "units"))
}

func TestOfflineResumeAndGitCheckin(t *testing.T) {
	o, _ := fixture(t)
	ctx := context.Background()
	st, e := Tick(ctx, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(st.Runs) != 1 {
		t.Fatalf("%+v", st)
	}
	r := activeRun(t, o)
	if r.State != "complete" {
		t.Fatalf("%+v", r)
	}
	notes, _ := os.ReadFile(filepath.Join(o.Host.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
	head, e := CheckNote(notes)
	if e != nil || !head.Complete {
		t.Fatalf("note: %+v %v", head, e)
	}
	// A new check-in that changes no execution input resumes the original pin.
	put(t, o.Root, "new-document.md", "checked in after the run\n")
	if _, e = Tick(ctx, o); e == nil || !strings.Contains(e.Error(), "uncommitted") {
		t.Fatalf("dirty checkout accepted: %v", e)
	}
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "new source check-in")
	if _, e = Tick(ctx, o); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(o.Host.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
	if string(after) != string(notes) {
		t.Fatal("unchanged completed note was rewritten, which would reset publishing checks")
	}
	r2 := activeRun(t, o)
	if r2.ID != r.ID || r2.Commit != r.Commit {
		t.Fatal("unchanged inputs lost pinned run")
	}
	recs, e := ledgerFor(o, r).Records(r.Slug)
	if e != nil {
		t.Fatal(e)
	}
	for _, rec := range recs {
		if rec.Attempt != 1 {
			t.Fatalf("done unit repeated: %+v", rec)
		}
	}
	// A committed spec change supersedes old work and starts a fresh pin.
	p := filepath.Join(o.Root, "data/experiments/host/host0001.md")
	b, _ := os.ReadFile(p)
	put(t, o.Root, "data/experiments/host/host0001.md", strings.Replace(string(b), "id: replay", "id: replay2", 1)+"Changed description.\n")
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "restate spec")
	if _, e = Tick(ctx, o); e != nil {
		t.Fatal(e)
	}
	oldNote, _ := os.ReadFile(filepath.Join(o.Host.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
	if strings.Contains(string(oldNote), "replay2/") {
		t.Fatal("superseded note was rendered against the new spec")
	}
	r3 := activeRun(t, o)
	if r3.ID == r.ID || r3.Commit == r.Commit {
		t.Fatal("spec change did not supersede run")
	}
}

func TestFailuresHaveCappedRetriesAndExplicitRecovery(t *testing.T) {
	o, bin := fixture(t)
	put(t, bin, "lidar-state-estimation-baseline", "#!/bin/sh\necho broken >&2\nexit 1\n")
	if _, e := Tick(context.Background(), o); e != nil {
		t.Fatal(e)
	}
	r := activeRun(t, o)
	if r.State != "failed" {
		t.Fatalf("%+v", r)
	}
	recs, _ := ledgerFor(o, r).Records(r.Slug)
	u := experiments.Unit{Step: "replay", Arm: "a0", Case: "kirk0"}
	if recs[u.ID()].Attempt != 3 {
		t.Fatalf("%+v", recs)
	}
	if _, e := Tick(context.Background(), o); e != nil {
		t.Fatal(e)
	}
	recs, _ = ledgerFor(o, r).Records(r.Slug)
	if recs[u.ID()].Attempt != 3 {
		t.Fatal("retry cap reset on restart")
	}
	put(t, bin, "lidar-state-estimation-baseline", `#!/bin/sh
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-out" ]; then shift; out="$1"; fi
 shift
done
mkdir -p "$out"
printf '%s\n' '{"cases":[{"first_run_frames":10,"duration_seconds":1}]}' > "$out/phase0-summary.json"
`)
	o.RetryFailed = true
	if _, e := Tick(context.Background(), o); e != nil {
		t.Fatal(e)
	}
	recs, _ = ledgerFor(o, r).Records(r.Slug)
	if recs[u.ID()].State != experiments.UnitDone || recs[u.ID()].Attempt != 4 {
		t.Fatalf("failed recovery: %+v", recs[u.ID()])
	}
	for _, n := range []int{1, 2, 3, 4} {
		path := filepath.Join(o.Host.EvidenceRoot, "runs", r.Slug, r.ID, filepath.FromSlash(u.ID()), fmt.Sprintf("attempt-%d", n), "tool.log")
		if _, e := os.Stat(path); e != nil {
			t.Fatal("lost previous attempt", e)
		}
	}
}

func TestInterruptedRunResumesAndCorruptEvidenceRefuses(t *testing.T) {
	o, _ := fixture(t)
	ctx := context.Background()
	if _, e := Tick(ctx, o); e != nil {
		t.Fatal(e)
	}
	r := activeRun(t, o)
	l := ledgerFor(o, r)
	set, e := experiments.Load(o.Root)
	if e != nil {
		t.Fatal(e)
	}
	sp := set.Get(r.Slug)
	u := experiments.Unit{Step: "replay", Arm: "a0", Case: "kirk0"}
	// Simulate a killed process after its durable start, before completion.
	_, e = l.Apply(sp, experiments.Mark{Unit: u, To: experiments.Start, Force: true, Worker: o.Host.Worker, Class: o.Host.Class, Now: time.Now().UTC(), SpecDigest: r.SpecDigest}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Tick(ctx, o); e != nil {
		t.Fatal(e)
	}
	recs, _ := l.Records(r.Slug)
	if recs[u.ID()].Attempt != 3 || recs[u.ID()].State != experiments.UnitDone {
		t.Fatalf("interrupted unit not recovered: %+v", recs)
	}
	put(t, recs[u.ID()].Out, "tool-summary.json", "corrupt")
	// An invalid key records a publication error if publication is attempted.
	o.Host.Publish = &Publication{PrivateKey: filepath.Join(t.TempDir(), "missing-key")}
	before := activeRun(t, o).Updated
	if _, e = Tick(ctx, o); e == nil || !strings.Contains(e.Error(), "changed evidence") {
		t.Fatalf("corrupt evidence accepted: %v", e)
	}
	after := activeRun(t, o)
	if after.PublishError != "" || !after.Updated.Equal(before) {
		t.Fatal("corrupt run was saved or published before evidence validation")
	}
}

func TestSkipUnsupportedMajorAndStaleFingerprint(t *testing.T) {
	o, _ := fixture(t)
	put(t, o.Root, "data/experiments/host/host0002.md", "---\nspec_version: 2.1\nfuture_contract: true\n---\n")
	put(t, o.Root, "data/experiments/host/host0003.md", "---\nid: host0003\nstatus: approved\nunknown_field: stale\n---\n")
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "future specs")
	o.DryRun = true
	st, e := Tick(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if len(st.Runs) != 1 || len(st.Skipped) != 2 {
		t.Fatalf("%+v", st)
	}
	p := filepath.Join(o.Root, "data/experiments/host/host0001.md")
	b, _ := os.ReadFile(p)
	b = []byte(strings.Replace(string(b), "fingerprint: ", "fingerprint: 000000000000 # ", 1))
	put(t, o.Root, "data/experiments/host/host0001.md", string(b))
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "drift")
	st, e = Tick(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if len(st.Runs) != 0 || !strings.Contains(st.Skipped["host0001"], "changed under") {
		t.Fatalf("drift not skipped: %+v", st)
	}
}

func TestLockPauseAndDiskGate(t *testing.T) {
	o, _ := fixture(t)
	dir := filepath.Join(o.Host.EvidenceRoot, "state")
	os.MkdirAll(dir, 0755)
	lock, e := os.OpenFile(filepath.Join(dir, "tick.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	st, e := Tick(context.Background(), o)
	if e != nil || st.State != "busy" {
		t.Fatalf("%+v %v", st, e)
	}
	unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	put(t, dir, "PAUSE", "")
	st, e = Tick(context.Background(), o)
	if e != nil || st.State != "paused" {
		t.Fatalf("%+v %v", st, e)
	}
	os.Remove(filepath.Join(dir, "PAUSE"))
	o.Host.DiskFloorGB = 1e12
	st, e = Tick(context.Background(), o)
	if e != nil || !strings.Contains(st.Skipped["host0001"], "disk wait") {
		t.Fatalf("%+v %v", st, e)
	}
}

func TestToolTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "tool", "#!/bin/sh\nsleep 30 &\nwait\n")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	e := command(ctx, dir, filepath.Join(dir, "tool"), dir, nil)
	if e == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("timeout: %v (%s)", e, time.Since(start))
	}
}

func TestBuildTimeoutKillsDescendants(t *testing.T) {
	o, _ := fixture(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "orphan")
	put(t, dir, "make", "#!/bin/sh\n(sleep 0.3; touch \"$BUILD_ORPHAN_MARKER\") &\nwait\n")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BUILD_ORPHAN_MARKER", marker)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	sha := gitTest(t, o.Root, "rev-parse", "HEAD")
	if _, _, err := build(ctx, o.Root, o.Host.EvidenceRoot, sha); err == nil {
		t.Fatal("timed-out build succeeded")
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("build descendant survived cancellation")
	}
}

func TestGuardRejectsMutatedNotesAndHarnessSourceChanges(t *testing.T) {
	o, _ := fixture(t)
	if _, e := Tick(context.Background(), o); e != nil {
		t.Fatal(e)
	}
	r := activeRun(t, o)
	note, _ := os.ReadFile(filepath.Join(o.Host.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
	path := "data/experiments/runs/" + r.Slug + "/" + r.ID + ".md"
	base := gitTest(t, o.Root, "rev-parse", "HEAD")
	put(t, o.Root, path, string(note))
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "note")
	if e := GuardNotes(context.Background(), o.Root, base, true); e != nil {
		t.Fatal(e)
	}
	base = gitTest(t, o.Root, "rev-parse", "HEAD")
	put(t, o.Root, path, string(note)+"changed\n")
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "edit note")
	if e := GuardNotes(context.Background(), o.Root, base, false); e == nil {
		t.Fatal("edited note accepted")
	}
	// New source on a harness branch is forbidden, even with no note changes.
	base = gitTest(t, o.Root, "rev-parse", "HEAD")
	put(t, o.Root, "source.go", "package source\n")
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "source")
	if e := GuardNotes(context.Background(), o.Root, base, true); e == nil {
		t.Fatal("harness source change accepted")
	}
	if _, e := CheckNote(make([]byte, NoteLimit+1)); e == nil {
		t.Fatal("oversize note accepted")
	}
}

func TestBudgetCountsEveryAttempt(t *testing.T) {
	o, _ := fixture(t)
	dir := filepath.Join(o.Host.EvidenceRoot, "state/host0001/run")
	now := time.Now().UTC()
	for i := 1; i <= 3; i++ {
		if e := archiveAttempt(dir, experiments.UnitRecord{Unit: experiments.Unit{Step: "x"}, State: experiments.UnitFailed, Attempt: i, Started: now.Add(-time.Duration(i) * time.Second), Finished: now}); e != nil {
			t.Fatal(e)
		}
	}
	o.Host.DailyHours = 0.0001
	if e := budget(o.Host); e == nil {
		t.Fatal("attempt history omitted from budget")
	}
}

func TestReplayKeepsHostLockAfterParentDescriptorCloses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "host.lock")
	lock, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if e = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		t.Fatal(e)
	}
	put(t, dir, "tool", "#!/bin/sh\ntouch started\nsleep 20\n")
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), lockContextKey{}, lock))
	defer cancel()
	c := toolCommand(ctx, dir, filepath.Join(dir, "tool"), nil)
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	// Closing the parent's descriptor models SIGKILL; no explicit unlock runs.
	lock.Close()
	probe, e := os.OpenFile(path, os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer probe.Close()
	if e = unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB); e == nil {
		t.Fatal("surviving replay did not retain host lock")
	}
	cancel()
	if e = c.Wait(); e == nil {
		t.Fatal("cancelled tool unexpectedly succeeded")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		e = unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lock did not release after descendants exited: %v", e)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestManualClosureGatesNewClaims(t *testing.T) {
	o, _ := fixture(t)
	p := filepath.Join(o.Root, "data/experiments/host/host0001.md")
	b, _ := os.ReadFile(p)
	body := strings.Replace(string(b), "steps:\n", "steps:\n - id: implement\n   kind: manual:implement\n   estimate: {human: 1m}\n", 1)
	put(t, o.Root, "data/experiments/host/host0001.md", body)
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "manual predecessor")
	o.DryRun = true
	st, e := Tick(context.Background(), o)
	if e != nil || len(st.Runs) != 0 || !strings.Contains(st.Skipped["host0001"], "manual closure") {
		t.Fatalf("unclosed manual claimed: %+v %v", st, e)
	}
	ancestor := gitTest(t, o.Root, "rev-parse", "HEAD")
	body = strings.Replace(body, "kind: manual:implement", "kind: manual:implement\n   closed: "+ancestor, 1)
	put(t, o.Root, "data/experiments/host/host0001.md", body)
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "close implementation")
	o.DryRun = false
	st, e = Tick(context.Background(), o)
	if e != nil || len(st.Runs) != 1 {
		t.Fatalf("closed manual did not release work: %+v %v", st, e)
	}
	r := activeRun(t, o)
	if r.State != "complete" {
		t.Fatalf("%+v", r)
	}
}

func TestStagingArchiveRetainsEvidenceUntilCopySucceeds(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	put(t, stage, "evidence/case.db", "failed-run evidence")
	target := filepath.Join(root, "archive")
	put(t, root, "archive", "blocked destination")
	if e := archiveStage(stage, target); e == nil {
		t.Fatal("copy into file succeeded")
	}
	if _, e := os.Stat(filepath.Join(stage, "evidence/case.db")); e != nil {
		t.Fatal("failed archive lost source", e)
	}
	if e := os.Remove(target); e != nil {
		t.Fatal(e)
	}
	if e := archiveStage(stage, target); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(target, "evidence/case.db"))
	if e != nil || string(b) != "failed-run evidence" {
		t.Fatalf("archive=%s %v", b, e)
	}
	if _, e = os.Stat(stage); !os.IsNotExist(e) {
		t.Fatal("successful archive left scratch behind")
	}
}

func TestBuildVersionFromMakefile(t *testing.T) {
	dir := t.TempDir()
	if _, e := buildVersion(dir); e == nil {
		t.Fatal("a checkout with no version source must fail")
	}
	put(t, dir, "Makefile", "# build\nVERSION := 0.5.1-pre52\nOTHER_VERSION := 9\n")
	if v, e := buildVersion(dir); e != nil || v != "0.5.1-pre52" {
		t.Fatalf("Makefile version = %q, %v", v, e)
	}
	put(t, dir, "VERSION", "0.5.2\n")
	if v, e := buildVersion(dir); e != nil || v != "0.5.2" {
		t.Fatalf("VERSION file must win, got %q, %v", v, e)
	}
}

func TestToolCommandPassesEligibilityConfiguration(t *testing.T) {
	t.Setenv("VELOCITY_COHORT_MANIFEST", "/m.json")
	t.Setenv("VELOCITY_ANNOTATION_ROOT", "/a")
	t.Setenv("VELOCITY_ELIGIBILITY_DB", "/l.db")
	t.Setenv("VELOCITY_UNRELATED", "no")
	env := strings.Join(toolCommand(context.Background(), t.TempDir(), "true", nil).Env, "\n")
	for _, want := range []string{"VELOCITY_COHORT_MANIFEST=/m.json", "VELOCITY_ANNOTATION_ROOT=/a", "VELOCITY_ELIGIBILITY_DB=/l.db"} {
		if !strings.Contains(env, want) {
			t.Fatalf("executor environment lacks %s:\n%s", want, env)
		}
	}
	if strings.Contains(env, "VELOCITY_UNRELATED") {
		t.Fatal("other variables must stay scrubbed")
	}
}
