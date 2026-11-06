package eligibility_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
)

type env struct {
	t        *testing.T
	root     string // annotation root; splits/ beneath it
	captures string
	ledger   labelworkdb.UseLedger
	manifest string
	extra    map[string][]string
	access   eligibility.AccessControl
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	d, err := db.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	e := &env{t: t, root: filepath.Join(dir, "annotation"), captures: filepath.Join(dir, "captures"), ledger: labelworkdb.UseLedger{DB: d.DB}}
	for _, p := range []string{e.root, e.captures} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e *env) service() *eligibility.Service {
	return eligibility.New(eligibility.Config{
		ManifestPath: e.manifest, SplitsDir: filepath.Join(e.root, "splits"), Ledger: e.ledger, Access: e.access,
		ExtraHashes: func() map[string][]string { return e.extra },
	})
}

func (e *env) request(s *eligibility.Service, purpose eligibility.Purpose, paths ...string) eligibility.Request {
	req := eligibility.Request{Purpose: purpose, EntryPoint: "test", Actor: "tester"}
	for _, p := range paths {
		req.Inputs = append(req.Inputs, s.CaptureInput(p, 0, 0))
	}
	return req
}

func codes(d eligibility.Decision) []string {
	var out []string
	for _, r := range d.Reasons {
		if r.Blocking {
			out = append(out, r.Code)
		}
	}
	for _, in := range d.Inputs {
		for _, r := range in.Reasons {
			if r.Blocking {
				out = append(out, r.Code)
			}
		}
	}
	return out
}

func has(list []string, code string) bool {
	for _, c := range list {
		if c == code {
			return true
		}
	}
	return false
}

func (e *env) uses() []eligibility.Use {
	uses, _, err := e.ledger.List(eligibility.UseQuery{Limit: 500})
	if err != nil {
		e.t.Fatal(err)
	}
	return uses
}

// Without a manifest every capture is unassigned, and development use fails
// closed; production processing is neither gated nor recorded.
func TestNoManifestFailsClosedAndProductionIsUnaffected(t *testing.T) {
	e := newEnv(t)
	path, _ := fx.Capture(t, e.captures, "kirk0.pcapng", "synthetic capture")
	s := e.service()
	d, err := s.Authorize(e.request(s, eligibility.PurposeTuning, path))
	if !errors.Is(err, eligibility.ErrNotEligible) || d.Allowed {
		t.Fatalf("tuning without a manifest: %v %+v", err, d)
	}
	if c := codes(d); !has(c, eligibility.CodeNotConfigured) || !has(c, eligibility.CodeUnassigned) {
		t.Fatal(c)
	}
	if got := e.uses(); len(got) != 4 || got[0].Decision != "denied" {
		t.Fatalf("the refusal is recorded once per measure family: %+v", got)
	}
	prod, err := s.Authorize(e.request(s, eligibility.PurposeProduction, path))
	if err != nil || !prod.Allowed || prod.Gated || prod.Recorded {
		t.Fatalf("production: %v %+v", err, prod)
	}
	if len(e.uses()) != 4 {
		t.Fatal("production must not be recorded")
	}
	st := s.Status()
	if st.Complete || st.Manifest.State != "absent" || st.Purposes[0].Available {
		t.Fatalf("%+v", st)
	}
}

// A capture the manifest assigns to development may be tuned on, and the use
// is recorded with its provenance. Renaming the file changes nothing.
func TestDevelopmentCaptureByHashSurvivesRename(t *testing.T) {
	e := newEnv(t)
	path, sha := fx.Capture(t, e.captures, "a.pcapng", "development bytes")
	e.manifest = fx.Manifest(t, e.root, map[string]any{
		"sites":    []any{fx.Site("dev-site", "development"), fx.Site("qual-site", "qualification")},
		"captures": []any{fx.CaptureEntry("cap-a", "dev-site", sha, "a.pcapng")},
	})
	s := e.service()
	req := e.request(s, eligibility.PurposeTuning, path)
	req.Measures = []eligibility.Measure{eligibility.MeasureIdentity}
	req.RunID, req.Experiment, req.ConfigDigest = "run-1", "l5id0014", "sha256:cfg"
	d, err := s.Authorize(req)
	if err != nil || !d.Allowed || !d.Recorded {
		t.Fatalf("%v %+v", err, d)
	}
	u := e.uses()
	if len(u) != 1 || u[0].Decision != "allowed" || u[0].SiteID != "dev-site" || u[0].CaptureSHA256 != sha ||
		u[0].RunID != "run-1" || u[0].Experiment != "l5id0014" || u[0].PolicyDigest != d.PolicyDigest || u[0].Actor != "tester" {
		t.Fatalf("%+v", u)
	}
	renamed := filepath.Join(e.captures, "fresh-heldout.pcapng")
	if err := os.Rename(path, renamed); err != nil {
		t.Fatal(err)
	}
	d, err = s.Authorize(e.request(s, eligibility.PurposeTuning, renamed))
	if err != nil || d.Inputs[0].Assignment.SiteID != "dev-site" {
		t.Fatalf("a renamed capture keeps its assignment: %v %+v", err, d)
	}
}

// Unknown, conflicting and hash-mismatched captures fail closed; so does an
// invalid manifest, including one that splits a site across cohorts.
func TestUnknownConflictingAndInvalidFailClosed(t *testing.T) {
	e := newEnv(t)
	_, sha := fx.Capture(t, e.captures, "a.pcapng", "listed bytes")
	edited, _ := fx.Capture(t, t.TempDir(), "a.pcapng", "edited bytes")
	unknown, _ := fx.Capture(t, e.captures, "b.pcapng", "never listed")
	e.manifest = fx.Manifest(t, e.root, map[string]any{
		"sites":    []any{fx.Site("dev-site", "development")},
		"captures": []any{fx.CaptureEntry("cap-a", "dev-site", sha, "a.pcapng")},
	})
	s := e.service()
	d, _ := s.Authorize(e.request(s, eligibility.PurposeDevelopmentEvaluation, unknown))
	if d.Allowed || !has(codes(d), eligibility.CodeUnassigned) {
		t.Fatalf("unknown: %+v", d)
	}
	d, _ = s.Authorize(e.request(s, eligibility.PurposeDevelopmentEvaluation, edited))
	if d.Allowed || !has(codes(d), eligibility.CodeConflict) {
		t.Fatalf("same name, other bytes: %+v", d)
	}
	d, _ = s.Authorize(e.request(s, eligibility.PurposeTuning, filepath.Join(e.captures, "missing.pcapng")))
	if d.Allowed || !has(codes(d), eligibility.CodeIdentityUnknown) {
		t.Fatalf("unreadable: %+v", d)
	}
	cap := fx.CaptureEntry("cap-a", "dev-site", sha, "a.pcapng")
	cap["cohort"] = "qualification"
	e.manifest = fx.Manifest(t, e.root, map[string]any{"sites": []any{fx.Site("dev-site", "development")}, "captures": []any{cap}})
	s = e.service()
	d, _ = s.Authorize(e.request(s, eligibility.PurposeTuning, unknown))
	if d.Allowed || !has(codes(d), eligibility.CodeManifestInvalid) {
		t.Fatalf("a site split across cohorts invalidates the manifest: %+v", d)
	}
	if st := s.Status(); st.Manifest.State != "invalid" || !strings.Contains(st.Manifest.Error, "conflicts with site") {
		t.Fatalf("%+v", st.Manifest)
	}
	dup := fx.CaptureEntry("cap-b", "dev-site", sha, "copy.pcapng")
	e.manifest = fx.Manifest(t, e.root, map[string]any{"sites": []any{fx.Site("dev-site", "development")},
		"captures": []any{fx.CaptureEntry("cap-a", "dev-site", sha, "a.pcapng"), dup}})
	if d, _ := e.service().Authorize(e.request(e.service(), eligibility.PurposeTuning, unknown)); !has(codes(d), eligibility.CodeManifestInvalid) {
		t.Fatalf("one capture with two entries: %+v", d)
	}
}

// What a frozen split holds out is refused to tuning whether or not the
// request names the split, under a new run ID and after a rename, once the
// label set is bound to the capture. A held-out score under the split and an
// annotation review both read it, and both are recorded.
func TestHeldOutSplitProtectsWithoutTheSplitArgument(t *testing.T) {
	e := newEnv(t)
	path, sha := fx.Capture(t, e.captures, "efolsom.pcapng", "held-out bytes")
	e.manifest = fx.Manifest(t, e.root, map[string]any{
		"sites":    []any{fx.Site("efolsom", "development")},
		"captures": []any{fx.CaptureEntry("efolsom-1", "efolsom", sha, "efolsom.pcapng")},
	})
	p := fx.Pack(t, e.root, fx.PackOptions{Capture: "efolsom", Run: "run-a", PCAPBasename: "efolsom.pcapng"})
	f := fx.Freeze(t, e.root, "label0001-r1.json", p, annotation.SplitRoleHeldOut, nil)
	s := e.service()

	d, err := s.Authorize(e.request(s, eligibility.PurposeTuning, path))
	if err == nil || !has(codes(d), eligibility.CodeHeldOut) {
		t.Fatalf("tuning on held-out data without naming the split: %+v", d)
	}
	if st := s.Status(); st.Registry.HeldOutSpans != 1 || st.Registry.WeakIdentity != 1 {
		t.Fatalf("unbound label set is a weak identity: %+v", st.Registry)
	}
	// Bound to its capture, a renamed copy is still the held-out capture.
	e.extra = map[string][]string{p.Manifest.PackDigest: {sha}}
	copyPath := filepath.Join(t.TempDir(), "renamed-new-capture.pcapng")
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(copyPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	req := e.request(s, eligibility.PurposeTuning, copyPath)
	req.RunID = "brand-new-run"
	if d, err := s.Authorize(req); err == nil || !has(codes(d), eligibility.CodeHeldOut) {
		t.Fatalf("renamed copy under a new run ID: %+v", d)
	}
	if d, err := s.Authorize(e.request(s, eligibility.PurposeDeveloperDiagnostic, copyPath)); err == nil || !has(codes(d), eligibility.CodeHeldOut) {
		t.Fatalf("diagnostics: %+v", d)
	}
	score := e.request(s, eligibility.PurposeDevelopmentEvaluation, path)
	score.HeldOutScore, score.SplitDigest = true, f.SplitDigest
	if d, err := s.Authorize(score); err != nil || !d.Recorded {
		t.Fatalf("held-out score under the split: %v %+v", err, d)
	}
	if d, err := s.Authorize(e.request(s, eligibility.PurposeAnnotationReview, path)); err != nil || !d.Allowed {
		t.Fatalf("annotation review: %v %+v", err, d)
	}
	summary, err := e.ledger.Summary()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range summary {
		if row.HeldOutScores != 1 || row.ReviewUses != 1 || row.DevelopmentUses != 0 || row.Denied != 3 {
			t.Fatalf("%+v", row)
		}
	}
	// A client cannot claim a held-out score without naming a split.
	claim := e.request(s, eligibility.PurposeTuning, path)
	claim.HeldOutScore = true
	if d, _ := s.Authorize(claim); !has(codes(d), eligibility.CodeBadRequest) {
		t.Fatalf("%+v", d)
	}
}

// A new split lineage does not forget what another lineage tuned on, and a
// measure read for development is not held out for that measure afterwards.
// An unread measure does not override a stricter cohort.
func TestExposureIsMeasureSpecificAndCrossesLineages(t *testing.T) {
	e := newEnv(t)
	kirk, kirkSHA := fx.Capture(t, e.captures, "kirk0.pcapng", "kirk bytes")
	other, otherSHA := fx.Capture(t, e.captures, "c.pcapng", "other bytes")
	val, valSHA := fx.Capture(t, e.captures, "v.pcapng", "validation bytes")
	e.manifest = fx.Manifest(t, e.root, map[string]any{
		"sites": []any{fx.Site("kirk", "development"), fx.Site("val", "validation")},
		"captures": []any{fx.CaptureEntry("kirk0", "kirk", kirkSHA, "kirk0.pcapng"), fx.CaptureEntry("c", "kirk", otherSHA, "c.pcapng"),
			fx.CaptureEntry("v", "val", valSHA, "v.pcapng")},
	})
	p := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk0", Run: "run-a", PCAPBasename: "kirk0.pcapng"})
	fx.Freeze(t, e.root, "kirk0-tuning-r1.json", p, annotation.SplitRoleTuning, nil)
	e.extra = map[string][]string{p.Manifest.PackDigest: {kirkSHA}}
	s := e.service()

	score := e.request(s, eligibility.PurposeDevelopmentEvaluation, kirk)
	score.HeldOutScore, score.SplitDigest = true, "sha256:another-lineage"
	if d, _ := s.Authorize(score); !has(codes(d), eligibility.CodeTunedInLineage) {
		t.Fatalf("held out in a new lineage after tuning in another: %+v", d)
	}

	tune := e.request(s, eligibility.PurposeTuning, other)
	tune.Measures = []eligibility.Measure{eligibility.MeasureIdentity}
	if _, err := s.Authorize(tune); err != nil {
		t.Fatal(err)
	}
	identity := e.request(s, eligibility.PurposeDevelopmentEvaluation, other)
	identity.HeldOutScore, identity.SplitDigest = true, "sha256:next"
	identity.Measures = []eligibility.Measure{eligibility.MeasureIdentity}
	if d, _ := s.Authorize(identity); !has(codes(d), eligibility.CodeExposed) {
		t.Fatalf("identity was read for development: %+v", d)
	}
	geometry := identity
	geometry.Measures = []eligibility.Measure{eligibility.MeasureGeometry}
	geometry.Inputs = []eligibility.Input{s.CaptureInput(other, 0, 0)}
	if d, err := s.Authorize(geometry); err != nil {
		t.Fatalf("geometry was never read: %v %+v", err, d)
	}
	strict := geometry
	strict.Inputs = []eligibility.Input{s.CaptureInput(val, 0, 0)}
	if d, _ := s.Authorize(strict); d.Allowed || !has(codes(d), eligibility.CodeValidation) {
		t.Fatalf("an unread measure on a validation capture: %+v", d)
	}
}

// The blind qualification cohort refuses development, and refuses its own
// review and evaluation while no storage-level access control exists. With
// one in place a custodian's evaluation is allowed and recorded.
func TestQualificationNeedsRealAccessControls(t *testing.T) {
	e := newEnv(t)
	path, sha := fx.Capture(t, e.captures, "q.pcapng", "qualification bytes")
	e.manifest = fx.Manifest(t, e.root, map[string]any{
		"sites":    []any{fx.Site("q-site", "qualification")},
		"captures": []any{fx.CaptureEntry("q", "q-site", sha, "q.pcapng")},
		"access":   map[string]any{"qualification_storage_restricted": true, "custodians": []string{"custodian"}},
	})
	s := e.service()
	if d, _ := s.Authorize(e.request(s, eligibility.PurposeTuning, path)); !has(codes(d), eligibility.CodeQualification) {
		t.Fatalf("%+v", d)
	}
	req := e.request(s, eligibility.PurposeQualificationEvaluation, path)
	req.Actor = "custodian"
	if d, _ := s.Authorize(req); !has(codes(d), eligibility.CodeAccessAbsent) {
		t.Fatalf("a manifest's claim is not an access control: %+v", d)
	}
	e.access = fakeAccess{}
	s = e.service()
	req.Inputs = []eligibility.Input{s.CaptureInput(path, 0, 0)}
	d, err := s.Authorize(req)
	if err != nil || !d.Recorded {
		t.Fatalf("authorised qualification evaluation: %v %+v", err, d)
	}
	req.Actor = "developer"
	if d, _ := s.Authorize(req); !has(codes(d), eligibility.CodeNotCustodian) {
		t.Fatalf("%+v", d)
	}
}

type fakeAccess struct{}

func (fakeAccess) QualificationStorageEnforced() bool { return true }

// A live sweep needs its sensor assigned; an unreadable split fails every
// development request closed; and a use that cannot be recorded is refused.
func TestLiveUnreadableSplitAndNoLedger(t *testing.T) {
	e := newEnv(t)
	path, sha := fx.Capture(t, e.captures, "a.pcapng", "dev bytes")
	e.manifest = fx.Manifest(t, e.root, map[string]any{
		"sites": []any{fx.Site("dev", "development")}, "captures": []any{fx.CaptureEntry("a", "dev", sha, "a.pcapng")},
		"live_sources": []any{map[string]any{"sensor_id": "hesai-dev", "site_id": "dev"}},
	})
	s := e.service()
	live := eligibility.Request{Purpose: eligibility.PurposeTuning, EntryPoint: "test", Actor: "t",
		Inputs: []eligibility.Input{{Identity: eligibility.Live("hesai-other")}}}
	if d, _ := s.Authorize(live); !has(codes(d), eligibility.CodeUnassigned) {
		t.Fatalf("%+v", d)
	}
	live.Inputs = []eligibility.Input{{Identity: eligibility.Live("hesai-dev")}}
	if _, err := s.Authorize(live); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(e.root, "splits"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.root, "splits", "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Authorize(e.request(s, eligibility.PurposeTuning, path)); !has(codes(d), eligibility.CodeRegistryIncomplete) {
		t.Fatalf("%+v", d)
	}
	os.Remove(filepath.Join(e.root, "splits", "broken.json"))
	noLedger := eligibility.New(eligibility.Config{ManifestPath: e.manifest, SplitsDir: filepath.Join(e.root, "splits")})
	if d, _ := noLedger.Authorize(e.request(noLedger, eligibility.PurposeTuning, path)); !has(codes(d), eligibility.CodeLedgerUnavailable) {
		t.Fatalf("%+v", d)
	}
	noRoot := eligibility.New(eligibility.Config{ManifestPath: e.manifest, Ledger: e.ledger})
	if d, _ := noRoot.Authorize(e.request(noRoot, eligibility.PurposeTuning, path)); !has(codes(d), eligibility.CodeRegistryIncomplete) {
		t.Fatalf("no annotation root: %+v", d)
	}
}

// The ledger is append-only.
func TestLedgerIsAppendOnly(t *testing.T) {
	e := newEnv(t)
	path, _ := fx.Capture(t, e.captures, "a.pcapng", "x")
	s := e.service()
	_, _ = s.Authorize(e.request(s, eligibility.PurposeTuning, path))
	if _, err := e.ledger.DB.Exec(`DELETE FROM lidar_data_uses`); err == nil {
		t.Fatal("delete allowed")
	}
	if _, err := e.ledger.DB.Exec(`UPDATE lidar_data_uses SET decision = 'allowed'`); err == nil {
		t.Fatal("update allowed")
	}
}
