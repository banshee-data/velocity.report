package perframeeval

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// The scene is evalfixture's near-face fixture: a car of length 4.4 m and width
// 1.8 m driving along +x at 10 m/s, 30 m ahead of the sensor and 5 m to its
// left at sample 0. The sensor is at the origin, behind the car and to its
// right, so it sees the rear face and the right face; the reviewed mask holds
// exactly those returns, ten on each face. A body that puts both faces where
// they are has zero residuals, and any other has the offset.
const (
	nfLength  = evalfixture.NearFaceLength
	nfWidth   = evalfixture.NearFaceWidth
	nfSamples = evalfixture.NearFaceSamples
	nfObject  = evalfixture.NearFaceObject
)

type nfBody = evalfixture.NearFaceBody
type nfOptions = evalfixture.NearFaceOptions

type nfFixture struct{ packDir, splitPath string }

func writeNearFacePack(t *testing.T, o nfOptions) nfFixture {
	t.Helper()
	f, err := evalfixture.WriteNearFacePack(t.TempDir(), o)
	if err != nil {
		t.Fatal(err)
	}
	return nfFixture{packDir: f.PackDir, splitPath: f.SplitManifestPath}
}

func writeNearFaceDB(t *testing.T, path string, arms map[string]nfBody) {
	t.Helper()
	if err := evalfixture.WriteNearFaceDB(path, arms); err != nil {
		t.Fatal(err)
	}
}

func nfOpts(f nfFixture) NearFaceOptions {
	o := DefaultNearFaceOptions()
	o.PackDir, o.SplitManifestPath, o.Split, o.AllowTuningSplit = f.packDir, f.splitPath, "tune", true
	return o
}

func nfArm(label, dbPath, params string) ArmSpec {
	return ArmSpec{Label: label, DBPath: dbPath, ParamHash: params, Stage: StageOnline, DeclaredBaseline: true}
}

func nfNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-3 {
		t.Fatalf("%s = %.4f, want %.4f", name, got, want)
	}
}

func nfStratum(t *testing.T, r NearFaceArmResult, name string) NearFaceStratum {
	t.Helper()
	for _, s := range r.Strata {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("arm %s has no stratum %q (has %+v)", r.Arm.Label, name, r.Strata)
	return NearFaceStratum{}
}

func TestNearFaceResidualsAreTheOffsetOfTheBelievedFaces(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{
		"exact":      {},
		"sensorward": {DX: -0.3},               // centre 0.3 m toward the sensor along the car
		"left":       {DY: 0.2},                // centre 0.2 m to the left
		"long":       {Length: nfLength + 0.4}, // length believed 0.4 m too long: rear face 0.2 m out
	})
	opts := nfOpts(f)
	opts.IncludeInstants = true
	report, err := ScoreNearFaces(opts, []ArmSpec{
		nfArm("exact", dbPath, "exact"), nfArm("sensorward", dbPath, "sensorward"),
		nfArm("left", dbPath, "left"), nfArm("long", dbPath, "long"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != NearFaceSchema || len(report.Arms) != 4 || report.Reference.HeldOut || len(report.Caveats) < 5 {
		t.Fatalf("report %+v", report)
	}
	byLabel := map[string]NearFaceArmResult{}
	for _, a := range report.Arms {
		byLabel[a.Arm.Label] = a
		if a.Accounting.Masks != nfSamples || a.Accounting.Matched != nfSamples || a.Accounting.Scored != nfSamples || len(a.Instants) != nfSamples {
			t.Fatalf("%s accounting %+v", a.Arm.Label, a.Accounting)
		}
		if a.Arm.Table != "lidar_track_solid_bodies" {
			t.Fatalf("%s reads %q", a.Arm.Label, a.Arm.Table)
		}
	}

	exact := nfStratum(t, byLabel["exact"], "all")
	nfNear(t, "exact end normal", exact.EndNormal.Mean, 0)
	nfNear(t, "exact side normal", exact.SideNormal.Mean, 0)
	nfNear(t, "exact tangent", exact.EndTangent.MeanAbs, 0)
	if exact.EndNormal.N != nfSamples || exact.EndTangent.N != nfSamples {
		t.Fatalf("exact counts %+v", exact)
	}

	// 0.3 m toward the sensor along the axis: the believed rear face is 0.3 m
	// beyond the returns, toward the sensor. The side face does not move.
	sensorward := nfStratum(t, byLabel["sensorward"], "all")
	nfNear(t, "sensorward end normal", sensorward.EndNormal.Mean, 0.3)
	nfNear(t, "sensorward side normal", sensorward.SideNormal.Mean, 0)

	// 0.2 m to the left, away from the sensor's side: the believed right face
	// is inside the returns (negative), and the face span's midpoint is 0.2 m
	// right of the believed centre.
	left := nfStratum(t, byLabel["left"], "all")
	nfNear(t, "left side normal", left.SideNormal.Mean, -0.2)
	nfNear(t, "left end normal", left.EndNormal.Mean, 0)
	nfNear(t, "left tangent", left.EndTangent.MeanAbs, 0.2)

	long := nfStratum(t, byLabel["long"], "all")
	nfNear(t, "long end normal", long.EndNormal.Mean, 0.2)

	// The paired comparison is against the first arm: 0.3 m worse on the end face.
	var paired *NearFacePair
	for i := range byLabel["sensorward"].Paired {
		if byLabel["sensorward"].Paired[i].Face == "end_normal" {
			paired = &byLabel["sensorward"].Paired[i]
		}
	}
	if paired == nil || paired.Against != "exact" || paired.Both != nfSamples {
		t.Fatalf("paired %+v", byLabel["sensorward"].Paired)
	}
	nfNear(t, "paired delta", paired.Delta, 0.3)
	nfNear(t, "share lower", paired.ThisLower, 0)
	if len(byLabel["exact"].Paired) != 0 {
		t.Fatalf("the first arm is compared with itself: %+v", byLabel["exact"].Paired)
	}
	// Every instant carries the geometry it was scored on.
	in := byLabel["exact"].Instants[0]
	if in.SampleID != 0 || in.ObjectID != nfObject || in.Returns != 20 || in.EndNormalM == nil || in.SideNormalM == nil || !in.ExtentEvidence {
		t.Fatalf("instant %+v", in)
	}
	if in.SensorAxialM >= -nfLength/2 || in.SensorLateralM >= -nfWidth/2 {
		t.Fatalf("the sensor should be behind and to the right: %+v", in)
	}
	nfNear(t, "range", in.RangeM, math.Hypot(30, 5))
	// The centre distance is the sanity check that arm and pack share a frame.
	// The mask holds only the faces the sensor sees, so its footprint centre is
	// 1.2 m toward the sensor from the body's: the offset the mask position has.
	nfNear(t, "centre distance", byLabel["exact"].Accounting.CentreDistance.Median, 1.2)
}

func TestNearFaceAccountsForWhatItDoesNotScore(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{
		"medoid":    {Ref: l5tracks.ReferenceClusterMedoid},
		"noheading": {NoHeading: true},
		"far":       {Far: true},
		"gappy":     {Missing: func(i int) bool { return i%2 == 0 }},
		"prior":     {Prior: true},
		"noextent":  {NoExtent: true},
	})
	opts := nfOpts(f)
	score := func(params string, tweak func(*NearFaceOptions)) NearFaceArmResult {
		o := opts
		if tweak != nil {
			tweak(&o)
		}
		r, err := ScoreNearFaces(o, []ArmSpec{nfArm(params, dbPath, params)})
		if err != nil {
			t.Fatalf("%s: %v", params, err)
		}
		return r.Arms[0]
	}
	if a := score("medoid", nil); a.Accounting.Unscored[NearFaceNotBodyCentre] != nfSamples || a.Accounting.Scored != 0 {
		t.Fatalf("medoid %+v", a.Accounting)
	}
	if a := score("noheading", nil); a.Accounting.Unscored[NearFaceNoOrientation] != nfSamples {
		t.Fatalf("noheading %+v", a.Accounting)
	}
	if a := score("far", nil); a.Accounting.Unscored[NearFaceUnmatched] != nfSamples || a.Accounting.Matched != 0 {
		t.Fatalf("far %+v", a.Accounting)
	}
	if a := score("gappy", nil); a.Accounting.Unscored[NearFaceUnmatched] != nfSamples/2 || a.Accounting.Scored != nfSamples/2 {
		t.Fatalf("gappy %+v", a.Accounting)
	}
	// A prior extent is scored, in its own stratum.
	a := score("prior", nil)
	if a.Accounting.Scored != nfSamples {
		t.Fatalf("prior %+v", a.Accounting)
	}
	nfStratum(t, a, "extent_class_prior")
	for _, s := range a.Strata {
		if s.Name == "extent_evidence" {
			t.Fatalf("a prior arm has an evidence stratum")
		}
	}
	// Too few returns, and a sensor inside the believed box.
	if a := score("prior", func(o *NearFaceOptions) { o.MinReturns = 50 }); a.Accounting.Unscored[NearFaceTooFewReturns] != nfSamples {
		t.Fatalf("min returns %+v", a.Accounting)
	}
	if a := score("prior", func(o *NearFaceOptions) { o.SensorXM, o.SensorYM = 30, 5 }); a.Accounting.Unscored[NearFaceNoVisibleFace] == 0 {
		t.Fatalf("a sensor inside the box saw a face: %+v", a.Accounting)
	}
	// A body that believes no extent has no face to place.
	if a := score("noextent", nil); a.Accounting.Unscored[NearFaceNoExtent] != nfSamples {
		t.Fatalf("no extent %+v", a.Accounting)
	}
}

func TestNearFaceWithholdsATangentTheFaceCannotSupport(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{"exact": {}})
	arm := []ArmSpec{nfArm("exact", dbPath, "exact")}
	run := func(tweak func(*NearFaceOptions)) NearFaceArmResult {
		o := nfOpts(f)
		tweak(&o)
		r, err := ScoreNearFaces(o, arm)
		if err != nil {
			t.Fatal(err)
		}
		return r.Arms[0]
	}
	// The span covers the whole Width: asking for more than that withholds it,
	// and so does allowing less, but the normal residual still counts.
	a := run(func(o *NearFaceOptions) { o.MinSpanCoverage, o.MaxSpanCoverage = 1.2, 2 })
	if a.Accounting.Unscored[nearFaceTangentPartial] != nfSamples || nfStratum(t, a, "all").EndTangent.N != 0 || nfStratum(t, a, "all").EndNormal.N != nfSamples {
		t.Fatalf("partial %+v / %+v", a.Accounting, nfStratum(t, a, "all"))
	}
	a = run(func(o *NearFaceOptions) { o.MinSpanCoverage, o.MaxSpanCoverage = 0.5, 0.8 })
	if a.Accounting.Unscored[nearFaceTangentWide] != nfSamples {
		t.Fatalf("wide %+v", a.Accounting)
	}
	// Twenty returns in all but only twelve within the end face's band.
	a = run(func(o *NearFaceOptions) { o.MinReturns = 14 })
	if a.Accounting.Unscored[nearFaceTangentTooFew] != nfSamples || a.Accounting.Scored != nfSamples {
		t.Fatalf("too few %+v", a.Accounting)
	}
	// A third of the rear face is a partial face.
	g := writeNearFacePack(t, nfOptions{PartialRear: true})
	o := nfOpts(g)
	r, err := ScoreNearFaces(o, arm)
	if err != nil {
		t.Fatal(err)
	}
	if r.Arms[0].Accounting.Unscored[nearFaceTangentPartial] != nfSamples {
		t.Fatalf("partial rear %+v", r.Arms[0].Accounting)
	}
}

func TestNearFaceRefusesWhatItCannotScore(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{"exact": {}})
	arm := []ArmSpec{nfArm("exact", dbPath, "exact")}
	bad := map[string]func(*NearFaceOptions){
		"negative tolerance": func(o *NearFaceOptions) { o.FrameToleranceNanos = -1 },
		"negative slack":     func(o *NearFaceOptions) { o.GateSlackMetres = -1 },
		"one return":         func(o *NearFaceOptions) { o.MinReturns = 1 },
		"quantile too high":  func(o *NearFaceOptions) { o.FaceQuantile = 0.5 },
		"negative quantile":  func(o *NearFaceOptions) { o.FaceQuantile = -0.1 },
		"no band":            func(o *NearFaceOptions) { o.FaceBandMetres = 0 },
		"no min coverage":    func(o *NearFaceOptions) { o.MinSpanCoverage = 0 },
		"max below min":      func(o *NearFaceOptions) { o.MaxSpanCoverage = 0.1 },
		"bad policy":         func(o *NearFaceOptions) { o.Policy.Position = "nowhere" },
		"no split":           func(o *NearFaceOptions) { o.Split = "" },
		"unknown split":      func(o *NearFaceOptions) { o.Split = "nope" },
		"tuning not allowed": func(o *NearFaceOptions) { o.AllowTuningSplit = false },
		"missing pack":       func(o *NearFaceOptions) { o.PackDir = filepath.Join(o.PackDir, "missing") },
		"missing split":      func(o *NearFaceOptions) { o.SplitManifestPath = filepath.Join(filepath.Dir(o.PackDir), "missing.json") },
	}
	for name, tweak := range bad {
		o := nfOpts(f)
		tweak(&o)
		if _, err := ScoreNearFaces(o, arm); err == nil {
			t.Fatalf("%s: scored", name)
		}
	}
	if _, err := ScoreNearFaces(nfOpts(f), nil); err == nil || !strings.Contains(err.Error(), "no arm") {
		t.Fatalf("no arms: %v", err)
	}
	if _, err := ScoreNearFaces(nfOpts(f), []ArmSpec{nfArm("x", filepath.Join(t.TempDir(), "absent.db"), "")}); err == nil {
		t.Fatal("an absent database scored")
	}
	// An object that is only proposed has no scored mask.
	p := writeNearFacePack(t, nfOptions{Proposed: true})
	if _, err := ScoreNearFaces(nfOpts(p), arm); err == nil || !strings.Contains(err.Error(), "no scored mask") {
		t.Fatalf("Proposed: %v", err)
	}
	// A pinned revision the pack does not have.
	m, err := annotation.LoadSplitManifest(f.splitPath)
	if err != nil {
		t.Fatal(err)
	}
	m.SidecarRevision = 99
	pinned := filepath.Join(t.TempDir(), "pinned.json")
	if err := evalfixture.WriteSplitManifest(pinned, *m); err != nil {
		t.Fatal(err)
	}
	o := nfOpts(f)
	o.SplitManifestPath = pinned
	if _, err := ScoreNearFaces(o, arm); err == nil {
		t.Fatal("a missing pinned revision scored")
	}
	// A split manifest for another pack.
	m.SidecarRevision, m.PackDigest = 1, "sha256:other"
	other := filepath.Join(t.TempDir(), "other.json")
	if err := evalfixture.WriteSplitManifest(other, *m); err != nil {
		t.Fatal(err)
	}
	o.SplitManifestPath = other
	if _, err := ScoreNearFaces(o, arm); err == nil {
		t.Fatal("another pack's manifest scored")
	}
}

func TestNearFaceMatchesOneToOneNearestFirst(t *testing.T) {
	masks := []nearFaceMask{
		{sampleID: 0, timestamp: 1_000_000_000, object: "a", cx: 0, cy: 0, diag: 1},
		{sampleID: 0, timestamp: 1_000_000_000, object: "b", cx: 0.4, cy: 0, diag: 1},
	}
	bodies := []PredictedBody{
		{TrackKey: "t1", TimestampNs: 1_000_000_000, XM: 0.5, YM: 0},
		{TrackKey: "t2", TimestampNs: 1_000_000_000, XM: 9, YM: 0},
		{TrackKey: "t3", TimestampNs: 2_000_000_000, XM: 0, YM: 0}, // another frame
	}
	opts := DefaultNearFaceOptions()
	got := matchNearFaces(masks, bodies, opts)
	// Mask b is nearest to t1 (0.1 m), so a, which would take it at 0.5 m, gets nothing.
	if len(got) != 1 || got[1] != 0 {
		t.Fatalf("matches %v", got)
	}
	// The same distance breaks by mask, then by body, so the result is stable.
	tie := []nearFaceMask{
		{timestamp: 1_000_000_000, cx: 0, cy: 0, diag: 1}, {timestamp: 1_000_000_000, cx: 0, cy: 0, diag: 1},
	}
	two := []PredictedBody{{TimestampNs: 1_000_000_000, XM: 0.2}, {TimestampNs: 1_000_000_000, XM: 0.2}}
	if m := matchNearFaces(tie, two, opts); len(m) != 2 || m[0] != 0 || m[1] != 1 {
		t.Fatalf("tie %v", m)
	}
	// A physical body is placed at its centre, not its anchor.
	phys := []PredictedBody{{TimestampNs: 1_000_000_000, Physical: true, XM: 50, CentreXM: 0.1}}
	if m := matchNearFaces(masks[:1], phys, opts); len(m) != 1 {
		t.Fatalf("physical %v", m)
	}
}

func TestResidualStatistics(t *testing.T) {
	if s := summariseResiduals(nil); s.N != 0 || s.Mean != 0 {
		t.Fatalf("empty %+v", s)
	}
	s := summariseResiduals([]float64{-0.2, 0.0, 0.05, 0.3})
	nfNear(t, "mean", s.Mean, 0.0375)
	nfNear(t, "median", s.Median, 0.025)
	nfNear(t, "mean abs", s.MeanAbs, 0.1375)
	nfNear(t, "max abs", s.MaxAbs, 0.3)
	nfNear(t, "share over 10 cm", s.Over10cm, 0.5)
	nfNear(t, "p95", s.P95Abs, 0.285) // 0.2 + 0.85 of the way to 0.3
	if one := summariseResiduals([]float64{-0.4}); one.Median != -0.4 || one.P99Abs != 0.4 {
		t.Fatalf("single %+v", one)
	}
	for r, want := range map[float64]string{0: "range_0_15m", 14.9: "range_0_15m", 15: "range_15_30m", 30: "range_30_60m", 60: "range_60m_plus"} {
		if got := rangeStratum(r); got != want {
			t.Fatalf("rangeStratum(%g) = %s, want %s", r, got, want)
		}
	}
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	nfNear(t, "outward high", outwardExtreme(vals, 1, 0.1), 9)
	nfNear(t, "outward low", outwardExtreme(vals, -1, 0.1), 2)
	nfNear(t, "nothing trimmed below the share", outwardExtreme(vals, 1, 0.05), 10)
}

func TestNearFaceKeepsOnlyTheFramesTheStrataNeed(t *testing.T) {
	// Instants spread over every range band come out in the fixed order.
	acc := map[string]*stratumAccum{}
	for _, name := range []string{"range_60m_plus", "all", "range_15_30m", "extent_class_prior"} {
		acc[name] = &stratumAccum{}
	}
	if got := strings.Join(orderedStrata(acc), ","); got != "all,extent_class_prior,range_15_30m,range_60m_plus" {
		t.Fatalf("order %s", got)
	}
	a := &stratumAccum{}
	one, two := 0.1, -0.2
	a.add(NearFaceInstant{EndNormalM: &one, SideNormalM: &two, EndTangentM: &two})
	a.add(NearFaceInstant{})
	st := a.stratum("x")
	if st.EndNormal.N != 1 || st.SideNormal.N != 1 || st.EndTangent.N != 1 || st.EndTangent.Mean != 0.2 {
		t.Fatalf("stratum %+v", st)
	}
}

func TestNearFaceReportsATuningSplitAsNotHeldOut(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{"exact": {}})
	r, err := ScoreNearFaces(nfOpts(f), []ArmSpec{nfArm("exact", dbPath, "exact")})
	if err != nil {
		t.Fatal(err)
	}
	var said bool
	for _, c := range r.Caveats {
		said = said || strings.Contains(c, "may not be quoted as held out")
	}
	if !said || r.Reference.Split != "tune" || r.Reference.SplitRole != annotation.SplitRoleTuning || r.Reference.SidecarRevision != 1 ||
		len(r.Reference.Episodes) != 1 || r.Reference.PackDigest == "" || r.Reference.FrozenDigest != "" {
		t.Fatalf("reference %+v / caveats %v", r.Reference, r.Caveats)
	}
}

func TestNearFaceReadsAFrozenSplit(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	m, err := annotation.LoadSplitManifest(f.splitPath)
	if err != nil {
		t.Fatal(err)
	}
	draft := annotation.SplitDraft{
		Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
		Packs: []annotation.DraftPack{{Dir: f.packDir, Splits: m.Splits, Episodes: m.Episodes}},
	}
	frozen, err := annotation.FreezeSplit(annotation.FreezeOptions{
		Draft: &draft, Author: "operator", Now: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		BuildVersion: "test", BuildGitSHA: "abc", GuardSeconds: annotation.DefaultSplitGuardSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "frozen.json")
	if err := annotation.WriteFrozenSplit(path, frozen); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{"exact": {}})
	o := nfOpts(f)
	o.SplitManifestPath = path
	r, err := ScoreNearFaces(o, []ArmSpec{nfArm("exact", dbPath, "exact")})
	if err != nil {
		t.Fatal(err)
	}
	if r.Reference.FrozenDigest != frozen.SplitDigest || r.Arms[0].Accounting.Scored != nfSamples {
		t.Fatalf("frozen: %+v / %+v", r.Reference, r.Arms[0].Accounting)
	}
}

func TestNearFaceInstantsAreOrderedAndPairsSkipWhatOneArmLacks(t *testing.T) {
	a, b := 0.1, 0.3
	in := map[instantKey]NearFaceInstant{
		{1, "z"}: {SampleID: 1, ObjectID: "z"}, {0, "y"}: {SampleID: 0, ObjectID: "y"}, {0, "x"}: {SampleID: 0, ObjectID: "x"},
	}
	got := sortedInstants(in)
	if got[0].ObjectID != "x" || got[1].ObjectID != "y" || got[2].ObjectID != "z" {
		t.Fatalf("order %+v", got)
	}
	this := NearFaceArmResult{Arm: ArmIdentity{Label: "this"}, scored: map[instantKey]NearFaceInstant{
		{0, "o"}: {EndNormalM: &a, SideNormalM: &a}, {1, "o"}: {EndNormalM: &b}, {2, "o"}: {EndNormalM: &a},
	}}
	other := NearFaceArmResult{Arm: ArmIdentity{Label: "other"}, scored: map[instantKey]NearFaceInstant{
		{0, "o"}: {EndNormalM: &b, SideNormalM: &b}, {1, "o"}: {EndNormalM: &a}, // no side residual at 1; sample 2 absent
	}}
	pairs := pairNearFaces(this, other)
	if len(pairs) != 2 || pairs[0].Both != 2 || pairs[1].Both != 1 {
		t.Fatalf("pairs %+v", pairs)
	}
	nfNear(t, "end delta", pairs[0].Delta, 0)         // |0.1|,|0.3| against |0.3|,|0.1|
	nfNear(t, "share lower", pairs[0].ThisLower, 0.5) // better at sample 0 only
	nfNear(t, "side delta", pairs[1].Delta, -0.2)
	if empty := pairNearFaces(NearFaceArmResult{scored: map[instantKey]NearFaceInstant{}}, other); empty[0].Both != 0 || empty[0].Delta != 0 {
		t.Fatalf("empty %+v", empty)
	}
}

func TestNearFaceMarkdownReadsAsTheReportDoes(t *testing.T) {
	f := writeNearFacePack(t, nfOptions{})
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{"exact": {}, "sensorward": {DX: -0.3}, "medoid": {Ref: l5tracks.ReferenceClusterMedoid}})
	r, err := ScoreNearFaces(nfOpts(f), []ArmSpec{nfArm("exact", dbPath, "exact"), nfArm("sensorward", dbPath, "sensorward"), nfArm("medoid", dbPath, "medoid")})
	if err != nil {
		t.Fatal(err)
	}
	md := RenderNearFaceMarkdown(*r)
	for _, want := range []string{
		"# Near-face residuals", "| Annotation revision | 1 |", "role tuning, held out: false", "## sensorward",
		"| all | end normal | 6 |", "| all | end tangent (abs) | 6 |", "| extent_evidence |", "| range_30_60m |",
		"| exact | end_normal | 6 |", "Not scored: not_body_centre 6;", "## Caveats", "may not be quoted as held out",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, md)
		}
	}
	// A frozen split is named in the reading copy.
	r.Reference.FrozenDigest = "sha256:frozen"
	if !strings.Contains(RenderNearFaceMarkdown(*r), "| Frozen split | `sha256:frozen` |") {
		t.Fatal("the frozen split is not named")
	}
	// An arm with nothing scored still reads.
	if !strings.Contains(md, "## medoid") {
		t.Fatalf("the medoid arm is Missing:\n%s", md)
	}
}

func TestFirstBodyNsIsWhereTheEstimatesStart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "e.db")
	writeNearFaceDB(t, dbPath, map[string]nfBody{"late": {Missing: func(i int) bool { return i < 2 }}, "none": {Missing: func(int) bool { return true }}})
	got, err := FirstBodyNs(nfArm("late", dbPath, "late"))
	if err != nil || got != evalfixture.NearFaceSampleNs(2)+300_000 {
		t.Fatalf("first body at %d, %v", got, err)
	}
	if _, err := FirstBodyNs(nfArm("none", dbPath, "none")); err == nil {
		t.Fatal("an arm with no rows has a first row")
	}
	if _, err := FirstBodyNs(nfArm("absent", filepath.Join(t.TempDir(), "absent.db"), "")); err == nil {
		t.Fatal("an absent database has a first row")
	}
}
