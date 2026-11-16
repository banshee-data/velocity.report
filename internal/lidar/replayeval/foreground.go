package replayeval

import "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"

func replayFrameAdapter(cfg Config) *l9endpoints.FrameAdapter {
	a := l9endpoints.NewFrameAdapter(cfg.SensorID)
	a.SplitStreaming = cfg.ForegroundOnly
	return a
}
