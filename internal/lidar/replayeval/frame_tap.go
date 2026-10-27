package replayeval

import (
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// FrameTapFrame is one published frame as Config.FrameTap sees it: the
// frame's returns, the published mask over them (true for foreground) and
// L3's own, the tracker's confirmed tracks after the frame's update, the P11
// surface once one is fitted (nil before, and always without a reader of it
// such as box_ground or surface_ground), and the L3 and L4 settings the frame
// was classified and clipped under. Everything is the pipeline's own and must
// be treated as read-only.
type FrameTapFrame struct {
	At     time.Time
	Scored bool
	Frame  *l2frames.LiDARFrame
	Mask   []bool
	// L3Mask is L3's classification of the same returns, before the track
	// guard gives the returns it holds inside a tracked object's box the
	// foreground class in Mask; it is Mask where the guard changes nothing.
	L3Mask  []bool
	Tracks  []*l5tracks.TrackedObject
	Surface *l3grid.RegionalGroundSurface
	// Background is the L3 parameters in force, for the closeness window,
	// and AzimuthBins the L3 grid's azimuth resolution.
	Background  l3grid.BackgroundParams
	AzimuthBins int
	// HeightBandFloor is L4's lower clip in sensor Z when the height band
	// clips, the frame's case without surface_ground.
	HeightBandFloor float64
}

// frameTapAdapter hands each frame the pipeline publishes to a FrameTap
// before the adapter it wraps, as leakAdapter does. It changes nothing the
// adapter produces.
type frameTapAdapter struct {
	inner interface {
		pipeline.VisualiserAdapter
		AdaptEmptyFrameWithMask(frame *l2frames.LiDARFrame, mask []bool) interface{}
	}
	tracker l5tracks.TrackerInterface
	tap     func(FrameTapFrame)
	start   int64
	surface *atomic.Pointer[l3grid.RegionalGroundSurface]
	bm      *l3grid.BackgroundManager
	floor   float64
	// l3Frame and l3Mask are the frame the pipeline last handed its L3 mask
	// for, and that mask.
	l3Frame *l2frames.LiDARFrame
	l3Mask  []bool
}

// observeL3 is the pipeline's L3MaskObserver: it keeps the mask for the
// frame about to be published.
func (a *frameTapAdapter) observeL3(frame *l2frames.LiDARFrame, mask []bool) {
	a.l3Frame, a.l3Mask = frame, mask
}

func (a *frameTapAdapter) observe(frame *l2frames.LiDARFrame, mask []bool) {
	if frame == nil {
		return
	}
	f := FrameTapFrame{At: frame.StartTimestamp, Scored: frame.StartTimestamp.UnixNano() >= a.start,
		Frame: frame, Mask: mask, L3Mask: mask, HeightBandFloor: a.floor}
	if a.l3Frame == frame && len(a.l3Mask) == len(mask) {
		f.L3Mask = a.l3Mask
	}
	if a.tracker != nil {
		f.Tracks = a.tracker.GetConfirmedTracks()
	}
	if a.surface != nil {
		f.Surface = a.surface.Load()
	}
	if a.bm != nil {
		f.Background = a.bm.GetParams()
		if a.bm.Grid != nil {
			f.AzimuthBins = a.bm.Grid.AzimuthBins
		}
	}
	a.tap(f)
}

func (a *frameTapAdapter) AdaptFrame(frame *l2frames.LiDARFrame, mask []bool, clusters []l4perception.WorldCluster,
	tracker l5tracks.TrackerInterface, debugFrame interface{}) interface{} {
	a.observe(frame, mask)
	return a.inner.AdaptFrame(frame, mask, clusters, tracker, debugFrame)
}

func (a *frameTapAdapter) AdaptEmptyFrame(frame *l2frames.LiDARFrame) interface{} {
	return a.inner.AdaptEmptyFrame(frame)
}

func (a *frameTapAdapter) AdaptEmptyFrameWithMask(frame *l2frames.LiDARFrame, mask []bool) interface{} {
	a.observe(frame, mask)
	return a.inner.AdaptEmptyFrameWithMask(frame, mask)
}
