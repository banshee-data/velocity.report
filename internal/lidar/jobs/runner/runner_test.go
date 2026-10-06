package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

// A test kind, so the loop is exercised without libpcap or a staged tool.
const kindTest = "test_kind"

func init() {
	jobs.Kinds[kindTest] = jobs.Kind{
		Name: kindTest, Tool: "in-process", OnMain: true, Summary: "summary.json",
		ValidateParams: func(json.RawMessage) error { return nil },
	}
}

// fakeExecutor writes what it is told to and returns what it is told to.
type fakeExecutor struct {
	summary json.RawMessage
	err     error
	// block, when set, makes Run wait for ctx to end, for cancel tests.
	block bool
	ran   atomic.Int32
	// sawCaptures records the paths the executor was given.
	sawCaptures []string
}

func (f *fakeExecutor) Run(ctx context.Context, req jobs.JobRequest, env Env) (json.RawMessage, error) {
	f.ran.Add(1)
	for _, c := range env.Captures {
		f.sawCaptures = append(f.sawCaptures, c.Path)
	}
	env.Log("fake executor running %s", req.Kind)
	env.Report(jobs.Progress{Current: 1, Total: 2, Detail: "halfway"})
	if err := os.WriteFile(filepath.Join(env.BundleDir, "output.txt"), []byte("hello"), 0o644); err != nil {
		return nil, err
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.summary, nil
}

type harness struct {
	t        *testing.T
	root     string
	captures string
	store    *Store
	runner   *Runner
	exec     *fakeExecutor
	clock    time.Time
	manifest jobs.CaptureManifest
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	root := t.TempDir()
	captureRoot := filepath.Join(root, "pcap")
	if err := os.MkdirAll(filepath.Join(captureRoot, "sf"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Two "captures" of known bytes.
	files := map[string][]byte{"sf/morg0.pcapng": bytes.Repeat([]byte("m"), 4096), "sf/kirk0.pcapng": bytes.Repeat([]byte("k"), 2048)}
	var manifest jobs.CaptureManifest
	manifest.SchemaVersion, manifest.SensorID = 1, "hesai-pandar40p"
	for i, rel := range []string{"sf/morg0.pcapng", "sf/kirk0.pcapng"} {
		if err := os.WriteFile(filepath.Join(captureRoot, rel), files[rel], 0o644); err != nil {
			t.Fatal(err)
		}
		manifest.Captures = append(manifest.Captures, jobs.Capture{
			LogicalID: strings.TrimSuffix(filepath.Base(rel), ".pcapng"), Ordinal: i, RelativePath: rel,
			ByteSize: int64(len(files[rel])), SHA256: jobs.DigestBytes(files[rel]), Container: "pcapng", UDPPort: 2369,
		})
	}
	store, err := Open(filepath.Join(root, "work"))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, root: root, captures: captureRoot, store: store, manifest: manifest,
		exec:  &fakeExecutor{summary: json.RawMessage(`{"ok": true, "frames": 42}`)},
		clock: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)}
	h.runner = &Runner{
		Store: store, Captures: NewCaptures(captureRoot, store.Root()),
		Profile:   jobs.WorkerProfile{WorkerID: "test-worker", Hostname: "h", OS: "linux", Arch: "amd64", CPUs: 2, Version: "t", GitSHA: "339548fbe88926851b93178f09c3155d39e7957e"},
		Executors: map[string]Executor{kindTest: h.exec}, PollInterval: 10 * time.Millisecond,
		Now: func() time.Time { h.clock = h.clock.Add(time.Second); return h.clock },
	}
	return h
}

func (h *harness) request() jobs.JobRequest {
	return jobs.JobRequest{
		Kind: kindTest, CaptureManifest: h.manifest,
		Tuning: json.RawMessage(`{"l3": {"engine": "ema_baseline_v1", "background_update_fraction": 0.02}, "l4": {"engine": "dbscan_xy_v1", "dbscan_xy_v1": {"foreground_dbscan_eps": 0.8}}}`),
		Code:   jobs.CodeIdentity{GitSHA: "339548fbe88926851b93178f09c3155d39e7957e", BuildTags: []string{"pcap"}},
		Replay: jobs.ReplayContract{SensorID: "hesai-pandar40p", DurationSeconds: 60, WarmupSeconds: 10},
		Note:   "test",
	}
}

func (h *harness) drain() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		next, err := h.store.NextQueued()
		if err != nil {
			h.t.Fatal(err)
		}
		if next == nil {
			return
		}
		h.runner.RunOne(ctx, *next)
	}
}

// MARK: the loop

func TestAJobGoesFromQueuedToAcceptedWithABundle(t *testing.T) {
	h := newHarness(t)
	rec, err := h.store.Submit(h.request(), h.runner.now())
	if err != nil {
		t.Fatal(err)
	}
	if rec.Attempt.State != jobs.StateQueued || !rec.RunIdentityDigest.Valid() {
		t.Fatalf("submitted = %+v", rec.Attempt)
	}

	h.drain()

	done, err := h.store.Get(rec.Attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Attempt.State != jobs.StateAccepted {
		t.Fatalf("state = %s, failure %+v", done.Attempt.State, done.Attempt.Failure)
	}
	if done.Attempt.WorkerID != "test-worker" || done.Attempt.StartedAt == nil || done.Attempt.FinishedAt == nil {
		t.Errorf("attempt = %+v", done.Attempt)
	}
	if done.Attempt.Progress.Detail != "halfway" {
		t.Errorf("progress not recorded: %+v", done.Attempt.Progress)
	}
	m, err := h.store.ReadBundle(rec.Attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	_ = json.Unmarshal(m.Summary, &summary)
	if m.Outcome != "completed" || m.IdentityDigest != rec.RunIdentityDigest || summary["frames"] != 42.0 {
		t.Errorf("bundle = %+v", m)
	}
	if len(m.Files) != 1 || m.Files[0].Path != "output.txt" || m.Files[0].SHA256 != jobs.DigestBytes([]byte("hello")) {
		t.Errorf("files = %+v", m.Files)
	}
	if bd, _ := m.Digest(); done.Attempt.BundleDigest != bd {
		t.Errorf("attempt records bundle %s, manifest digests to %s", done.Attempt.BundleDigest.Short(), bd.Short())
	}
	// The executor was given the verified files, in manifest order.
	if len(h.exec.sawCaptures) != 2 || !strings.HasSuffix(h.exec.sawCaptures[0], "morg0.pcapng") {
		t.Errorf("executor saw %v", h.exec.sawCaptures)
	}
	lines, _ := h.store.LogTail(rec.Attempt.AttemptID, 0)
	if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "accepted") {
		t.Errorf("log = %v", lines)
	}
}

func TestAFailedRunKeepsItsEvidenceAsAPartialBundle(t *testing.T) {
	h := newHarness(t)
	h.exec.err = errors.New("the tool exited 3")
	rec, _ := h.store.Submit(h.request(), h.runner.now())

	h.drain()

	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateFailed {
		t.Fatalf("state = %s", done.Attempt.State)
	}
	if done.Attempt.Failure == nil || !done.Attempt.Failure.Partial || !strings.Contains(done.Attempt.Failure.Reason, "exited 3") {
		t.Errorf("failure = %+v", done.Attempt.Failure)
	}
	// The output the tool wrote before failing is still there, and the
	// bundle says it is partial.
	if _, err := os.Stat(filepath.Join(h.store.BundlePath(rec.Attempt.AttemptID), "output.txt")); err != nil {
		t.Errorf("evidence removed: %v", err)
	}
	m, err := h.store.ReadBundle(rec.Attempt.AttemptID)
	if err != nil || m.Outcome != "partial" {
		t.Errorf("bundle = %+v, %v", m, err)
	}
}

func TestACaptureWhoseBytesDifferFailsClosed(t *testing.T) {
	h := newHarness(t)
	req := h.request()
	// The manifest names bytes this worker does not have.
	req.CaptureManifest.Captures[1].SHA256 = jobs.DigestBytes([]byte("something else"))
	rec, _ := h.store.Submit(req, h.runner.now())

	h.drain()

	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateFailed || !strings.Contains(done.Attempt.Failure.Reason, "not the same bytes") {
		t.Errorf("state %s, failure %+v", done.Attempt.State, done.Attempt.Failure)
	}
	if h.exec.ran.Load() != 0 {
		t.Error("the executor ran over unverified captures")
	}
}

func TestACaptureIsFoundByContentWhenTheHintIsWrong(t *testing.T) {
	h := newHarness(t)
	req := h.request()
	req.CaptureManifest.Captures[0].RelativePath = "somewhere/else/morg0.pcapng"
	rec, _ := h.store.Submit(req, h.runner.now())
	h.drain()
	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateAccepted {
		t.Errorf("state %s, failure %+v", done.Attempt.State, done.Attempt.Failure)
	}
}

func TestADigestIsCachedUntilTheFileChanges(t *testing.T) {
	h := newHarness(t)
	var hashes int
	log := func(format string, args ...any) {
		if strings.HasPrefix(format, "hashing") {
			hashes++
		}
	}
	if _, err := h.runner.Captures.Resolve(h.manifest, log); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Captures.Resolve(h.manifest, log); err != nil {
		t.Fatal(err)
	}
	if hashes != 2 {
		t.Errorf("two captures resolved twice hashed %d times, want 2", hashes)
	}
	// The cache survives a restart of the worker.
	fresh := NewCaptures(h.captures, h.store.Root())
	if _, err := fresh.Resolve(h.manifest, log); err != nil {
		t.Fatal(err)
	}
	if hashes != 2 {
		t.Errorf("a fresh Captures rehashed: %d", hashes)
	}
	if got := len(fresh.Verified()); got != 2 {
		t.Errorf("advertises %d verified captures, want 2", got)
	}
	// A replaced file is hashed again, and no longer matches.
	path := filepath.Join(h.captures, "sf", "kirk0.pcapng")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(path, future, future)
	_, err := fresh.Resolve(h.manifest, log)
	if err == nil || !strings.Contains(err.Error(), "not the same bytes") {
		t.Errorf("a replaced file passed: %v", err)
	}
	if hashes != 3 {
		t.Errorf("the replaced file was not rehashed: %d", hashes)
	}
}

func TestCancellingARunningAttemptLeavesItCancelledWithItsOutput(t *testing.T) {
	h := newHarness(t)
	h.exec.block = true
	rec, _ := h.store.Submit(h.request(), h.runner.now())
	next, _ := h.store.NextQueued()
	finished := make(chan struct{})
	go func() {
		h.runner.RunOne(context.Background(), *next)
		close(finished)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for h.runner.Current() != rec.Attempt.AttemptID && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !h.runner.Cancel(rec.Attempt.AttemptID) {
		t.Fatal("cancel found nothing running")
	}
	<-finished
	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateCancelled {
		t.Errorf("state = %s", done.Attempt.State)
	}
	if h.runner.Cancel("att_nothing") {
		t.Error("cancelled an attempt that was not running")
	}
}

func TestARestartMarksWhatWasInProgressLost(t *testing.T) {
	h := newHarness(t)
	rec, _ := h.store.Submit(h.request(), h.runner.now())
	queued, _ := h.store.Submit(h.request(), h.runner.now())
	// The worker got as far as running, then the process died.
	_, _ = h.store.Transition(rec.Attempt.AttemptID, jobs.StateLeased, jobs.ActorHub, h.runner.now())
	_, _ = h.store.Transition(rec.Attempt.AttemptID, jobs.StateRunning, jobs.ActorWorker, h.runner.now())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = h.runner.Run(ctx)

	lost, _ := h.store.Get(rec.Attempt.AttemptID)
	if lost.Attempt.State != jobs.StateLost || lost.Attempt.Failure == nil || !lost.Attempt.Failure.Partial {
		t.Errorf("in-progress attempt after restart = %+v", lost.Attempt)
	}
	// And the queued one ran.
	done, _ := h.store.Get(queued.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateAccepted {
		t.Errorf("queued attempt after restart = %s", done.Attempt.State)
	}
}

func TestAKindThisBuildCannotRunIsRefused(t *testing.T) {
	h := newHarness(t)
	if err := h.runner.Available(jobs.KindBenchmark); err == nil {
		t.Error("benchmark available with no executor registered")
	}
	req := h.request()
	req.Kind = jobs.KindBenchmark
	rec, _ := h.store.Submit(req, h.runner.now())
	h.drain()
	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateFailed || !strings.Contains(done.Attempt.Failure.Reason, "cannot run") {
		t.Errorf("%s %+v", done.Attempt.State, done.Attempt.Failure)
	}
}

// MARK: store

func TestTheStoreRefusesIdsItDidNotMake(t *testing.T) {
	h := newHarness(t)
	for _, id := range []string{"", "../record.json", "a/b", `a\b`, "..", "att_x/../../etc"} {
		if _, err := h.store.Get(id); err == nil {
			t.Errorf("id %q was looked up", id)
		}
	}
	if _, err := h.store.Get("att_20260921T120000.000000_deadbeef"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a well-formed missing id: %v", err)
	}
}

func TestTheStoreKeepsToTheContractsTransitions(t *testing.T) {
	h := newHarness(t)
	rec, _ := h.store.Submit(h.request(), h.runner.now())
	id := rec.Attempt.AttemptID
	if _, err := h.store.Transition(id, jobs.StateRunning, jobs.ActorWorker, h.runner.now()); err == nil {
		t.Error("queued went straight to running")
	}
	if _, err := h.store.Transition(id, jobs.StateLeased, jobs.ActorWorker, h.runner.now()); err == nil {
		t.Error("a worker leased")
	}
	if _, err := h.store.Transition(id, jobs.StateLeased, jobs.ActorHub, h.runner.now()); err != nil {
		t.Error(err)
	}
	got, _ := h.store.Get(id)
	if got.Attempt.State != jobs.StateLeased {
		t.Errorf("state on disk = %s", got.Attempt.State)
	}
}

func TestListingIsOldestFirstAndTheQueueSkipsCampaignParents(t *testing.T) {
	h := newHarness(t)
	a, _ := h.store.Submit(h.request(), h.runner.now())
	b, _ := h.store.Submit(h.request(), h.runner.now())
	all, _ := h.store.List()
	if len(all) != 2 || all[0].Attempt.AttemptID != a.Attempt.AttemptID || all[1].Attempt.AttemptID != b.Attempt.AttemptID {
		t.Errorf("list = %v", all)
	}
	next, _ := h.store.NextQueued()
	if next == nil || next.Attempt.AttemptID != a.Attempt.AttemptID {
		t.Errorf("next = %v", next)
	}
}

// MARK: campaigns

func (h *harness) campaign(names ...string) Campaign {
	c := Campaign{Kind: kindTest, Base: h.request(), Note: "pass"}
	for i, n := range names {
		c.Configs = append(c.Configs, CampaignConfig{
			Name: n, Overrides: map[string]any{"l4.dbscan_xy_v1.foreground_dbscan_eps": 0.5 + float64(i)*0.1},
			Experiments: []string{"heading_flip", "cascade"},
		})
	}
	return c
}

func TestACampaignIsOneChildPerConfigWithTheOverridesApplied(t *testing.T) {
	h := newHarness(t)
	requests, names, err := h.campaign("eps05", "eps06").Expand()
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || names[1] != "eps06" {
		t.Fatalf("expanded to %d: %v", len(requests), names)
	}
	var tuning map[string]any
	_ = json.Unmarshal(requests[1].Tuning, &tuning)
	eps := tuning["l4"].(map[string]any)["dbscan_xy_v1"].(map[string]any)["foreground_dbscan_eps"]
	if eps != 0.6 {
		t.Errorf("override not applied: eps = %v", eps)
	}
	// Untouched keys stay, experiments are sorted, and each config has its
	// own identity.
	if tuning["l3"].(map[string]any)["engine"] != "ema_baseline_v1" {
		t.Error("the base document was not kept")
	}
	if requests[0].Experiments[0] != "cascade" {
		t.Errorf("experiments = %v", requests[0].Experiments)
	}
	_, d0, _ := requests[0].Identity()
	_, d1, _ := requests[1].Identity()
	if d0 == d1 {
		t.Error("two configs, one identity")
	}
}

func TestAnOverrideMustNameAKeyTheDocumentHas(t *testing.T) {
	h := newHarness(t)
	c := h.campaign("typo")
	c.Configs[0].Overrides = map[string]any{"l4.dbscan_xy_v1.foreground_dbscan_epsilon": 0.5}
	if _, _, err := c.Expand(); err == nil || !strings.Contains(err.Error(), "does not have") {
		t.Errorf("a misspelt override was applied: %v", err)
	}
	c = h.campaign("a", "a")
	if _, _, err := c.Expand(); err == nil {
		t.Error("two configs with one name")
	}
}

func TestACampaignRunsItsConfigsAndSummarisesThem(t *testing.T) {
	h := newHarness(t)
	parent, err := h.store.SubmitCampaign(h.campaign("eps05", "eps06", "eps07"), h.runner.now())
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Children) != 3 {
		t.Fatalf("children = %v", parent.Children)
	}
	h.drain()

	done, _ := h.store.Get(parent.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateAccepted {
		t.Fatalf("campaign = %s %+v", done.Attempt.State, done.Attempt.Failure)
	}
	for _, id := range parent.Children {
		c, _ := h.store.Get(id)
		if c.Attempt.State != jobs.StateAccepted || c.Parent != parent.Attempt.AttemptID || c.Label == "" {
			t.Errorf("child %s: %s parent=%q label=%q", id, c.Attempt.State, c.Parent, c.Label)
		}
	}
	raw, err := os.ReadFile(filepath.Join(h.store.BundlePath(parent.Attempt.AttemptID), "campaign.json"))
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Configs []struct {
			Config  string          `json:"config"`
			State   jobs.State      `json:"state"`
			Summary json.RawMessage `json:"summary"`
		} `json:"configs"`
	}
	_ = json.Unmarshal(raw, &table)
	if len(table.Configs) != 3 || table.Configs[2].Config != "eps07" || !strings.Contains(string(table.Configs[0].Summary), "42") {
		t.Errorf("campaign table = %s", raw)
	}
	if h.exec.ran.Load() != 3 {
		t.Errorf("executor ran %d times", h.exec.ran.Load())
	}
	if !done.RunIdentityDigest.Valid() {
		t.Error("campaign parent has no run identity: a hub cannot list it or check a bundle against it")
	}
	bundle, err := h.store.ReadBundle(done.Attempt.AttemptID)
	if err != nil {
		t.Fatalf("campaign parent bundle.json: %v", err)
	}
	if bundle.Outcome != "completed" || bundle.IdentityDigest != done.RunIdentityDigest {
		t.Errorf("campaign parent bundle = %+v", bundle)
	}
	found := false
	for _, f := range bundle.Files {
		found = found || f.Path == "campaign.json"
	}
	if !found {
		t.Errorf("campaign parent bundle files = %+v, want campaign.json listed", bundle.Files)
	}
}

// A campaign parent is never itself queued for RunOne (NextQueued skips any
// record with children), so finishParent normally only ever runs from a
// child's own transition. If the last child to finish was instead marked
// lost by a restart, nothing else will ever call it, and the parent would
// stay queued forever without the reconciliation Run does after recovery.
func TestARestartReconcilesAnOrphanedCampaignParent(t *testing.T) {
	h := newHarness(t)
	parent, err := h.store.SubmitCampaign(h.campaign("a", "b"), h.runner.now())
	if err != nil {
		t.Fatal(err)
	}

	// The first child finishes normally, well before the restart.
	first, _ := h.store.Get(parent.Children[0])
	h.runner.RunOne(context.Background(), first)
	if done, _ := h.store.Get(parent.Children[0]); done.Attempt.State != jobs.StateAccepted {
		t.Fatalf("first child = %s", done.Attempt.State)
	}

	// The second was running when the worker died.
	second := parent.Children[1]
	if _, err := h.store.Transition(second, jobs.StateLeased, jobs.ActorHub, h.runner.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Transition(second, jobs.StateRunning, jobs.ActorWorker, h.runner.now()); err != nil {
		t.Fatal(err)
	}

	// Nothing is left queued: with both children now terminal (one accepted
	// before the restart, one about to be marked lost by it), only the
	// restart's own recovery step can ever finalise this parent.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = h.runner.Run(ctx)

	done, _ := h.store.Get(parent.Attempt.AttemptID)
	if !done.Attempt.State.Terminal() {
		t.Fatalf("campaign parent after restart = %s, want terminal", done.Attempt.State)
	}
	if done.Attempt.State != jobs.StateFailed {
		t.Errorf("campaign parent state = %s, want failed: one child was lost", done.Attempt.State)
	}
	if _, err := h.store.ReadBundle(done.Attempt.AttemptID); err != nil {
		t.Errorf("campaign parent bundle.json after restart: %v", err)
	}
}

func TestACampaignWithAFailedConfigIsFailedButKeepsTheRest(t *testing.T) {
	h := newHarness(t)
	parent, _ := h.store.SubmitCampaign(h.campaign("good", "bad"), h.runner.now())
	// Fail the second child only.
	calls := 0
	h.runner.Executors[kindTest] = execFunc(func(ctx context.Context, req jobs.JobRequest, env Env) (json.RawMessage, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("boom")
		}
		return json.RawMessage(`{"ok": true}`), nil
	})
	h.drain()
	done, _ := h.store.Get(parent.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateFailed {
		t.Errorf("campaign = %s", done.Attempt.State)
	}
	good, _ := h.store.Get(parent.Children[0])
	if good.Attempt.State != jobs.StateAccepted {
		t.Errorf("the good config = %s", good.Attempt.State)
	}
}

type execFunc func(context.Context, jobs.JobRequest, Env) (json.RawMessage, error)

func (f execFunc) Run(ctx context.Context, req jobs.JobRequest, env Env) (json.RawMessage, error) {
	return f(ctx, req, env)
}

// MARK: API

func (h *harness) api(t *testing.T) (*httptest.Server, func(method, path string, body any, token string) (*http.Response, []byte)) {
	srv := httptest.NewServer((&API{Runner: h.runner, Token: "secret"}).Handler())
	t.Cleanup(srv.Close)
	call := func(method, path string, body any, token string) (*http.Response, []byte) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, &buf)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, data
	}
	return srv, call
}

func TestEverythingButHealthNeedsTheToken(t *testing.T) {
	h := newHarness(t)
	_, call := h.api(t)
	if resp, _ := call("GET", "/health", nil, ""); resp.StatusCode != 200 {
		t.Errorf("health = %d", resp.StatusCode)
	}
	for _, p := range []string{"/api/worker/status", "/api/worker/jobs", "/api/worker/jobs/x", "/api/worker/jobs/x/bundle.tar"} {
		if resp, _ := call("GET", p, nil, ""); resp.StatusCode != 401 {
			t.Errorf("%s without a token = %d", p, resp.StatusCode)
		}
		if resp, _ := call("GET", p, nil, "wrong"); resp.StatusCode != 401 {
			t.Errorf("%s with the wrong token = %d", p, resp.StatusCode)
		}
	}
	if resp, _ := call("POST", "/api/worker/jobs", h.request(), ""); resp.StatusCode != 401 {
		t.Errorf("submit without a token = %d", resp.StatusCode)
	}
	// No token configured at all: nothing is served.
	srv := httptest.NewServer((&API{Runner: h.runner}).Handler())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/worker/status", nil)
	req.Header.Set("Authorization", "Bearer ")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 503 {
		t.Errorf("a worker with no token served a request: %d", resp.StatusCode)
	}
}

func TestSubmitWatchAndFetchOverHTTP(t *testing.T) {
	h := newHarness(t)
	_, call := h.api(t)

	resp, body := call("POST", "/api/worker/jobs", h.request(), "secret")
	if resp.StatusCode != 201 {
		t.Fatalf("submit = %d %s", resp.StatusCode, body)
	}
	var created Listing
	_ = json.Unmarshal(body, &created)
	if created.State != jobs.StateQueued {
		t.Fatalf("created = %+v", created)
	}

	resp, body = call("GET", "/api/worker/status", nil, "secret")
	var status Status
	_ = json.Unmarshal(body, &status)
	// The test kind is not a registry kind, so none of the registry's is
	// available on this build.
	if resp.StatusCode != 200 || status.Queued != 1 || status.Worker.WorkerID != "test-worker" || len(status.Kinds) != 0 {
		t.Errorf("status = %d %+v", resp.StatusCode, status)
	}
	resp, body = call("GET", "/api/worker/kinds", nil, "secret")
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte(`"name":"benchmark"`)) || !bytes.Contains(body, []byte(`"available":false`)) {
		t.Errorf("kinds = %d %s", resp.StatusCode, body)
	}

	h.drain()

	resp, body = call("GET", "/api/worker/jobs?state=accepted", nil, "secret")
	var list []Listing
	_ = json.Unmarshal(body, &list)
	if resp.StatusCode != 200 || len(list) != 1 || list[0].AttemptID != created.AttemptID || !list[0].BundleDigest.Valid() {
		t.Errorf("list = %d %s", resp.StatusCode, body)
	}
	resp, body = call("GET", "/api/worker/jobs/"+created.AttemptID+"/log", nil, "secret")
	if resp.StatusCode != 200 || !strings.Contains(string(body), "fake executor") {
		t.Errorf("log = %d %s", resp.StatusCode, body)
	}
	resp, body = call("GET", "/api/worker/jobs/"+created.AttemptID+"/bundle", nil, "secret")
	var m jobs.BundleManifest
	_ = json.Unmarshal(body, &m)
	if resp.StatusCode != 200 || m.Outcome != "completed" {
		t.Errorf("bundle = %d %s", resp.StatusCode, body)
	}
	resp, body = call("GET", "/api/worker/jobs/"+created.AttemptID+"/files/output.txt", nil, "secret")
	if resp.StatusCode != 200 || string(body) != "hello" {
		t.Errorf("file = %d %q", resp.StatusCode, body)
	}
	resp, body = call("GET", "/api/worker/jobs/"+created.AttemptID+"/bundle.tar", nil, "secret")
	if resp.StatusCode != 200 {
		t.Fatalf("tar = %d", resp.StatusCode)
	}
	names := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(body))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(tr)
		names[hdr.Name] = string(content)
	}
	if names["output.txt"] != "hello" || !strings.Contains(names["bundle.json"], created.AttemptID) {
		t.Errorf("tar holds %v", names)
	}
}

func TestBundleFilesCannotBeReadOutsideTheBundle(t *testing.T) {
	h := newHarness(t)
	_, call := h.api(t)
	_, body := call("POST", "/api/worker/jobs", h.request(), "secret")
	var created Listing
	_ = json.Unmarshal(body, &created)
	h.drain()
	// A file beside the bundle: the attempt's own record.
	for _, p := range []string{"../record.json", "..%2Frecord.json", "a/../../record.json", "/etc/passwd"} {
		resp, body := call("GET", "/api/worker/jobs/"+created.AttemptID+"/files/"+p, nil, "secret")
		if resp.StatusCode == 200 || bytes.Contains(body, []byte("run_identity")) {
			t.Errorf("%s: %d %s", p, resp.StatusCode, body)
		}
	}
	if resp, _ := call("GET", "/api/worker/jobs/"+created.AttemptID+"/files/missing.txt", nil, "secret"); resp.StatusCode != 404 {
		t.Errorf("missing file = %d", resp.StatusCode)
	}
	// The mux normalises ".." away before a handler sees it. A symbolic link
	// inside the bundle that points outside it is the case the handler's own
	// check exists for.
	dir := h.store.BundlePath(created.AttemptID)
	if err := os.Symlink(filepath.Join(dir, "..", "record.json"), filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	resp, body := call("GET", "/api/worker/jobs/"+created.AttemptID+"/files/escape", nil, "secret")
	if resp.StatusCode == 200 || bytes.Contains(body, []byte("run_identity")) {
		t.Errorf("a symlink out of the bundle was served: %d %s", resp.StatusCode, body)
	}
}

func TestSubmitRejectsWhatTheContractDoes(t *testing.T) {
	h := newHarness(t)
	_, call := h.api(t)
	bad := h.request()
	bad.Code.GitSHA = "short"
	if resp, body := call("POST", "/api/worker/jobs", bad, "secret"); resp.StatusCode != 400 || !bytes.Contains(body, []byte("git_sha")) {
		t.Errorf("bad request = %d %s", resp.StatusCode, body)
	}
	unknown := h.request()
	unknown.Kind = jobs.KindBenchmark
	if resp, _ := call("POST", "/api/worker/jobs", unknown, "secret"); resp.StatusCode != 501 {
		t.Errorf("a kind this build cannot run = %d", resp.StatusCode)
	}
	// A field the contract does not have is not silently dropped.
	req, _ := http.NewRequest("POST", "", nil)
	_ = req
	raw := map[string]any{"kind": kindTest, "command": "rm -rf /"}
	if resp, body := call("POST", "/api/worker/jobs", raw, "secret"); resp.StatusCode != 400 || !bytes.Contains(body, []byte("unknown field")) {
		t.Errorf("unknown field = %d %s", resp.StatusCode, body)
	}
	if resp, _ := call("GET", "/api/worker/jobs/att_missing", nil, "secret"); resp.StatusCode != 404 {
		t.Errorf("missing attempt = %d", resp.StatusCode)
	}
}

func TestACampaignOverHTTPAndItsCancel(t *testing.T) {
	h := newHarness(t)
	_, call := h.api(t)
	resp, body := call("POST", "/api/worker/campaigns", h.campaign("a", "b"), "secret")
	if resp.StatusCode != 201 {
		t.Fatalf("campaign = %d %s", resp.StatusCode, body)
	}
	var parent Listing
	_ = json.Unmarshal(body, &parent)
	if parent.Children != 2 {
		t.Errorf("parent = %+v", parent)
	}
	resp, body = call("POST", "/api/worker/jobs/"+parent.AttemptID+"/cancel", nil, "secret")
	if resp.StatusCode != 200 {
		t.Fatalf("cancel = %d %s", resp.StatusCode, body)
	}
	all, _ := h.store.List()
	for _, rec := range all {
		if rec.Attempt.State != jobs.StateCancelled {
			t.Errorf("%s (%s) = %s after the campaign was cancelled", rec.Attempt.AttemptID, rec.Label, rec.Attempt.State)
		}
	}
	// Cancelling again is a conflict, not a second cancellation.
	if resp, _ := call("POST", "/api/worker/jobs/"+parent.AttemptID+"/cancel", nil, "secret"); resp.StatusCode != 409 {
		t.Errorf("second cancel = %d", resp.StatusCode)
	}
}

// MARK: tool executor

// A stager that "builds" a shell script, so the tool executor's argument
// assembly, working directory and summary reading are exercised without Go
// or git on the path of the test.
type scriptStager struct {
	dir  string
	seen []string
}

func (s *scriptStager) Stage(ctx context.Context, tool string, code jobs.CodeIdentity, log func(string, ...any)) (string, string, error) {
	s.seen = append(s.seen, tool+"@"+code.GitSHA)
	bin := filepath.Join(s.dir, tool)
	script := "#!/bin/sh\n" +
		"# record the args and cwd, then write the summary the kind expects\n" +
		"out=''; summary=''; while [ $# -gt 0 ]; do case \"$1\" in -out) out=$2; shift;; -json) summary=$2; shift;; esac; shift; done\n" +
		"pwd > \"$(dirname \"${out:-$summary}\")/cwd.txt\" 2>/dev/null || true\n" +
		"if [ -n \"$out\" ]; then mkdir -p \"$out\"; printf '{\"cases\":[{\"baseline_equal\":true}]}' > \"$(dirname \"$out\")/phase0-summary.json\"; fi\n" +
		"if [ -n \"$summary\" ]; then printf '{\"score\":1}' > \"$summary\"; fi\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		return "", "", err
	}
	return bin, s.dir, nil
}

func TestTheBaselineToolIsInvokedAsTheHarnessInvokesIt(t *testing.T) {
	h := newHarness(t)
	stager := &scriptStager{dir: t.TempDir()}
	h.runner.Tools = stager
	h.runner.Executors[jobs.KindStateEstimationBaseline] = Executors[jobs.KindStateEstimationBaseline]
	req := h.request()
	req.Kind = jobs.KindStateEstimationBaseline
	req.Params = json.RawMessage(`{"case": "morg0", "surface_ground": true}`)
	req.Experiments = []string{"cascade"}
	rec, _ := h.store.Submit(req, h.runner.now())

	h.drain()

	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateAccepted {
		lines, _ := h.store.LogTail(rec.Attempt.AttemptID, 0)
		t.Fatalf("state %s, failure %+v, log %v", done.Attempt.State, done.Attempt.Failure, lines)
	}
	if len(stager.seen) != 1 || stager.seen[0] != "lidar-state-estimation-baseline@339548fbe88926851b93178f09c3155d39e7957e" {
		t.Errorf("staged %v", stager.seen)
	}
	dir := h.store.BundlePath(rec.Attempt.AttemptID)
	cwd, _ := os.ReadFile(filepath.Join(dir, "cwd.txt"))
	wantDir, _ := filepath.EvalSymlinks(stager.dir)
	if strings.TrimSpace(string(cwd)) != wantDir {
		t.Errorf("the tool ran in %q, not the staged tree %q", strings.TrimSpace(string(cwd)), stager.dir)
	}
	lines, _ := h.store.LogTail(rec.Attempt.AttemptID, 0)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"-case morg0", "-surface-ground", "-experiment cascade", "-duration 60", "-warmup 10", "-pcap-root " + h.captures} {
		if !strings.Contains(joined, want) {
			t.Errorf("command line lacks %q:\n%s", want, joined)
		}
	}
	// The corpus the tool was pointed at names the verified captures by
	// their paths under the root, in manifest order.
	index, _ := os.ReadFile(filepath.Join(dir, "corpus-index.json"))
	if !strings.Contains(string(index), `"sf/morg0.pcapng"`) || !strings.Contains(string(index), `"sf/kirk0.pcapng"`) {
		t.Errorf("index = %s", index)
	}
	m, _ := h.store.ReadBundle(rec.Attempt.AttemptID)
	if !strings.Contains(string(m.Summary), "baseline_equal") {
		t.Errorf("summary = %s", m.Summary)
	}
}

func TestAToolThatIsNotOnTheWorkerFailsTheAttemptNotTheWorker(t *testing.T) {
	h := newHarness(t)
	h.runner.Executors[jobs.KindStateEstimationBaseline] = Executors[jobs.KindStateEstimationBaseline]
	req := h.request()
	req.Kind = jobs.KindStateEstimationBaseline
	req.Params = json.RawMessage(`{"case": "morg0"}`)
	rec, _ := h.store.Submit(req, h.runner.now())
	h.drain()
	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateFailed || !strings.Contains(done.Attempt.Failure.Reason, "no tool stager") {
		t.Errorf("%s %+v", done.Attempt.State, done.Attempt.Failure)
	}
}

func TestABundleThatIsNotTheJobsIsNotAccepted(t *testing.T) {
	h := newHarness(t)
	rec, _ := h.store.Submit(h.request(), h.runner.now())
	h.drain()
	done, _ := h.store.Get(rec.Attempt.AttemptID)
	if done.Attempt.State != jobs.StateAccepted {
		t.Fatal(done.Attempt.State)
	}
	// The same bundle, presented for a job with another identity: what a
	// hub would see if a worker uploaded the wrong directory.
	other := done
	other.RunIdentityDigest = jobs.DigestBytes([]byte("another job"))
	if _, err := h.runner.verifyBundle(other, jobs.BundleManifest{}); err == nil || !strings.Contains(err.Error(), "not the job's") {
		t.Errorf("a bundle for another identity verified: %v", err)
	}
	// And a bundle whose file has been altered since its manifest was written.
	if err := os.WriteFile(filepath.Join(h.store.BundlePath(rec.Attempt.AttemptID), "output.txt"), []byte("HELLO"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.verifyBundle(done, jobs.BundleManifest{}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("an altered file verified: %v", err)
	}
}
