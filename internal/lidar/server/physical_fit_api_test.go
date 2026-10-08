package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// fitTestPack writes ten samples of a stationary 4.5 by 1.8 m box side-on to
// the sensor, 8 m away below its height, whose near side and roof return,
// under root/<name>/pack with every mask reviewed.
func fitTestPack(t *testing.T, root, name string) *annotation.Pack {
	t.Helper()
	var pts annotation.Points
	add := func(x, y, z float32) {
		pts.X, pts.Y, pts.Z = append(pts.X, x), append(pts.Y, y), append(pts.Z, z)
	}
	for x := float32(-2.25); x <= 2.2501; x += 0.05 {
		for _, z := range []float32{-2.0, -1.5, -1.0} {
			add(x, 7.1, z)
		}
		for y := float32(7.1); y <= 8.9001; y += 0.2 {
			add(x, y, -0.8)
		}
	}
	block, err := annotation.EncodePoints(pts)
	if err != nil {
		t.Fatal(err)
	}
	var samples []annotation.Sample
	var blocks [][]byte
	for i := 0; i < 10; i++ {
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
	all := make([]int, len(pts.X))
	for i := range all {
		all[i] = i
	}
	s := annotation.NewSidecar(p)
	s.Change = annotation.Provenance{Author: "op", Operation: "label"}
	s.Objects = []annotation.Object{{ObjectID: "car-1", Class: "car", Confidence: 1, Status: annotation.StatusReviewed}}
	for i := 0; i < 10; i++ {
		s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: "car-1", SampleID: i, PointIndices: all,
			Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed})
	}
	if err := annotation.SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	return p
}

func fitBody(state map[string]any, object string, samples ...int) map[string]any {
	return map[string]any{
		"pack": state["pack"], "pack_digest": state["pack_digest"],
		"membership_revision": state["membership_revision"], "membership_digest": state["membership_digest"],
		"object_id": object, "samples": samples, "author": "op", "session": "s1",
	}
}

// The window's flow over the real HTTP boundary: fit an object, place the
// proposals in a draft, and save it through the ordinary edit path. The fit
// itself stores nothing.
func TestPhysicalFitAPIFitsAndTheDraftSaves(t *testing.T) {
	root := t.TempDir()
	p := fitTestPack(t, root, "run-a")
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}

	state := c.load("run-a/pack", p.Manifest.PackDigest)
	status, out := c.do(http.MethodPost, "/api/annotations/physical/fit", fitBody(state, "car-1", 4))
	if status != http.StatusOK {
		t.Fatalf("fit: %d %v", status, out)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var res annotation.FitResult
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) == 0 || res.Import == nil || len(res.Import.Objects) != 1 || res.Import.Objects[0].Body == nil {
		t.Fatalf("fit result %+v", res)
	}
	if after := c.load("run-a/pack", p.Manifest.PackDigest); after["exists"] != false {
		t.Fatal("the fit stored a document")
	}
	status, saved := c.do(http.MethodPost, "/api/annotations/physical/save", editBody(state, res.Import.Objects))
	if status != http.StatusOK {
		t.Fatalf("saving the fitted draft: %d %v", status, saved)
	}
}

func TestPhysicalFitAPIRefusals(t *testing.T) {
	root := t.TempDir()
	p := fitTestPack(t, root, "run-a")
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}
	state := c.load("run-a/pack", p.Manifest.PackDigest)

	stale := fitBody(state, "car-1")
	stale["membership_revision"] = 99
	missingAuthor := fitBody(state, "car-1")
	missingAuthor["author"] = " "
	noDigest := fitBody(state, "car-1")
	noDigest["membership_digest"] = ""
	for _, tc := range []struct {
		name   string
		method string
		body   any
		status int
		code   string
	}{
		{"GET", http.MethodGet, nil, http.StatusMethodNotAllowed, physicalCodeBadRequest},
		{"stale membership", http.MethodPost, stale, http.StatusConflict, physicalCodeMembershipChanged},
		{"no author", http.MethodPost, missingAuthor, http.StatusBadRequest, physicalCodeBadRequest},
		{"no object", http.MethodPost, fitBody(state, ""), http.StatusBadRequest, physicalCodeBadRequest},
		{"no membership digest", http.MethodPost, noDigest, http.StatusBadRequest, physicalCodeBadRequest},
		{"unknown object", http.MethodPost, fitBody(state, "ghost"), http.StatusUnprocessableEntity, physicalCodeInvalid},
		{"unknown field", http.MethodPost, `{"pack":"run-a/pack","nope":1}`, http.StatusBadRequest, physicalCodeBadRequest},
	} {
		status, out := c.do(tc.method, "/api/annotations/physical/fit", tc.body)
		if status != tc.status || out["code"] != tc.code {
			t.Errorf("%s: %d %v, want %d %s", tc.name, status, out, tc.status, tc.code)
		}
	}
	unconfigured := &Server{}
	umux := http.NewServeMux()
	unconfigured.RegisterRoutes(umux)
	usrv := httptest.NewServer(umux)
	defer usrv.Close()
	if status, _ := (physicalClient{t: t, base: usrv.URL}).do(http.MethodPost, "/api/annotations/physical/fit", fitBody(state, "car-1")); status != http.StatusNotImplemented {
		t.Errorf("no annotation directory: %d, want 501", status)
	}
}
