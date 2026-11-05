package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
)

type workspaceEnv struct {
	t      *testing.T
	root   string
	pcaps  string
	dbPath string
	db     *db.DB
}

func newWorkspaceEnv(t *testing.T) *workspaceEnv {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &workspaceEnv{t: t, root: filepath.Join(dir, "annotation"), pcaps: filepath.Join(dir, "pcaps"), dbPath: filepath.Join(dir, "w.db")}
	for _, p := range []string{e.root, e.pcaps} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.db, err = db.NewDB(e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.db.Close() })
	return e
}

// server is a fresh server over the same directories and database, as a
// restart would be: nothing it reports comes from a previous one's memory.
func (e *workspaceEnv) server(manifest string, configure ...func(*Server)) (*httptest.Server, *Server) {
	ws := &Server{annotationPacksDir: e.root, db: e.db, pcapSafeDir: e.pcaps, cohortManifestPath: manifest, sensorID: "hesai-test"}
	for _, c := range configure {
		c(ws)
	}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	e.t.Cleanup(srv.Close)
	return srv, ws
}

func call(t *testing.T, method, url string, body any) (int, map[string]any, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(actorHeader, "tester")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, string(raw)
}

func rowsOf(t *testing.T, resp map[string]any) []map[string]any {
	t.Helper()
	items, _ := resp["items"].([]any)
	var out []map[string]any
	for _, it := range items {
		out = append(out, it.(map[string]any))
	}
	return out
}

func layerCounts(t *testing.T, layers any, name string) []any {
	t.Helper()
	for _, l := range layers.([]any) {
		lm := l.(map[string]any)
		if lm["layer"] == name {
			return lm["counts"].([]any)
		}
	}
	t.Fatalf("no layer %s in %v", name, layers)
	return nil
}

// Two runs' packs of one capture share one label set: one row, its review
// work counted once, and the same after a restart.
func TestWorkspaceCountsASharedLabelSetOnce(t *testing.T) {
	e := newWorkspaceEnv(t)
	a := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk0", Run: "run-a", PCAPBasename: "kirk0.pcapng", Objects: []string{"car-1", "car-2"},
		Status: annotation.StatusProposed})
	b := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk0", Run: "run-b", PCAPBasename: "kirk0.pcapng", NoLabels: true})
	fx.Attach(t, b, fx.LabelSetDir(t, a))
	for restart := 0; restart < 2; restart++ {
		srv, _ := e.server("")
		status, resp, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
		if status != 200 {
			t.Fatal(status, resp)
		}
		rows := rowsOf(t, resp)
		if len(rows) != 1 || rows[0]["kind"] != "label_set" || len(rows[0]["packs"].([]any)) != 2 {
			t.Fatalf("one label set, two views: %+v", rows)
		}
		totals := resp["totals"].(map[string]any)
		if totals["label_sets"] != float64(1) || totals["packs"] != float64(2) {
			t.Fatalf("%+v", totals)
		}
		review := layerCounts(t, totals["layers"], "object_review")[0].(map[string]any)
		if review["done"] != float64(0) || review["total"] != float64(2) {
			t.Fatalf("two objects, not four: %+v", review)
		}
		spans := layerCounts(t, totals["layers"], "spans")[0].(map[string]any)
		if spans["total"] != nil {
			t.Fatalf("the window's road users are unknown: %+v", spans)
		}
		if rows[0]["stage"] != "in_review" || rows[0]["cohort"] != "unassigned" {
			t.Fatalf("%v %v", rows[0]["stage"], rows[0]["cohort"])
		}
	}
}

// A source that cannot be read is unavailable, not empty, and a label set
// that cannot be read leaves every total unknown rather than smaller.
func TestWorkspaceUnreadableSourcesAreUnknown(t *testing.T) {
	e := newWorkspaceEnv(t)
	ws := &Server{}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	bare := httptest.NewServer(mux)
	defer bare.Close()
	_, resp, _ := call(t, http.MethodGet, bare.URL+"/api/lidar/workspace", nil)
	sources := resp["sources"].(map[string]any)
	for _, s := range []string{"annotation_root", "database", "workflow"} {
		if sources[s].(map[string]any)["available"] != false {
			t.Fatalf("%s: %+v", s, sources[s])
		}
	}
	good := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk0", Run: "run-a", PCAPBasename: "kirk0.pcapng"})
	bad := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk1", Run: "run-a", PCAPBasename: "kirk1.pcapng"})
	_ = good
	if err := os.WriteFile(filepath.Join(fx.LabelSetDir(t, bad), labelset.ManifestFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, _ := e.server("")
	_, resp, _ = call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
	totals := resp["totals"].(map[string]any)
	review := layerCounts(t, totals["layers"], "object_review")[0].(map[string]any)
	if review["total"] != nil {
		t.Fatalf("a total that leaves a set out is not a total: %+v", review)
	}
	found := false
	for _, r := range rowsOf(t, resp) {
		if contains(asStrings(r["flags"]), "unavailable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the unreadable pack is flagged: %+v", rowsOf(t, resp))
	}
}

func asStrings(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}

// Reviewing an object moves object review only; a review made against an
// older revision is refused; workflow notes persist across a restart and a
// stale workflow write is refused.
func TestWorkspaceReviewAndWorkflowWrites(t *testing.T) {
	e := newWorkspaceEnv(t)
	p := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk0", Run: "run-a", PCAPBasename: "kirk0.pcapng", Objects: []string{"car-1", "car-2"},
		Status: annotation.StatusProposed, StartNs: 1_762_000_000_123_456_789})
	srv, _ := e.server("")
	_, resp, raw := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
	row := rowsOf(t, resp)[0]
	key := row["key"].(string)
	ls := row["label_set"].(map[string]any)
	if !strings.Contains(raw, `"generated_at_ns":"`) {
		t.Fatal("nanosecond times are strings")
	}
	_, detail, rawDetail := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace/item?key="+key, nil)
	if !strings.Contains(rawDetail, `"first_ns":"1762000000123456789"`) {
		t.Fatalf("a span's capture time is served exactly: %s", rawDetail[:min(len(rawDetail), 400)])
	}
	if len(detail["objects"].([]any)) != 2 {
		t.Fatal(detail["objects"])
	}
	review := map[string]any{"label_set_id": ls["id"], "base_revision": ls["revision"], "base_state": ls["state_digest"],
		"object_id": "car-1", "status": "reviewed", "reviewer": "ana"}
	status, out, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/annotations/objects/review", review)
	if status != 200 {
		t.Fatal(status, out)
	}
	progress := out["progress"].(map[string]any)
	if progress["reviewed_objects"] != float64(1) || progress["reviewed_masks"] != float64(0) {
		t.Fatalf("an object review reviews no mask: %+v", progress)
	}
	if status, out, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/annotations/objects/review", review); status != http.StatusConflict {
		t.Fatalf("a review against the old revision: %d %+v", status, out)
	}
	sidecar, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range sidecar.Masks {
		if m.Status != annotation.StatusProposed {
			t.Fatalf("masks keep their own review: %+v", m)
		}
	}

	item := map[string]any{"item_key": key, "layer": "spans", "state": "tbd", "tbd_reason": "undecided_criterion",
		"note": "is a parked car a road user?", "assignee": "ana", "updated_by": "ana", "base_revision": 0}
	if status, out, _ := call(t, http.MethodPut, srv.URL+"/api/lidar/workspace/workflow", item); status != 200 || out["revision"] != float64(1) {
		t.Fatal(status, out)
	}
	if status, _, _ := call(t, http.MethodPut, srv.URL+"/api/lidar/workspace/workflow", item); status != http.StatusConflict {
		t.Fatalf("a second write from revision 0: %d", status)
	}
	item["tbd_reason"] = ""
	item["base_revision"] = 1
	if status, _, _ := call(t, http.MethodPut, srv.URL+"/api/lidar/workspace/workflow", item); status != http.StatusBadRequest {
		t.Fatalf("TBD without a reason: %d", status)
	}
	restarted, _ := e.server("")
	_, resp, _ = call(t, http.MethodGet, restarted.URL+"/api/lidar/workspace?assignee=ana&flag=tbd", nil)
	rows := rowsOf(t, resp)
	if len(rows) != 1 || len(rows[0]["workflow"].([]any)) != 1 {
		t.Fatalf("the note survives a restart and filters: %+v", rows)
	}
	review2 := layerCounts(t, rows[0]["progress"].(map[string]any)["layers"], "object_review")[0].(map[string]any)
	if review2["done"] != float64(1) {
		t.Fatalf("%+v", review2)
	}
	if _, resp, _ = call(t, http.MethodGet, restarted.URL+"/api/lidar/workspace?assignee=nobody", nil); resp["total"] != float64(0) {
		t.Fatal(resp["total"])
	}
}

// A freeze pins the revision; a later review moves the head, and the row
// says the split's revision is historical while the split file is unchanged.
func TestWorkspaceFreezePinsSurviveLaterReview(t *testing.T) {
	e := newWorkspaceEnv(t)
	p := fx.Pack(t, e.root, fx.PackOptions{Capture: "kirk0", Run: "run-a", PCAPBasename: "kirk0.pcapng", Objects: []string{"car-1", "car-2"}})
	srv, _ := e.server("")
	handle := fx.Handle(t, e.root, p)
	draft := splitDraftFor(handle)
	draft["packs"].([]any)[0].(map[string]any)["splits"] = []any{map[string]any{"name": "tune", "role": "tuning", "object_ids": []string{"car-1", "car-2"}}}
	draft["packs"].([]any)[0].(map[string]any)["episodes"] = []any{map[string]any{"episode_id": "ep-1", "split": "tune",
		"object_ids": []string{"car-1", "car-2"}, "frame_intervals": []any{map[string]any{"first_sample": 0, "last_sample": 2}}}}
	status, preview, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/annotations/split/preview", map[string]any{"draft": draft})
	if status != 200 || preview["would_freeze"] != true {
		t.Fatal(status, preview)
	}
	if status, out, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/annotations/split/freeze",
		map[string]any{"draft": draft, "author": "ana", "output": "kirk0-r1.json"}); status != 200 {
		t.Fatal(status, out)
	}
	before, _ := os.ReadFile(filepath.Join(e.root, splitsDirName, "kirk0-r1.json"))
	_, resp, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
	row := rowsOf(t, resp)[0]
	if row["stage"] != "frozen" {
		t.Fatalf("%v", row["stage"])
	}
	ls := row["label_set"].(map[string]any)
	review := map[string]any{"label_set_id": ls["id"], "base_revision": ls["revision"], "base_state": ls["state_digest"],
		"object_id": "car-2", "status": "rejected", "reviewer": "bo"}
	if status, out, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/annotations/objects/review", review); status != 200 {
		t.Fatal(status, out)
	}
	_, resp, _ = call(t, http.MethodGet, srv.URL+"/api/lidar/workspace?flag=stale_revision", nil)
	rows := rowsOf(t, resp)
	if len(rows) != 1 {
		t.Fatalf("the head moved past the frozen revision: %+v", resp)
	}
	split := rows[0]["splits"].([]any)[0].(map[string]any)
	if split["head_moved"] != true || split["pinned_revision"].(float64) >= split["head_revision"].(float64) {
		t.Fatalf("%+v", split)
	}
	after, _ := os.ReadFile(filepath.Join(e.root, splitsDirName, "kirk0-r1.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("a frozen split is never rewritten")
	}
	f, err := annotation.LoadFrozenSplit(filepath.Join(e.root, splitsDirName, "kirk0-r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.Bind(p); err != nil {
		t.Fatalf("the split still binds its pinned revision: %v", err)
	}
}

type countingSweep struct{ starts atomic.Int32 }

func (c *countingSweep) Start(context.Context, interface{}) error { c.starts.Add(1); return nil }
func (c *countingSweep) GetState() interface{}                    { return map[string]any{} }
func (c *countingSweep) Stop()                                    {}

// Protected input is refused before a runner is started or a parameter set
// applied, whatever the request body claims about its purpose or role, and
// whether or not a split is named. With the capture assigned to development
// and nothing holding it out, the same sweep starts and is recorded.
func TestEntryPointsRefuseProtectedInputBeforeLoading(t *testing.T) {
	e := newWorkspaceEnv(t)
	capturePath, sha := fx.Capture(t, e.pcaps, "efolsom.pcapng", "synthetic held-out capture")
	_, devSHA := fx.Capture(t, e.pcaps, "kirk0.pcapng", "synthetic development capture")
	sweep := &countingSweep{}
	body := map[string]any{"data_source": "pcap", "pcap_file": "efolsom.pcapng", "purpose": "production", "role": "tuning", "split": ""}

	srv, _ := e.server("", func(ws *Server) { ws.sweepRunner = sweep })
	status, out, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/sweep/start", body)
	if status != http.StatusForbidden || !strings.Contains(out["error"].(string), "no cohort manifest") {
		t.Fatalf("no manifest: %d %+v", status, out)
	}

	manifest := fx.Manifest(t, e.root, map[string]any{
		"sites":    []any{fx.Site("efolsom", "development"), fx.Site("kirk", "development")},
		"captures": []any{fx.CaptureEntry("efolsom-1", "efolsom", sha, "efolsom.pcapng"), fx.CaptureEntry("kirk0", "kirk", devSHA, "kirk0.pcapng")},
	})
	held := fx.Pack(t, e.root, fx.PackOptions{Capture: "efolsom", Run: "run-a", PCAPBasename: "efolsom.pcapng"})
	fx.Freeze(t, e.root, "label0001-r1.json", held, annotation.SplitRoleHeldOut, nil)
	srv, _ = e.server(manifest, func(ws *Server) { ws.sweepRunner = sweep })
	status, out, _ = call(t, http.MethodPost, srv.URL+"/api/lidar/sweep/start", body)
	if status != http.StatusForbidden || !strings.Contains(out["error"].(string), "held out by frozen split label0001-r1.json") {
		t.Fatalf("held out without naming the split: %d %+v", status, out)
	}
	// A renamed copy is the same bytes.
	copyPath := filepath.Join(e.pcaps, "fresh.pcapng")
	b, _ := os.ReadFile(capturePath)
	if err := os.WriteFile(copyPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	fx.Bind(t, fx.LabelSetDir(t, held), "efolsom.pcapng", sha)
	body["pcap_file"] = "fresh.pcapng"
	if status, out, _ = call(t, http.MethodPost, srv.URL+"/api/lidar/sweep/start", body); status != http.StatusForbidden {
		t.Fatalf("renamed copy: %d %+v", status, out)
	}
	replay := map[string]any{"pcap_file": "fresh.pcapng", "tuning_config": "experiments/l5id0014-a1.json"}
	if status, out, _ = call(t, http.MethodPost, srv.URL+"/api/lidar/pcap/start?sensor_id=hesai-test", replay); status != http.StatusForbidden {
		t.Fatalf("experiment arm replay: %d %+v", status, out)
	}
	if sweep.starts.Load() != 0 {
		t.Fatal("a refused sweep must not start")
	}

	body["pcap_file"] = "kirk0.pcapng"
	if status, out, _ = call(t, http.MethodPost, srv.URL+"/api/lidar/sweep/start", body); status != 200 || sweep.starts.Load() != 1 {
		t.Fatalf("development capture: %d %+v", status, out)
	}
	_, uses, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/eligibility/uses?decision=allowed", nil)
	list := uses["uses"].([]any)
	if len(list) != 4 {
		t.Fatalf("one allowed use per measure family: %+v", uses)
	}
	u := list[0].(map[string]any)
	if u["capture_sha256"] != devSHA || u["entry_point"] != "POST /api/lidar/sweep/start" || !strings.Contains(u["actor"].(string), "tester") {
		t.Fatalf("%+v", u)
	}
	_, status2, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/eligibility", nil)
	if status2["protection_complete"] != false || len(status2["coverage"].([]any)) == 0 {
		t.Fatalf("%+v", status2)
	}
}

// Blind qualification data is listed as counts only: no pack handles, object
// lists or label-set paths, and review is refused while no access control
// protects it.
func TestWorkspaceRedactsQualificationItems(t *testing.T) {
	e := newWorkspaceEnv(t)
	p := fx.Pack(t, e.root, fx.PackOptions{Capture: "qsite", Run: "run-a", PCAPBasename: "q.pcapng", Status: annotation.StatusProposed})
	sha := "sha256:" + strings.Repeat("ab", 32)
	fx.Bind(t, fx.LabelSetDir(t, p), "q.pcapng", sha)
	manifest := fx.Manifest(t, e.root, map[string]any{
		"sites": []any{fx.Site("q", "qualification")}, "captures": []any{fx.CaptureEntry("q1", "q", sha, "q.pcapng")},
	})
	srv, _ := e.server(manifest)
	_, resp, raw := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
	row := rowsOf(t, resp)[0]
	if row["protected"] != true || row["stage"] != "protected" || strings.Contains(raw, "qsite/packs/run-a") || strings.Contains(raw, "car-1") {
		t.Fatalf("%s", raw)
	}
	_, detail, rawDetail := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace/item?key="+row["key"].(string), nil)
	if detail["restricted"] != true || len(detail["objects"].([]any)) != 0 || strings.Contains(rawDetail, "car-1") {
		t.Fatalf("%s", rawDetail)
	}
	ls := row["label_set"].(map[string]any)
	review := map[string]any{"label_set_id": ls["id"], "base_revision": ls["revision"], "base_state": ls["state_digest"],
		"object_id": "car-1", "status": "reviewed", "reviewer": "ana"}
	if status, out, _ := call(t, http.MethodPost, srv.URL+"/api/lidar/annotations/objects/review", review); status != http.StatusForbidden {
		t.Fatalf("%d %+v", status, out)
	}
}

// The inventory is paged by the server: a page holds what it asks for and
// says how many match.
func TestWorkspacePagesServerSide(t *testing.T) {
	e := newWorkspaceEnv(t)
	for _, c := range []string{"a", "b", "c"} {
		fx.Pack(t, e.root, fx.PackOptions{Capture: c, Run: "run-a", PCAPBasename: c + ".pcapng"})
	}
	srv, _ := e.server("")
	_, resp, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace?limit=1&offset=1", nil)
	if resp["total"] != float64(3) || len(rowsOf(t, resp)) != 1 || resp["offset"] != float64(1) {
		t.Fatalf("%+v", resp)
	}
	_, resp, _ = call(t, http.MethodGet, srv.URL+"/api/lidar/workspace?limit=1000", nil)
	if resp["limit"] != float64(maxWorkspacePage) {
		t.Fatalf("page size is capped: %v", resp["limit"])
	}
}

// A plan is bound to a window, its steps are read from that window's stores,
// and the pack the visualiser opens is named by its path; a stale binding is
// refused.
func TestWorkspacePlansFollowTheirBoundWindow(t *testing.T) {
	e := newWorkspaceEnv(t)
	p := fx.Pack(t, e.root, fx.PackOptions{Capture: "efolsom", Run: "run-a", PCAPBasename: "efolsom.pcapng", Status: annotation.StatusProposed})
	srv, _ := e.server("", func(ws *Server) { ws.plansRoot = filepath.Join("..", "..", "..") })
	_, resp, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
	row := rowsOf(t, resp)[0]
	if row["packs"].([]any)[0].(map[string]any)["abs_dir"] != p.Dir {
		t.Fatalf("the pack's path for VelocityVisualiser: %+v", row["packs"])
	}
	plans := resp["plans"].(map[string]any)
	if plans["available"] != true || len(resp["candidates"].([]any)) != 1 {
		t.Fatalf("%+v %+v", plans, resp["candidates"])
	}
	bind := map[string]any{"plan_id": "label0003", "item_key": row["key"], "bound_by": "ana", "base_revision": 0}
	if status, out, _ := call(t, http.MethodPut, srv.URL+"/api/lidar/workspace/plan-binding", bind); status != 200 {
		t.Fatal(status, out)
	}
	if status, _, _ := call(t, http.MethodPut, srv.URL+"/api/lidar/workspace/plan-binding", bind); status != http.StatusConflict {
		t.Fatalf("stale binding: %d", status)
	}
	_, resp, _ = call(t, http.MethodGet, srv.URL+"/api/lidar/workspace", nil)
	var l3 map[string]any
	for _, pl := range resp["plans"].(map[string]any)["plans"].([]any) {
		if pl.(map[string]any)["id"] == "label0003" {
			l3 = pl.(map[string]any)
		}
	}
	steps := l3["progress"].([]any)
	sel, spans := steps[0].(map[string]any), steps[1].(map[string]any)
	if sel["status"] != "blocked" || !strings.Contains(sel["detail"].(string), "no recorded selection") {
		t.Fatalf("a pack without a recorded draw: %+v", sel)
	}
	if spans["status"] != "in_progress" || l3["binding"].(map[string]any)["item_key"] != row["key"] {
		t.Fatalf("%+v", l3)
	}
	_, detail, _ := call(t, http.MethodGet, srv.URL+"/api/lidar/workspace/item?key="+row["key"].(string), nil)
	if len(detail["plans"].([]any)) != 1 {
		t.Fatalf("the item shows the plan run on it: %+v", detail["plans"])
	}
}
