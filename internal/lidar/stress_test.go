package lidar

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type noopBgStore struct{}

func (n *noopBgStore) InsertBgSnapshot(s *BgSnapshot) (int64, error) { return 1, nil }

func makeRandomFrame(rings, azBins, points int) []PointPolar {
	pts := make([]PointPolar, 0, points)
	for i := 0; i < points; i++ {
		ch := (i % rings) + 1
		az := float64(i%azBins)*360.0/float64(azBins) + rand.Float64()*0.2
		dist := 5.0 + rand.Float64()*0.5
		pts = append(pts, PointPolar{Channel: ch, Azimuth: az, Distance: dist})
	}
	return pts
}

// Stress test: run many concurrent ProcessFramePolar calls while Persist runs
// to surface any data races between readers and writers in BackgroundGrid.
func TestStressProcessFramePolarConcurrency(t *testing.T) {
	rand.Seed(42)
	rings := 40
	azBins := 360
	params := BackgroundParams{
		BackgroundUpdateFraction:       0.02,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		FreezeDurationNanos:            int64(1e9),
		NeighborConfirmationCount:      3,
	}

	mgr := NewBackgroundManager("stress-sensor", rings, azBins, params, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	workers := 8
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					pts := makeRandomFrame(rings, azBins, 1000)
					mgr.ProcessFramePolar(pts)
				}
			}
		}()
	}

	// Persist concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		store := &noopBgStore{}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = mgr.Persist(store, "stress-test")
			}
		}
	}()

	wg.Wait()
}

// Longer-running stress test: run for 10 seconds with higher concurrency to
// exercise the BackgroundGrid under sustained load. This is intentionally
// longer; run non-race by default. If you want to run under the race detector,
// expect a much slower run.
func TestStressProcessFramePolarLongRunning(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	rings := 40
	azBins := 360
	params := BackgroundParams{
		BackgroundUpdateFraction:       0.02,
		ClosenessSensitivityMultiplier: 3.0,
		SafetyMarginMeters:             0.5,
		FreezeDurationNanos:            int64(1e9),
		NeighborConfirmationCount:      3,
	}

	mgr := NewBackgroundManager("stress-sensor-long", rings, azBins, params, nil)
	if mgr == nil {
		t.Fatal("failed to create BackgroundManager")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	// increase concurrency for a harder stress test
	workers := 32
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// larger per-frame point count to increase pressure
					pts := makeRandomFrame(rings, azBins, 5000)
					mgr.ProcessFramePolar(pts)
				}
			}
		}()
	}

	// Persist concurrently at a steady rate
	wg.Add(1)
	go func() {
		defer wg.Done()
		store := &noopBgStore{}
		// persist more frequently under heavy load
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = mgr.Persist(store, "stress-test-long")
			}
		}
	}()

	wg.Wait()
}
