package replayeval

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const surveyParams = "sha256:params"

// surveyManifestFor is a default replay's manifest with an evidence source.
func surveyManifestFor() surveyManifest {
	return surveyManifest{
		SourceSHA256s: []string{"sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("b", 64)},
		ParamsSHA256:  surveyParams, BuildVersion: "0.5.2", BuildGitSHA: "abc123", BuildStamped: true,
		MeasurementSourceMode: "medoid_v0", ObservationSourceID: "source/v1/test", FramesRecorded: 42,
	}
}

func surveyOptions() CoverageSurveyOptions {
	return CoverageSurveyOptions{CaseID: "site", Tool: "survey-test",
		RangePercentile: DefaultCoverageRangePercentile, MinSectorGapDeg: DefaultCoverageMinSectorGapDeg}
}

// polar places estimates at the given range and azimuth, anticlockwise from
// +X, one track per estimate unless tracks says otherwise.
type polar struct {
	rangeM, azimuthDeg float64
	track              int64
}

func estimatesAt(points ...polar) []observationsqlite.TrackEstimate {
	out := make([]observationsqlite.TrackEstimate, len(points))
	for i, p := range points {
		rad := p.azimuthDeg * math.Pi / 180
		out[i] = observationsqlite.TrackEstimate{
			EstimateID: fmt.Sprintf("estimate/%d", i), ParamHash: surveyParams, CreationSequence: p.track,
			X: float32(p.rangeM * math.Cos(rad)), Y: float32(p.rangeM * math.Sin(rad)),
		}
	}
	return out
}

// A replay whose estimates reach every quarter of the circle declares the
// full circle, out to its farthest estimate rounded up, and records how.
func TestMeasureCoverageFullCircle(t *testing.T) {
	var points []polar
	for i := 0; i < 12; i++ { // every 30 degrees: no empty arc reaches 90
		points = append(points, polar{rangeM: float64(10 + i), azimuthDeg: float64(30*i) + 5.5, track: int64(i % 3)})
	}
	points = append(points, polar{rangeM: 91.72, azimuthDeg: 100.5})
	s, err := measureCoverage(surveyOptions(), surveyManifestFor(), estimatesAt(points...))
	if err != nil {
		t.Fatal(err)
	}
	d := s.Declaration
	if d.MaxRangeMetres != 92 || d.MinRangeMetres != 0 || d.AzimuthHalfWidthDeg != 180 || d.AzimuthCentreDeg != 0 ||
		d.SensorXMetres != 0 || d.SensorYMetres != 0 {
		t.Fatalf("declaration %+v, want 92 m and the full circle about the origin", d)
	}
	st := s.Stats
	joined := "sha256:" + strings.Repeat("a", 64) + "\nsha256:" + strings.Repeat("b", 64)
	if want := "sha256:" + hex.EncodeToString(sha256Sum([]byte(joined))); st.CaptureSetSHA256 != want {
		t.Fatalf("capture set digest %s, want %s", st.CaptureSetSHA256, want)
	}
	if st.OnlineEstimates != 13 || st.Tracks != 3 || st.FramesRecorded != 42 || st.RangeMetres.Max != 91.72 ||
		st.RangeMetres.Min != 10 || st.RangeAtPercentile != 91.72 || st.SensorOrigin.Source != OriginTrackingTransformIdentity ||
		st.DeclarationID != d.ID() || st.CaseID != "site" || !st.BuildStamped || st.Azimuth.Sector ||
		st.Azimuth.OccupiedDegrees != 13 || st.Azimuth.LargestEmptyArcDeg != 29 {
		t.Fatalf("statistics %+v", st)
	}
	if st.Azimuth.EstimatesPer10Deg[0] != 1 || st.Azimuth.EstimatesPer10Deg[10] != 1 || st.Azimuth.EstimatesPer10Deg[1] != 0 {
		t.Fatalf("counts per 10 degrees %v", st.Azimuth.EstimatesPer10Deg)
	}
	for _, want := range []string{
		"measured by survey-test (build 0.5.2, git abc123, stamped): case site, captures " + st.CaptureSetSHA256,
		"default replay params " + surveyParams, "max_range_m is the maximum online-estimate range, 91.72 m, rounded up",
		"full circle: the largest arc without an estimate is 29 deg, under the 90 deg that makes a sector",
	} {
		if !strings.Contains(d.Source, want) {
			t.Errorf("source %q lacks %q", d.Source, want)
		}
	}
}

// An arc no estimate ever reaches makes the declaration the sector outside
// it, including when the sector crosses +X.
func TestMeasureCoverageSector(t *testing.T) {
	cases := []struct {
		name               string
		from, to           float64 // estimates every 5 degrees in [from, to]
		centre, halfWidth  float32
		emptyFrom, emptyDg int
	}{
		{"kirk0-like, facing -Y", 50, 255, 153, 103, 256, 154},
		{"across +X", 300, 385, -17, 43, 26, 274},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var points []polar
			for a := tc.from; a <= tc.to; a += 5 {
				points = append(points, polar{rangeM: 20, azimuthDeg: math.Mod(a+0.5, 360)})
			}
			s, err := measureCoverage(surveyOptions(), surveyManifestFor(), estimatesAt(points...))
			if err != nil {
				t.Fatal(err)
			}
			d, az := s.Declaration, s.Stats.Azimuth
			if !az.Sector || az.LargestEmptyArcFromDeg != tc.emptyFrom || az.LargestEmptyArcDeg != tc.emptyDg ||
				d.AzimuthCentreDeg != tc.centre || d.AzimuthHalfWidthDeg != tc.halfWidth {
				t.Fatalf("declaration %+v, azimuth %+v", d, az)
			}
			if !strings.Contains(d.Source, fmt.Sprintf("no estimate in the %d deg from %d (sector threshold 90 deg)", tc.emptyDg, tc.emptyFrom)) {
				t.Fatalf("source %q does not name the empty arc", d.Source)
			}
			// Every estimate lies inside the declared sector.
			for _, p := range points {
				off := math.Abs(math.Remainder(p.azimuthDeg-float64(d.AzimuthCentreDeg), 360))
				if off > float64(d.AzimuthHalfWidthDeg) {
					t.Fatalf("estimate at %g deg is outside the sector %+v", p.azimuthDeg, d)
				}
			}
		})
	}
}

// A percentile below 100 declares that rank, and says so; an unstamped build
// says so too.
func TestMeasureCoveragePercentile(t *testing.T) {
	var points []polar
	for i := 1; i <= 10; i++ {
		points = append(points, polar{rangeM: float64(i) + 0.25, azimuthDeg: float64(36 * i)})
	}
	opts := surveyOptions()
	opts.RangePercentile = 90
	m := surveyManifestFor()
	m.BuildStamped = false
	s, err := measureCoverage(opts, m, estimatesAt(points...))
	if err != nil {
		t.Fatal(err)
	}
	if s.Declaration.MaxRangeMetres != 10 || s.Stats.RangeAtPercentile != 9.25 || s.Stats.RangeMetres.P50 != 5.25 {
		t.Fatalf("declaration %+v, statistics %+v", s.Declaration, s.Stats.RangeMetres)
	}
	for _, want := range []string{"max_range_m is the p90 online-estimate range, 9.25 m", "git abc123, unstamped"} {
		if !strings.Contains(s.Declaration.Source, want) {
			t.Errorf("source %q lacks %q", s.Declaration.Source, want)
		}
	}
}

func TestMeasureCoverageRefusals(t *testing.T) {
	m := surveyManifestFor()
	if _, err := measureCoverage(surveyOptions(), m, nil); err == nil || !strings.Contains(err.Error(), "no online estimates") {
		t.Fatalf("no estimates: %v", err)
	}
	other := estimatesAt(polar{rangeM: 10})
	other[0].ParamHash = "sha256:other"
	if _, err := measureCoverage(surveyOptions(), m, other); err == nil || !strings.Contains(err.Error(), "the replay's is "+surveyParams) {
		t.Fatalf("another version's estimates: %v", err)
	}
	if _, err := measureCoverage(surveyOptions(), m, estimatesAt(polar{})); err == nil || !strings.Contains(err.Error(), "max_range_m must be positive") {
		t.Fatalf("an envelope of nothing: %v", err)
	}
}

func TestCoverageSurveyOptionsValidate(t *testing.T) {
	for name, mutate := range map[string]func(*CoverageSurveyOptions){
		"no case":        func(o *CoverageSurveyOptions) { o.CaseID = "" },
		"no tool":        func(o *CoverageSurveyOptions) { o.Tool = " " },
		"percentile 0":   func(o *CoverageSurveyOptions) { o.RangePercentile = 0 },
		"percentile 101": func(o *CoverageSurveyOptions) { o.RangePercentile = 101 },
		"gap 0":          func(o *CoverageSurveyOptions) { o.MinSectorGapDeg = 0 },
		"gap 361":        func(o *CoverageSurveyOptions) { o.MinSectorGapDeg = 361 },
		"gap NaN":        func(o *CoverageSurveyOptions) { o.MinSectorGapDeg = math.NaN() },
	} {
		opts := surveyOptions()
		mutate(&opts)
		if err := opts.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := surveyOptions().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLargestEmptyArc(t *testing.T) {
	var full [360]int
	for i := range full {
		full[i] = 1
	}
	if from, width := largestEmptyArc(full); width != 0 || from != 0 {
		t.Fatalf("no empty arc: from %d width %d", from, width)
	}
	var one [360]int
	one[100] = 3
	if from, width := largestEmptyArc(one); from != 101 || width != 359 {
		t.Fatalf("one occupied degree: from %d width %d", from, width)
	}
}

func writeSurveyManifest(t *testing.T, dir string, m any) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "replay_manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Only a default replay with an evidence database can be surveyed.
func TestReadSurveyManifestRefusesAnythingButTheDefaultReplay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*surveyManifest)
		want   string
	}{
		{"experiments", func(m *surveyManifest) { m.Experiments = []string{"solid_body"} }, "ran experiments [solid_body]"},
		{"measurement model", func(m *surveyManifest) { m.MeasurementSourceMode = "obb_centre_v1" }, `measured "obb_centre_v1"`},
		{"no evidence", func(m *surveyManifest) { m.ObservationSourceID = "" }, "wrote no evidence database"},
		{"no captures", func(m *surveyManifest) { m.SourceSHA256s = nil }, "names no capture digests"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m := surveyManifestFor()
			tc.mutate(&m)
			writeSurveyManifest(t, dir, m)
			if _, err := readSurveyManifest(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
	dir := t.TempDir()
	if _, err := readSurveyManifest(dir); err == nil || !strings.Contains(err.Error(), "replay_manifest.json") {
		t.Fatalf("missing manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "replay_manifest.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSurveyManifest(dir); err == nil || !strings.Contains(err.Error(), "parse replay manifest") {
		t.Fatalf("broken manifest: %v", err)
	}
}

// seedSurveyEvidence writes online estimates, each with the immutable
// observation it links to, into a new evidence database.
func seedSurveyEvidence(t *testing.T, path, sourceID string, estimates []observationsqlite.TrackEstimate) *db.DB {
	t.Helper()
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	for i, e := range estimates {
		obs := fmt.Sprintf("observation/%d", i)
		if _, err := database.Exec(`INSERT INTO lidar_observations
			(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
			 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
			VALUES (?, 1, ?, 'calibration/test', 'sensor', 'frame', ?, ?, 1, '{}', 1)`, obs, sourceID, i, i); err != nil {
			t.Fatal(err)
		}
		e.TrackID, e.ObservationID, e.SourceID, e.CalibrationID = fmt.Sprintf("track-%d", e.CreationSequence), obs, sourceID, "calibration/test"
		e.FrameUnixNanos, e.EstimatorID, e.ObservationModelID, e.Stage, e.MeasurementSource = int64(i), "cv_kf_v1", "medoid_v0", "online", "medoid_v0"
		e.Reference, e.Support = l5tracks.ReferenceClusterMedoid, l5tracks.SupportObserved
		r := observationsqlite.TrackResidual{EstimateID: e.EstimateID, ObservationID: obs, Disposition: "accepted", Reason: "association_accepted"}
		if err := observationsqlite.InsertStateEstimate(database, e, r); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

// The survey end to end over an evidence database, and its refusals before
// and after reading it.
func TestSurveyContinuityCoverageReadsTheEvidence(t *testing.T) {
	dir := t.TempDir()
	m := surveyManifestFor()
	writeSurveyManifest(t, dir, m)
	database := seedSurveyEvidence(t, filepath.Join(dir, "evidence.db"), m.ObservationSourceID,
		estimatesAt(polar{rangeM: 30.2, azimuthDeg: 10}, polar{rangeM: 12, azimuthDeg: 200, track: 1}))
	s, err := SurveyContinuityCoverage(dir, database, surveyOptions())
	if err != nil {
		t.Fatal(err)
	}
	if s.Declaration.MaxRangeMetres != 31 || s.Stats.OnlineEstimates != 2 || s.Stats.Tracks != 2 || s.Declaration.AzimuthHalfWidthDeg == 180 {
		t.Fatalf("survey %+v", s)
	}

	bad := surveyOptions()
	bad.CaseID = ""
	if _, err := SurveyContinuityCoverage(dir, database, bad); err == nil {
		t.Fatal("options without a case were accepted")
	}
	if _, err := SurveyContinuityCoverage(t.TempDir(), database, surveyOptions()); err == nil {
		t.Fatal("a replay without a manifest was surveyed")
	}
	empty := seedSurveyEvidence(t, filepath.Join(dir, "empty.db"), m.ObservationSourceID, nil)
	if _, err := SurveyContinuityCoverage(dir, empty, surveyOptions()); err == nil || !strings.Contains(err.Error(), "no online estimates") {
		t.Fatalf("an empty database: %v", err)
	}
	empty.Close()
	if _, err := SurveyContinuityCoverage(dir, empty, surveyOptions()); err == nil || !strings.Contains(err.Error(), "list track estimates") {
		t.Fatalf("a closed database: %v", err)
	}
}

func testSurvey(t *testing.T, caseID string, rangeM float64) *CoverageSurvey {
	t.Helper()
	opts := surveyOptions()
	opts.CaseID = caseID
	s, err := measureCoverage(opts, surveyManifestFor(), estimatesAt(polar{rangeM: rangeM, azimuthDeg: 45}, polar{rangeM: 5, azimuthDeg: 225}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Surveys accumulate into one set, readable by the corpus tool, with their
// statistics beside it; a case is never declared twice.
func TestAddCoverageSurveyAccumulatesWithoutReplacing(t *testing.T) {
	dir := t.TempDir()
	setPath := filepath.Join(dir, "continuity-coverage.json")
	if got := CoverageSurveyStatsPath(setPath); got != filepath.Join(dir, "continuity-coverage.survey.json") {
		t.Fatalf("stats path %s", got)
	}
	if got := CoverageSurveyStatsPath("set"); got != "set.survey.json" {
		t.Fatalf("stats path %s", got)
	}
	first, second := testSurvey(t, "a", 40), testSurvey(t, "b", 60.5)
	for _, s := range []*CoverageSurvey{first, second} {
		if err := AddCoverageSurvey(setPath, s); err != nil {
			t.Fatal(err)
		}
	}
	set, err := LoadContinuityCoverageSet(setPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 2 || set["a"] != first.Declaration || set["b"].MaxRangeMetres != 61 {
		t.Fatalf("set %+v", set)
	}
	_, stats, err := readCoverageSurveyFiles(setPath)
	if err != nil || len(stats) != 2 || stats["b"].DeclarationID != second.Declaration.ID() {
		t.Fatalf("statistics %+v, %v", stats, err)
	}
	if err := AddCoverageSurvey(setPath, testSurvey(t, "a", 80)); err == nil || !strings.Contains(err.Error(), "case a is already declared") {
		t.Fatalf("a second survey of a: %v", err)
	}
	if err := CheckCoverageSurveyTargets(setPath, []string{"c", "b"}); err == nil {
		t.Fatal("a target already declaring b was accepted")
	}
	invalid := testSurvey(t, "c", 10)
	invalid.Declaration.MaxRangeMetres = 0
	if err := AddCoverageSurvey(setPath, invalid); err == nil || !strings.Contains(err.Error(), "max_range_m") {
		t.Fatalf("an invalid declaration: %v", err)
	}
}

func TestAddCoverageSurveyRefusesDamagedTargets(t *testing.T) {
	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, setPath string)
		want  string
	}{
		{"invalid set", func(t *testing.T, setPath string) { write(t, setPath, `{"x": {"source": ""}}`) }, "case x"},
		{"broken statistics", func(t *testing.T, setPath string) { write(t, CoverageSurveyStatsPath(setPath), "{") }, "parse coverage survey statistics"},
		{"unreadable statistics", func(t *testing.T, setPath string) {
			if err := os.Mkdir(CoverageSurveyStatsPath(setPath), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "read coverage survey statistics"},
		{"set not writable", func(t *testing.T, setPath string) {
			if err := os.Mkdir(setPath+".tmp", 0o755); err != nil {
				t.Fatal(err)
			}
		}, "write continuity coverage set"},
		{"statistics not writable", func(t *testing.T, setPath string) {
			if err := os.Mkdir(CoverageSurveyStatsPath(setPath)+".tmp", 0o755); err != nil {
				t.Fatal(err)
			}
		}, "write coverage survey statistics"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPath := filepath.Join(t.TempDir(), "set.json")
			tc.setup(t, setPath)
			if err := AddCoverageSurvey(setPath, testSurvey(t, "a", 10)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}
