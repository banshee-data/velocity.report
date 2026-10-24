package replayeval

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// leakTracker answers GetConfirmedTracks with a fixed set; nothing else of
// the interface is called.
type leakTracker struct {
	l5tracks.TrackerInterface
	tracks []*l5tracks.TrackedObject
}

func (f *leakTracker) GetConfirmedTracks() []*l5tracks.TrackedObject { return f.tracks }

func TestBackgroundLeakCountsReturnsInsideObservedConfirmedBoxes(t *testing.T) {
	box := &l5tracks.TrackedObject{TrackID: "car", ObjectClass: "car", X: 0, Y: 10, LatestZ: 1,
		OBBLength: 4, OBBWidth: 2, OBBHeight: 2, VX: 5}
	coasting := &l5tracks.TrackedObject{TrackID: "coast", X: 20, Y: 0, LatestZ: 1,
		OBBLength: 4, OBBWidth: 2, OBBHeight: 2, Misses: 1}
	tracker := &leakTracker{tracks: []*l5tracks.TrackedObject{box, coasting}}
	frame := &l2frames.LiDARFrame{Points: []l2frames.Point{
		{X: 0, Y: 10, Z: 1.5},  // in the box, above the bottom 0.3 m: background
		{X: 1, Y: 10, Z: 0.1},  // in the box, near its base: background
		{X: 0, Y: 10, Z: 1},    // in the box: foreground
		{X: 0, Y: 10, Z: -0.5}, // under the base: outside
		{X: 20, Y: 0, Z: 1},    // in the coasting track's box: not counted
	}}
	mask := []bool{false, false, true, false, false}
	l := newBackgroundLeak(nil)
	l.observe(frame, mask, tracker)
	l.observe(frame, mask[:2], tracker) // a mask not indexed as the points is skipped
	r := l.finish(nil)
	if r.Frames != 1 || r.FramesWithTracks != 1 || r.TrackBoxes != 1 || r.Tracks != 1 {
		t.Fatalf("frames %d with tracks %d boxes %d tracks %d", r.Frames, r.FramesWithTracks, r.TrackBoxes, r.Tracks)
	}
	if r.BackgroundPoints != 4 || r.ForegroundPoints != 1 || r.BackgroundInTrackBoxes != 2 ||
		r.ForegroundInTrackBoxes != 1 || r.BackgroundAboveBase != 1 {
		t.Fatalf("report %+v", r)
	}
	if len(r.WorstTracks) != 1 || r.WorstTracks[0].TrackID != "car" || r.WorstTracks[0].Background != 2 {
		t.Fatalf("worst tracks %+v", r.WorstTracks)
	}
}

// The settled background is split by what each cell held before the first
// moving box covered it: one ring at elevation 0, a wall at 15 m in bins
// 0-9 of 360; a moving box over bins 4-6 after the checkpoint; then bins 4
// and 5 learn a surface 2 m nearer, bin 6 keeps the wall.
func TestBackgroundLeakSplitsTheSettledBackgroundUnderMovingBoxes(t *testing.T) {
	params := l3grid.BackgroundConfigFromActiveTuning(config.MustLoadDefaultConfig()).ToBackgroundParams()
	params.DisableRegionOverrides = true
	params.TrackGuard = nil
	bm := l3grid.NewBackgroundManagerDI("leak", 1, 360, params, nil)
	if err := bm.SetRingElevations([]float64{0}); err != nil {
		t.Fatal(err)
	}
	bm.SetReplayMode(true)
	epoch := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	feed := func(frame int, rangeOf func(bin int) float64) {
		pts := make([]l3grid.PointPolar, 0, 10)
		for b := 0; b < 10; b++ {
			pts = append(pts, l3grid.PointPolar{Channel: 1, Azimuth: float64(b) + 0.5, Distance: rangeOf(b)})
		}
		if _, err := bm.ProcessFramePolarWithMaskAt(pts, epoch.Add(time.Duration(frame)*100*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	wall := func(int) float64 { return 15 }
	for f := 0; f < 100; f++ {
		feed(f, wall)
	}

	az := 5.0 * math.Pi / 180
	car := &l5tracks.TrackedObject{TrackID: "car", X: float32(15 * math.Sin(az)), Y: float32(15 * math.Cos(az)), LatestZ: 0,
		OBBLength: 1, OBBWidth: 6, OBBHeight: 2, VX: 5}
	still := &l5tracks.TrackedObject{TrackID: "still", X: float32(15 * math.Sin(az)), Y: float32(15 * math.Cos(az)), LatestZ: 0,
		OBBLength: 1, OBBWidth: 6, OBBHeight: 2}
	tracker := &leakTracker{}
	l := newBackgroundLeak(bm)
	empty := &l2frames.LiDARFrame{}
	for f := 0; f < 60; f++ {
		if f == 55 {
			tracker.tracks = []*l5tracks.TrackedObject{car, still}
		} else {
			tracker.tracks = nil
		}
		l.observe(empty, nil, tracker)
	}
	if len(l.checkpoints) != 2 || len(l.moving) != 1 || l.movingFrame[0] != 55 || len(l.boxes) != 2 {
		t.Fatalf("checkpoints %d moving %d at %v boxes %d", len(l.checkpoints), len(l.moving), l.movingFrame, len(l.boxes))
	}
	// Bins 4 and 5 learn something 2 m nearer, as a leaking vehicle would:
	// fed long enough, the EMA settles on it.
	learn := func(b int) float64 {
		if b == 4 || b == 5 {
			return 13
		}
		return 15
	}
	for f := 100; f < 3000; f++ {
		feed(f, learn)
	}
	ranges := bm.SnapshotRanges()
	if ranges[4] > 13.1 || ranges[5] > 13.1 || ranges[6] < 14.9 {
		t.Fatalf("L3 did not learn the nearer surface: ranges %v", ranges[:10])
	}
	r := l.finish(bm)
	if r.SnapshotPoints != 10 {
		t.Fatalf("%d snapshot points, want 10", r.SnapshotPoints)
	}
	// A cell's point is at its bin's start azimuth, as the snapshot draws
	// it. The box (x in [0.81, 1.81], y in [11.9, 17.9]) holds bins 4 and 5
	// at 13 m and bin 6 at 15 m; bin 3 at 15 m and bin 7 fall outside.
	if r.SnapshotAboveBase != 3 || r.SnapshotLearnedUnderMovingBoxes != 2 || r.SnapshotHeldUnderMovingBoxes != 1 ||
		r.SnapshotUnsettledBeforeMovingBoxes != 0 || r.SnapshotInTrackBoxes != r.SnapshotInMovingTrackBoxes {
		t.Fatalf("report %+v", r)
	}
}

func TestBoxIndexFindsTheEarliestBox(t *testing.T) {
	boxes := []l5tracks.PublishedBox{
		{X: 10, Y: 0, Z: 1, Length: 2, Width: 2, Height: 2},
		{X: 0, Y: 0, Z: 1, Length: 4, Width: 2, Height: 2},
		{X: 0.5, Y: 0, Z: 1, Length: 2, Width: 2, Height: 2},
	}
	ix := newBoxIndex(boxes)
	if got := ix.first(0.5, 0, 1, 0); got != 1 {
		t.Fatalf("first box at (0.5, 0) is %d, want 1", got)
	}
	if got := ix.first(0.5, 0, 0.2, 0.3); got != -1 {
		t.Fatalf("a point 0.2 m above the base passed a 0.3 m clearance: box %d", got)
	}
	if got := ix.first(5, 0, 1, 0); got != -1 {
		t.Fatalf("a point between the boxes is in box %d", got)
	}
}
