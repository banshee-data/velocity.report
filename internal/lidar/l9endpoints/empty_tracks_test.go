package l9endpoints

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestEmptyFramePublishesCoastedTracksWithoutDetections(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tk.Update([]l5tracks.WorldCluster{{CentroidX: 10, PointsCount: 20}}, base)
	tk.UpdateGap(base.Add(100 * time.Millisecond))
	frame := &l2frames.LiDARFrame{StartTimestamp: base.Add(100 * time.Millisecond)}
	a := NewFrameAdapter("empty-tracks")
	bundle := toFrameBundle(t, a.AdaptEmptyFrameWithTracks(frame, nil, tk))
	if bundle.FrameType != FrameTypeEmpty || bundle.PointCloud != nil || bundle.Clusters != nil {
		t.Fatal("empty publication invented detection geometry")
	}
	if bundle.Tracks == nil || len(bundle.Tracks.Tracks) != 1 || bundle.Tracks.Tracks[0].Misses != 1 || bundle.Tracks.Tracks[0].ObservationCount != 1 {
		t.Fatal("coasted state missing from empty frame")
	}
	trail := bundle.Tracks.Trails[0]
	if trail.Points[len(trail.Points)-1].TimestampNanos != frame.StartTimestamp.UnixNano() || bundle.FrameID != 1 {
		t.Fatal("empty publication lost capture time or allocated an extra frame")
	}
}
