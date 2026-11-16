package replayeval

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

func TestReplayForegroundPayloadKeepsOnlyClassifiedForeground(t *testing.T) {
	frame := &l2frames.LiDARFrame{StartTimestamp: time.Unix(100, 0), Points: []l2frames.Point{{X: 1}, {X: 2}, {X: 3}}}
	for _, foreground := range []bool{false, true} {
		adapter := replayFrameAdapter(Config{SensorID: "sensor", ForegroundOnly: foreground})
		bundle := adapter.AdaptFrame(frame, []bool{true, false, true}, nil, nil, nil).(*l9endpoints.FrameBundle)
		want := 3
		if foreground {
			want = 2
		}
		if bundle.PointCloud.PointCount != want {
			t.Fatalf("foreground=%v: got %d points", foreground, bundle.PointCloud.PointCount)
		}
		if foreground && (bundle.FrameType != l9endpoints.FrameTypeForeground || bundle.PointCloud.X[1] != 3) {
			t.Fatal("foreground recording retained a background point or lost its frame type")
		}
		bundle.PointCloud.Release()
	}
}
