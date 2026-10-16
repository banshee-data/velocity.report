package perframeeval

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// pts makes n distinct points along x from x0, at height z.
func pts(x0 float32, n int, z float32) [][3]float32 {
	out := make([][3]float32, n)
	for i := range out {
		out[i] = [3]float32{x0 + float32(i)*0.1, 1, z}
	}
	return out
}

// One frame of five objects, each case built by hand:
//
//	truck A  10 returns: 6 in cluster 1 (with 2 returns of nothing reviewed),
//	         2 in cluster 2, 2 in none. Retention 0.8, two fragments, main 0.6.
//	truck B  4 returns: 3 in cluster 3, which also holds 2 of car C's.
//	car C    4 returns: 2 in cluster 3, 2 in none. B and C are merged.
//	car D    100 returns in a cluster of 200 sampled at 50, 20 of them D's:
//	         an expansion of 20 x 200 / 50 = 80, confirmed 20.
//	ped E    3 returns, in no cluster: missed.
func retentionFrameFixture() ([]RetentionMask, []sqlite.ClusterSample, map[pointKey]bool) {
	a, b, c, d, e := pts(0, 10, 1), pts(10, 4, 1), pts(20, 4, 1), pts(30, 100, 1), pts(50, 3, 1)
	unreviewed := pts(60, 132, 1)
	masks := []RetentionMask{
		{SampleID: 7, TimestampNs: 700, ObjectID: "truck_a", Class: "truck", RangeM: 10, Points: a},
		{SampleID: 7, TimestampNs: 700, ObjectID: "truck_b", Class: "truck", RangeM: 25, Points: b},
		{SampleID: 7, TimestampNs: 700, ObjectID: "car_c", Class: "car", RangeM: 20, Points: c},
		{SampleID: 7, TimestampNs: 700, ObjectID: "car_d", Class: "car", RangeM: 40, Points: d},
		{SampleID: 7, TimestampNs: 700, ObjectID: "ped_e", Class: "pedestrian", RangeM: 60, Points: e},
	}
	capped := append(slices.Clone(d[:20]), unreviewed[:30]...)
	clusters := []sqlite.ClusterSample{
		{ObservationID: "o1", ClusterID: 1, PointsCount: 8, Sample: append(slices.Clone(a[:6]), unreviewed[100:102]...)},
		{ObservationID: "o2", ClusterID: 2, PointsCount: 2, Sample: slices.Clone(a[6:8])},
		{ObservationID: "o3", ClusterID: 3, PointsCount: 5, Sample: append(slices.Clone(b[:3]), c[:2]...)},
		{ObservationID: "o4", ClusterID: 4, PointsCount: 200, Sample: capped},
		// A cluster with points outside the pack's domain: a foreground
		// return the recording did not have.
		{ObservationID: "o5", ClusterID: 5, PointsCount: 2, Sample: [][3]float32{{-5, -5, -5}, {-6, -6, -6}}},
	}
	domain := map[pointKey]bool{}
	for _, set := range [][][3]float32{a, b, c, d, e, unreviewed} {
		for _, p := range set {
			domain[keyOf(p)] = true
		}
	}
	return masks, clusters, domain
}

func TestMeasureFrameRetentionFragmentationAndMerging(t *testing.T) {
	masks, clusters, domain := retentionFrameFixture()
	instants, merges, stats := measureFrame(masks, clusters, domain)
	by := map[string]RetentionInstant{}
	for _, in := range instants {
		by[in.ObjectID] = in
	}

	a := by["truck_a"]
	if a.MaskPoints != 10 || a.ConfirmedPoints != 8 || !near(a.Retention, 0.8) || a.Fragments != 2 || !near(a.MainShare, 0.6) || a.Estimated {
		t.Fatalf("truck A = %+v, want 8 of 10 retained over two clusters, main share 0.6, exact", a)
	}
	if len(a.MergedWith) != 0 {
		t.Fatalf("truck A merged with %v; the other returns in its cluster belong to nothing reviewed", a.MergedWith)
	}

	b, c := by["truck_b"], by["car_c"]
	if !near(b.Retention, 0.75) || b.Fragments != 1 || !slices.Equal(b.MergedWith, []string{"car_c"}) {
		t.Fatalf("truck B = %+v, want 3 of 4 in one cluster, merged with car C", b)
	}
	if !near(c.Retention, 0.5) || c.Fragments != 1 || !slices.Equal(c.MergedWith, []string{"truck_b"}) {
		t.Fatalf("car C = %+v, want 2 of 4 in one cluster, merged with truck B", c)
	}
	if len(merges) != 1 || merges[0].ClusterID != 3 || len(merges[0].Objects) != 2 ||
		merges[0].Objects[0].ObjectID != "truck_b" || merges[0].Objects[0].SampleHits != 3 ||
		merges[0].Objects[1].ObjectID != "car_c" || merges[0].Objects[1].SampleHits != 2 {
		t.Fatalf("merges = %+v, want cluster 3 holding truck B (3) and car C (2)", merges)
	}

	d := by["car_d"]
	if d.ConfirmedPoints != 20 || !near(d.ClusteredPoints, 80) || !near(d.Retention, 0.8) || !d.Estimated ||
		len(d.Clusters) != 1 || !d.Clusters[0].Capped || d.Clusters[0].SampleSize != 50 {
		t.Fatalf("car D = %+v, want 20 confirmed expanded to 80 of 100 from a capped cluster", d)
	}

	e := by["ped_e"]
	if e.Fragments != 0 || e.Retention != 0 || e.ConfirmedPoints != 0 {
		t.Fatalf("pedestrian E = %+v, want missed", e)
	}

	wantSample := 8 + 2 + 5 + 50 + 2
	if stats.clusters != 5 || stats.capped != 1 || stats.largestSample != 50 || stats.samplePoints != wantSample || stats.inPack != wantSample-2 {
		t.Fatalf("stats = %+v, want 5 clusters, 1 capped, largest 50, %d sample points of which 2 are not pack points", stats, wantSample)
	}
}

// An expansion can overshoot the mask when the sample happens to hold more of
// it than its share: retention is clamped at one, the confirmed count is not.
func TestMeasureFrameClampsAnExpansionAtTheMask(t *testing.T) {
	m := pts(0, 4, 1)
	instants, _, _ := measureFrame(
		[]RetentionMask{{ObjectID: "car", Class: "car", Points: m}},
		[]sqlite.ClusterSample{{ObservationID: "o", ClusterID: 1, PointsCount: 100, Sample: slices.Clone(m[:3])}}, nil)
	if in := instants[0]; in.ConfirmedPoints != 3 || in.ClusteredPoints != 4 || in.Retention != 1 || in.MainShare != 1 {
		t.Fatalf("instant = %+v, want 3 confirmed and the expansion clamped at the mask's 4", in)
	}
}

func TestRangeBands(t *testing.T) {
	edges := []float64{20, 35, 50}
	for _, tc := range []struct {
		r    float64
		want string
	}{{0, "range_0_20m"}, {19.99, "range_0_20m"}, {20, "range_20_35m"}, {34.9, "range_20_35m"},
		{35, "range_35_50m"}, {50, "range_50m_plus"}, {120, "range_50m_plus"}} {
		if got := rangeBand(tc.r, edges); got != tc.want {
			t.Errorf("rangeBand(%g) = %s, want %s", tc.r, got, tc.want)
		}
	}
	want := []string{"range_0_20m", "range_20_35m", "range_35_50m", "range_50m_plus"}
	if got := rangeBandNames(edges); !slices.Equal(got, want) {
		t.Fatalf("band names = %v, want %v", got, want)
	}
}

func TestSummariseRetentionPoolsByClassRangeAndObject(t *testing.T) {
	masks, clusters, domain := retentionFrameFixture()
	instants, merges, _ := measureFrame(masks, clusters, domain)
	// The same merge again in a second frame, in two clusters: one more
	// frame for the pair, not two.
	second := slices.Clone(masks[1:3])
	for i := range second {
		second[i].SampleID, second[i].TimestampNs = 8, 800
	}
	b, c := second[0].Points, second[1].Points
	more, moreMerges, _ := measureFrame(second, []sqlite.ClusterSample{
		{ObservationID: "p1", ClusterID: 1, PointsCount: 2, Sample: [][3]float32{b[0], c[0]}},
		{ObservationID: "p2", ClusterID: 2, PointsCount: 2, Sample: [][3]float32{b[1], c[1]}},
	}, nil)
	r := &RetentionReport{Instants: append(instants, more...), Merges: append(merges, moreMerges...),
		Reference: ReferenceIdentity{Split: "tune", SplitRole: annotation.SplitRoleTuning}}
	summariseRetentionReport(r, []float64{20, 35, 50})

	if r.Total.Instants != 7 || r.Total.Objects != 5 || r.Total.Missed != 1 || r.Total.Detected != 6 ||
		r.Total.Fragmented != 3 || r.Total.Merged != 4 || r.Total.MaxClusters != 2 || r.Total.EstimatedInstants != 1 {
		t.Fatalf("total = %+v", r.Total)
	}
	// Pooled: (8 + 3 + 2 + 80 + 0 + 2 + 2) of (10 + 4 + 4 + 100 + 3 + 4 + 4).
	if want := 97.0 / 129.0; !near(r.Total.Retention, want) {
		t.Fatalf("pooled retention %g, want %g", r.Total.Retention, want)
	}
	if !r.Approximate {
		t.Fatal("a capped cluster contributed, so the report must be approximate")
	}

	classes := map[string]RetentionSummary{}
	for _, s := range r.ByClass {
		classes[s.Name] = s
	}
	if len(r.ByClass) != 3 || classes["truck"].Instants != 3 || classes["car"].Instants != 3 || classes["pedestrian"].Missed != 1 {
		t.Fatalf("by class = %+v", r.ByClass)
	}

	// Truck A at 10 m; car C at exactly 20 m is in the upper band, with
	// truck B at 25 m; car D at 40 m; pedestrian E at 60 m.
	var names []string
	counts := map[string]int{}
	for _, s := range r.ByRange {
		names = append(names, s.Name)
		counts[s.Name] = s.Instants
	}
	if !slices.Equal(names, []string{"range_0_20m", "range_20_35m", "range_35_50m", "range_50m_plus"}) {
		t.Fatalf("range bands %v, want all four in order", names)
	}
	if counts["range_0_20m"] != 1 || counts["range_20_35m"] != 4 || counts["range_35_50m"] != 1 || counts["range_50m_plus"] != 1 {
		t.Fatalf("instants per band = %v", counts)
	}
	for _, in := range r.Instants {
		if in.Band == "" {
			t.Fatalf("instant %s at %d has no band", in.ObjectID, in.SampleID)
		}
	}

	if len(r.ByObject) != 5 || r.ByObject[0].Name != "car_c" || r.ByObject[0].Class != "car" || r.ByObject[0].Instants != 2 {
		t.Fatalf("by object = %+v, want five objects sorted, car C with two instants", r.ByObject)
	}
	if len(r.MergePairs) != 1 || r.MergePairs[0] != (RetentionMergePair{A: "car_c", B: "truck_b", Frames: 2}) {
		t.Fatalf("merge pairs = %+v, want car C and truck B in two frames", r.MergePairs)
	}
	if !strings.Contains(strings.Join(r.Caveats, "\n"), "not held out") {
		t.Fatalf("caveats %v do not say the split is not held out", r.Caveats)
	}
}

func retentionFixtureOptions(t *testing.T) (RetentionOptions, *evalfixture.Fixture) {
	t.Helper()
	dir := t.TempDir()
	f, err := evalfixture.Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultRetentionOptions()
	opts.Reference.PackDir, opts.Reference.SplitManifestPath, opts.Reference.Split = f.PackDir, f.SplitManifestPath, evalfixture.SplitHeldOut
	opts.DBPath = filepath.Join(dir, "observations.db")
	if err := evalfixture.WriteRetentionDB(opts.DBPath, f.PackDir); err != nil {
		t.Fatal(err)
	}
	return opts, f
}

// The whole path: the pack, split and annotation bound as the per-frame
// evaluator binds them, the replay's clusters read from an evidence database,
// and every case of evalfixture.WriteRetentionDB measured as it says.
func TestMeasureRetentionAgainstAFixtureReplay(t *testing.T) {
	opts, _ := retentionFixtureOptions(t)
	r, err := MeasureRetention(opts)
	if err != nil {
		t.Fatal(err)
	}
	ev := r.Evidence
	if ev.SourceID != evalfixture.RetentionSourceID || ev.Database != "observations.db" || ev.Frames != 7 ||
		ev.InstantsOutsideWindow != 1 || ev.InstantsWithoutFrame != 1 || ev.FramesWithoutEvidence != 1 ||
		ev.DomainShare != 1 || ev.CappedClusters != 1 {
		t.Fatalf("evidence = %+v", ev)
	}
	got := map[int]RetentionInstant{}
	for _, in := range r.Instants {
		if in.ObjectID != evalfixture.CarA {
			t.Fatalf("measured %s; only %s is scored in the held-out split", in.ObjectID, evalfixture.CarA)
		}
		got[in.SampleID] = in
	}
	if len(got) != 7 {
		t.Fatalf("measured samples %v, want 0 to 4, 7 and 8", r.Instants)
	}
	if _, ok := got[evalfixture.OccludedSample]; ok {
		t.Fatal("the occluded sample is not certifiable and must not be measured")
	}
	for sample, want := range map[int]struct {
		retention float64
		fragments int
		estimated bool
	}{0: {1, 2, false}, 1: {1, 1, false}, 2: {0.5, 1, false}, 3: {1, 1, true}, 4: {0, 0, false}, 7: {0, 0, false}, 8: {0, 0, false}} {
		in := got[sample]
		if !near(in.Retention, want.retention) || in.Fragments != want.fragments || in.Estimated != want.estimated {
			t.Errorf("sample %d = %+v, want retention %g over %d clusters, estimated %v", sample, in, want.retention, want.fragments, want.estimated)
		}
	}
	if s := r.Total; s.Instants != 7 || s.Missed != 3 || s.Fragmented != 1 || s.Merged != 0 || s.Objects != 1 {
		t.Fatalf("total = %+v", s)
	}
	if !r.Approximate || r.Reference.Split != evalfixture.SplitHeldOut || !r.Reference.HeldOut || r.Reference.Digest == "" {
		t.Fatalf("report header: approximate %v, reference %+v", r.Approximate, r.Reference)
	}
	md := RenderRetentionMarkdown(*r)
	for _, want := range []string{"**Approximate.**", "| all | 7 |", "range_0_20m", "No cluster held returns of two reviewed objects."} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestMeasureRetentionSourcesAndSplits(t *testing.T) {
	opts, _ := retentionFixtureOptions(t)
	opts.SourceID = "source/v1/elsewhere"
	if _, err := MeasureRetention(opts); err == nil || !strings.Contains(err.Error(), "no observations from source") {
		t.Fatalf("unknown source: err = %v", err)
	}
	opts.SourceID = ""
	opts.FrameToleranceNanos = 0
	opts.MinDomainShare = 1.0
	if _, err := MeasureRetention(opts); err != nil {
		t.Fatalf("exact frames and an all-pack sample must pass a 100%% floor: %v", err)
	}

	tuning, _ := retentionFixtureOptions(t)
	tuning.Reference.Split = evalfixture.SplitTuning
	if _, err := MeasureRetention(tuning); err == nil {
		t.Fatal("a tuning split was measured without -allow-tuning-split")
	}
	// Car B is the tuning split's object and no cluster holds it: misses,
	// not a refusal, since the clusters are still the pack's points.
	tuning.Reference.AllowTuningSplit = true
	r, err := MeasureRetention(tuning)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total.Detected != 0 || r.Total.Missed != r.Total.Instants || r.Reference.HeldOut {
		t.Fatalf("car B total = %+v; it is in no cluster", r.Total)
	}
}

// Evidence that is not in the pack's point domain is refused rather than
// reported as zero retention; so is evidence whose frames do not line up
// with the pack's samples, and evidence that kept no sample to match.
func TestMeasureFramesRefusesEvidenceThatDoesNotMatchThePack(t *testing.T) {
	opts, f := retentionFixtureOptions(t)
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	t1, t2 := evalfixture.SampleTime(1), evalfixture.SampleTime(2)
	masks := []RetentionMask{
		{SampleID: 1, TimestampNs: t1, ObjectID: "x", Class: "car", Points: [][3]float32{{1, 1, 1}}},
		{SampleID: 2, TimestampNs: t2, ObjectID: "x", Class: "car", Points: [][3]float32{{2, 1, 1}}},
	}
	cluster := func(ts int64, sample [][3]float32) sqlite.ClusterSample {
		return sqlite.ClusterSample{ObservationID: "o", FrameUnixNanos: ts, ClusterID: 1, PointsCount: 1, Sample: sample}
	}
	run := func(first, last int64, clusters []sqlite.ClusterSample) error {
		r := &RetentionReport{Evidence: RetentionEvidence{FirstFrameNs: first, LastFrameNs: last}}
		return measureFrames(r, pack, masks, clusters, opts)
	}

	elsewhere := [][3]float32{{99, 99, 99}}
	if err := run(t1, t2, []sqlite.ClusterSample{cluster(t1, elsewhere), cluster(t2, elsewhere)}); err == nil ||
		!strings.Contains(err.Error(), "not in this pack's point domain") {
		t.Fatalf("foreign points: err = %v", err)
	}

	const late = 50_000_000
	if err := run(t1+late, t2+late, []sqlite.ClusterSample{cluster(t1+late, elsewhere), cluster(t2+late, elsewhere)}); err == nil ||
		!strings.Contains(err.Error(), "do not align") {
		t.Fatalf("frames 50 ms late: err = %v", err)
	}

	empty := cluster(t1, nil)
	empty.PointsCount = 40
	if err := run(t1, t2, []sqlite.ClusterSample{empty}); err == nil || !strings.Contains(err.Error(), "-sample-points") {
		t.Fatalf("sample-less cluster: err = %v", err)
	}
}

func TestRetentionOptionsValidate(t *testing.T) {
	base := DefaultRetentionOptions()
	base.DBPath = "x.db"
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RetentionOptions){
		"no database":       func(o *RetentionOptions) { o.DBPath = "" },
		"negative tol":      func(o *RetentionOptions) { o.FrameToleranceNanos = -1 },
		"no bands":          func(o *RetentionOptions) { o.RangeBandsM = nil },
		"descending bands":  func(o *RetentionOptions) { o.RangeBandsM = []float64{35, 20} },
		"zero band":         func(o *RetentionOptions) { o.RangeBandsM = []float64{0, 20} },
		"domain share > 1":  func(o *RetentionOptions) { o.MinDomainShare = 1.5 },
		"bad policy status": func(o *RetentionOptions) { o.Reference.Policy.Status = "maybe" },
	} {
		o := base
		o.RangeBandsM = slices.Clone(base.RangeBandsM)
		mutate(&o)
		if err := o.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
