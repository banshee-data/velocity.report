package l2frames

import (
	"sync"
	"testing"
	"time"
)

// A replay's reader can stall longer than the buffer timeout when its host is
// short of memory. Live, an idle frame is finalised after the timeout because
// the sensor has stopped sending; in a replay (a blocking builder) the reader
// sets the pace, so the stalled rotation must stay whole and end on its
// azimuth wrap. Before this the cleanup cut it in two and discarded the
// remainder as an incomplete frame (l3bg0020).
func TestReplayDoesNotCutAStalledRotation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		blocking bool
		wantCut  bool
	}{{"live finalises an idle frame", false, true}, {"a replay keeps its rotation whole", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var frames []*LiDARFrame
			fb := NewFrameBuilder(FrameBuilderConfig{
				SensorID:       "stall-" + tc.name,
				MinFramePoints: 1,
				BufferTimeout:  5 * time.Millisecond,
				FrameCallback: func(f *LiDARFrame) {
					mu.Lock()
					frames = append(frames, f)
					mu.Unlock()
				},
			})
			defer fb.Close()
			fb.SetBlockOnFrameChannel(tc.blocking)

			base := time.Unix(1_800_000_000, 0)
			half := func(from, to float64, n int) {
				pts := make([]PointPolar, 0, n)
				for i := 0; i < n; i++ {
					az := from + (to-from)*float64(i)/float64(n)
					pts = append(pts, PointPolar{Channel: 1, Azimuth: az, Distance: 10,
						Timestamp: base.Add(time.Duration(az*1000) * time.Microsecond).UnixNano()})
				}
				fb.AddPointsPolar(pts)
			}
			half(0, 180, 50) // the first half of a rotation
			fb.mu.Lock()
			if fb.currentFrame != nil {
				fb.currentFrame.EndWallTime = time.Now().Add(-time.Second) // the reader stalled
			}
			fb.mu.Unlock()
			fb.cleanupFrames()

			mu.Lock()
			cut := len(frames) > 0
			mu.Unlock()
			// cleanupFrames finalises through the channel when not blocking.
			if !tc.blocking {
				deadline := time.Now().Add(time.Second)
				for time.Now().Before(deadline) && !cut {
					time.Sleep(5 * time.Millisecond)
					mu.Lock()
					cut = len(frames) > 0
					mu.Unlock()
				}
			}
			if cut != tc.wantCut {
				t.Fatalf("frame finalised during the stall = %v, want %v", cut, tc.wantCut)
			}
			if tc.blocking {
				half(180, 359, 50) // the rest of the rotation
				fb.FlushPendingFrames()
				fb.WaitForCallbacks()
				mu.Lock()
				defer mu.Unlock()
				if len(frames) != 1 || frames[0].PointCount != 100 {
					n := -1
					if len(frames) > 0 {
						n = frames[0].PointCount
					}
					t.Fatalf("got %d frame(s), the first with %d points; want one whole rotation of 100", len(frames), n)
				}
			}
		})
	}
}
