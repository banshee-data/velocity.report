package replayeval

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestTrackerTapSeesEachUpdateAfterIt(t *testing.T) {
	tracker := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	start := time.Unix(1_700_000_000, 0)
	var frames []TrackerTapFrame
	tap := &tapTracker{TrackerInterface: tracker, start: start.Add(200 * time.Millisecond).UnixNano(),
		tap: func(f TrackerTapFrame) { frames = append(frames, f) }}
	for i := 0; i < 4; i++ {
		c := l5tracks.WorldCluster{ClusterID: int64(i + 1), CentroidX: 5 + float32(i)*0.5, CentroidY: 3,
			BoundingBoxLength: 4, BoundingBoxWidth: 2, PointsCount: 40}
		tap.Update([]l5tracks.WorldCluster{c}, start.Add(time.Duration(i)*100*time.Millisecond))
	}
	tap.AdvanceMisses(start.Add(400 * time.Millisecond))
	if len(frames) != 5 {
		t.Fatalf("%d tap frames, want 5", len(frames))
	}
	if frames[0].Scored || !frames[2].Scored {
		t.Fatalf("scored flags %v %v: the boundary is the third frame", frames[0].Scored, frames[2].Scored)
	}
	if len(frames[1].Associations) != 1 || frames[1].Associations[0] == "" || len(frames[1].Tracks) != 1 {
		t.Fatalf("second frame: associations %v, %d tracks", frames[1].Associations, len(frames[1].Tracks))
	}
	if len(frames[4].Clusters) != 0 || len(frames[4].Tracks) != 1 {
		t.Fatalf("a frame without clusters reaches the tap with none and the coasting track")
	}
}
