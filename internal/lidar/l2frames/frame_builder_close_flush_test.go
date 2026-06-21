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

func TestCloseFlushesBufferedFramesInCaptureOrder(t *testing.T) {
	frames := make(chan *LiDARFrame, 3)
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "close-order",
		MinFramePoints: 1,
		FrameCallback: func(frame *LiDARFrame) {
			frames <- frame
		},
	})
	fb.SetBlockOnFrameChannel(true)
	t0 := time.Unix(1_000, 0)
	fb.frameBuffer = map[string]*LiDARFrame{
		"late":  {FrameID: "late", StartTimestamp: t0.Add(2 * time.Second), PointCount: 1},
		"early": {FrameID: "early", StartTimestamp: t0, PointCount: 1},
	}
	fb.currentFrame = &LiDARFrame{FrameID: "middle", StartTimestamp: t0.Add(time.Second), PointCount: 1}
	fb.Close()

	for i, want := range []string{"early", "middle", "late"} {
		select {
		case frame := <-frames:
			if frame.FrameID != want {
				t.Fatalf("frame %d = %q, want %q", i, frame.FrameID, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing frame %q", want)
		}
	}
}
