package l2frames

import (
	"testing"
	"time"
)

func TestCloseFlushesCurrentFrame(t *testing.T) {
	frames := make(chan *LiDARFrame, 1)
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "close-flush",
		MinFramePoints: 1,
		FrameCallback: func(frame *LiDARFrame) {
			frames <- frame
		},
	})
	fb.SetBlockOnFrameChannel(true)
	fb.AddPointsPolar([]PointPolar{{Channel: 1, Azimuth: 10, Distance: 5, Timestamp: time.Now().UnixNano()}})
	fb.Close()

	select {
	case frame := <-frames:
		if frame == nil || len(frame.PolarPoints) != 1 {
			t.Fatalf("unexpected flushed frame: %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not flush the current frame")
	}
}
