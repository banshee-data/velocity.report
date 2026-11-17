package followingreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/google/uuid"
)

type fixture struct {
	root    string
	run     *sqlite.Run
	receipt receipt
	doc     map[string]any
	slice   reviewSlice
	bytes   []byte
}

func writeJSON(t *testing.T, path string, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	return b
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	vr := filepath.Join(root, "vrlog")
	if e := os.Mkdir(vr, 0700); e != nil {
		t.Fatal(e)
	}
	source := l4bobserve.CaptureSource{ClipID: "clip", CapturePaths: []string{"/source/capture.pcap"}, CaptureSHA256s: []string{strings.Repeat("a", 64)}, ExtractorID: "extractor", Window: &l4bobserve.ReplayWindow{DurationNanos: 1000}}
	id, e := l4bobserve.SourceID(source)
	if e != nil {
		t.Fatal(e)
	}
	c := fieldrun.EvidenceContext{SiteNamespace: "test", SiteID: "site", SourceID: id, BindingID: "binding", BuildGitSHA: "build", PolicyID: "policy", SourceManifest: &source}
	v := l8behaviour.InteractionVersion{EstimateStage: l8behaviour.StageOnline, EstimatorID: "estimator", ObsModelID: "published_box/model", MethodID: "method", ParamHash: "params"}
	h := recorder.LogHeader{StartNs: 1000, EndNs: 1100, TotalFrames: 2, SensorID: "sensor", SourceType: "pcap", BuildGitSHA: "build", ParamsHash: "params"}
	hb := writeJSON(t, filepath.Join(vr, "header.json"), h)
	cal := l4bobserve.Calibration{SensorID: "sensor", FromFrame: "sensor", ToFrame: "sensor", Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calID, e := l4bobserve.CalibrationID(cal)
	if e != nil {
		t.Fatal(e)
	}
	path := l8behaviour.LocalPathResult{FollowerTrackID: "follower", MethodID: "path-method", Path: &l8behaviour.LocalPath{ID: "geometry", EstimateStage: l8behaviour.StageOnline, Vertices: []l8behaviour.PathVertex{{X: 0, Y: 0, ArcM: 0}, {X: 10, Y: 0, ArcM: 10}}}}
	doc := map[string]any{"schema": fieldrun.EvidenceSchema, "context": c, "analysis_version": v, "trajectories": []any{map[string]any{"samples": []any{1, true, nil, "escaped\\string"}}}, "analysis": map[string]any{"paths": []l8behaviour.LocalPathResult{path}, "timelines": []any{}}, "rows": []any{}}
	canonicalPath := filepath.Join(root, "canonical.json")
	cb := writeJSON(t, canonicalPath, doc)
	rows := []fieldrun.EvidenceRow{{FollowerTrackID: "follower", CaptureUnixNanos: 1000, IntervalNanos: 100, Decision: l8behaviour.PairingDecision{FollowerTrackID: "follower", CaptureUnixNanos: 1000}}, {FollowerTrackID: "follower", CaptureUnixNanos: 1100, Decision: l8behaviour.PairingDecision{FollowerTrackID: "follower", CaptureUnixNanos: 1100}}}
	s := reviewSlice{Schema: "following_review_slice_v1", Context: c, Version: v, Review: sliceMeta{digest(cb), 1000, 1101, digest(hb), "build", calID}, Rows: rows}
	slicePath := filepath.Join(root, "minute.json")
	sb := writeJSON(t, slicePath, s)
	r := receipt{Schema: "following_review_preparation_v1", Context: c, Calibration: cal, Header: h, VRLOG: vr, Evidence: canonicalPath, Rows: 2, Purpose: "annotation_review", Slices: []sliceFile{{slicePath, digest(sb), 1000, 1101, 2}}}
	writeJSON(t, filepath.Join(root, "complete.json"), r)
	return &fixture{root, &sqlite.Run{RunID: "run", Status: "completed", VRLogPath: vr, SensorID: "sensor", BuildGitSHA: "build"}, r, doc, s, sb}
}

func TestSessionExactEvidenceGeometryAndCache(t *testing.T) {
	f := newFixture(t)
	var cache Cache
	s, e := cache.Open(f.run)
	if e != nil {
		t.Fatal(e)
	}
	if s.Metadata.Schema != "following_review_session_v1" || s.Metadata.Binding.EvidenceSHA256 != s.Metadata.EvidenceSHA256 || s.Metadata.Start != 1000 || s.Metadata.End != 1101 {
		t.Fatal(s.Metadata)
	}
	b, rows, e := s.Evidence(1100)
	if e != nil || string(b) != string(f.bytes) || len(rows) != 2 || Digest(b) != f.receipt.Slices[0].SHA256 {
		t.Fatal(e)
	}
	g, e := s.Geometry(1000, "follower")
	if e != nil || len(g.Paths) != 1 || g.Paths[0].Path.Vertices[1].X != 10 || !g.RecordingCoordinates || g.CoordinateFrameID != "sensor" {
		t.Fatal(g, e)
	}
	if _, e = s.Geometry(1000, "missing"); e == nil {
		t.Fatal("unknown follower")
	}
	if _, e = s.Geometry(999, ""); e == nil {
		t.Fatal("unknown time")
	}
	s2, e := cache.Open(f.run)
	if e != nil || s2 != s {
		t.Fatal("cache miss", e)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := cache.Open(f.run); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	// A changed signed slice is checked even on a metadata-cache hit.
	if e = os.WriteFile(f.receipt.Slices[0].Path, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Evidence(1000); e == nil {
		t.Fatal("changed slice accepted")
	}
	if _, e = s.Geometry(1000, ""); e == nil {
		t.Fatal("geometry ignored changed slice")
	}
}

func TestSessionRefusesUntrustedReceiptAndRun(t *testing.T) {
	cases := map[string]func(*fixture){
		"missing recording": func(f *fixture) { f.run.VRLogPath = "" }, "unfinished": func(f *fixture) { f.run.Status = "running" }, "absent path": func(f *fixture) { f.run.VRLogPath = "/absent" },
		"wrong run sensor": func(f *fixture) { f.run.SensorID = "other" }, "relative run path": func(f *fixture) { f.run.VRLogPath = "relative" }, "source kind": func(f *fixture) { f.run.SourceType = "live" }, "clip identity": func(f *fixture) { f.run.ClipID = "other" }, "capture path": func(f *fixture) { f.run.SourcePath = "/wrong/capture.pcap" }, "wrong run build": func(f *fixture) { f.run.BuildGitSHA = "other" }, "wrong run window": func(f *fixture) { n := int64(2); f.run.FrameStartNs = &n },
		"schema": func(f *fixture) { f.receipt.Schema = "other" }, "purpose": func(f *fixture) { f.receipt.Purpose = "tuning" }, "rows": func(f *fixture) { f.receipt.Rows = 3 }, "slices absent": func(f *fixture) { f.receipt.Slices = nil },
		"vrlog": func(f *fixture) { f.receipt.VRLOG = f.root }, "escape": func(f *fixture) {
			f.receipt.Evidence = filepath.Join(t.TempDir(), "outside")
			writeJSON(t, f.receipt.Evidence, f.doc)
		},
		"header": func(f *fixture) { f.receipt.Header.BuildGitSHA = "other" }, "window overflow": func(f *fixture) {
			f.receipt.Header.EndNs = 1<<63 - 1
			writeJSON(t, filepath.Join(f.run.VRLogPath, "header.json"), f.receipt.Header)
		},
		"context": func(f *fixture) { f.receipt.Context.SiteID = "" }, "source": func(f *fixture) { f.receipt.Context.SourceID = "other" }, "source window": func(f *fixture) { f.receipt.Context.SourceManifest.Window = nil },
		"calibration": func(f *fixture) { f.receipt.Calibration.SensorID = "other" }, "slice interval": func(f *fixture) { f.receipt.Slices[0].End = 1200 }, "slice rows": func(f *fixture) { f.receipt.Slices[0].Rows = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			mutate(f)
			writeJSON(t, filepath.Join(f.root, "complete.json"), f.receipt)
			if _, e := new(Cache).Open(f.run); e == nil {
				t.Fatal("accepted mismatch")
			}
		})
	}
}

func TestSessionRefusesCanonicalAndSliceMutations(t *testing.T) {
	for _, what := range []string{"canonical identity", "canonical digest", "slice binding", "slice duplicate", "slice timestamp", "slice overflow", "slice decision", "slice point", "slice invalid json"} {
		t.Run(what, func(t *testing.T) {
			f := newFixture(t)
			switch what {
			case "canonical identity":
				f.doc["context"] = fieldrun.EvidenceContext{}
				writeJSON(t, f.receipt.Evidence, f.doc)
			case "canonical digest":
				f.doc["rows"] = []any{1}
				writeJSON(t, f.receipt.Evidence, f.doc)
			case "slice binding":
				f.slice.Review.HeaderSHA = "other"
			case "slice duplicate":
				f.slice.Rows[1] = f.slice.Rows[0]
			case "slice timestamp":
				f.slice.Rows[0].CaptureUnixNanos = 0
			case "slice overflow":
				f.slice.Rows[0].IntervalNanos = 1<<63 - 1
			case "slice decision":
				f.slice.Rows[0].Decision.FollowerTrackID = "other"
			case "slice point":
				// Inject decoded point below to avoid serialising incomplete metric provenance.
			case "slice invalid json":
			}
			if !strings.HasPrefix(what, "canonical") {
				b := writeJSON(t, f.receipt.Slices[0].Path, f.slice)
				if what == "slice point" {
					var raw map[string]any
					_ = json.Unmarshal(b, &raw)
					raw["rows"].([]any)[0].(map[string]any)["point"] = map[string]any{"follower_track_id": "other"}
					b = writeJSON(t, f.receipt.Slices[0].Path, raw)
				}
				if what == "slice invalid json" {
					b = []byte("{")
					if e := os.WriteFile(f.receipt.Slices[0].Path, b, 0600); e != nil {
						t.Fatal(e)
					}
				}
				f.receipt.Slices[0].SHA256 = digest(b)
				writeJSON(t, filepath.Join(f.root, "complete.json"), f.receipt)
			}
			if _, e := new(Cache).Open(f.run); e == nil {
				t.Fatal("accepted mutation")
			}
		})
	}
}

func TestAuditRecordValidation(t *testing.T) {
	f := newFixture(t)
	s, e := new(Cache).Open(f.run)
	if e != nil {
		t.Fatal(e)
	}
	r := Record{ID: uuid.NewString(), Reviewer: "operator", SavedAt: time.Now().UTC(), FollowerTrackID: "follower", StartNanos: 1000, EndExclusiveNanos: 1100, IdentityContinuity: "unknown", LeaderAndPath: "notVisible", VisibleEndpoints: "disputed", SpeedEvidence: "consistent", Conclusion: "insufficientEvidence", Notes: "visibility uncertain", Playback: Playback{RunID: "run"}}
	if e = ValidateRecord(r, f.slice.Rows, "run", s.Metadata.Start, s.Metadata.End); e != nil {
		t.Fatal(e)
	}
	cases := []func(*Record){func(r *Record) { r.ID = "bad" }, func(r *Record) { r.Reviewer = "" }, func(r *Record) { r.Notes = "" }, func(r *Record) { r.SavedAt = time.Time{} }, func(r *Record) { r.StartNanos = 1001 }, func(r *Record) { r.EndExclusiveNanos = 999 }, func(r *Record) { r.EndExclusiveNanos = 1101 }, func(r *Record) { r.StartNanos = 999 }, func(r *Record) { r.IdentityContinuity = "bad" }, func(r *Record) { r.Conclusion = "consistent" }, func(r *Record) { r.Conclusion = "bad" }, func(r *Record) { r.Playback.RunID = "other" }}
	for i, mutate := range cases {
		copy := r
		mutate(&copy)
		if e = ValidateRecord(copy, f.slice.Rows, "run", 1000, 1101); e == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	r.Conclusion = "consistent"
	r.IdentityContinuity = "consistent"
	r.LeaderAndPath = "consistent"
	r.VisibleEndpoints = "consistent"
	if e = ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); e != nil {
		t.Fatal(e)
	}
	r.Conclusion = "disagreement"
	r.StartNanos = 1100
	r.EndExclusiveNanos = 1100
	if e = ValidateRecord(r, f.slice.Rows, "run", 1000, 1101); e != nil {
		t.Fatal(e)
	}
}
