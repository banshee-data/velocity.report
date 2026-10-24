package replayeval

import (
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// backgroundLeakFile is where Config.BackgroundLeak writes its report.
const backgroundLeakFile = "background_leak.json"

// BackgroundLeakReport measures how much of the replay's moving objects the
// background keeps (the v0.5.13 backlog item: the background leaks moving
// objects). It reads the two things a site model takes from a replay:
//
//   - the classified frames a full-frames client accumulates: each frame's
//     returns as published (0 background, 1 foreground), counted inside and
//     outside the boxes of the confirmed tracks observed in that frame;
//   - the settled background at the end of the replay (the snapshot split
//     streaming sends), counted inside and outside every such box the
//     replay saw.
//
// A box is the track's published box (l5tracks.PublishedBox), not grown:
// what it holds is the object as the tracker drew it. A return under the
// box's base is outside it, so the road a vehicle stands on is not counted.
// Coasting tracks are left out, since their box is a prediction.
type BackgroundLeakReport struct {
	Frames int `json:"frames"`
	// FramesWithTracks counts frames with at least one observed confirmed track.
	FramesWithTracks int `json:"frames_with_tracks"`
	// TrackBoxes counts box-frames: one per observed confirmed track per frame.
	TrackBoxes int64 `json:"track_boxes"`
	Tracks     int   `json:"tracks"`

	BackgroundPoints int64 `json:"background_points"`
	ForegroundPoints int64 `json:"foreground_points"`
	// BackgroundInTrackBoxes is the leak in the frames: background returns
	// inside an observed confirmed track's box in the same frame.
	BackgroundInTrackBoxes int64 `json:"background_in_track_boxes"`
	ForegroundInTrackBoxes int64 `json:"foreground_in_track_boxes"`
	// BackgroundAboveBase is the part of BackgroundInTrackBoxes at least
	// leakBaseClearanceMetres above the box's base. A box's base is its
	// cluster's lowest return, which can be the road at a wheel, so the
	// returns just above it are as often the road as the vehicle.
	BackgroundAboveBase int64 `json:"background_in_track_boxes_above_base"`

	// SnapshotPoints is the settled background at the end of the replay;
	// SnapshotInTrackBoxes the part of it inside any box the replay saw.
	SnapshotPoints       int `json:"snapshot_points"`
	SnapshotInTrackBoxes int `json:"snapshot_in_track_boxes"`
	// SnapshotInMovingTrackBoxes counts only the boxes drawn while their
	// track moved at movingTrackMps or more. A static structure a box
	// overlaps (a kerb, a hedge tracked as an object) is inside some box;
	// it is inside a moving vehicle's box only where the vehicle was.
	SnapshotInMovingTrackBoxes int `json:"snapshot_in_moving_track_boxes"`
	// SnapshotAboveBase is the part of SnapshotInMovingTrackBoxes at least
	// leakBaseClearanceMetres above the box's base: the settled background
	// where a moving vehicle's body was.
	SnapshotAboveBase int `json:"snapshot_in_moving_track_boxes_above_base"`
	// SnapshotAboveBase splits by what the cell held before a moving box
	// first covered it, read from the settled background at the last
	// checkpoint (every leakCheckpointFrames frames) before that box:
	//
	//   - SnapshotLearnedUnderMovingBoxes: settled then, and its settled
	//     range now at least leakRangeChangeMetres nearer. The cell learned
	//     something in front of what it held, where a vehicle was: the leak.
	//   - SnapshotHeldUnderMovingBoxes: settled then, at a range within
	//     leakRangeChangeMetres of today's. It held the same surface before
	//     the vehicle came, so the box overlaps static structure.
	//   - SnapshotUnsettledBeforeMovingBoxes: not settled then (or no
	//     checkpoint before the box), so the replay cannot say.
	//
	// The rest moved farther away: settled on something nearer before the
	// box, then learned what is behind it.
	SnapshotLearnedUnderMovingBoxes    int `json:"snapshot_learned_under_moving_track_boxes"`
	SnapshotHeldUnderMovingBoxes       int `json:"snapshot_held_under_moving_track_boxes"`
	SnapshotUnsettledBeforeMovingBoxes int `json:"snapshot_unsettled_before_moving_track_boxes"`

	// WorstTracks are the tracks whose boxes held the most background
	// returns over the replay, most first, for reading the leak back.
	WorstTracks []BackgroundLeakTrack `json:"worst_tracks,omitempty"`
}

// BackgroundLeakTrack is one track's share of the leak in the frames.
type BackgroundLeakTrack struct {
	TrackID    string `json:"track_id"`
	Class      string `json:"class,omitempty"`
	Frames     int    `json:"frames"`
	Background int64  `json:"background_in_box"`
	Foreground int64  `json:"foreground_in_box"`
}

type leakTrack struct {
	class      string
	frames     int
	background int64
	foreground int64
}

// backgroundLeak accumulates a BackgroundLeakReport frame by frame.
type backgroundLeak struct {
	report BackgroundLeakReport
	bm     *l3grid.BackgroundManager
	boxes  []l5tracks.PublishedBox
	moving []l5tracks.PublishedBox
	// movingFrame is the frame (0-based) each of moving was drawn in.
	movingFrame []int
	// checkpoints[i] is bm.SnapshotRanges() taken before frame
	// i*leakCheckpointFrames was counted.
	checkpoints [][]float32
	tracks      map[string]*leakTrack
}

// movingTrackMps is the speed from which a box counts as a moving track's
// for BackgroundLeakReport.SnapshotInMovingTrackBoxes.
const movingTrackMps = 1.0

// leakBaseClearanceMetres is how far above a box's base a return must be to
// count in the report's _above_base figures.
const leakBaseClearanceMetres = 0.3

// leakCheckpointFrames is how often the settled background's ranges are
// kept, to read back what a cell held before a moving box covered it.
const leakCheckpointFrames = 50

// leakRangeChangeMetres is how far a settled cell's range must move to
// count as having learned something else.
const leakRangeChangeMetres = 0.3

// newBackgroundLeak measures against bm's settled background; with a nil
// bm only the per-frame figures are counted.
func newBackgroundLeak(bm *l3grid.BackgroundManager) *backgroundLeak {
	return &backgroundLeak{bm: bm, tracks: map[string]*leakTrack{}}
}

// observe counts one published frame against the tracker's observed
// confirmed tracks.
func (l *backgroundLeak) observe(frame *l2frames.LiDARFrame, mask []bool, tracker l5tracks.TrackerInterface) {
	if l == nil || frame == nil || len(mask) != len(frame.Points) {
		return
	}
	frameIndex := l.report.Frames
	if l.bm != nil && frameIndex%leakCheckpointFrames == 0 {
		l.checkpoints = append(l.checkpoints, l.bm.SnapshotRanges())
	}
	l.report.Frames++
	type box struct {
		id string
		b  l5tracks.PublishedBox
		// The box's bounds in the plane, a cheap first test.
		minX, maxX, minY, maxY float64
	}
	var boxes []box
	if tracker != nil {
		for _, t := range tracker.GetConfirmedTracks() {
			if t.Misses != 0 {
				continue
			}
			b := t.PublishedBox()
			if !(b.Length > 0 && b.Width > 0 && b.Height > 0) {
				continue
			}
			r := 0.5 * math.Hypot(float64(b.Length), float64(b.Width))
			boxes = append(boxes, box{id: t.TrackID, b: b,
				minX: float64(b.X) - r, maxX: float64(b.X) + r, minY: float64(b.Y) - r, maxY: float64(b.Y) + r})
			l.boxes = append(l.boxes, b)
			if t.Speed() >= movingTrackMps {
				l.moving = append(l.moving, b)
				l.movingFrame = append(l.movingFrame, frameIndex)
			}
			lt := l.tracks[t.TrackID]
			if lt == nil {
				lt = &leakTrack{}
				l.tracks[t.TrackID] = lt
			}
			lt.class = t.ObjectClass
			lt.frames++
		}
	}
	if len(boxes) > 0 {
		l.report.FramesWithTracks++
		l.report.TrackBoxes += int64(len(boxes))
	}
	for i, p := range frame.Points {
		fg := mask[i]
		if fg {
			l.report.ForegroundPoints++
		} else {
			l.report.BackgroundPoints++
		}
		for j := range boxes {
			bx := &boxes[j]
			if p.X < bx.minX || p.X > bx.maxX || p.Y < bx.minY || p.Y > bx.maxY {
				continue
			}
			if !bx.b.Contains(p.X, p.Y, p.Z, 0, 0) {
				continue
			}
			lt := l.tracks[bx.id]
			if fg {
				l.report.ForegroundInTrackBoxes++
				lt.foreground++
			} else {
				l.report.BackgroundInTrackBoxes++
				lt.background++
				if p.Z >= float64(bx.b.Z)-float64(bx.b.Height)/2+leakBaseClearanceMetres {
					l.report.BackgroundAboveBase++
				}
			}
			break
		}
	}
}

// finish counts the settled background against every box seen and returns
// the report.
func (l *backgroundLeak) finish(bm *l3grid.BackgroundManager) BackgroundLeakReport {
	r := l.report
	r.Tracks = len(l.tracks)
	l.countSnapshot(&r, bm)
	worst := make([]BackgroundLeakTrack, 0, len(l.tracks))
	for id, t := range l.tracks {
		if t.background == 0 {
			continue
		}
		worst = append(worst, BackgroundLeakTrack{TrackID: id, Class: t.class, Frames: t.frames,
			Background: t.background, Foreground: t.foreground})
	}
	sort.Slice(worst, func(i, j int) bool {
		if worst[i].Background != worst[j].Background {
			return worst[i].Background > worst[j].Background
		}
		return worst[i].TrackID < worst[j].TrackID
	})
	if len(worst) > 10 {
		worst = worst[:10]
	}
	r.WorstTracks = worst
	return r
}

// countSnapshot counts the settled background at the end against the
// boxes the replay saw, and splits the part under moving boxes by what each
// cell held before the first of them covered it.
func (l *backgroundLeak) countSnapshot(r *BackgroundLeakReport, bm *l3grid.BackgroundManager) {
	snap, err := bm.GenerateBackgroundSnapshot()
	if err != nil || snap == nil || snap.AzimuthBins <= 0 {
		return
	}
	ranges := bm.SnapshotRanges()
	all, moving := newBoxIndex(l.boxes), newBoxIndex(l.moving)
	azBinResDeg := 360.0 / float64(snap.AzimuthBins)
	for idx, rng := range ranges {
		if rng <= 0 {
			continue
		}
		ring, azBin := idx/snap.AzimuthBins, idx%snap.AzimuthBins
		if ring >= len(snap.RingElevations) {
			continue
		}
		x, y, z := l2frames.SphericalToCartesian(float64(rng), float64(azBin)*azBinResDeg, float64(snap.RingElevations[ring]))
		r.SnapshotPoints++
		if all.first(x, y, z, 0) >= 0 {
			r.SnapshotInTrackBoxes++
		}
		if moving.first(x, y, z, 0) >= 0 {
			r.SnapshotInMovingTrackBoxes++
		}
		j := moving.first(x, y, z, leakBaseClearanceMetres)
		if j < 0 {
			continue
		}
		r.SnapshotAboveBase++
		// The last checkpoint taken strictly before the box's frame.
		f := l.movingFrame[j]
		ck := (f - 1) / leakCheckpointFrames
		if f <= 0 || ck >= len(l.checkpoints) || l.checkpoints[ck][idx] <= 0 {
			r.SnapshotUnsettledBeforeMovingBoxes++
			continue
		}
		switch before := l.checkpoints[ck][idx]; {
		case before-rng >= leakRangeChangeMetres:
			r.SnapshotLearnedUnderMovingBoxes++
		case rng-before < leakRangeChangeMetres:
			r.SnapshotHeldUnderMovingBoxes++
		}
	}
}

// boxIndex finds the boxes holding a point by way of a one-metre grid of
// the boxes' plane bounds.
type boxIndex struct {
	boxes []l5tracks.PublishedBox
	grid  map[[2]int32][]int32
}

func newBoxIndex(boxes []l5tracks.PublishedBox) boxIndex {
	ix := boxIndex{boxes: boxes, grid: map[[2]int32][]int32{}}
	for i, b := range boxes {
		r := 0.5 * math.Hypot(float64(b.Length), float64(b.Width))
		x0, x1 := int32(math.Floor(float64(b.X)-r)), int32(math.Floor(float64(b.X)+r))
		y0, y1 := int32(math.Floor(float64(b.Y)-r)), int32(math.Floor(float64(b.Y)+r))
		for gx := x0; gx <= x1; gx++ {
			for gy := y0; gy <= y1; gy++ {
				k := [2]int32{gx, gy}
				ix.grid[k] = append(ix.grid[k], int32(i))
			}
		}
	}
	return ix
}

// first returns the lowest index of a box holding the point at least
// clearance above its base, or -1. Boxes are kept in the order they were
// drawn, so it is the earliest such box.
func (ix boxIndex) first(x, y, z, clearance float64) int {
	for _, j := range ix.grid[[2]int32{int32(math.Floor(x)), int32(math.Floor(y))}] {
		b := ix.boxes[j]
		if z >= float64(b.Z)-float64(b.Height)/2+clearance && b.Contains(x, y, z, 0, 0) {
			return int(j)
		}
	}
	return -1
}

// leakAdapter measures each frame the pipeline publishes, with the mask it
// publishes, before handing the frame to the adapter it wraps. It changes
// nothing the adapter produces.
type leakAdapter struct {
	inner interface {
		pipeline.VisualiserAdapter
		AdaptEmptyFrameWithMask(frame *l2frames.LiDARFrame, mask []bool) interface{}
	}
	tracker l5tracks.TrackerInterface
	leak    *backgroundLeak
}

func (a *leakAdapter) AdaptFrame(frame *l2frames.LiDARFrame, mask []bool, clusters []l4perception.WorldCluster,
	tracker l5tracks.TrackerInterface, debugFrame interface{}) interface{} {
	a.leak.observe(frame, mask, tracker)
	return a.inner.AdaptFrame(frame, mask, clusters, tracker, debugFrame)
}

func (a *leakAdapter) AdaptEmptyFrame(frame *l2frames.LiDARFrame) interface{} {
	return a.inner.AdaptEmptyFrame(frame)
}

func (a *leakAdapter) AdaptEmptyFrameWithMask(frame *l2frames.LiDARFrame, mask []bool) interface{} {
	a.leak.observe(frame, mask, a.tracker)
	return a.inner.AdaptEmptyFrameWithMask(frame, mask)
}
