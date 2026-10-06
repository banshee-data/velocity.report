package l3grid

import (
	"testing"
	"time"
)

// polarRing builds one frame's worth of points across a single ring, at a
// constant range. Enough to drive the per-point path without depending on a
// capture. Channel is 1-based: channel 1 is ring 0.
func polarRing(ring int, azBins int, rangeM float64, ts int64) []PointPolar {
	pts := make([]PointPolar, 0, azBins)
	for i := 0; i < azBins; i++ {
		pts = append(pts, PointPolar{
			Channel:   ring + 1,
			Azimuth:   float64(i) * (360.0 / float64(azBins)),
			Distance:  rangeM,
			Timestamp: ts,
		})
	}
	return pts
}

// TestProcessFrameDefaultsZeroTimestamp covers the zero-time fallback.
// ProcessFramePolarWithMask forwards a zero time when the caller has no frame
// timestamp, and settling is measured against it — anchoring warm-up at the
// zero time would make it appear to have elapsed in the year 1.
func TestProcessFrameDefaultsZeroTimestamp(t *testing.T) {
	bm := NewBackgroundManagerDI(t.Name(), 2, 8, BackgroundParams{
		SeedFromFirstObservation: true,
		BackgroundUpdateFraction: 0.5,
		SafetyMarginMetres:       20,
	}, nil)

	mask, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, 8, 10, 0), time.Time{})
	if err != nil {
		t.Fatalf("ProcessFramePolarWithMaskAt: %v", err)
	}
	if mask == nil {
		t.Fatal("expected a mask")
	}
	if bm.StartTime.IsZero() {
		t.Error("a zero frame time should fall back to the wall clock, not anchor at the zero time")
	}
}

// TestProcessFrameDefaultsNoiseFraction covers the noise-fraction fallback. A
// zero relative noise term would leave every observation with no tolerance at
// all, so the fallback is what keeps an unset parameter from turning the whole
// scene into foreground. The check is an equivalence: unset must behave exactly
// as the 0.01 default it falls back to.
func TestProcessFrameDefaultsNoiseFraction(t *testing.T) {
	params := func(noise float32) BackgroundParams {
		return BackgroundParams{
			SeedFromFirstObservation: true,
			BackgroundUpdateFraction: 0.5,
			SafetyMarginMetres:       0.2,
			NoiseRelativeFraction:    noise,
		}
	}

	unset := NewBackgroundManagerDI(t.Name()+"-unset", 2, 8, params(0), nil)
	explicit := NewBackgroundManagerDI(t.Name()+"-explicit", 2, 8, params(0.01), nil)

	for i := 0; i < 4; i++ {
		if _, err := unset.ProcessFramePolarWithMaskAt(polarRing(0, 8, 10, int64(i)), time.Now()); err != nil {
			t.Fatalf("unset frame %d: %v", i, err)
		}
		if _, err := explicit.ProcessFramePolarWithMaskAt(polarRing(0, 8, 10, int64(i)), time.Now()); err != nil {
			t.Fatalf("explicit frame %d: %v", i, err)
		}
	}

	// A slightly divergent frame: whether each point clears the tolerance is
	// exactly what the noise fraction decides.
	unsetMask, err := unset.ProcessFramePolarWithMaskAt(polarRing(0, 8, 10.15, 9), time.Now())
	if err != nil {
		t.Fatalf("unset probe frame: %v", err)
	}
	explicitMask, err := explicit.ProcessFramePolarWithMaskAt(polarRing(0, 8, 10.15, 9), time.Now())
	if err != nil {
		t.Fatalf("explicit probe frame: %v", err)
	}

	if len(unsetMask) != len(explicitMask) {
		t.Fatalf("mask lengths differ: %d vs %d", len(unsetMask), len(explicitMask))
	}
	for i := range unsetMask {
		if unsetMask[i] != explicitMask[i] {
			t.Fatalf("point %d: unset noise fraction gave %v, the 0.01 default gave %v; "+
				"the fallback is not applying", i, unsetMask[i], explicitMask[i])
		}
	}
}

// TestProcessFrameCapsNeighbourSearchRadius covers the search-radius cap. The
// radius follows the neighbour-confirmation count, and an operator can set that
// arbitrarily high; without the cap each point would scan the whole ring.
func TestProcessFrameCapsNeighbourSearchRadius(t *testing.T) {
	bm := NewBackgroundManagerDI(t.Name(), 2, 64, BackgroundParams{
		SeedFromFirstObservation:   true,
		BackgroundUpdateFraction:   0.5,
		SafetyMarginMetres:         0.1,
		NoiseRelativeFraction:      0.01,
		NeighbourConfirmationCount: 25, // far above the cap of 10
	}, nil)

	for i := 0; i < 3; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, 64, 10, int64(i)), time.Now()); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}

	// A frame at a very different range exercises the neighbour confirmation
	// walk for every point.
	if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, 64, 3, 4), time.Now()); err != nil {
		t.Fatalf("divergent frame: %v", err)
	}
}

// TestProcessFrameDrainsConfidenceWithoutFloor covers the full-drain arm. With
// no confidence floor configured a cell that keeps disagreeing must be allowed
// to fall all the way to zero, which is what lets a moved sensor relearn its
// scene instead of defending a background that is no longer there.
func TestProcessFrameDrainsConfidenceWithoutFloor(t *testing.T) {
	const azBins = 8
	bm := NewBackgroundManagerDI(t.Name(), 1, azBins, BackgroundParams{
		SeedFromFirstObservation: true,
		BackgroundUpdateFraction: 0.05,
		SafetyMarginMetres:       0.05,
		NoiseRelativeFraction:    0.001,
		MinConfidenceFloor:       0, // explicit: allow a full drain
	}, nil)

	// Build confidence at one range.
	for i := 0; i < 8; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, azBins, 10, int64(i)), time.Now()); err != nil {
			t.Fatalf("seed frame %d: %v", i, err)
		}
	}
	seeded := totalConfidence(bm)
	if seeded == 0 {
		t.Fatal("expected the seeding frames to build some confidence")
	}

	// Then disagree, repeatedly.
	for i := 0; i < 40; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, azBins, 2, int64(100+i)), time.Now()); err != nil {
			t.Fatalf("divergent frame %d: %v", i, err)
		}
	}

	if drained := totalConfidence(bm); drained >= seeded {
		t.Errorf("confidence = %d after divergence, want less than the seeded %d: "+
			"cells are not draining with no floor configured", drained, seeded)
	}
}

// totalConfidence sums TimesSeenCount across the grid.
func totalConfidence(bm *BackgroundManager) uint32 {
	bm.Grid.mu.RLock()
	defer bm.Grid.mu.RUnlock()
	var total uint32
	for i := range bm.Grid.Cells {
		total += bm.Grid.Cells[i].TimesSeenCount
	}
	return total
}

// TestProcessFrameRestoresRegionsFromStore covers the early-restore path. A
// scene that matches a previous run can adopt its regions and skip the rest of
// settling, which is the whole point of persisting them.
func TestProcessFrameRestoresRegionsFromStore(t *testing.T) {
	const (
		rings      = 2
		azBins     = 8
		sourcePath = "/captures/restore.pcapng"
	)

	store := newMockRegionStore()
	bm := NewBackgroundManagerDI(t.Name(), rings, azBins, BackgroundParams{
		SeedFromFirstObservation: true,
		BackgroundUpdateFraction: 0.5,
		SafetyMarginMetres:       20,
		NoiseRelativeFraction:    0.01,
		WarmupMinFrames:          100,     // long enough that restore beats it
		WarmupDurationNanos:      1 << 62, // and so does the duration
		PostSettleUpdateFraction: 0.5,
	}, store)
	bm.SetSourcePath(sourcePath)

	// Seed a region snapshot the restore will match on source path.
	snap := &RegionSnapshot{
		SensorID:    bm.Grid.SensorID,
		SourcePath:  sourcePath,
		RegionCount: 1,
		RegionsJSON: `[{"id":0,"cell_list":[0,1],"cell_count":2,"mean_variance":0.1,` +
			`"params":{"noise_relative_fraction":0.02,"neighbour_confirmation_count":3,"settle_update_fraction":0.05}}]`,
	}
	store.regionSnapshots["path:"+bm.Grid.SensorID+":"+sourcePath] = snap

	// Run past the restore threshold; the restore is attempted once.
	for i := 0; i < regionRestoreMinFrames+2; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, azBins, 10, int64(i)), time.Now()); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}

	bm.Grid.mu.RLock()
	defer bm.Grid.mu.RUnlock()
	if !bm.Grid.regionRestoreAttempted {
		t.Error("the region restore was never attempted despite a store and enough frames")
	}
}
