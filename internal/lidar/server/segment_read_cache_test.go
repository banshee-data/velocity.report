package server

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// age sets the modification time of every file under each path, so that a
// cache treats them as settled.
func age(t *testing.T, when time.Time, paths ...string) {
	t.Helper()
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Chtimes(path, when, when)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// rewrite replaces a file's content and gives it back its modification time,
// as a filesystem with a coarse clock would show a rewrite in the same tick.
func rewrite(t *testing.T, path string, data []byte) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) != info.Size() {
		t.Fatalf("rewrite of %s changes its size: %d to %d", path, info.Size(), len(data))
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
}

// A tree is stamped only when every change in it would show: a link is not
// followed by the walk, so a tree holding one is never kept.
func TestStampTreeRefusesWhatItCannotWatch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "frames"), 0755); err != nil {
		t.Fatal(err)
	}
	chunk := filepath.Join(root, "frames", "chunk_0000.pb")
	if err := os.WriteFile(chunk, []byte("frames"), 0644); err != nil {
		t.Fatal(err)
	}
	stamped, ok := stampTree(root)
	if !ok || len(stamped) != 3 {
		t.Fatalf("plain tree: %+v %v", stamped, ok)
	}
	if again, _ := stampTree(root); !again.equal(stamped) {
		t.Fatal("an unchanged tree stamped differently")
	}
	if err := os.WriteFile(chunk, []byte("frames, longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if changed, ok := stampTree(root); !ok || changed.equal(stamped) {
		t.Fatal("a rewritten file did not change the stamp")
	}
	if _, ok := stampTree(filepath.Join(root, "absent")); ok {
		t.Fatal("stamped a tree that is not there")
	}
	if err := os.Symlink(chunk, filepath.Join(root, "frames", "chunk_0001.pb")); err != nil {
		t.Fatal(err)
	}
	if _, ok := stampTree(root); ok {
		t.Fatal("stamped a tree holding a link")
	}
}

func TestSegmentRankingCarriesItsStrip(t *testing.T) {
	ws, _ := segmentServer(t)
	read := func(path string) map[string]any {
		t.Helper()
		response := callSegment(t, ws, "GET", path, nil, ws.handleSegments)
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 200 {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		return body
	}
	if _, found := read("/api/lidar/segments?run_id=run&finder=following")["strip_svg"]; found {
		t.Fatal("a ranking carried a strip nobody asked for")
	}
	carried, _ := read("/api/lidar/segments?run_id=run&finder=following&strip=1")["strip_svg"].(string)
	strip := callSegment(t, ws, "GET", "/api/lidar/segments/strip?run_id=run&finder=following", nil, ws.handleSegmentStrip)
	if strip.Code != 200 || carried == "" || carried != strip.Body.String() {
		t.Fatalf("the carried strip is not the strip endpoint's:\n%s\n%s", carried, strip.Body.String())
	}
}

func TestPackListingsAreReadAgainOnlyWhenTheirFilesChange(t *testing.T) {
	ws, _ := segmentServer(t)
	dir := filepath.Join(ws.annotationPacksDir, "one")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"dataset_id":"dataset","pack_digest":"sha256:pack"}`), 0644); err != nil {
		t.Fatal(err)
	}
	sidecar := func(status annotation.ReviewStatus) []byte {
		side := annotation.Sidecar{PackDigest: "sha256:pack",
			Objects: []annotation.Object{{ObjectID: "a", Class: "car", Status: status}},
			Masks:   []annotation.FrameMask{{ObjectID: "a", Status: status}}}
		b, err := json.Marshal(side)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	sidecarPath := filepath.Join(dir, "annotations.json")
	if err := os.WriteFile(sidecarPath, sidecar(annotation.StatusReviewed), 0644); err != nil {
		t.Fatal(err)
	}
	status := func() string {
		t.Helper()
		response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks)
		var body struct {
			Packs []packListing `json:"packs"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 200 || len(body.Packs) != 1 {
			t.Fatalf("inventory: %d %s", response.Code, response.Body.String())
		}
		return body.Packs[0].Status
	}

	// A pack written a moment ago is read on every request: a coarse clock
	// could give its next rewrite the same time.
	if got := status(); got != "reviewed" {
		t.Fatalf("fresh pack: %s", got)
	}
	rewrite(t, sidecarPath, sidecar(annotation.StatusProposed))
	if got := status(); got != "packed" {
		t.Fatalf("a pack written within the settle time was not read again: %s", got)
	}

	// A settled pack is read once. Unchanged stamps are an unchanged pack:
	// the sidecar rewritten behind them is not read.
	hourAgo := time.Now().Add(-time.Hour)
	age(t, hourAgo, dir)
	if got := status(); got != "packed" {
		t.Fatalf("settled pack: %s", got)
	}
	rewrite(t, sidecarPath, sidecar(annotation.StatusReviewed))
	if got := status(); got != "packed" {
		t.Fatalf("an unchanged pack was read again: %s", got)
	}
	// Any change to a file it was read from reads it again.
	age(t, hourAgo.Add(time.Minute), sidecarPath)
	if got := status(); got != "reviewed" {
		t.Fatalf("a rewritten sidecar was not read again: %s", got)
	}
	proposals := filepath.Join(dir, "proposals")
	if err := os.Mkdir(proposals, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proposals, "layer.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if listed := ws.packListings.list([]string{dir}, time.Now()); listed[0].ProposalLayers != 1 {
		t.Fatalf("a new proposal layer was not counted: %+v", listed[0])
	}
	// A pack that is gone is forgotten.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks); !strings.Contains(response.Body.String(), `"count":0`) || len(ws.packListings.entries) != 0 {
		t.Fatalf("removed pack: %s %d", response.Body.String(), len(ws.packListings.entries))
	}
}

func TestRecordingSeriesIsDecodedOnceAndMarkedPerRequest(t *testing.T) {
	ws, _ := segmentServer(t)
	recording := replayedRun(t, ws, "replayed")
	rank := func(finder string) ([]segments.Window, error) {
		windows, _, err := ws.findRunSegments(segmentRequest{RunID: "replayed", Finder: finder})
		return windows, err
	}

	// A recording still being written is decoded on every request.
	if _, err := rank("following"); err != nil {
		t.Fatal(err)
	}
	if ws.segmentRecordings.path != "" {
		t.Fatalf("an unsettled recording was kept: %s", ws.segmentRecordings.path)
	}

	hourAgo := time.Now().Add(-time.Hour)
	age(t, hourAgo, recording)
	following, err := rank("following")
	if err != nil || len(following) != 1 || following[0].PairFrames != 5 {
		t.Fatalf("settled recording: %+v %v", following, err)
	}
	flagged, err := rank("split_flags")
	if err != nil || len(flagged) != 1 || flagged[0].Events != 5 {
		t.Fatalf("split marks: %+v %v", flagged, err)
	}

	// The settled recording is not decoded again: its frames, damaged behind
	// unchanged stamps, still rank as they did.
	var chunk string
	if err := filepath.WalkDir(recording, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "chunk_") {
			chunk = path
		}
		return err
	}); err != nil || chunk == "" {
		t.Fatalf("recording has no frame chunk: %v", err)
	}
	info, err := os.Stat(chunk)
	if err != nil {
		t.Fatal(err)
	}
	rewrite(t, chunk, make([]byte, info.Size()))
	again, err := rank("following")
	if err != nil || len(again) != 1 || again[0].ID != following[0].ID {
		t.Fatalf("a settled recording was decoded again: %+v %v", again, err)
	}

	// The marks are the run's, read on every request.
	if _, err := ws.db.Exec(`UPDATE lidar_run_tracks SET is_split_candidate=0 WHERE run_id='replayed'`); err != nil {
		t.Fatal(err)
	}
	if flagged, err := rank("split_flags"); err != nil || len(flagged) != 0 {
		t.Fatalf("a cleared mark was kept with the recording: %+v %v", flagged, err)
	}

	// Each caller has its own series: sorting one leaves the next as read.
	first, err := ws.segmentRecordings.load(recording, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	want := slices.Clone(first)
	slices.Reverse(first)
	first[0].X = -1
	second, err := ws.segmentRecordings.load(recording, nil, time.Now())
	if err != nil || !slices.Equal(second, want) {
		t.Fatalf("a caller's change reached the kept series: %v", err)
	}

	// A change to the recording reads it again: the damaged frames rank as
	// what they now are, and a missing chunk is reported.
	age(t, hourAgo.Add(time.Minute), chunk)
	if damaged, err := rank("following"); err == nil && len(damaged) == 1 {
		t.Fatalf("a changed recording was not read again: %+v", damaged)
	}
	if err := os.Remove(chunk); err != nil {
		t.Fatal(err)
	}
	if _, err := rank("following"); err == nil || !strings.Contains(err.Error(), "recording could not be read") {
		t.Fatalf("a recording missing a chunk: %v", err)
	}
}
