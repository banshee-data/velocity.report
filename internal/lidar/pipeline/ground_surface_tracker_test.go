package pipeline

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// surfaceRecorder is a tracker that records every ground surface it is handed.
type surfaceRecorder struct {
	mockTrackerCov
	surfaces []l4perception.HeightSurface
}

func (r *surfaceRecorder) SetGroundSurface(s l4perception.HeightSurface) {
	r.surfaces = append(r.surfaces, s)
}

// The surface fitted for the tracker's box must not change the clip, and the
// clip's surface must not reach the tracker unless the box asked for it. The
// fixture's points are at z = 0 and the stand-in road a metre below them, so
// the surface clip keeps every point while a height band at 99 m removes them
// all: whether the tracker is updated says which clip ran.
func TestGroundSurfaceForTrackerLeavesTheClipAlone(t *testing.T) {
	road := l3grid.RegionalGroundSurface{CellMetres: 10, Global: l3grid.GroundSurface{C: -1}}
	fits := 0
	orig := fitGroundSurface
	fitGroundSurface = func(*l3grid.BackgroundManager, float64) (l3grid.RegionalGroundSurface, error) {
		fits++
		return road, nil
	}
	t.Cleanup(func() { fitGroundSurface = orig })

	run := func(useSurface, forTracker bool) (*surfaceRecorder, *l3grid.RegionalGroundSurface) {
		sensorID := fmt.Sprintf("ground-surface-tracker-%v-%v-%s", useSurface, forTracker, t.Name())
		rec := &surfaceRecorder{}
		var fit atomic.Pointer[l3grid.RegionalGroundSurface]
		cfg := &TrackingPipelineConfig{
			SensorID: sensorID, BackgroundManager: makeTestBgManager(t, sensorID), Tracker: rec,
			RemoveGround: true, HeightBandFloor: 99, HeightBandCeiling: 100,
			UseSurfaceGround: useSurface, GroundSurfaceForTracker: forTracker, GroundSurfaceFit: &fit,
		}
		cb := cfg.NewFrameCallback()
		now := time.Unix(1_700_000_000, 0)
		for i := 0; i < 5; i++ {
			cb(ended(makeStableFrame("seed", now.Add(time.Duration(i)*100*time.Millisecond), 20)))
		}
		for i := 0; i < 10; i++ {
			cb(ended(makeForegroundFrame("fg", now.Add(time.Duration(600+i*100)*time.Millisecond), 20, 5)))
		}
		return rec, fit.Load()
	}

	rec, fitted := run(false, true)
	if len(rec.surfaces) != 1 {
		t.Fatalf("the tracker was handed %d surfaces, want the one fitted", len(rec.surfaces))
	}
	if _, ok := rec.surfaces[0].(l3grid.BlendedSurface); !ok {
		t.Fatalf("the tracker was handed a %T, want the blended surface", rec.surfaces[0])
	}
	if fitted == nil {
		t.Fatal("the fit was not published for the caller")
	}
	if rec.updateCalls != 0 {
		t.Fatalf("the tracker was updated %d times: the surface fitted for the box changed the clip", rec.updateCalls)
	}
	if fits != 1 {
		t.Fatalf("the surface was fitted %d times, want once", fits)
	}

	rec, _ = run(true, false)
	if len(rec.surfaces) != 0 {
		t.Fatal("the clip's surface reached a tracker whose box did not ask for it")
	}
	if rec.updateCalls == 0 {
		t.Fatal("the surface clip did not run: every point went to the height band")
	}

	fits = 0
	if rec, fitted = run(false, false); fits != 0 || fitted != nil || len(rec.surfaces) != 0 {
		t.Fatalf("with neither option, %d fits and %d surfaces handed over, want none", fits, len(rec.surfaces))
	}
}

// A surface fitted for a report is published and read only: the height band
// still clips every point, and the tracker is handed nothing.
func TestGroundSurfaceForReportIsReadNeverApplied(t *testing.T) {
	road := l3grid.RegionalGroundSurface{CellMetres: 10, Global: l3grid.GroundSurface{C: -1}}
	fits := 0
	orig := fitGroundSurface
	fitGroundSurface = func(*l3grid.BackgroundManager, float64) (l3grid.RegionalGroundSurface, error) {
		fits++
		return road, nil
	}
	t.Cleanup(func() { fitGroundSurface = orig })

	sensorID := "ground-surface-report-" + t.Name()
	rec := &surfaceRecorder{}
	var fit atomic.Pointer[l3grid.RegionalGroundSurface]
	cfg := &TrackingPipelineConfig{
		SensorID: sensorID, BackgroundManager: makeTestBgManager(t, sensorID), Tracker: rec,
		RemoveGround: true, HeightBandFloor: 99, HeightBandCeiling: 100,
		GroundSurfaceForReport: true, GroundSurfaceFit: &fit,
	}
	cb := cfg.NewFrameCallback()
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 5; i++ {
		cb(ended(makeStableFrame("seed", now.Add(time.Duration(i)*100*time.Millisecond), 20)))
	}
	for i := 0; i < 10; i++ {
		cb(ended(makeForegroundFrame("fg", now.Add(time.Duration(600+i*100)*time.Millisecond), 20, 5)))
	}
	if fit.Load() == nil {
		t.Fatal("the report's fit was not published")
	}
	if fits != 1 {
		t.Fatalf("the surface was fitted %d times, want once", fits)
	}
	if len(rec.surfaces) != 0 {
		t.Fatal("the report's surface reached the tracker")
	}
	if rec.updateCalls != 0 {
		t.Fatalf("the tracker was updated %d times: the report's surface changed the clip", rec.updateCalls)
	}
}
