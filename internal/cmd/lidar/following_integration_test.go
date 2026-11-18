//go:build pcap

package lidar

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coredb "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/followingreview"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

func TestFollowingProjectedGapFollowsCurvedPathWithoutExtrapolation(t *testing.T) {
	p := &l8behaviour.LocalPath{Vertices: []l8behaviour.PathVertex{{X: 0, Y: 0, ArcM: 0}, {X: 3, Y: 0, ArcM: 3}, {X: 3, Y: 4, ArcM: 7}}}
	got := projectedGapPath(p, 1, 5)
	if len(got) != 3 || got[0] != [2]float64{1, 0} || got[1] != [2]float64{3, 0} || got[2] != [2]float64{3, 2} {
		t.Fatal(got)
	}
	for _, bounds := range [][2]float64{{-1, 3}, {1, 8}, {3, 3}, {4, 2}} {
		if projectedGapPath(p, bounds[0], bounds[1]) != nil {
			t.Fatal(bounds)
		}
	}
	if projectedGapPath(nil, 1, 2) != nil {
		t.Fatal("missing path")
	}
}

func followingJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return b
}

// A real recorded stream and the public preparation contract, not mocked
// canonical identities. Its opening frame deliberately has no evidence row.
func followingCorpusFixture(t *testing.T) (string, *sqlite.Run, string) {
	t.Helper()
	root := t.TempDir()
	vr := filepath.Join(root, "vrlog")
	r, err := recorder.NewRecorder(vr, "sensor")
	if err != nil {
		t.Fatal(err)
	}
	r.SetProvenance("pcap", "capture.pcap", "params", 1)
	r.SetDeterministicConfig("", "", "", "params", "", "", "", "", nil)
	for i, at := range []int64{1_000_000_000, 1_100_000_000, 1_200_000_000} {
		tracks := &l9endpoints.TrackSet{TimestampNanos: at, Tracks: []l9endpoints.Track{{TrackID: "follower", State: l9endpoints.TrackStateConfirmed}, {TrackID: "leader", State: l9endpoints.TrackStateConfirmed}}}
		if err = r.Record(&l9endpoints.FrameBundle{FrameID: uint64(i), TimestampNanos: at, SensorID: "sensor", FrameType: l9endpoints.FrameTypeForeground, Tracks: tracks}); err != nil {
			t.Fatal(err)
		}
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	hb, err := os.ReadFile(filepath.Join(vr, "header.json"))
	if err != nil {
		t.Fatal(err)
	}
	var h recorder.LogHeader
	if err = json.Unmarshal(hb, &h); err != nil {
		t.Fatal(err)
	}
	source := l4bobserve.CaptureSource{ClipID: "clip", CapturePaths: []string{"/capture.pcap"}, CaptureSHA256s: []string{strings.Repeat("a", 64)}, ExtractorID: "extractor", Window: &l4bobserve.ReplayWindow{DurationNanos: 2_000_000_000}}
	sourceID, err := l4bobserve.SourceID(source)
	if err != nil {
		t.Fatal(err)
	}
	c := fieldrun.EvidenceContext{SiteNamespace: "test", SiteID: "site", SourceID: sourceID, BindingID: "binding", BuildGitSHA: h.BuildGitSHA, PolicyID: "policy", SourceManifest: &source}
	v := l8behaviour.InteractionVersion{EstimateStage: l8behaviour.StageOnline, EstimatorID: "estimator", ObsModelID: "published_box/model", MethodID: "method", ParamHash: "params"}
	cal := l4bobserve.Calibration{SensorID: "sensor", FromFrame: "sensor", ToFrame: "sensor", Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calID, err := l4bobserve.CalibrationID(cal)
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, "canonical.json")
	cb := followingJSON(t, canonical, map[string]any{"schema": fieldrun.EvidenceSchema, "context": c, "analysis_version": v, "analysis": map[string]any{"paths": []any{}}})
	gap, seconds, demand := 10.0, 1.0, 0.2
	prov := l8behaviour.Provenance{Version: l8behaviour.VersionProvenance{EstimateStage: l8behaviour.StageOnline, EstimatorID: "estimator", ObsModelID: "model", MethodID: "method", ParamHash: "params"}, Input: l8behaviour.InputProvenance{ContributingTrackIDs: []string{"follower", "leader"}, FirstUnixNanos: 1_100_000_000, LastUnixNanos: 1_100_000_000, ObservedFrames: 2}}
	spatial, err := l8behaviour.NewSuppressedMeasurement(l8behaviour.MetricFollowingSpatialGap, l8behaviour.ReasonEstimateNotFinal, prov)
	if err != nil {
		t.Fatal(err)
	}
	timeGap, err := l8behaviour.NewSuppressedMeasurement(l8behaviour.MetricFollowingNetTimeGap, l8behaviour.ReasonEstimateNotFinal, prov)
	if err != nil {
		t.Fatal(err)
	}
	rows := []fieldrun.EvidenceRow{{FollowerTrackID: "follower", CaptureUnixNanos: 1_100_000_000, IntervalNanos: 100_000_000, Bucket: "following_eligible", Decision: l8behaviour.PairingDecision{FollowerTrackID: "follower", LeaderTrackID: "leader", CaptureUnixNanos: 1_100_000_000, Candidates: []l8behaviour.LeaderCandidate{{TrackID: "leader", Disposition: l8behaviour.DispositionLeader}}}, Point: &l8behaviour.FollowingPoint{FollowerTrackID: "follower", LeaderTrackID: "leader", CaptureUnixNanos: 1_100_000_000, Gap: &l8behaviour.GapEstimate{ValueM: gap, LeaderSource: l8behaviour.EndpointTemporallyInferred, FollowerSource: l8behaviour.EndpointTemporallyInferred}, TimeGap: &l8behaviour.TimeGapEstimate{ValueS: &seconds}, SpatialGap: spatial, NetTimeGap: timeGap}, DRAC: fieldrun.EvidenceDRAC{Status: "supported", Calculation: &l8behaviour.DRACResult{Status: l8behaviour.DRACSupported, ValueMPS2: &demand}}}, {FollowerTrackID: "follower", CaptureUnixNanos: 1_200_000_000, Bucket: "record_gap", Decision: l8behaviour.PairingDecision{FollowerTrackID: "follower", CaptureUnixNanos: 1_200_000_000}}}
	slice := filepath.Join(root, "minute.json")
	sb := followingJSON(t, slice, map[string]any{"schema": "following_review_slice_v1", "context": c, "analysis_version": v, "rows": rows, "review_slice": map[string]any{"canonical_evidence_sha256": followingreview.Digest(cb), "recording_header_sha256": followingreview.Digest(hb), "recording_build_git_sha": h.BuildGitSHA, "calibration_id": calID, "sample_start_nanos": 1_100_000_000, "sample_end_exclusive_nanos": 1_200_000_001}})
	followingJSON(t, filepath.Join(root, "complete.json"), map[string]any{"schema": "following_review_preparation_v1", "context": c, "calibration": cal, "recording_header": h, "vrlog": vr, "evidence": canonical, "rows": 2, "purpose": "annotation_review", "slices": []any{map[string]any{"path": slice, "sha256": followingreview.Digest(sb), "sample_start_nanos": 1_100_000_000, "sample_end_exclusive_nanos": 1_200_000_001, "rows": 2}}})
	path := filepath.Join(t.TempDir(), "registered.db")
	db, err := coredb.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	run := &sqlite.Run{RunID: "paired", Status: "completed", SensorID: "sensor", SourceType: "pcap", SourcePath: "/capture.pcap", VRLogPath: vr, BuildGitSHA: h.BuildGitSHA}
	if err = sqlite.NewRunStore(db).InsertRun(run); err != nil {
		t.Fatal(err)
	}
	return path, run, slice
}

func TestFollowingCorpusPublishesPairedSceneAndNoPartialSuccess(t *testing.T) {
	db, run, slice := followingCorpusFixture(t)
	out := filepath.Join(t.TempDir(), "corpus")
	if err := analyseCorpus(db, []string{run.RunID}, out, true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "complete.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Recordings []followingreview.Analysis `json:"recordings"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Recordings) != 1 || result.Recordings[0].Histogram[2].Count != 1 {
		t.Fatal(string(b))
	}
	if _, err = os.Stat(filepath.Join(out, "recording-000", "scene", "header.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(out, "recording-000", "manifest.json")); err != nil {
		t.Fatal("standalone scene manifest:", err)
	}
	index := filepath.Join(run.VRLogPath, "index.bin")
	if err = os.Rename(index, index+".saved"); err != nil {
		t.Fatal(err)
	}
	if err = analyseCorpus(db, []string{run.RunID}, filepath.Join(t.TempDir(), "broken-recording"), true); err == nil {
		t.Fatal("broken scene accepted")
	}
	if err = os.Rename(index+".saved", index); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "partial")
	if err = analyseCorpus(db, []string{run.RunID, "missing"}, missing, false); err == nil {
		t.Fatal("partial corpus accepted")
	}
	if _, err = os.Stat(filepath.Join(missing, "complete.json")); !os.IsNotExist(err) {
		t.Fatal("partial success marker")
	}
	writeFailure := errors.New("disk full")
	if err = analyseCorpusWrite(db, []string{run.RunID}, filepath.Join(t.TempDir(), "write-failure"), false, func(string, []byte, os.FileMode) error { return writeFailure }); !errors.Is(err, writeFailure) {
		t.Fatal(err)
	}
	var cache followingreview.Cache
	s, err := cache.Open(run)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(slice, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = exportFollowingScene(s, filepath.Join(t.TempDir(), "changed-scene"), followingreview.Analysis{}); err == nil {
		t.Fatal("changed evidence accepted")
	}
	if err = analyseCorpus(db, []string{run.RunID}, filepath.Join(t.TempDir(), "changed-corpus"), false); err == nil {
		t.Fatal("changed preparation accepted")
	}
}

func TestFollowingCorpusRefusesInvalidSampleOrder(t *testing.T) {
	db, run, slice := followingCorpusFixture(t)
	b, err := os.ReadFile(slice)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err = decoder.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	rows := doc["rows"].([]any)
	rows[0], rows[1] = rows[1], rows[0]
	b = followingJSON(t, slice, doc)
	receipt := filepath.Join(filepath.Dir(run.VRLogPath), "complete.json")
	rb, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var r map[string]any
	decoder = json.NewDecoder(bytes.NewReader(rb))
	decoder.UseNumber()
	if err = decoder.Decode(&r); err != nil {
		t.Fatal(err)
	}
	r["slices"].([]any)[0].(map[string]any)["sha256"] = followingreview.Digest(b)
	followingJSON(t, receipt, r)
	if err = analyseCorpus(db, []string{run.RunID}, filepath.Join(t.TempDir(), "unordered"), false); err == nil || !strings.Contains(err.Error(), "unordered") {
		t.Fatal("expected sample-order refusal, got", err)
	}
}

func TestFollowingCLIParsingAndRunnerErrors(t *testing.T) {
	args := []string{"--db", "db", "--runs", "one,two", "--output", "out", "--scenes"}
	if code := followingMain(args, func(db string, ids []string, out string, scenes bool) error {
		if db != "db" || len(ids) != 2 || out != "out" || !scenes {
			t.Fatal(db, ids, out, scenes)
		}
		return nil
	}); code != 0 {
		t.Fatal(code)
	}
	if code := followingMain(args, func(string, []string, string, bool) error { return errors.New("refused") }); code != 1 {
		t.Fatal(code)
	}
	if code := FollowingMain([]string{"--unknown"}); code != 2 {
		t.Fatal(code)
	}
	if err := analyseCorpus(filepath.Join(t.TempDir(), "missing.db"), []string{"one"}, filepath.Join(t.TempDir(), "out"), false); err == nil {
		t.Fatal("missing database accepted")
	}
}
