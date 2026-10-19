package l5tracks

import "testing"

type countingObserver struct{ frames []int64 }

func (c *countingObserver) ObserveFilterFrame(frame FilterFrame) {
	c.frames = append(c.frames, frame.FrameUnixNanos)
}

func TestFilterStepObserversFanOutInOrderAndSkipNil(t *testing.T) {
	a, b := &countingObserver{}, &countingObserver{}
	var fanout FilterStepObservers = []FilterStepObserver{a, nil, b}
	tk := NewTracker(DefaultTrackerConfig())
	tk.SetFilterStepObserver(fanout)
	for k := 0; k < 5; k++ {
		c := tdCluster(float32(k), 0, 0)
		c.ClusterID = int64(k + 1)
		tk.Update([]WorldCluster{c}, tdAt(1+0.1*float64(k)))
	}
	if len(a.frames) != 5 || len(b.frames) != 5 {
		t.Fatalf("observers saw %d and %d frames, want 5 each", len(a.frames), len(b.frames))
	}
	for i := range a.frames {
		if a.frames[i] != b.frames[i] || a.frames[i] != tdAt(1+0.1*float64(i)).UnixNano() {
			t.Fatalf("frame %d: %d and %d, want the Update's capture time", i, a.frames[i], b.frames[i])
		}
	}
}
