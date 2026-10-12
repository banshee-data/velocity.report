package server

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
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
	pack, err := annotation.Export(annotation.ExportConfig{VRLOGPath: writeTestVRLOG(t, 1),
		OutDir: filepath.Join(ws.annotationPacksDir, "one"), Coverage: annotation.CoverageForegroundOnly})
	if err != nil {
		t.Fatal(err)
	}
	dir := pack.Dir
	save := func(status annotation.ReviewStatus) *labelset.LabelSet {
		t.Helper()
		side, err := annotation.LoadSidecar(pack)
		if err != nil {
			t.Fatal(err)
		}
		side.Objects = []annotation.Object{{ObjectID: "a", Class: "car", Status: status}}
		side.Masks = []annotation.FrameMask{{ObjectID: "a", SampleID: 0, PointIndices: []int{0},
			Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: status}}
		ls, err := labelset.SaveMembership(pack, side, labelset.Change{Author: "op", Operation: "review"})
		if err != nil {
			t.Fatal(err)
		}
		return ls
	}
	labels := filepath.Join(save(annotation.StatusReviewed).Dir, labelset.ManifestFile)
	listing := func() packListing {
		t.Helper()
		response := callSegment(t, ws, "GET", "/api/annotations/packs", nil, ws.handleAnnotationPacks)
		var body struct {
			Packs []packListing `json:"packs"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 200 || len(body.Packs) != 1 {
			t.Fatalf("inventory: %d %s", response.Code, response.Body.String())
		}
		return body.Packs[0]
	}

	// A pack written a moment ago is read on every request: a coarse clock
	// could give its next rewrite the same time.
	if got := listing(); got.Status != "reviewed" {
		t.Fatalf("fresh pack: %+v", got)
	}
	save(annotation.StatusProposed)
	if got := listing(); got.Status != "packed" {
		t.Fatalf("a pack written within the settle time was not read again: %+v", got)
	}

	// A settled pack is read once. Unchanged stamps are an unchanged pack:
	// the label set manifest rewritten behind them is not read.
	hourAgo := time.Now().Add(-time.Hour)
	age(t, hourAgo, dir, labels)
	name := listing().LabelSetName
	if name == "" {
		t.Fatal("the listing does not name the label set")
	}
	b, err := os.ReadFile(labels)
	if err != nil {
		t.Fatal(err)
	}
	renamed := strings.ToUpper(name)
	rewrite(t, labels, bytes.Replace(b, []byte(`"`+name+`"`), []byte(`"`+renamed+`"`), 1))
	if got := listing(); got.LabelSetName != name {
		t.Fatalf("an unchanged pack was read again: %+v", got)
	}
	// Any change to a file it was read from reads it again.
	age(t, hourAgo.Add(time.Minute), labels)
	if got := listing(); got.LabelSetName != renamed {
		t.Fatalf("a rewritten label set was not read again: %+v", got)
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
