package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/followingreview"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/google/uuid"
)

func reviewJSON(t *testing.T, path string, v any) []byte {
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

func setupFollowingReview(t *testing.T) (*Server, string, []byte) {
	t.Helper()
	// Use the production connection setup: busy_timeout is present in the
	// DSN for every pooled connection, including concurrent append writers.
	db, cleanup := dbpkg.NewTestDB(t)
	t.Cleanup(cleanup)
	ws := &Server{db: db}
	root := t.TempDir()
	vr := filepath.Join(root, "vrlog")
	if e := os.Mkdir(vr, 0700); e != nil {
		t.Fatal(e)
	}
	source := l4bobserve.CaptureSource{ClipID: "clip", CapturePaths: []string{"/capture.pcap"}, CaptureSHA256s: []string{strings.Repeat("a", 64)}, ExtractorID: "extractor", Window: &l4bobserve.ReplayWindow{DurationNanos: 1000}}
	id, e := l4bobserve.SourceID(source)
	if e != nil {
		t.Fatal(e)
	}
	c := fieldrun.EvidenceContext{SiteNamespace: "test", SiteID: "site", SourceID: id, BindingID: "binding", BuildGitSHA: "build", PolicyID: "policy", SourceManifest: &source}
	v := l8behaviour.InteractionVersion{EstimateStage: l8behaviour.StageOnline, EstimatorID: "estimator", ObsModelID: "published_box/model", MethodID: "method", ParamHash: "params"}
	h := recorder.LogHeader{StartNs: 1000, EndNs: 1100, TotalFrames: 2, SensorID: "sensor", SourceType: "pcap", BuildGitSHA: "build", ParamsHash: "params"}
	hb := reviewJSON(t, filepath.Join(vr, "header.json"), h)
	cal := l4bobserve.Calibration{SensorID: "sensor", FromFrame: "sensor", ToFrame: "sensor", Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calID, e := l4bobserve.CalibrationID(cal)
	if e != nil {
		t.Fatal(e)
	}
	path := map[string]any{"follower_track_id": "follower", "method_id": "path", "path": map[string]any{"id": "geometry", "estimate_stage": "online", "vertices": []any{map[string]any{"x": 0, "y": 0, "arc_m": 0}, map[string]any{"x": 10, "y": 0, "arc_m": 10}}}}
	canonical := filepath.Join(root, "canonical.json")
	cb := reviewJSON(t, canonical, map[string]any{"schema": fieldrun.EvidenceSchema, "context": c, "analysis_version": v, "analysis": map[string]any{"paths": []any{path}}})
	rows := []fieldrun.EvidenceRow{{FollowerTrackID: "follower", CaptureUnixNanos: 1000, IntervalNanos: 100, Decision: l8behaviour.PairingDecision{FollowerTrackID: "follower", CaptureUnixNanos: 1000, Candidates: []l8behaviour.LeaderCandidate{{TrackID: "alternative", Disposition: l8behaviour.DispositionCompeting}}}}, {FollowerTrackID: "follower", CaptureUnixNanos: 1100, Decision: l8behaviour.PairingDecision{FollowerTrackID: "follower", CaptureUnixNanos: 1100}}}
	slice := filepath.Join(root, "minute.json")
	sb := reviewJSON(t, slice, map[string]any{"schema": "following_review_slice_v1", "context": c, "analysis_version": v, "rows": rows, "review_slice": map[string]any{"canonical_evidence_sha256": followingreview.Digest(cb), "recording_header_sha256": followingreview.Digest(hb), "recording_build_git_sha": "build", "calibration_id": calID, "sample_start_nanos": 1000, "sample_end_exclusive_nanos": 1101}})
	reviewJSON(t, filepath.Join(root, "complete.json"), map[string]any{"schema": "following_review_preparation_v1", "context": c, "calibration": cal, "recording_header": h, "vrlog": vr, "evidence": canonical, "rows": 2, "purpose": "annotation_review", "slices": []any{map[string]any{"path": slice, "sha256": followingreview.Digest(sb), "sample_start_nanos": 1000, "sample_end_exclusive_nanos": 1101, "rows": 2}}})
	run := &sqlite.Run{RunID: "review-run", SensorID: "sensor", SourceType: "pcap", SourcePath: "/capture.pcap", Status: "completed", VRLogPath: vr, BuildGitSHA: "build"}
	if e = sqlite.NewRunStore(db).InsertRun(run); e != nil {
		t.Fatal(e)
	}
	return ws, root, sb
}

func callReview(ws *Server, method, action string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/api/lidar/runs/review-run/following-review"+action, bytes.NewReader(body))
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, r)
	return w
}

func reviewAuditRequest(t *testing.T, ws *Server) (followingreview.Metadata, followingreview.Record) {
	t.Helper()
	w := callReview(ws, "GET", "", nil)
	if w.Code != 200 {
		t.Fatalf("metadata %d: %s", w.Code, w.Body.String())
	}
	var m followingreview.Metadata
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		t.Fatal(e)
	}
	r := followingreview.Record{ID: uuid.NewString(), Reviewer: "operator", SavedAt: time.Now().UTC(), FollowerTrackID: "follower", StartNanos: 1000, EndExclusiveNanos: 1100, IdentityContinuity: "unknown", LeaderAndPath: "disputed", VisibleEndpoints: "notVisible", SpeedEvidence: "consistent", Conclusion: "disagreement", Notes: "candidate requires review", Playback: followingreview.Playback{RunID: "review-run"}}
	return m, r
}

func auditBody(t *testing.T, m followingreview.Metadata, r followingreview.Record) []byte {
	t.Helper()
	b, e := json.Marshal(map[string]any{"binding": m.Binding, "record": r})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestFollowingReviewRoutesAndAuditRoundTrip(t *testing.T) {
	ws, _, raw := setupFollowingReview(t)
	m, r := reviewAuditRequest(t, ws)
	r.Playback.FrameRecordingPath = m.RecordingPath
	r.Playback.FrameRecordingHeaderSHA256 = m.HeaderSHA256
	w := callReview(ws, "GET", "/evidence?at_ns=1000", nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), raw) || w.Header().Get("X-Evidence-SHA256") != followingreview.Digest(raw) || w.Header().Get("X-Canonical-Evidence-SHA256") != m.EvidenceSHA256 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = callReview(ws, "GET", "/geometry?at_ns=1000&follower=follower", nil)
	var g followingreview.Geometry
	if e := json.Unmarshal(w.Body.Bytes(), &g); e != nil || w.Code != 200 || len(g.Paths) != 1 || !g.RecordingCoordinates {
		t.Fatal(w.Code, e)
	}
	for i, status := range []int{201, 200} {
		w = callReview(ws, "POST", "/audit", auditBody(t, m, r))
		if w.Code != status {
			t.Fatalf("attempt %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w = callReview(ws, "GET", "/audit", nil)
	var a followingreview.Audit
	if e := json.Unmarshal(w.Body.Bytes(), &a); e != nil || w.Code != 200 || a.Schema != "following_audit_v1" || a.Binding != m.Binding || len(a.Records) != 1 {
		t.Fatal(w.Code, e, a)
	}
	var retained followingreview.Record
	if e := json.Unmarshal(a.Records[0], &retained); e != nil || retained.Playback.FrameRecordingPath != m.RecordingPath || retained.Playback.FrameRecordingHeaderSHA256 != m.HeaderSHA256 {
		t.Fatalf("frame recording context lost: %+v, %v", retained.Playback, e)
	}
	r.Notes = "different immutable content"
	if w = callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	// A retained record survives a new Server instance: it is not UI memory.
	restarted := &Server{db: ws.db}
	if w = callReview(restarted, "GET", "/audit", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "candidate requires review") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestFollowingReviewRouteFailures(t *testing.T) {
	ws, root, _ := setupFollowingReview(t)
	m, r := reviewAuditRequest(t, ws)
	for _, c := range []struct {
		method, action string
		status         int
	}{{"POST", "", 405}, {"PUT", "/audit", 405}, {"GET", "/unknown", 404}, {"GET", "/evidence", 400}, {"GET", "/evidence?at_ns=9223372036854775808", 400}, {"GET", "/evidence?at_ns=1", 409}, {"GET", "/geometry?at_ns=1", 409}, {"GET", "/geometry?at_ns=1000&follower=unknown", 409}} {
		if w := callReview(ws, c.method, c.action, nil); w.Code != c.status {
			t.Fatalf("%s %s=%d: %s", c.method, c.action, w.Code, w.Body.String())
		}
	}
	if w := callReview(&Server{}, "GET", "", nil); w.Code != 503 {
		t.Fatal(w.Code)
	}
	missing := httptest.NewRequest("GET", "/api/lidar/runs/unknown/following-review", nil)
	w := httptest.NewRecorder()
	ws.handleRunTrackAPI(w, missing)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	for _, b := range [][]byte{[]byte("{"), []byte(`{"unknown":1}`), append(auditBody(t, m, r), []byte("{}")...), []byte(strings.Repeat("x", 128<<10+1))} {
		if w := callReview(ws, "POST", "/audit", b); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	stale := m
	stale.Binding.EvidenceSHA256 = "sha256:" + strings.Repeat("f", 64)
	if w := callReview(ws, "POST", "/audit", auditBody(t, stale, r)); w.Code != 409 {
		t.Fatal(w.Code)
	}
	r.StartNanos = 1001
	if w := callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 400 {
		t.Fatal(w.Code)
	}
	r.StartNanos = 999
	if w := callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if e := os.WriteFile(filepath.Join(root, "minute.json"), []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	if w := callReview(ws, "GET", "/evidence?at_ns=1000", nil); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if e := os.Remove(filepath.Join(root, "complete.json")); e != nil {
		t.Fatal(e)
	}
	if w := callReview(ws, "GET", "", nil); w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestFollowingReviewProposedLeaderAndConcurrentAppends(t *testing.T) {
	ws, _, _ := setupFollowingReview(t)
	m, r := reviewAuditRequest(t, ws)
	alternative := "alternative"
	r.ProposedLeaderTrackID = &alternative
	if w := callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, id := range []string{"follower", "absent"} {
		r.ID = uuid.NewString()
		r.ProposedLeaderTrackID = &id
		if w := callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	r.ProposedLeaderTrackID = nil
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := r
			copy.ID = uuid.NewString()
			b, e := json.Marshal(map[string]any{"binding": m.Binding, "record": copy})
			if e != nil {
				t.Error(e)
				return
			}
			if w := callReview(ws, "POST", "/audit", b); w.Code != 201 {
				t.Errorf("append %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	w := callReview(ws, "GET", "/audit", nil)
	var a followingreview.Audit
	if e := json.Unmarshal(w.Body.Bytes(), &a); e != nil || len(a.Records) != 9 {
		t.Fatal(e, len(a.Records), w.Body.String())
	}
}

func TestFollowingReviewDatabaseFailuresAndAuditLimit(t *testing.T) {
	t.Run("registered run read", func(t *testing.T) {
		ws, _, _ := setupFollowingReview(t)
		if e := ws.db.Close(); e != nil {
			t.Fatal(e)
		}
		if w := callReview(ws, "GET", "", nil); w.Code != 500 {
			t.Fatal(w.Code)
		}
	})
	for _, method := range []string{"GET", "POST"} {
		t.Run("audit storage "+method, func(t *testing.T) {
			ws, _, _ := setupFollowingReview(t)
			m, r := reviewAuditRequest(t, ws)
			if _, e := ws.db.Exec("DROP TABLE lidar_following_audit_records"); e != nil {
				t.Fatal(e)
			}
			if w := callReview(ws, method, "/audit", auditBody(t, m, r)); w.Code != 500 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	t.Run("record cap is atomic", func(t *testing.T) {
		ws, _, _ := setupFollowingReview(t)
		m, r := reviewAuditRequest(t, ws)
		if w := callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		raw, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		_, e = ws.db.Exec(`WITH RECURSIVE ids(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM ids WHERE n<9999) INSERT INTO lidar_following_audit_records(run_id,evidence_digest,record_id,record_json) SELECT ?,?,printf('00000000-0000-4000-8000-%012d',n),json_set(?,'$.id',printf('00000000-0000-4000-8000-%012d',n)) FROM ids`, m.RunID, m.EvidenceSHA256, string(raw))
		if e != nil {
			t.Fatal(e)
		}
		r.ID = uuid.NewString()
		if w := callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 413 {
			t.Fatal(w.Code, w.Body.String())
		}
		var n int
		if e = ws.db.QueryRow("SELECT count(*) FROM lidar_following_audit_records").Scan(&n); e != nil || n != 10000 {
			t.Fatal(e, n)
		}
	})
}

func TestFollowingReviewAuditPlaybackWireCompatibility(t *testing.T) {
	ws, _, _ := setupFollowingReview(t)
	m, r := reviewAuditRequest(t, ws)
	for _, kind := range []string{"omitted", "null", "empty object", "null ids", "empty ids"} {
		t.Run(kind, func(t *testing.T) {
			r.ID = uuid.NewString()
			var payload map[string]any
			if e := json.Unmarshal(auditBody(t, m, r), &payload); e != nil {
				t.Fatal(e)
			}
			record := payload["record"].(map[string]any)
			switch kind {
			case "omitted":
				delete(record, "playback")
			case "null":
				record["playback"] = nil
			case "empty object":
				record["playback"] = map[string]any{}
			case "null ids":
				record["playback"] = map[string]any{"frameSourceIDs": nil}
			case "empty ids":
				record["playback"] = map[string]any{"frameSourceIDs": []string{}}
			}
			body, e := json.Marshal(payload)
			if e != nil {
				t.Fatal(e)
			}
			w := callReview(ws, "POST", "/audit", body)
			if w.Code != 201 {
				t.Fatal(w.Code, w.Body.String())
			}
			var response struct {
				Record struct {
					Playback struct {
						FrameSourceIDs json.RawMessage `json:"frameSourceIDs"`
					} `json:"playback"`
				} `json:"record"`
			}
			if e = json.Unmarshal(w.Body.Bytes(), &response); e != nil || string(response.Record.Playback.FrameSourceIDs) != "[]" {
				t.Fatal(e, w.Body.String())
			}
		})
	}
	w := callReview(ws, "GET", "/audit", nil)
	var audit followingreview.Audit
	if e := json.Unmarshal(w.Body.Bytes(), &audit); e != nil || len(audit.Records) != 5 {
		t.Fatal(e, w.Body.String())
	}
	for _, raw := range audit.Records {
		var record struct {
			Playback struct {
				FrameSourceIDs json.RawMessage `json:"frameSourceIDs"`
			} `json:"playback"`
		}
		if e := json.Unmarshal(raw, &record); e != nil || string(record.Playback.FrameSourceIDs) != "[]" {
			t.Fatal(e, string(raw))
		}
	}
	empty := ""
	r.ID = uuid.NewString()
	r.ProposedLeaderTrackID = &empty
	if w = callReview(ws, "POST", "/audit", auditBody(t, m, r)); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestFollowingReviewNewAnalysisKeepsProducerAndAuditVersionsSeparate(t *testing.T) {
	ws, root, _ := setupFollowingReview(t)
	old, r := reviewAuditRequest(t, ws)
	if w := callReview(ws, "POST", "/audit", auditBody(t, old, r)); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	read := func(name string) map[string]any {
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if e = json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	canonical := read("canonical.json")
	canonical["context"].(map[string]any)["analysis_build_git_sha"] = "new-analysis-build"
	cb := reviewJSON(t, filepath.Join(root, "canonical.json"), canonical)
	slice := read("minute.json")
	slice["context"].(map[string]any)["analysis_build_git_sha"] = "new-analysis-build"
	slice["review_slice"].(map[string]any)["canonical_evidence_sha256"] = followingreview.Digest(cb)
	sb := reviewJSON(t, filepath.Join(root, "minute.json"), slice)
	receipt := read("complete.json")
	receipt["context"].(map[string]any)["analysis_build_git_sha"] = "new-analysis-build"
	receipt["slices"].([]any)[0].(map[string]any)["sha256"] = followingreview.Digest(sb)
	reviewJSON(t, filepath.Join(root, "complete.json"), receipt)
	current, _ := reviewAuditRequest(t, ws)
	if current.EvidenceSHA256 == old.EvidenceSHA256 || current.Binding.AnalysisBuildGitSHA != "new-analysis-build" {
		t.Fatal(current)
	}
	w := callReview(ws, "GET", "/audit", nil)
	var audit followingreview.Audit
	if e := json.Unmarshal(w.Body.Bytes(), &audit); e != nil || len(audit.Records) != 0 {
		t.Fatal(e, w.Body.String())
	}
	if w = callReview(ws, "POST", "/audit", auditBody(t, old, r)); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = callReview(ws, "POST", "/audit", auditBody(t, current, r)); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	retained, e := sqlite.NewFollowingAuditStore(ws.db).ListFollowingAudit(old.RunID, old.EvidenceSHA256)
	if e != nil || len(retained) != 1 {
		t.Fatal(e, len(retained))
	}
	// Reanalysis cannot rewrite the registered recording producer identity.
	header := read("vrlog/header.json")
	header["build_git_sha"] = "forged-producer"
	reviewJSON(t, filepath.Join(root, "vrlog/header.json"), header)
	receipt["recording_header"] = header
	reviewJSON(t, filepath.Join(root, "complete.json"), receipt)
	if w = callReview(ws, "GET", "", nil); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}
