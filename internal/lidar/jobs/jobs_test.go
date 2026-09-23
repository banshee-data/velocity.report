package jobs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A manifest as the state-estimation baseline tool writes one, plus the two
// optional fields this package adds.
func goodManifest() CaptureManifest {
	return CaptureManifest{
		SchemaVersion: 1, SensorID: "hesai-pandar40p",
		Captures: []Capture{
			{LogicalID: "morg0", Ordinal: 0, RelativePath: "sf/morg0.pcapng", ByteSize: 1_073_741_824,
				SHA256: DigestBytes([]byte("morg0")), Container: "pcapng", UDPPort: 2369},
			{LogicalID: "kirk0", Ordinal: 1, RelativePath: "sf/kirk0.pcapng", ByteSize: 712_000_000,
				SHA256: DigestBytes([]byte("kirk0")), Container: "pcapng", UDPPort: 2369},
		},
	}
}

func goodRequest() JobRequest {
	return JobRequest{
		Kind:            KindStateEstimationBaseline,
		CaptureManifest: goodManifest(),
		Tuning:          json.RawMessage(`{"l3": {"engine": "ema_baseline_v1"}, "l4": {"engine": "dbscan_xy_v1"}}`),
		Experiments:     []string{"cascade", "heading_flip"},
		Code:            CodeIdentity{GitSHA: "339548fbe88926851b93178f09c3155d39e7957e", BuildTags: []string{"pcap"}},
		Replay:          ReplayContract{SensorID: "hesai-pandar40p", DurationSeconds: 120, WarmupSeconds: 30},
		Params:          json.RawMessage(`{"case": "morg0"}`),
		Worker:          "swan",
		Note:            "smoke",
	}
}

// MARK: digests

func TestDigestFormIsFixed(t *testing.T) {
	d := DigestBytes([]byte("abc"))
	if d != "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("sha256 of abc = %s", d)
	}
	if !d.Valid() || d.Short() != "ba7816bf8f01" {
		t.Errorf("valid=%v short=%s", d.Valid(), d.Short())
	}
	for _, bad := range []Digest{"", "ba7816bf", "md5:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"sha256:zz7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"} {
		if bad.Valid() {
			t.Errorf("%q counted as a valid digest", bad)
		}
	}
}

func TestDigestFileMatchesBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "capture.bin")
	if err := os.WriteFile(p, []byte("not a pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, n, err := DigestFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 || d != DigestBytes([]byte("not a pcap")) {
		t.Errorf("digest %s over %d bytes", d, n)
	}
	if _, _, err := DigestFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing file digested")
	}
}

// The same value must give the same bytes however it was typed or ordered:
// otherwise the same job would have two identities.
func TestCanonicalJSONIsOrderAndTypeIndependent(t *testing.T) {
	a := map[string]any{"b": 1, "a": []any{map[string]any{"y": 2.5, "x": "s"}}, "n": nil}
	b := map[string]any{"n": nil, "a": []any{map[string]any{"x": "s", "y": 2.5}}, "b": 1.0}
	da, err := DigestCanonical(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := DigestCanonical(b)
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Errorf("same value, two digests: %s and %s", da.Short(), db.Short())
	}
	got, _ := canonicalJSON(a)
	if string(got) != `{"a":[{"x":"s","y":2.5}],"b":1,"n":null}` {
		t.Errorf("canonical form = %s", got)
	}
	// A different value is a different digest.
	c := map[string]any{"b": 2, "a": []any{map[string]any{"y": 2.5, "x": "s"}}, "n": nil}
	if dc, _ := DigestCanonical(c); dc == da {
		t.Error("different values, one digest")
	}
}

// MARK: manifest

func TestManifestDigestIgnoresWhereTheFilesAre(t *testing.T) {
	here := goodManifest()
	there := goodManifest()
	there.Captures[0].RelativePath = "mnt/pcap/morg0.pcapng"
	there.Captures[1].RelativePath = "kirk0.pcapng"
	dh, err := here.Digest()
	if err != nil {
		t.Fatal(err)
	}
	dt, err := there.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if dh != dt {
		t.Errorf("the same bytes at different paths gave different manifest digests")
	}
	// The order of the bytes is part of the input.
	swapped := goodManifest()
	swapped.Captures[0], swapped.Captures[1] = swapped.Captures[1], swapped.Captures[0]
	swapped.Captures[0].Ordinal, swapped.Captures[1].Ordinal = 0, 1
	ds, err := swapped.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if ds == dh {
		t.Error("reordered captures gave the same manifest digest")
	}
	if got := here.CaptureDigests(); len(got) != 2 || got[0] != here.Captures[0].SHA256 {
		t.Errorf("capture digests = %v", got)
	}
}

// Pinned: a change to canonicalisation or to what is in the identity must
// fail here, in the branch that made it, before it invalidates a hub's index.
func TestManifestDigestIsPinned(t *testing.T) {
	d, err := goodManifest().Digest()
	if err != nil {
		t.Fatal(err)
	}
	const pinned = "sha256:0569f43da2cf2241c950d0debe1405c51b69681c2711d84ec48f0bb708c8317d"
	if string(d) != pinned {
		t.Errorf("manifest digest = %s\nIf the canonical form changed on purpose, update the pin and say so in the commit.", d)
	}
}

func TestManifestRejectsWhatItMust(t *testing.T) {
	cases := map[string]func(*CaptureManifest){
		"wrong schema":       func(m *CaptureManifest) { m.SchemaVersion = 2 },
		"no sensor":          func(m *CaptureManifest) { m.SensorID = " " },
		"no captures":        func(m *CaptureManifest) { m.Captures = nil },
		"ordinal gap":        func(m *CaptureManifest) { m.Captures[1].Ordinal = 2 },
		"no id":              func(m *CaptureManifest) { m.Captures[0].LogicalID = "" },
		"absolute path":      func(m *CaptureManifest) { m.Captures[0].RelativePath = "/Volumes/lidar/morg0.pcapng" },
		"parent step":        func(m *CaptureManifest) { m.Captures[0].RelativePath = "../elsewhere/morg0.pcapng" },
		"hidden parent step": func(m *CaptureManifest) { m.Captures[0].RelativePath = "sf/../../etc/passwd" },
		"backslashes":        func(m *CaptureManifest) { m.Captures[0].RelativePath = `sf\morg0.pcapng` },
		"unclean path":       func(m *CaptureManifest) { m.Captures[0].RelativePath = "sf//./morg0.pcapng" },
		"inside via parent":  func(m *CaptureManifest) { m.Captures[0].RelativePath = "sf/../sf/morg0.pcapng" },
		"zero bytes":         func(m *CaptureManifest) { m.Captures[0].ByteSize = 0 },
		"bad digest":         func(m *CaptureManifest) { m.Captures[0].SHA256 = "abc" },
		"same bytes twice":   func(m *CaptureManifest) { m.Captures[1].SHA256 = m.Captures[0].SHA256 },
		"unknown container":  func(m *CaptureManifest) { m.Captures[0].Container = "tar" },
		"port out of range":  func(m *CaptureManifest) { m.Captures[0].UDPPort = 70000 },
	}
	for name, mutate := range cases {
		m := goodManifest()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := m.Digest(); err == nil {
			t.Errorf("%s: digested", name)
		}
	}
	if err := goodManifest().Validate(); err != nil {
		t.Errorf("the good manifest: %v", err)
	}
}

// The state-estimation baseline tool's manifest names its captures the same
// way, so one it wrote decodes and validates here without translation.
func TestStateEstimationManifestFieldNamesDecode(t *testing.T) {
	raw := `{"schema_version": 1, "sensor_id": "hesai-pandar40p", "captures": [
		{"id": "morg0", "ordinal": 0, "relative_path": "sf/morg0.pcapng", "byte_size": 12,
		 "sha256": "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}]}`
	var m CaptureManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("a tool-shaped manifest was refused: %v", err)
	}
}

// MARK: request and identity

func TestIdentityIsWhatWasComputedNotWhereOrWhen(t *testing.T) {
	base := goodRequest()
	_, d1, err := base.Identity()
	if err != nil {
		t.Fatal(err)
	}
	// Where it runs, and what the operator wrote, are not the computation.
	elsewhere := goodRequest()
	elsewhere.Worker = "nas"
	elsewhere.Note = "again"
	elsewhere.CaptureManifest.Captures[0].RelativePath = "other/root/morg0.pcapng"
	elsewhere.Code.GitSHA = strings.ToUpper(elsewhere.Code.GitSHA)
	elsewhere.Tuning = json.RawMessage(`{"l4":{"engine":"dbscan_xy_v1"},"l3":{"engine":"ema_baseline_v1"}}`)
	_, d2, err := elsewhere.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Errorf("worker, note, path hints, case of the commit and key order changed the identity")
	}

	// Each comparable part does change it.
	changes := map[string]func(*JobRequest){
		"tuning":      func(r *JobRequest) { r.Tuning = json.RawMessage(`{"l3": {"engine": "ema_baseline_v1"}}`) },
		"experiments": func(r *JobRequest) { r.Experiments = []string{"cascade"} },
		"commit":      func(r *JobRequest) { r.Code.GitSHA = "0000000000000000000000000000000000000000" },
		"build tags":  func(r *JobRequest) { r.Code.BuildTags = nil },
		"window":      func(r *JobRequest) { r.Replay.DurationSeconds = 60 },
		"warm-up":     func(r *JobRequest) { r.Replay.WarmupSeconds = 20 },
		"params":      func(r *JobRequest) { r.Params = json.RawMessage(`{"case": "morg0", "surface_ground": true}`) },
		"captures":    func(r *JobRequest) { r.CaptureManifest.Captures = r.CaptureManifest.Captures[:1] },
		"kind": func(r *JobRequest) {
			r.Kind = KindTrackScorecard
			r.Params = json.RawMessage(`{"evidence_job": "job_1"}`)
		},
	}
	for name, change := range changes {
		r := goodRequest()
		change(&r)
		_, d, err := r.Identity()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if d == d1 {
			t.Errorf("changing the %s left the identity the same", name)
		}
	}
}

func TestIdentityIsPinned(t *testing.T) {
	id, d, err := goodRequest().Identity()
	if err != nil {
		t.Fatal(err)
	}
	if id.OutputSchema != OutputSchemaVersion || id.Kind != KindStateEstimationBaseline {
		t.Errorf("identity = %+v", id)
	}
	const pinned = "sha256:ce74fd087048996b1e09dca89b7c8931a67d208875eeb77d0af89e446bcd6421"
	if string(d) != pinned {
		t.Errorf("run identity digest = %s\nIf what is in the identity changed on purpose, update the pin and say so in the commit.", d)
	}
}

func TestRequestRejectsWhatItMust(t *testing.T) {
	cases := map[string]func(*JobRequest){
		"unknown kind":          func(r *JobRequest) { r.Kind = "shell" },
		"no tuning":             func(r *JobRequest) { r.Tuning = nil },
		"tuning not json":       func(r *JobRequest) { r.Tuning = json.RawMessage(`{`) },
		"short commit":          func(r *JobRequest) { r.Code.GitSHA = "339548f" },
		"commit not hex":        func(r *JobRequest) { r.Code.GitSHA = "zz9548fbe88926851b93178f09c3155d39e7957e" },
		"bad tree digest":       func(r *JobRequest) { r.Code.SourceTreeDigest = "abc" },
		"unsorted tags":         func(r *JobRequest) { r.Code.BuildTags = []string{"pcap", "cgo"} },
		"no replay sensor":      func(r *JobRequest) { r.Replay.SensorID = "" },
		"sensor mismatch":       func(r *JobRequest) { r.Replay.SensorID = "other" },
		"negative window":       func(r *JobRequest) { r.Replay.DurationSeconds = -1 },
		"warm-up eats window":   func(r *JobRequest) { r.Replay.WarmupSeconds = 120 },
		"unsorted experiments":  func(r *JobRequest) { r.Experiments = []string{"heading_flip", "cascade"} },
		"duplicate experiments": func(r *JobRequest) { r.Experiments = []string{"cascade", "cascade"} },
		"empty experiment":      func(r *JobRequest) { r.Experiments = []string{""} },
		"bad manifest":          func(r *JobRequest) { r.CaptureManifest.Captures = nil },
		"unknown param":         func(r *JobRequest) { r.Params = json.RawMessage(`{"case": "morg0", "cmd": "rm -rf /"}`) },
		"case is a path":        func(r *JobRequest) { r.Params = json.RawMessage(`{"case": "../morg0"}`) },
		"no case":               func(r *JobRequest) { r.Params = json.RawMessage(`{}`) },
	}
	for name, mutate := range cases {
		r := goodRequest()
		mutate(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := goodRequest().Validate(); err != nil {
		t.Errorf("the good request: %v", err)
	}
}

// MARK: kinds

func TestEveryKindHasAToolASummaryAndDefaults(t *testing.T) {
	if len(KindNames()) != len(Kinds) {
		t.Errorf("KindNames lists %d, registry has %d", len(KindNames()), len(Kinds))
	}
	for _, name := range KindNames() {
		k, ok := Kinds[name]
		if !ok || k.Name != name || k.Tool == "" || k.Summary == "" || k.ValidateParams == nil {
			t.Errorf("kind %s is incomplete: %+v", name, k)
			continue
		}
		// Every kind but those needing a target accepts no params at all.
		err := k.ValidateParams(nil)
		needsTarget := name == KindStateEstimationBaseline || name == KindTrackScorecard
		if (err == nil) == needsTarget {
			t.Errorf("kind %s with no params: err=%v, needsTarget=%v", name, err, needsTarget)
		}
	}
	if !Kinds[KindBenchmark].OnMain || Kinds[KindStateEstimationBaseline].OnMain {
		t.Error("OnMain is wrong about which tools main has")
	}
}

func TestKindParamsAreChecked(t *testing.T) {
	good := map[string]string{
		KindBenchmark:               `{"profile": "detect", "repeats": 5}`,
		KindStateEstimationBaseline: `{"case": "kirk0", "surface_ground": true, "keep_evidence": false}`,
		KindTrackScorecard:          `{"evidence_job": "job_7", "scoring_start_seconds": 30}`,
		KindVRLOGRecord:             `{"include_points": true, "settle_first": true}`,
	}
	bad := map[string][]string{
		KindBenchmark:               {`{"profile": "l9"}`, `{"repeats": 26}`, `{"repeats": -1}`, `{"pcap": "/etc"}`},
		KindStateEstimationBaseline: {`{"case": ""}`, `{"case": "a b"}`, `{"case": "x", "out": "/tmp"}`},
		KindTrackScorecard:          {`{}`, `{"evidence_job": "j", "scoring_start_seconds": -1}`},
		KindVRLOGRecord:             {`{"path": "/Volumes"}`, `{"include_points": "yes"}`},
	}
	for name, params := range good {
		if err := Kinds[name].ValidateParams(json.RawMessage(params)); err != nil {
			t.Errorf("%s refused %s: %v", name, params, err)
		}
	}
	for name, list := range bad {
		for _, params := range list {
			if err := Kinds[name].ValidateParams(json.RawMessage(params)); err == nil {
				t.Errorf("%s accepted %s", name, params)
			}
		}
	}
}

// MARK: attempts

func TestTheStateMachineIsThePoolPlans(t *testing.T) {
	allowed := []struct {
		from, to State
		by       Actor
	}{
		{StateQueued, StateLeased, ActorHub},
		{StateQueued, StateCancelled, ActorOperator},
		{StateLeased, StateRunning, ActorWorker},
		{StateLeased, StateFailed, ActorWorker},
		{StateLeased, StateLost, ActorHub},
		{StateRunning, StateUploading, ActorWorker},
		{StateRunning, StateFailed, ActorWorker},
		{StateRunning, StateLost, ActorHub},
		{StateRunning, StateCancelled, ActorOperator},
		{StateUploading, StateVerifying, ActorHub},
		{StateUploading, StateLost, ActorHub},
		{StateVerifying, StateAccepted, ActorHub},
		{StateVerifying, StateFailed, ActorHub},
	}
	for _, tr := range allowed {
		if err := Transition(tr.from, tr.to, tr.by); err != nil {
			t.Errorf("%s -> %s by %s refused: %v", tr.from, tr.to, tr.by, err)
		}
	}
	refused := []struct {
		from, to State
		by       Actor
	}{
		// A worker cannot claim, accept, or resurrect.
		{StateQueued, StateLeased, ActorWorker},
		{StateVerifying, StateAccepted, ActorWorker},
		{StateLost, StateRunning, ActorWorker},
		{StateFailed, StateQueued, ActorHub},
		{StateAccepted, StateRunning, ActorHub},
		// Nothing skips a state.
		{StateQueued, StateRunning, ActorHub},
		{StateLeased, StateAccepted, ActorHub},
		{StateRunning, StateVerifying, ActorWorker},
		// Only a person cancels, and not once the hub is verifying.
		{StateRunning, StateCancelled, ActorHub},
		{StateVerifying, StateCancelled, ActorOperator},
	}
	for _, tr := range refused {
		if err := Transition(tr.from, tr.to, tr.by); err == nil {
			t.Errorf("%s -> %s by %s allowed", tr.from, tr.to, tr.by)
		}
	}
	for _, s := range []State{StateAccepted, StateFailed, StateCancelled, StateLost} {
		if !s.Terminal() {
			t.Errorf("%s is not terminal", s)
		}
	}
	for _, s := range []State{StateQueued, StateLeased, StateRunning, StateUploading, StateVerifying} {
		if s.Terminal() {
			t.Errorf("%s is terminal", s)
		}
	}
}

func TestALeaseLapsesOnlyWhileAWorkerHoldsIt(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Second)
	a := Attempt{State: StateRunning, LeaseExpires: &past}
	if !a.LeaseExpired(now) {
		t.Error("a running attempt whose lease is past is not lost")
	}
	a.State = StateQueued
	if a.LeaseExpired(now) {
		t.Error("a queued attempt has no lease to lose")
	}
	a.State = StateAccepted
	if a.LeaseExpired(now) {
		t.Error("an accepted attempt cannot be lost")
	}
	a.State = StateRunning
	a.LeaseExpires = nil
	if a.LeaseExpired(now) {
		t.Error("no lease recorded is not an expired lease")
	}
}

// MARK: bundles

func goodBundle(t *testing.T) BundleManifest {
	t.Helper()
	id, d, err := goodRequest().Identity()
	if err != nil {
		t.Fatal(err)
	}
	return BundleManifest{
		OutputSchema: OutputSchemaVersion, Identity: id, IdentityDigest: d,
		JobID: "job_1", AttemptID: "att_1",
		Worker:     WorkerProfile{WorkerID: "swan", Hostname: "swan", OS: "linux", Arch: "amd64", CPUs: 16, Version: "0.5.1", GitSHA: "339548fbe88926851b93178f09c3155d39e7957e"},
		StartedUTC: "2026-09-21T10:00:00Z", FinishedUTC: "2026-09-21T10:20:00Z", Outcome: "completed",
		Summary: json.RawMessage(`{"baseline_equal": true}`),
		Files: []BundleFile{
			{Path: "phase0-summary.json", Bytes: 812, SHA256: DigestBytes([]byte("summary"))},
			{Path: "morg0/first/tracking_baseline.json", Bytes: 40_120, SHA256: DigestBytes([]byte("baseline"))},
			{Path: "tool-stdout.txt", Bytes: 0, SHA256: DigestBytes(nil)},
		},
	}
}

func TestABundleIsOneThingHoweverOftenItIsSent(t *testing.T) {
	a, err := goodBundle(t).Digest()
	if err != nil {
		t.Fatal(err)
	}
	b, err := goodBundle(t).Digest()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("the same bundle has two digests")
	}
	other := goodBundle(t)
	other.Files[0].SHA256 = DigestBytes([]byte("different summary"))
	c, _ := other.Digest()
	if c == a {
		t.Error("a bundle with a different file has the same digest")
	}
}

func TestABundleThatLiesAboutItsIdentityIsRefused(t *testing.T) {
	b := goodBundle(t)
	b.Identity.Replay.DurationSeconds = 600
	if err := b.Validate(); err == nil || !strings.Contains(err.Error(), "identity digest") {
		t.Errorf("an identity that does not digest to its own digest was accepted: %v", err)
	}
	b = goodBundle(t)
	b.IdentityDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	if err := b.Validate(); err == nil {
		t.Error("a wrong identity digest was accepted")
	}
}

func TestBundleRejectsWhatItMust(t *testing.T) {
	cases := map[string]func(*BundleManifest){
		"other schema":          func(b *BundleManifest) { b.OutputSchema = 2 },
		"no job":                func(b *BundleManifest) { b.JobID = "" },
		"no worker":             func(b *BundleManifest) { b.Worker.WorkerID = "" },
		"odd outcome":           func(b *BundleManifest) { b.Outcome = "done" },
		"completed, no summary": func(b *BundleManifest) { b.Summary = nil },
		"summary not json":      func(b *BundleManifest) { b.Summary = json.RawMessage(`{`) },
		"file outside bundle":   func(b *BundleManifest) { b.Files[0].Path = "../../etc/passwd" },
		"file absolute":         func(b *BundleManifest) { b.Files[0].Path = "/tmp/x" },
		"lists own manifest":    func(b *BundleManifest) { b.Files[0].Path = BundleManifestFile },
		"file twice":            func(b *BundleManifest) { b.Files[1].Path = b.Files[0].Path },
		"negative bytes":        func(b *BundleManifest) { b.Files[0].Bytes = -1 },
		"bad file digest":       func(b *BundleManifest) { b.Files[0].SHA256 = "x" },
	}
	for name, mutate := range cases {
		b := goodBundle(t)
		mutate(&b)
		if err := b.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A partial bundle is evidence, and carries no summary.
	partial := goodBundle(t)
	partial.Outcome = "partial"
	partial.Summary = nil
	if err := partial.Validate(); err != nil {
		t.Errorf("a partial bundle without a summary was refused: %v", err)
	}
}
