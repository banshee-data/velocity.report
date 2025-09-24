package lidar

import (
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestRace_ToASCProcessFramePolar runs concurrent callers of ProcessFramePolar
// and ToASCPoints to ensure there are no data races under the -race detector.
func TestRace_ToASCProcessFramePolar(t *testing.T) {
	rings := 8
	azBins := 360
	// create manager without a BgStore (persistence disabled) for a lightweight test
	bm := NewBackgroundManager("race-sensor", rings, azBins, BackgroundParams{}, nil)
	if bm == nil || bm.Grid == nil {
		t.Fatal("failed to create BackgroundManager")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	writers := 32
	readers := 32
	pointsPerFrame := 5000

	// Start writer goroutines that repeatedly call ProcessFramePolar
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			for {
				select {
				case <-stop:
					return
				default:
				}
				// craft a synthetic frame with many points to stress updates
				pts := make([]PointPolar, 0, pointsPerFrame)
				// randomize some azimuth and distances
				for i := 0; i < pointsPerFrame; i++ {
					ch := (i % rings) + 1
					az := float64(r.Intn(azBins))
					d := 1.0 + r.Float64()*50.0
					pts = append(pts, PointPolar{Channel: ch, Azimuth: az, Distance: d})
				}
				bm.ProcessFramePolar(pts)
			}
		}(w)
	}

	// Start reader goroutines that repeatedly call ToASCPoints
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = bm.ToASCPoints()
			}
		}()
	}

	// Run the concurrent workload for a longer period (10s) to increase stress
	time.Sleep(10 * time.Second)
	close(stop)
	wg.Wait()
}
