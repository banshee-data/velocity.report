package replayeval

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestTrackerTapPreservesUnavailableGapSupport(t *testing.T) {
	at := time.Unix(1_700_000_000, 0)
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tk.Update([]l5tracks.WorldCluster{{CentroidX: 10}}, at)
	var frames []TrackerTapFrame
	tap := &tapTracker{TrackerInterface: tk, start: at.UnixNano(), tap: func(f TrackerTapFrame) { frames = append(frames, f) }}
	tap.UpdateGap(at.Add(100 * time.Millisecond))
	if len(frames) != 1 || !frames[0].Scored || len(frames[0].Clusters) != 0 || len(frames[0].Tracks) != 1 || frames[0].Tracks[0].LastSupport != l5tracks.SupportMissedUnknown {
		t.Fatal("instrumentation lost gap semantics or fabricated detections")
	}
}
