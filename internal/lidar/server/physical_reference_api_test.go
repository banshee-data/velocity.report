package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"golang.org/x/sys/unix"
)

func physF(v float64) *float64 { return &v }

// physicalTestPack writes a three-sample pack of one car, 4.5 by 1.8 m,
// centred on (10+i, 0) and driving along +x, under root/<name>/pack, with a
// reviewed membership mask in every sample.
func physicalTestPack(t *testing.T, root, name string) *annotation.Pack {
	t.Helper()
	var samples []annotation.Sample
	var blocks [][]byte
	for i := 0; i < 3; i++ {
		var pts annotation.Points
		for _, x := range []float32{float32(10+i) - 2.25, float32(10+i) + 2.25} {
			for _, y := range []float32{-0.9, 0.9} {
				for _, z := range []float32{0.3, 1.5} {
					pts.X, pts.Y, pts.Z = append(pts.X, x), append(pts.Y, y), append(pts.Z, z)
				}
			}
		}
		block, err := annotation.EncodePoints(pts)
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, annotation.Sample{SourceOrdinal: i, SourceFrameID: uint64(100 + i),
			TimestampNs: 1_000_000_000 + int64(i)*100_000_000, SensorID: "hesai-test", PointCount: len(pts.X)})
		blocks = append(blocks, block)
	}
	m := annotation.Manifest{
		Coverage: annotation.CoverageForegroundOnly,
		Source:   annotation.SourceProvenance{SensorID: "hesai-test", VRLOGHeaderSHA: "sha256:header", VRLOGFramesSHA: "sha256:frames"},
		Coordinate: annotation.CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor",
			Handedness: "right", OriginNote: "sensor origin"},
	}
	dir := filepath.Join(root, name, "pack")
	if err := annotation.WritePack(dir, m, samples, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := annotation.OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := annotation.NewSidecar(p)
	s.Change = annotation.Provenance{Author: "op", Operation: "label"}
	s.Objects = []annotation.Object{{ObjectID: "car-1", Class: "car", Confidence: 1, Status: annotation.StatusReviewed}}
	for i := 0; i < 3; i++ {
		s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: "car-1", SampleID: i, PointIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
			Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed})
	}
	if err := annotation.SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	return p
}

// physicalTestObject is car-1 with an observed full length, a centre
// keyframe at sample 0, and every review claiming more than a save allows.
func physicalTestObject() annotation.PhysicalObject {
	review := annotation.PhysicalReview{
		Status: annotation.StatusReviewed, Origin: annotation.OriginIndependent, Method: "manual_box",
		UncertaintyAssumptions: "read off the outline", Provenance: annotation.Provenance{Author: "op"},
	}
	unknown := annotation.DimensionBound{Status: annotation.EvidenceUnknown}
	return annotation.PhysicalObject{
		ObjectID: "car-1",
		Body: &annotation.BodyGeometry{
			BodyID: "body_a", AxisConvention: annotation.BodyAxisConvention,
			Length: annotation.DimensionBound{Status: annotation.EvidenceObserved, Span: annotation.SpanFull,
				LowerM: physF(4.3), UpperM: physF(4.7), Support: annotation.EvidenceSupport{Frames: []int{0}}},
			Width: unknown, Height: unknown, Review: review,
		},
		Keyframes: []annotation.PhysicalKeyframe{{
			KeyframeID: "kf_a", SampleID: 0, TimestampNs: 1_000_000_000,
			Anchor: annotation.PhysicalAnchor{Kind: annotation.AnchorBodyCentre},
			Position: annotation.PositionBound{Status: annotation.EvidenceObserved, XM: physF(10), YM: physF(0), BoundM: physF(0.2),
				Support: annotation.EvidenceSupport{Frames: []int{0}}},
			Yaw: annotation.YawBound{Status: annotation.EvidenceObserved, Axis: annotation.AxisResolved, YawRad: physF(0), BoundRad: physF(0.05),
				Support: annotation.EvidenceSupport{Frames: []int{0}}},
			Front:  annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
			Rear:   annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
			Review: review,
		}},
	}
}

type physicalClient struct {
	t    *testing.T
	base string
}

func (c physicalClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, err := json.Marshal(body)
			if err != nil {
				c.t.Fatal(err)
			}
			rd = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.t.Fatalf("%s %s: decode response: %v", method, path, err)
	}
	return resp.StatusCode, out
}

func (c physicalClient) load(pack, digest string) map[string]any {
	c.t.Helper()
	status, out := c.do(http.MethodGet, "/api/annotations/physical?pack="+url.QueryEscape(pack)+"&pack_digest="+url.QueryEscape(digest), nil)
	if status != http.StatusOK {
		c.t.Fatalf("load: %d %v", status, out)
	}
	return out
}

func editBody(state map[string]any, objects []annotation.PhysicalObject) map[string]any {
	return map[string]any{
		"pack": state["pack"], "pack_digest": state["pack_digest"],
		"base_revision": state["revision"], "base_digest": state["digest"],
		"membership_digest": state["membership_digest"], "author": "op", "session": "s1", "objects": objects,
	}
}

// The whole authoring round trip over the real HTTP boundary: load an empty
// set, validate, save a proposal, review body and keyframe separately, and
// refuse a stale save; the membership sidecar's bytes never change.
func TestPhysicalReferenceAPIRoundTrip(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "run-a")
	sidecarPath := filepath.Join(p.Dir, "annotations.json")
	sidecarBefore, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}

	state := c.load("run-a/pack", p.Manifest.PackDigest)
	if state["exists"] != false || state["revision"] != float64(1) || state["digest"] != "" || state["membership_digest"] == "" {
		t.Fatalf("empty pack state %v", state)
	}
	if want, _ := filepath.EvalSymlinks(p.Dir); state["pack_dir"] != want {
		t.Fatalf("pack_dir %v, want %s", state["pack_dir"], p.Dir)
	}

	status, out := c.do(http.MethodPost, "/api/annotations/physical/validate", editBody(state, []annotation.PhysicalObject{physicalTestObject()}))
	if status != http.StatusOK || out["valid"] != true {
		t.Fatalf("validate: %d %v", status, out)
	}
	if after := c.load("run-a/pack", p.Manifest.PackDigest); after["exists"] != false {
		t.Fatal("validate wrote the document")
	}

	status, out = c.do(http.MethodPost, "/api/annotations/physical/save", editBody(state, []annotation.PhysicalObject{physicalTestObject()}))
	if status != http.StatusOK {
		t.Fatalf("save: %d %v", status, out)
	}
	if got := out["reset_reviews"].([]any); len(got) != 2 {
		t.Fatalf("a save kept claimed reviews: %v", got)
	}
	saved := out["state"].(map[string]any)
	if saved["exists"] != true || saved["revision"] != float64(1) || saved["digest"] == "" {
		t.Fatalf("saved state %v", saved)
	}

	// A save from the old base is a conflict, not a last-writer-wins retry.
	status, out = c.do(http.MethodPost, "/api/annotations/physical/save", editBody(state, []annotation.PhysicalObject{physicalTestObject()}))
	if status != http.StatusConflict || out["code"] != "conflict" {
		t.Fatalf("stale save: %d %v", status, out)
	}

	review := func(s map[string]any, kind, record string) (int, map[string]any) {
		return c.do(http.MethodPost, "/api/annotations/physical/review", map[string]any{
			"pack": s["pack"], "pack_digest": s["pack_digest"], "base_revision": s["revision"], "base_digest": s["digest"],
			"membership_digest": s["membership_digest"], "kind": kind, "object_id": "car-1", "record_id": record,
			"reviewer": "rev", "session": "s2",
		})
	}
	status, bodyReviewed := review(saved, "body", "body_a")
	if status != http.StatusOK || bodyReviewed["revision"] != float64(2) {
		t.Fatalf("review body: %d %v", status, bodyReviewed)
	}
	doc := bodyReviewed["document"].(map[string]any)
	obj := doc["objects"].([]any)[0].(map[string]any)
	if obj["body"].(map[string]any)["review"].(map[string]any)["status"] != "reviewed" ||
		obj["keyframes"].([]any)[0].(map[string]any)["review"].(map[string]any)["status"] != "proposed" {
		t.Fatalf("reviewing the body reviewed the wrong records: %v", obj)
	}
	if status, out := review(bodyReviewed, "keyframe", "kf_missing"); status != http.StatusNotFound || out["code"] != "not_found" {
		t.Fatalf("review of a missing record: %d %v", status, out)
	}
	if status, out := review(saved, "keyframe", "kf_a"); status != http.StatusConflict {
		t.Fatalf("review from a stale base: %d %v", status, out)
	}
	if status, out := review(bodyReviewed, "keyframe", "kf_a"); status != http.StatusOK || out["revision"] != float64(3) {
		t.Fatalf("review keyframe: %d %v", status, out)
	}

	sidecarAfter, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sidecarBefore, sidecarAfter) {
		t.Fatal("physical edits changed the membership sidecar")
	}
}

// Requests are resolved beneath the packs directory, bound to the pack the
// client opened, decoded strictly, and refused with a code a client can act
// on.
func TestPhysicalReferenceAPIRefusals(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "run-a")
	other := physicalTestPack(t, t.TempDir(), "elsewhere")
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}
	digest := url.QueryEscape(p.Manifest.PackDigest)

	for name, tc := range map[string]struct {
		query      string
		status     int
		code       string
		errorMatch string
	}{
		"parent escape":      {"pack=../x&pack_digest=" + digest, 400, "bad_request", "not a pack directory"},
		"absolute":           {"pack=" + url.QueryEscape(other.Dir) + "&pack_digest=" + digest, 400, "bad_request", "not a pack directory"},
		"too deep":           {"pack=run-a/pack/x&pack_digest=" + digest, 400, "bad_request", "not a pack directory"},
		"not pack subdir":    {"pack=run-a/other&pack_digest=" + digest, 400, "bad_request", "not a pack directory"},
		"unclean":            {"pack=run-a//pack&pack_digest=" + digest, 400, "bad_request", "not a pack directory"},
		"missing digest":     {"pack=run-a/pack", 400, "bad_request", "pack_digest"},
		"missing pack":       {"pack=run-b&pack_digest=" + digest, 404, "pack_not_found", ""},
		"other pack digest":  {"pack=run-a/pack&pack_digest=sha256:other", 409, "pack_mismatch", ""},
		"bad revision":       {"pack=run-a/pack&pack_digest=" + digest + "&revision=0", 400, "bad_request", "revision"},
		"unretained version": {"pack=run-a/pack&pack_digest=" + digest + "&revision=4", 500, "internal", ""},
	} {
		t.Run(name, func(t *testing.T) {
			status, out := c.do(http.MethodGet, "/api/annotations/physical?"+tc.query, nil)
			if status != tc.status || out["code"] != tc.code || !strings.Contains(out["error"].(string), tc.errorMatch) {
				t.Fatalf("%d %v, want %d %s", status, out, tc.status, tc.code)
			}
		})
	}

	state := c.load("run-a/pack", p.Manifest.PackDigest)
	good := editBody(state, []annotation.PhysicalObject{physicalTestObject()})
	withExtra := editBody(state, []annotation.PhysicalObject{physicalTestObject()})
	withExtra["surprise"] = true
	membership := editBody(state, []annotation.PhysicalObject{physicalTestObject()})
	membership["membership_digest"] = "sha256:stale"
	anonymous := editBody(state, []annotation.PhysicalObject{physicalTestObject()})
	anonymous["author"] = " "
	noObjects := editBody(state, nil)
	delete(noObjects, "objects")
	uncited := physicalTestObject()
	uncited.Keyframes[0].Position.Support.Frames = []int{1}
	goodJSON, _ := json.Marshal(good)

	for name, tc := range map[string]struct {
		body   any
		status int
		code   string
	}{
		"unknown field":    {withExtra, 400, "bad_request"},
		"trailing data":    {string(goodJSON) + "}", 400, "bad_request"},
		"not json":         {"{", 400, "bad_request"},
		"no author":        {anonymous, 400, "bad_request"},
		"no objects":       {noObjects, 400, "bad_request"},
		"stale membership": {membership, 409, "membership_changed"},
		"oversized":        {`{"pack":"` + strings.Repeat("x", maxPhysicalRequestBytes) + `"}`, 413, "bad_request"},
	} {
		t.Run(name, func(t *testing.T) {
			status, out := c.do(http.MethodPost, "/api/annotations/physical/save", tc.body)
			if status != tc.status || out["code"] != tc.code {
				t.Fatalf("%d %v, want %d %s", status, out, tc.status, tc.code)
			}
		})
	}

	// An invalid edit is refused with the same diagnostics a validation gives.
	status, out := c.do(http.MethodPost, "/api/annotations/physical/save", editBody(state, []annotation.PhysicalObject{uncited}))
	if status != http.StatusUnprocessableEntity || out["valid"] != false || !strings.Contains(out["invalid"].(string), "own sample") {
		t.Fatalf("invalid save: %d %v", status, out)
	}
	ghost := physicalTestObject()
	ghost.ObjectID = "ghost"
	status, out = c.do(http.MethodPost, "/api/annotations/physical/validate", editBody(state, []annotation.PhysicalObject{ghost}))
	if status != http.StatusOK || out["valid"] != false || len(out["link_problems"].([]any)) == 0 {
		t.Fatalf("validate unlinked: %d %v", status, out)
	}
	if after := c.load("run-a/pack", p.Manifest.PackDigest); after["exists"] != false {
		t.Fatal("a refused request wrote the document")
	}
	if status, out := c.do(http.MethodGet, "/api/annotations/physical/save", nil); status != http.StatusMethodNotAllowed {
		t.Fatalf("GET save: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/physical/review", map[string]any{
		"pack": "run-a/pack", "pack_digest": p.Manifest.PackDigest, "kind": "body", "object_id": "car-1", "record_id": "b"}); status != http.StatusBadRequest {
		t.Fatalf("anonymous review: %d %v", status, out)
	}

	unconfigured := httptest.NewServer(func() *http.ServeMux { m := http.NewServeMux(); (&Server{}).RegisterRoutes(m); return m }())
	defer unconfigured.Close()
	if status, out := (physicalClient{t: t, base: unconfigured.URL}).do(http.MethodGet, "/api/annotations/physical?pack=a&pack_digest=b", nil); status != http.StatusNotImplemented || out["code"] != "not_configured" {
		t.Fatalf("unconfigured: %d %v", status, out)
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// Failures after a request is accepted: a symlink out of the packs
// directory, a writer holding the lock, a record the membership no longer
// supports, damaged storage, and a body that fails mid-read.
func TestPhysicalReferenceAPIStorageFailures(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "run-a")
	outside := physicalTestPack(t, t.TempDir(), "outside")
	if err := os.Symlink(filepath.Dir(outside.Dir), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}

	if status, out := c.do(http.MethodGet, "/api/annotations/physical?pack=escape/pack&pack_digest="+url.QueryEscape(outside.Manifest.PackDigest), nil); status != http.StatusBadRequest {
		t.Fatalf("symlink escape: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/physical", map[string]any{}); status != http.StatusMethodNotAllowed {
		t.Fatalf("POST load: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/physical/validate", map[string]any{"pack": "nope", "pack_digest": "d", "author": "op", "objects": []any{}}); status != http.StatusNotFound {
		t.Fatalf("validate against a missing pack: %d %v", status, out)
	}
	if status, out := c.do(http.MethodGet, "/api/annotations/physical/review", nil); status != http.StatusMethodNotAllowed {
		t.Fatalf("GET review: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/physical/review", "{"); status != http.StatusBadRequest {
		t.Fatalf("malformed review: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/physical/review", map[string]any{"pack": "nope", "pack_digest": "d", "reviewer": "r"}); status != http.StatusNotFound {
		t.Fatalf("review against a missing pack: %d %v", status, out)
	}

	// Another writer holds the annotation lock: validation still answers,
	// the save is refused as busy.
	state := c.load("run-a/pack", p.Manifest.PackDigest)
	lock, err := os.OpenFile(filepath.Join(p.Dir, ".annotations.lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	status, out := c.do(http.MethodPost, "/api/annotations/physical/save", editBody(state, []annotation.PhysicalObject{physicalTestObject()}))
	lock.Close()
	if status != http.StatusLocked || out["code"] != "busy" {
		t.Fatalf("save while locked: %d %v", status, out)
	}
	status, out = c.do(http.MethodPost, "/api/annotations/physical/save", editBody(state, []annotation.PhysicalObject{physicalTestObject()}))
	if status != http.StatusOK {
		t.Fatalf("save after the lock is released: %d %v", status, out)
	}
	saved := out["state"].(map[string]any)

	// Membership rejects the object: the load lists the record as stale and
	// its review is refused as invalid.
	s, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Objects[0].Status = annotation.StatusRejected
	s.Change = annotation.Provenance{Author: "op", Operation: "reject"}
	if err := annotation.SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	stale := c.load("run-a/pack", p.Manifest.PackDigest)
	if len(stale["stale"].([]any)) == 0 {
		t.Fatalf("the load did not report stale records: %v", stale)
	}
	status, out = c.do(http.MethodPost, "/api/annotations/physical/review", map[string]any{
		"pack": saved["pack"], "pack_digest": saved["pack_digest"], "base_revision": stale["revision"], "base_digest": stale["digest"],
		"membership_digest": stale["membership_digest"], "kind": "body", "object_id": "car-1", "record_id": "body_a", "reviewer": "rev"})
	if status != http.StatusUnprocessableEntity || out["code"] != "invalid" {
		t.Fatalf("review of a stale record: %d %v", status, out)
	}

	// A second revision, then a damaged head: retained history still reads,
	// and is not presented as the head; the head itself refuses.
	s.Objects[0].Status = annotation.StatusReviewed
	if err := annotation.SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	cur := c.load("run-a/pack", p.Manifest.PackDigest)
	if status, out := c.do(http.MethodPost, "/api/annotations/physical/save", editBody(cur, []annotation.PhysicalObject{})); status != http.StatusOK {
		t.Fatalf("second save: %d %v", status, out)
	}
	headPath := filepath.Join(p.Dir, "physical-references.json")
	if err := os.WriteFile(headPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, out = c.do(http.MethodGet, "/api/annotations/physical?pack=run-a/pack&revision=1&pack_digest="+url.QueryEscape(p.Manifest.PackDigest), nil)
	if status != http.StatusOK || out["head"] != false || out["revision"] != float64(1) {
		t.Fatalf("retained revision behind a damaged head: %d %v", status, out)
	}
	if status, out := c.do(http.MethodGet, "/api/annotations/physical?pack=run-a/pack&pack_digest="+url.QueryEscape(p.Manifest.PackDigest), nil); status != http.StatusInternalServerError {
		t.Fatalf("damaged head: %d %v", status, out)
	}
	if err := os.Remove(headPath); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(p.Dir, "physical-reference-revisions")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Dir, "annotations.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := c.do(http.MethodGet, "/api/annotations/physical?pack=run-a/pack&pack_digest="+url.QueryEscape(p.Manifest.PackDigest), nil); status != http.StatusInternalServerError {
		t.Fatalf("damaged sidecar: %d %v", status, out)
	}

	rec := httptest.NewRecorder()
	ws.handlePhysicalEdit(false)(rec, httptest.NewRequest(http.MethodPost, "/api/annotations/physical/validate", failingBody{}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "connection reset") {
		t.Fatalf("failed body read: %d %s", rec.Code, rec.Body.String())
	}
}

// A document whose content cannot be digested is an error, not a state with
// an empty digest a client could pin.
func TestPhysicalStateRefusesAnUndigestableDocument(t *testing.T) {
	p := physicalTestPack(t, t.TempDir(), "run-a")
	doc := annotation.NewPhysicalReferenceSet(p)
	obj := physicalTestObject()
	obj.Keyframes[0].Position.XM = physF(math.NaN())
	doc.Objects = []annotation.PhysicalObject{obj}
	if _, err := physicalStateOf(p, "run-a/pack", doc, doc, annotation.NewSidecar(p)); err == nil {
		t.Fatal("a non-finite document produced a state")
	}
	rec := httptest.NewRecorder()
	(&Server{}).writeSavedState(rec, p, "run-a/pack", &annotation.PhysicalEditOutcome{Document: doc, Membership: annotation.NewSidecar(p)},
		func(s *physicalPackState) any { return s })
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("an undigestable saved document answered %d", rec.Code)
	}
}
