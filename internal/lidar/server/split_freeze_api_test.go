package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// splitDraftFor is a one-pack draft over the reviewed test pack, with car-1
// in a tuning partition over every sample.
func splitDraftFor(handle string) map[string]any {
	return map[string]any{
		"schema": annotation.SplitDraftSchema, "schema_version": annotation.SplitDraftSchemaVersion,
		"packs": []any{map[string]any{
			"dir":    handle,
			"splits": []any{map[string]any{"name": "tune", "role": "tuning", "object_ids": []string{"car-1"}}},
			"episodes": []any{map[string]any{"episode_id": "ep-1", "split": "tune", "object_ids": []string{"car-1"},
				"frame_intervals": []any{map[string]any{"first_sample": 0, "last_sample": 2}}}},
		}},
	}
}

// The window previews, freezes once beneath the packs directory, lists what
// it froze, and is refused a second write under the same name.
func TestSplitFreezeAPIPreviewsFreezesAndLists(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "run-a")
	// A physical reference to pin, saved as a proposal and reviewed.
	state := func(c physicalClient) map[string]any { return c.load("run-a/pack", p.Manifest.PackDigest) }
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}
	s := state(c)
	if status, out := c.do(http.MethodPost, "/api/annotations/physical/save", editBody(s, []annotation.PhysicalObject{physicalTestObject()})); status != 200 {
		t.Fatalf("save: %d %v", status, out)
	}
	s = state(c)
	for _, r := range []struct{ kind, id string }{{"body", "body_a"}, {"keyframe", "kf_a"}} {
		status, out := c.do(http.MethodPost, "/api/annotations/physical/review", map[string]any{
			"pack": "run-a/pack", "pack_digest": p.Manifest.PackDigest, "base_revision": s["revision"], "base_digest": s["digest"],
			"membership_digest": s["membership_digest"], "kind": r.kind, "object_id": "car-1", "record_id": r.id, "reviewer": "rev"})
		if status != 200 {
			t.Fatalf("review %s: %d %v", r.kind, status, out)
		}
		s = out
	}

	status, preview := c.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("run-a/pack")})
	if status != 200 || preview["would_freeze"] != true {
		t.Fatalf("preview: %d %v", status, preview)
	}
	packs, _ := preview["packs"].([]any)
	if len(packs) != 1 {
		t.Fatalf("preview packs: %v", preview["packs"])
	}
	if packs[0].(map[string]any)["physical"] == nil {
		t.Fatalf("preview pinned no physical revision: %v", packs[0])
	}
	if entries, _ := os.ReadDir(filepath.Join(root, splitsDirName)); len(entries) != 0 {
		t.Fatal("preview wrote a split")
	}

	status, frozen := c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{
		"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "pilot.json", "note": "x"})
	if status != 400 {
		t.Fatalf("an unknown field (note) was accepted: %d %v", status, frozen)
	}
	status, frozen = c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{
		"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "pilot.json"})
	// The digest covers the freeze record, author and time included, so the
	// preview's is the digest of a split that was never written; the frozen
	// one is what a rerun cites.
	if status != 200 || frozen["split_digest"] == "" || frozen["split_digest"] == preview["split_digest"] {
		t.Fatalf("freeze: %d %v (preview said %v)", status, frozen, preview["split_digest"])
	}
	loaded, err := annotation.LoadFrozenSplit(filepath.Join(root, splitsDirName, "pilot.json"))
	if err != nil || loaded.SplitDigest != frozen["split_digest"] {
		t.Fatalf("the written split does not read back: %v", err)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{
		"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "pilot.json"}); status != 409 {
		t.Fatalf("a second write under one name: %d %v", status, out)
	}
	status, again := c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{
		"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "pilot-2.json", "supersedes": "pilot.json"})
	if status != 200 || again["revision"] != float64(2) {
		t.Fatalf("supersede: %d %v", status, again)
	}

	status, listing := c.do(http.MethodGet, "/api/annotations/splits", nil)
	splits, _ := listing["splits"].([]any)
	if status != 200 || len(splits) != 2 {
		t.Fatalf("list: %d %v", status, listing)
	}
	if err := os.WriteFile(filepath.Join(root, splitsDirName, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, listing = c.do(http.MethodGet, "/api/annotations/splits", nil)
	if splits, _ := listing["splits"].([]any); len(splits) != 3 || splits[0].(map[string]any)["error"] == nil {
		t.Fatalf("a broken split is listed with its error: %v", listing)
	}
}

// Refusals: unconfigured, wrong method, bad draft, pack handles that escape,
// names that are not file names, a missing supersedes, an author-less
// freeze, and a draft the freeze refuses (unreviewed membership), which the
// preview reports rather than refuses.
func TestSplitFreezeAPIRefusals(t *testing.T) {
	root := t.TempDir()
	p := physicalTestPack(t, root, "run-a")
	// A second pack whose object is not reviewed.
	q := physicalTestPack(t, root, "run-b")
	sq, err := annotation.LoadSidecar(q)
	if err != nil {
		t.Fatal(err)
	}
	sq.Objects[0].Status = annotation.StatusProposed
	sq.Change = annotation.Provenance{Author: "op", Operation: "unreview"}
	if err := annotation.SaveSidecar(q, sq); err != nil {
		t.Fatal(err)
	}
	_ = p
	ws := &Server{annotationPacksDir: root}
	mux := http.NewServeMux()
	ws.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := physicalClient{t: t, base: srv.URL}

	unconfigured := httptest.NewServer(func() *http.ServeMux { m := http.NewServeMux(); (&Server{}).RegisterRoutes(m); return m }())
	defer unconfigured.Close()
	u := physicalClient{t: t, base: unconfigured.URL}
	if status, out := u.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("run-a/pack")}); status != 501 {
		t.Fatalf("unconfigured preview: %d %v", status, out)
	}
	if status, out := u.do(http.MethodGet, "/api/annotations/splits", nil); status != 501 {
		t.Fatalf("unconfigured list: %d %v", status, out)
	}

	for name, tc := range map[string]struct {
		method, path string
		body         any
		status       int
		match        string
	}{
		"preview GET":        {http.MethodGet, "/api/annotations/split/preview", nil, 405, ""},
		"freeze GET":         {http.MethodGet, "/api/annotations/split/freeze", nil, 405, ""},
		"list POST":          {http.MethodPost, "/api/annotations/splits", map[string]any{}, 405, ""},
		"preview not json":   {http.MethodPost, "/api/annotations/split/preview", "{", 400, ""},
		"freeze not json":    {http.MethodPost, "/api/annotations/split/freeze", "{", 400, ""},
		"empty draft":        {http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": map[string]any{}}, 400, "no packs"},
		"escaping handle":    {http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("../run-a/pack")}, 400, "not a pack directory"},
		"absolute handle":    {http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor(p.Dir)}, 400, "not a pack directory"},
		"missing pack":       {http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("run-z/pack")}, 422, ""},
		"no author":          {http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-a/pack"), "output": "a.json"}, 400, "author"},
		"bad name":           {http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "../x.json"}, 400, "split name"},
		"not json name":      {http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "x.txt"}, 400, "split name"},
		"missing supersedes": {http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "x.json", "supersedes": "none.json"}, 400, "supersedes"},
		"unreviewed freeze":  {http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-b/pack"), "author": "op", "output": "b.json"}, 422, "review"},
		"manifest escaping":  {http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": map[string]any{"schema": annotation.SplitDraftSchema, "schema_version": annotation.SplitDraftSchemaVersion, "packs": []any{map[string]any{"dir": "run-a/pack", "split_manifest": "../m.json"}}}}, 400, "split_manifest"},
	} {
		t.Run(name, func(t *testing.T) {
			status, out := c.do(tc.method, tc.path, tc.body)
			if status != tc.status || (tc.match != "" && !strings.Contains(out["error"].(string), tc.match)) {
				t.Fatalf("%d %v, want %d %q", status, out, tc.status, tc.match)
			}
		})
	}

	// A draft may name a version 1 manifest beside the pack, relative to the
	// packs directory; one that escapes it is refused above.
	manifest := annotation.SplitManifest{
		Schema: annotation.SplitSchema, SchemaVersion: annotation.SplitSchemaVersion,
		PackDigest: p.Manifest.PackDigest, DatasetID: p.Manifest.DatasetID,
		Splits:   []annotation.Split{{Name: "tune", Role: annotation.SplitRoleTuning, ObjectIDs: []string{"car-1"}}},
		Episodes: []annotation.Episode{{EpisodeID: "ep-1", Split: "tune", ObjectIDs: []string{"car-1"}, FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: 2}}}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run-a", "split-v1.json"), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	status, viaManifest := c.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{
		"guard_seconds": 30,
		"draft": map[string]any{"schema": annotation.SplitDraftSchema, "schema_version": annotation.SplitDraftSchemaVersion,
			"packs": []any{map[string]any{"dir": "run-a/pack", "split_manifest": "run-a/split-v1.json"}}}})
	if status != 200 || viaManifest["would_freeze"] != true {
		t.Fatalf("preview through a version 1 manifest: %d %v", status, viaManifest)
	}
	// A pack handle that resolves outside the directory through a link.
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("escape/pack")}); status != 400 {
		t.Fatalf("symlinked handle: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("escape/pack"), "author": "op", "output": "x.json"}); status != 400 {
		t.Fatalf("symlinked handle on freeze: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("run-a/pack"), "supersedes": "../x.json"}); status != 400 {
		t.Fatalf("preview with a bad supersedes name: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("run-a/pack"), "supersedes": "none.json"}); status != 400 {
		t.Fatalf("preview with a missing supersedes: %d %v", status, out)
	}

	// The preview reports what stops a freeze rather than refusing.
	status, preview := c.do(http.MethodPost, "/api/annotations/split/preview", map[string]any{"draft": splitDraftFor("run-b/pack")})
	if status != 200 || preview["would_freeze"] != false {
		t.Fatalf("preview of an unreviewed pack: %d %v", status, preview)
	}
	if problems, _ := preview["membership_problems"].([]any); len(problems) == 0 {
		t.Fatalf("preview lists no membership problems: %v", preview)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, splitsDirName)); len(entries) != 0 {
		t.Fatal("a refused freeze left a file")
	}

	// Entries in splits/ that are not splits are not listed; a splits
	// directory that cannot be written refuses the freeze after the checks,
	// and one that is not a directory refuses everything that needs it.
	splits := filepath.Join(root, splitsDirName)
	if err := os.MkdirAll(filepath.Join(splits, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(splits, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := c.do(http.MethodGet, "/api/annotations/splits", nil); status != 200 || out["count"] != float64(0) {
		t.Fatalf("list with non-splits: %d %v", status, out)
	}
	if err := os.Chmod(splits, 0o500); err != nil {
		t.Fatal(err)
	}
	status, out := c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "unwritable.json"})
	_ = os.Chmod(splits, 0o755)
	if status != 500 || !strings.Contains(out["error"].(string), "write split") {
		t.Fatalf("freeze into an unwritable directory: %d %v", status, out)
	}
	if err := os.RemoveAll(splits); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(splits, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := c.do(http.MethodGet, "/api/annotations/splits", nil); status != 500 {
		t.Fatalf("list with splits as a file: %d %v", status, out)
	}
	if status, out := c.do(http.MethodPost, "/api/annotations/split/freeze", map[string]any{"draft": splitDraftFor("run-a/pack"), "author": "op", "output": "x.json"}); status != 500 {
		t.Fatalf("freeze with splits as a file: %d %v", status, out)
	}
}
