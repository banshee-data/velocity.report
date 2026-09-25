package perframeeval

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func fixture(t *testing.T) *evalfixture.Fixture {
	t.Helper()
	f, err := evalfixture.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func baseConfig(f *evalfixture.Fixture) Config {
	return Config{
		Reference: ReferenceOptions{
			PackDir: f.PackDir, SplitManifestPath: f.SplitManifestPath,
			Split: evalfixture.SplitHeldOut, Policy: annotation.DefaultReferencePolicy(),
		},
		Score: DefaultScoreOptions(),
		A:     ArmSpec{Label: "A", DBPath: f.DBPath, ParamHash: evalfixture.ParamsA},
		B:     ArmSpec{Label: "B", DBPath: f.DBPath, ParamHash: evalfixture.ParamsB},
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The scenario the fixture describes, counted by hand.
//
// Reference, reviewed-only: obj_car_a is scored at 9 samples (occluded at 5,
// so ignored there); obj_ped_p is only proposed, so ignored at all 10;
// obj_car_b and obj_noise_n are not this episode's objects, so ignored at all
// 10 each.
//
// Arm A follows everything exactly. Its obj_car_a track is absorbed at sample
// 5 and its other three tracks throughout: 1 + 30 absorbed, 9 matched.
//
// Arm B: obj_car_a by seq 1 at samples 0-3, missed at 4 (FN 1), ignored at 5,
// then seq 2 from 6 (one identity switch, one fragmentation). The ghost is
// two false positives. MOTA = 1 - (1 + 2 + 1)/9 = 5/9.
// Identity: the best pairing keeps 4 of 9 reference and 4 of 10 hypothesis
// detections: IDF1 = 8/19, IDP = 4/10, IDR = 4/9.
// HOTA, exact positions: TP 8, FN 1, FP 2 at every alpha, DetA = 8/11; each of
// the two pairings holds half of the object's true positives, AssA = 1/2;
// HOTA = sqrt(4/11).
func TestHeldOutComparisonHandCounted(t *testing.T) {
	f := fixture(t)
	c, err := Run(baseConfig(f))
	if err != nil {
		t.Fatal(err)
	}

	ref := c.Reference
	if !ref.HeldOut || ref.Split != evalfixture.SplitHeldOut || ref.SidecarRevision != 1 ||
		ref.PackDigest != f.PackDigest || len(ref.Episodes) != 1 || ref.Episodes[0] != evalfixture.EpisodeHeldOut ||
		!strings.HasPrefix(ref.Digest, "sha256:") || ref.Policy.Status != annotation.ReferenceReviewedOnly {
		t.Fatalf("reference identity %+v", ref)
	}
	stats := c.A.Episodes[0].Reference
	wantIgnored := map[string]int{"visibility": 1, "unreviewed": 10, IgnoreOutsideEpisode: 20}
	if stats.Frames != 10 || stats.ScoredPoints != 9 || stats.ScoredObjects != 1 || stats.FramesWithoutMasks != 0 ||
		len(stats.IgnoredByReason) != len(wantIgnored) {
		t.Fatalf("episode reference stats %+v", stats)
	}
	for reason, n := range wantIgnored {
		if stats.IgnoredByReason[reason] != n {
			t.Fatalf("ignored %s = %d, want %d (%+v)", reason, stats.IgnoredByReason[reason], n, stats.IgnoredByReason)
		}
	}

	a, b := c.Total.A, c.Total.B
	if a.NumGT != 9 || a.Matches != 9 || a.FN != 0 || a.FP != 0 || a.IDSwitches != 0 || a.Fragmentations != 0 ||
		a.MOTA != 1 || a.MOTP > 1e-6 || !near(a.IDF1, 1) || !near(a.HOTA, 1) {
		t.Fatalf("arm A %+v, want perfect", a)
	}
	if got := c.A.Episodes[0].Metrics.CLEARMOT.IgnoredHypotheses; got != 31 {
		t.Fatalf("arm A absorbed %d hypothesis points, want 31", got)
	}
	if b.NumGT != 9 || b.Matches != 8 || b.FN != 1 || b.FP != 2 || b.IDSwitches != 1 || b.Fragmentations != 1 {
		t.Fatalf("arm B counts %+v", b)
	}
	if !near(b.MOTA, 5.0/9) || !near(b.IDF1, 8.0/19) || !near(b.IDP, 0.4) || !near(b.IDR, 4.0/9) ||
		!near(b.DetA, 8.0/11) || !near(b.AssA, 0.5) || !near(b.HOTA, math.Sqrt(4.0/11)) {
		t.Fatalf("arm B ratios %+v", b)
	}
	if got := c.B.Episodes[0].Metrics.CLEARMOT.IgnoredHypotheses; got != 20 {
		t.Fatalf("arm B absorbed %d hypothesis points, want 20", got)
	}
	d := c.Total.Delta
	if d.IDSwitches != 1 || d.Fragmentations != 1 || d.FP != 2 || !near(d.MOTA, 5.0/9-1) || !near(d.IDF1, 8.0/19-1) {
		t.Fatalf("delta %+v", d)
	}
	if len(c.Paired) != 1 || c.Paired[0] != (PairedResult{EpisodeID: evalfixture.EpisodeHeldOut, A: a, B: b, Delta: d}) {
		t.Fatalf("paired %+v does not match the pooled result of its only episode", c.Paired)
	}

	arm := c.A.Arm
	if arm.Kind != ArmEstimates || arm.Stage != StageFinal || arm.DeclaredBaseline || arm.TrackKey != "creation_sequence" ||
		arm.SourceID != evalfixture.SourceID || arm.EstimatorID != evalfixture.EstimatorID || arm.Tracks != 4 ||
		arm.Database != "evidence.db" || c.B.Arm.Tracks != 5 {
		t.Fatalf("arm identity %+v / %+v", arm, c.B.Arm)
	}
	al := c.A.Episodes[0].Alignment
	if al.AlignedPoints != 40 || al.UnalignedPoints != 0 || al.MaxOffsetNanos != evalfixture.EstimateOffsetNs || al.Tracks != 4 {
		t.Fatalf("alignment %+v", al)
	}
	if len(c.Caveats) != 1 || !strings.Contains(c.Caveats[0], "upper bound") {
		t.Fatalf("caveats %q, want only the false-positive bound", c.Caveats)
	}

	md := RenderMarkdown(*c)
	for _, want := range []string{
		"# Per-frame comparison: B against A",
		"| MOTA | 1.0000 | 0.5556 | -0.4444 |",
		"| ID switches | 0 | 1 | +1 |",
		"| Fragmentations | 0 | 1 | +1 |",
		"| IDF1 | 1.0000 | 0.4211 | -0.5789 |",
		"## Episode hold-ep1",
		"| Gate                | 1 m + half footprint diagonal |",
		"| Held out            | true |",
		"upper bound",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

// Including proposals by request scores the pedestrian as well: 10 more
// reference points, all matched by both arms.
func TestIncludingProposalsScoresThePedestrian(t *testing.T) {
	f := fixture(t)
	cfg := baseConfig(f)
	cfg.Reference.Policy.Status = annotation.ReferenceIncludeProposed
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a, b := c.Total.A, c.Total.B
	if a.NumGT != 19 || a.Matches != 19 || a.MOTA != 1 {
		t.Fatalf("arm A %+v", a)
	}
	if b.NumGT != 19 || b.Matches != 18 || b.FN != 1 || b.FP != 2 || b.IDSwitches != 1 || !near(b.MOTA, 1-4.0/19) {
		t.Fatalf("arm B %+v", b)
	}
	if !strings.Contains(strings.Join(c.Caveats, " "), "proposed masks nobody reviewed") {
		t.Fatalf("caveats %q do not say proposals were scored", c.Caveats)
	}
	reviewed, err := Run(baseConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Reference.Digest == c.Reference.Digest {
		t.Fatal("a different policy produced the same reference digest")
	}
}

func TestTuningSplitIsRefusedForHeldOutScoring(t *testing.T) {
	f := fixture(t)
	cfg := baseConfig(f)
	cfg.Reference.Split = evalfixture.SplitTuning
	if _, err := Run(cfg); !errors.Is(err, annotation.ErrNotHeldOut) {
		t.Fatalf("tuning split scored as held out: error %v", err)
	}

	cfg.Reference.AllowTuningSplit = true
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.Reference.HeldOut || c.Reference.SplitRole != annotation.SplitRoleTuning {
		t.Fatalf("reference %+v claims to be held out", c.Reference)
	}
	if !strings.Contains(c.Caveats[0], "not a held-out result") {
		t.Fatalf("first caveat %q, want the held-out warning", c.Caveats[0])
	}
	// obj_car_b, tracked exactly by both arms at all ten samples. Arm B's
	// ghost is two false positives here as well: MOTA 1 - 2/10.
	if c.Total.A.NumGT != 10 || c.Total.A.MOTA != 1 || c.Total.B.FP != 2 || !near(c.Total.B.MOTA, 0.8) {
		t.Fatalf("tuning episode totals %+v", c.Total)
	}
}

func TestReferenceRefusals(t *testing.T) {
	f := fixture(t)
	dir := t.TempDir()
	cases := []struct {
		name   string
		mutate func(*annotation.SplitManifest)
		want   string
	}{
		{"manifest frozen against another pack", func(m *annotation.SplitManifest) {
			m.PackDigest = "sha256:" + strings.Repeat("0", 64)
		}, "frozen against pack"},
		{"manifest pins another revision", func(m *annotation.SplitManifest) { m.SidecarRevision = 2 }, "revision"},
		{"episode with nothing certifiable", func(m *annotation.SplitManifest) {
			m.Episodes[0].ObjectIDs = []string{evalfixture.Ped}
		}, "no certifiable reference point"},
		{"held-out episode scoring a tuning object", func(m *annotation.SplitManifest) {
			m.Episodes[0].ObjectIDs = []string{evalfixture.CarA, evalfixture.CarB}
		}, `of split "tune"`},
		{"episode past the end of the pack", func(m *annotation.SplitManifest) {
			m.Episodes[0].FrameIntervals = []annotation.FrameInterval{{FirstSample: 0, LastSample: evalfixture.Samples}}
		}, "the pack has 10 samples"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := f.Manifest()
			tc.mutate(&m)
			path := filepath.Join(dir, "split-"+string(rune('a'+i))+".json")
			if err := evalfixture.WriteSplitManifest(path, m); err != nil {
				t.Fatal(err)
			}
			opts := baseConfig(f).Reference
			opts.SplitManifestPath = path
			if _, err := LoadReference(opts); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want one mentioning %q", err, tc.want)
			}
		})
	}

	// An episode naming an object with no mask in its frames was frozen
	// against other labels, or mistyped.
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	bySample := map[int][]annotation.ReferencePoint{0: {{ObjectID: "a", SampleID: 0}}}
	ep := annotation.Episode{EpisodeID: "ep", ObjectIDs: []string{"a", "b"}, FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: 1}}}
	if _, err := buildEpisode(pack, ep, bySample, annotation.DefaultReferencePolicy()); err == nil ||
		!strings.Contains(err.Error(), `object "b", which has no mask`) {
		t.Fatalf("absent object: error %v", err)
	}
}

func TestFinalStageIsRequiredUnlessDeclared(t *testing.T) {
	f := fixture(t)
	cfg := baseConfig(f)
	cfg.A.Stage = "online"
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "declare the arm a baseline") {
		t.Fatalf("online arm without a declaration: error %v", err)
	}

	cfg.A.DeclaredBaseline = true
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.A.Arm.Stage != "online" || !c.A.Arm.DeclaredBaseline || c.B.Arm.Stage != StageFinal {
		t.Fatalf("arms %+v / %+v", c.A.Arm, c.B.Arm)
	}
	if !strings.Contains(strings.Join(c.Caveats, " "), "Arm A scores online-stage positions as a declared baseline") {
		t.Fatalf("caveats %q do not record the baseline", c.Caveats)
	}
	// The online estimates are arm A's positions again.
	if c.Total.A.MOTA != 1 {
		t.Fatalf("online arm A %+v", c.Total.A)
	}

	// A database holding only online estimates has nothing final to score,
	// and says what it does hold.
	onlineOnly := filepath.Join(t.TempDir(), "online.db")
	if err := evalfixture.WriteDB(onlineOnly, []string{"online"}); err != nil {
		t.Fatal(err)
	}
	_, err = LoadArm(ArmSpec{Label: "A", DBPath: onlineOnly, ParamHash: evalfixture.ParamsA})
	if err == nil || !strings.Contains(err.Error(), `stage="final"`) || !strings.Contains(err.Error(), "stage=online") {
		t.Fatalf("online-only database: error %v", err)
	}
}

func TestArmSelectionRefusesAmbiguityAndMixing(t *testing.T) {
	f := fixture(t)
	if _, err := LoadArm(ArmSpec{Label: "A", DBPath: f.DBPath}); err == nil || !strings.Contains(err.Error(), "2 estimate versions match") {
		t.Fatalf("ambiguous arm: error %v", err)
	}
	if _, err := LoadArm(ArmSpec{Label: "A", DBPath: f.DBPath, RunID: evalfixture.RunA, ParamHash: "x"}); err == nil {
		t.Fatal("an arm naming both a run and an estimate version was accepted")
	}
	if _, err := LoadArm(ArmSpec{DBPath: f.DBPath}); err == nil {
		t.Fatal("an unlabelled arm was accepted")
	}

	cfg := baseConfig(f)
	cfg.B = ArmSpec{Label: "B", DBPath: f.DBPath, RunID: evalfixture.RunB, DeclaredBaseline: true}
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "compare estimates with estimates") {
		t.Fatalf("mixed arm kinds: error %v", err)
	}
	cfg = baseConfig(f)
	cfg.B.Label = "A"
	if _, err := Run(cfg); err == nil {
		t.Fatal("two arms with one label were accepted")
	}
}

// Creation sequences restart on a tracker reset; a source that contains two
// runs cannot be keyed by them, and is refused.
func TestCreationSequenceCollisionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reset.db")
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	states := sqlite.NewStateEstimateStore(database.DB)
	for i, trackID := range []string{"uuid-before-reset", "uuid-after-reset"} {
		id := "estimate/" + trackID
		e := sqlite.TrackEstimate{
			EstimateID: id, TrackID: trackID, ObservationID: "o/" + id, SourceID: "s", CalibrationID: "c",
			FrameUnixNanos: int64(i + 1), EstimatorID: "e", ObservationModelID: "m", ParamHash: "p",
			Stage: StageFinal, MeasurementSource: "m", CreationSequence: 1,
		}
		if err := states.Insert(e, sqlite.TrackResidual{EstimateID: id, ObservationID: e.ObservationID, Disposition: "accepted", Reason: "r"}); err != nil {
			t.Fatal(err)
		}
	}
	database.Close()
	if _, err := LoadArm(ArmSpec{Label: "A", DBPath: path}); err == nil || !strings.Contains(err.Error(), "creation_sequence 1 names two tracks") {
		t.Fatalf("collision: error %v", err)
	}
}

// The analysis-run path scores the same positions as the estimate path, so
// with the same tracks it must give the same numbers. It also shows the run
// adapter renames tracks by content: the fixture's run track IDs sort against
// their order in time.
func TestAnalysisRunArmsScoreLikeEstimateArms(t *testing.T) {
	f := fixture(t)
	estimates, err := Run(baseConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	cfg := baseConfig(f)
	cfg.A = ArmSpec{Label: "A", DBPath: f.DBPath, RunID: evalfixture.RunA}
	cfg.B = ArmSpec{Label: "B", DBPath: f.DBPath, RunID: evalfixture.RunB}
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "declare the arm a baseline") {
		t.Fatalf("undeclared analysis run: error %v", err)
	}
	cfg.A.DeclaredBaseline, cfg.B.DeclaredBaseline = true, true
	runs, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if runs.Total.A != estimates.Total.A || runs.Total.B != estimates.Total.B {
		t.Fatalf("runs scored %+v / %+v, estimates %+v / %+v", runs.Total.A, runs.Total.B, estimates.Total.A, estimates.Total.B)
	}
	if runs.A.Arm.Kind != ArmAnalysisRun || runs.A.Arm.TrackKey != "run_track_order" || runs.A.Arm.Stage != StageOnline {
		t.Fatalf("run arm identity %+v", runs.A.Arm)
	}

	hyp, err := LoadArm(cfg.B)
	if err != nil {
		t.Fatal(err)
	}
	// First by first frame, then position: y = 0, 10, 20, 40 at sample 0,
	// then the car's second piece from sample 6.
	wantFirstY := []float32{0, 10, 20, 40, 0}
	for i, s := range hyp.Series {
		if s.ID != "run-00000"+string(rune('1'+i)) || s.Points[0].Y != wantFirstY[i] {
			t.Fatalf("series %d is %s starting at y=%v, want run-00000%d at y=%v", i, s.ID, s.Points[0].Y, i+1, wantFirstY[i])
		}
	}
	if _, err := LoadArm(ArmSpec{Label: "A", DBPath: f.DBPath, RunID: "no-such-run", DeclaredBaseline: true}); err == nil ||
		!strings.Contains(err.Error(), "no tracks") {
		t.Fatalf("missing run: error %v", err)
	}
}

func TestCompareArmsRefusesMismatchedScoring(t *testing.T) {
	f := fixture(t)
	cfg := baseConfig(f)
	ref, err := LoadReference(cfg.Reference)
	if err != nil {
		t.Fatal(err)
	}
	a, err := LoadArm(cfg.A)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadArm(cfg.B)
	if err != nil {
		t.Fatal(err)
	}
	ra, err := ScoreArm(ref, a, cfg.Score)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := ScoreArm(ref, b, cfg.Score)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompareArms(ra, rb); err != nil {
		t.Fatalf("matching arms refused: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*ArmResult)
		want   string
	}{
		{"gate", func(r *ArmResult) { r.Gate = l8analytics.FixedGate(2) }, "different gates"},
		{"tolerance", func(r *ArmResult) { r.FrameToleranceNanos++ }, "frame tolerances"},
		{"policy", func(r *ArmResult) { r.Reference.Policy.ScorePartialMasks = false }, "reference policies"},
		{"episode set", func(r *ArmResult) { r.Reference.Episodes = []string{"other"} }, "different episodes"},
		{"episode order", func(r *ArmResult) { r.Episodes[0].EpisodeID = "other" }, "different episodes"},
		{"reference digest", func(r *ArmResult) { r.Reference.Digest = "sha256:other" }, "different references"},
		{"hypothesis kind", func(r *ArmResult) { r.Arm.Kind = ArmAnalysisRun }, "different write paths"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := rb
			changed.Reference.Policy.RoadUserClasses = append([]string(nil), rb.Reference.Policy.RoadUserClasses...)
			changed.Reference.Episodes = append([]string(nil), rb.Reference.Episodes...)
			changed.Episodes = append([]EpisodeResult(nil), rb.Episodes...)
			tc.mutate(&changed)
			if _, err := CompareArms(ra, changed); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestReferenceIsReproducible(t *testing.T) {
	f := fixture(t)
	opts := baseConfig(f).Reference
	first, err := LoadReference(opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadReference(opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity.Digest != second.Identity.Digest {
		t.Fatal("the same reference loaded twice has two digests")
	}
	opts.Policy.Position = annotation.PositionPointMean
	third, err := LoadReference(opts)
	if err != nil {
		t.Fatal(err)
	}
	if third.Identity.Digest == first.Identity.Digest {
		t.Fatal("a different position rule kept the digest")
	}
}

func TestAlignment(t *testing.T) {
	ep := EpisodeReference{
		EpisodeID: "ep", Frames: []int64{1000, 1100, 1200, 2000, 2100},
		spans: []timeSpan{{1000, 1200}, {2000, 2100}},
	}
	hyp := []l8analytics.TrackSeries{{ID: "h", Points: []l8analytics.SeriesPoint{
		{TimestampNanos: 990},  // 10 early onto 1000
		{TimestampNanos: 1003}, // 3 late onto 1000: nearer, replaces the first
		{TimestampNanos: 1150}, // 50 from both 1100 and 1200: unaligned at tolerance 20
		{TimestampNanos: 1195}, // onto 1200
		{TimestampNanos: 1500}, // between the intervals: outside the episode
		{TimestampNanos: 900},  // before it
		{TimestampNanos: 2115}, // 15 after the last frame, inside the tolerance
	}}, {ID: "g", Points: []l8analytics.SeriesPoint{{TimestampNanos: 5000}}}}

	got, stats := alignToEpisode(hyp, ep, 20)
	want := AlignmentStats{AlignedPoints: 3, OutsideEpisodePoints: 3, UnalignedPoints: 1, DuplicatePoints: 1, MaxOffsetNanos: 15, Tracks: 1}
	if stats != want {
		t.Fatalf("stats %+v, want %+v", stats, want)
	}
	if len(got) != 1 || len(got[0].Points) != 3 || got[0].Points[0].TimestampNanos != 1000 ||
		got[0].Points[1].TimestampNanos != 1200 || got[0].Points[2].TimestampNanos != 2100 {
		t.Fatalf("aligned %+v", got)
	}
	if f := stats.UnalignedFraction(); !near(f, 1.0/5) {
		t.Fatalf("unaligned fraction %v, want 1/5", f)
	}

	// Scored for real: a tolerance tighter than the fixture's 0.3 ms offset
	// leaves every point unaligned, and the arm is refused, not scored empty.
	fx := fixture(t)
	cfg := baseConfig(fx)
	cfg.Score.FrameToleranceNanos = 100_000
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "further than 100000 ns") {
		t.Fatalf("misaligned arm: error %v", err)
	}
}

// The matcher keys frames by time, so an episode whose samples share one is
// refused rather than silently merged.
func TestEpisodeRefusesDuplicateTimestamps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	var samples []annotation.Sample
	var blocks [][]byte
	for i := 0; i < 2; i++ {
		block, err := annotation.EncodePoints(annotation.Points{X: []float32{0}, Y: []float32{0}, Z: []float32{0}})
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, annotation.Sample{SourceOrdinal: i, TimestampNs: 42, PointCount: 1})
		blocks = append(blocks, block)
	}
	if err := annotation.WritePack(dir, annotation.Manifest{Coverage: annotation.CoverageForegroundOnly}, samples, blocks); err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	ep := annotation.Episode{EpisodeID: "ep", ObjectIDs: []string{"o"}, FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: 1}}}
	if _, err := buildEpisode(pack, ep, nil, annotation.DefaultReferencePolicy()); err == nil || !strings.Contains(err.Error(), "share timestamp 42") {
		t.Fatalf("duplicate timestamps: error %v", err)
	}
}
