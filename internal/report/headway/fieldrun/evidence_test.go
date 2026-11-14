package fieldrun

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

func evidenceFixture(t *testing.T) Result {
	t.Helper()
	db := openDB(t)
	seedSolidBodies(t, db, steadyApproach(), onlineBodies)
	r, err := Analyse(db, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM lidar_interaction_events").Scan(&n); err != nil || n != 0 {
		t.Fatalf("read-only analyse wrote records: %d %v", n, err)
	}
	return r
}

func evidenceContext() EvidenceContext {
	return EvidenceContext{SiteNamespace: "survey", SiteID: "kirkham-9th", SourceID: testSource, BindingID: "fixture-binding", BuildGitSHA: "fixture-build", PolicyID: "fixture-policy"}
}

func TestEvidenceRepeatAndAccounting(t *testing.T) {
	r := evidenceFixture(t)
	c := evidenceContext()
	d, err := Evidence(r, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) == 0 {
		t.Fatal("no timeline rows")
	}
	var rows, buckets, drac int64
	for _, row := range d.Rows {
		rows += row.IntervalNanos
		if row.Point != nil && row.Point.Follower != nil {
			if row.FollowerSpeedMPS == nil || *row.FollowerSpeedMPS != row.Point.Follower.AlongSpeedMps {
				t.Fatal("speed not retained")
			}
		}
	}
	for _, v := range d.Accounting {
		buckets += v
	}
	for _, v := range d.DRACAccounting {
		drac += v
	}
	var want int64
	for _, tr := range d.Trajectories {
		want += tr.Samples[len(tr.Samples)-1].CaptureUnixNanos - tr.Samples[0].CaptureUnixNanos
	}
	if rows != want || buckets != want || drac != want {
		t.Fatalf("accounting rows=%d buckets=%d drac=%d expected=%d", rows, buckets, drac, want)
	}
	dir := t.TempDir()
	p, err := WriteEvidence(dir, d)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	var writers sync.WaitGroup
	for i := 0; i < 4; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			if _, e := WriteEvidence(dir, d); e != nil {
				t.Error(e)
			}
		}()
	}
	writers.Wait()
	// A different but internally consistent analysis cannot overwrite the same
	// declared identity. This catches forgotten method/config version changes.
	changed := r
	changed.Trajectories = append([]l8behaviour.Trajectory(nil), r.Trajectories...)
	changed.Trajectories[0].Samples = append([]l8behaviour.TrajectorySample(nil), r.Trajectories[0].Samples...)
	changed.Trajectories[0].Samples[0].X += 0.01
	changedDoc, e := Evidence(changed, c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = WriteEvidence(dir, changedDoc); e == nil {
		t.Fatal("different content overwrote identity")
	}
	r.Trajectories[0], r.Trajectories[1] = r.Trajectories[1], r.Trajectories[0]
	second, err := Evidence(r, c)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := WriteEvidence(dir, second)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p2)
	if p != p2 || !bytes.Equal(first, got) {
		t.Fatal("repeat changed deterministic bytes")
	}
	second.Rows[0].Bucket = "tampered"
	if _, err = WriteEvidence(dir, second); err == nil {
		t.Fatal("same-identity conflict overwritten")
	}
	d.Context.SiteID = "other-site"
	p3, err := WriteEvidence(dir, d)
	if err != nil || p3 == p {
		t.Fatalf("explicit site namespace collided: %s %v", p3, err)
	}
	d.Context = c
	d.Context.BuildGitSHA += "/revision"
	p4, err := WriteEvidence(dir, d)
	if err != nil || p4 == p {
		t.Fatalf("analysis revision collided: %s %v", p4, err)
	}
}

func TestEvidenceFullTimelineOcclusionAndGaps(t *testing.T) {
	r := evidenceFixture(t)
	// A missing sampled interval is separate from both predicted and observed
	// metric intervals. Every follower's terminal sample remains a zero row.
	for ti := range r.Trajectories {
		tr := &r.Trajectories[ti]
		tr.Samples = append(tr.Samples[:10], tr.Samples[12:]...)
	}
	d, err := Evidence(r, evidenceContext())
	if err != nil {
		t.Fatal(err)
	}
	if d.Accounting["record_gap"] == 0 {
		t.Fatal("record gap omitted")
	}
	zero := 0
	for _, row := range d.Rows {
		if row.IntervalNanos == 0 {
			zero++
		}
		if row.Bucket == "record_gap" && row.DRAC.Calculation != nil && row.DRAC.Calculation.ValueMPS2 != nil {
			t.Fatal("record gap became observed DRAC")
		}
	}
	if zero != len(r.Trajectories) {
		t.Fatal("terminal censoring lost")
	}
	// A follower with no other body still has its complete timeline, even
	// though there are no encounter rows to persist.
	r.Trajectories = r.Trajectories[:1]
	d, err = Evidence(r, evidenceContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != len(r.Trajectories[0].Samples) || len(d.Interactions) != 0 || d.Accounting["no_leader"] == 0 {
		t.Fatal("free-flow evidence omitted")
	}
}

func TestEvidenceRefusesUnboundOrConflictingInput(t *testing.T) {
	r := evidenceFixture(t)
	c := evidenceContext()
	for _, change := range []func(*EvidenceContext){func(c *EvidenceContext) { c.SourceID = "other" }, func(c *EvidenceContext) { c.SiteID = "" }, func(c *EvidenceContext) { c.SiteNamespace = "" }, func(c *EvidenceContext) { c.BindingID = "" }, func(c *EvidenceContext) { c.BuildGitSHA = "" }, func(c *EvidenceContext) { c.PolicyID = "" }} {
		bad := c
		change(&bad)
		if _, err := Evidence(r, bad); err == nil {
			t.Fatal("unbound context accepted")
		}
	}
	for _, change := range []func(*Result){func(r *Result) { r.Version.MethodID = "" }, func(r *Result) { r.Version.MethodID = "wrong" }, func(r *Result) { r.Trajectories[0].Passage.SiteID = "conflict" }, func(r *Result) { r.Trajectories = append(r.Trajectories, r.Trajectories[0]) }} {
		b, _ := json.Marshal(r)
		var bad Result
		json.Unmarshal(b, &bad)
		change(&bad)
		if _, err := Evidence(bad, c); err == nil {
			t.Fatal("conflicting source evidence accepted")
		}
	}
}

func TestAnalyseReadOnlyDatabase(t *testing.T) {
	db := openDB(t)
	seedSolidBodies(t, db, steadyApproach(), onlineBodies)
	// Query-only enforces the non-mutating contract even when the caller
	// passes an ordinary pooled connection rather than a read-only handle.
	if _, err := db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	a, err := Analyse(db, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Interactions) == 0 {
		t.Fatal("fixture lacks interactions")
	}
	if _, err = Run(db, Spec{SourceID: testSource, Stage: l8behaviour.StageOnline, SolidBodies: true}); err == nil {
		t.Fatal("control write unexpectedly succeeded")
	}
	if a.Estimates.SourceID != testSource {
		t.Fatal("source changed")
	}
}
