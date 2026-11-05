package pipeline

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// surfaceRecorder is a tracker that records every ground surface it is handed.
type surfaceRecorder struct {
	mockTrackerCov
	surfaces         []l4perception.HeightSurface
	detectionUpdates int
}

func (r *surfaceRecorder) SetGroundSurface(s l4perception.HeightSurface) {
	r.surfaces = append(r.surfaces, s)
}

func (r *surfaceRecorder) Update(clusters []l4perception.WorldCluster, ts time.Time) {
	r.mockTrackerCov.Update(clusters, ts)
	if len(clusters) > 0 {
		r.detectionUpdates++
	}
}

// The surface fitted for the tracker's box must not change the clip, and the
// clip's surface must not reach the tracker unless the box asked for it. The
// fixture's points are at z = 0 and the stand-in road a metre below them, so
// the surface clip keeps every point while a height band at 99 m removes them
// all: whether detections reach the tracker says which clip ran. Empty
// frames still advance its lifecycle.
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
	if rec.updateCalls != 15 || rec.detectionUpdates != 0 {
		t.Fatalf("the tracker was updated %d times: the surface fitted for the box changed the clip", rec.updateCalls)
	}
	if fits != 1 {
		t.Fatalf("the surface was fitted %d times, want once", fits)
	}

	rec, _ = run(true, false)
	if len(rec.surfaces) != 0 {
		t.Fatal("the clip's surface reached a tracker whose box did not ask for it")
	}
	if rec.detectionUpdates == 0 {
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
	if rec.updateCalls != 15 || rec.detectionUpdates != 0 {
		t.Fatalf("the tracker was updated %d times: the report's surface changed the clip", rec.updateCalls)
	}
}

func TestSurfaceGroundRuntimeRefitAndReset(t *testing.T) {
	old := fitGroundSurface
	calls := 0
	fail := false
	var cells []float64
	fitGroundSurface = func(_ *l3grid.BackgroundManager, cell float64) (l3grid.RegionalGroundSurface, error) {
		calls++
		cells = append(cells, cell)
		if fail {
			return l3grid.RegionalGroundSurface{}, fmt.Errorf("bad support")
		}
		return l3grid.RegionalGroundSurface{CellMetres: cell, Global: l3grid.GroundSurface{C: -1}}, nil
	}
	t.Cleanup(func() { fitGroundSurface = old })
	bm := makeTestBgManager(t, "surface-refresh")
	bm.Grid.SettlingComplete = true
	var settings atomic.Pointer[config.L4SurfaceGround]
	settings.Store(&config.L4SurfaceGround{Enabled: true, FloorMetres: 0, CeilingMetres: 2, CellMetres: 10})
	var published atomic.Pointer[l3grid.RegionalGroundSurface]
	cb := (&TrackingPipelineConfig{SensorID: "surface-refresh", BackgroundManager: bm, Tracker: &surfaceRecorder{}, RemoveGround: true, SurfaceGroundRuntime: &settings, GroundSurfaceFit: &published}).NewFrameCallback()
	now := time.Unix(1_700_000_000, 0)
	send := func(ms int) {
		cb(ended(makeStableFrame("surface-refresh", now.Add(time.Duration(ms)*time.Millisecond), 20)))
	}
	send(0)
	send(100)
	send(300)
	if calls != 1 || published.Load() == nil {
		t.Fatalf("initial/coalesced fits=%d", calls)
	}
	send(1200)
	if calls != 2 {
		t.Fatalf("changed background did not refresh: %d", calls)
	}
	settings.Store(&config.L4SurfaceGround{Enabled: false, FloorMetres: 0, CeilingMetres: 2, CellMetres: 10})
	send(1300)
	if published.Load() != nil {
		t.Fatal("disabled clip retained published fit")
	}
	settings.Store(&config.L4SurfaceGround{Enabled: true, FloorMetres: 0, CeilingMetres: 2, CellMetres: 5})
	send(1400)
	if calls != 3 || cells[len(cells)-1] != 5 {
		t.Fatal("runtime region change not applied")
	}
	if err := bm.ResetGrid(); err != nil {
		t.Fatal(err)
	}
	cb(ended(&l2frames.LiDARFrame{SensorID: "surface-refresh", StartTimestamp: now.Add(1500 * time.Millisecond)}))
	if published.Load() != nil {
		t.Fatal("reset retained old surface on an empty frame")
	}
	bm.Grid.SettlingComplete = true
	send(1600)
	if calls != 4 {
		t.Fatalf("reset/re-settle did not rebuild: %d", calls)
	}
	fail = true
	send(2800)
	if calls != 5 || published.Load() != nil {
		t.Fatal("failed refit retained stale surface")
	}
	fail = false
	send(2900)
	if calls != 5 {
		t.Fatal("failed-fit retry was not bounded")
	}
	send(3900)
	if calls != 6 || published.Load() == nil {
		t.Fatal("refit did not recover")
	}
	send(1700)
	if calls != 7 {
		t.Fatal("capture-time rewind did not refresh")
	}
	cb(ended(&l2frames.LiDARFrame{SensorID: "surface-refresh", StartTimestamp: now.Add(5 * time.Second)}))
	if calls != 7 {
		t.Fatal("unchanged background was refitted")
	}
}

func TestSurfaceGroundExplicitZeroFloorAndSnapshot(t *testing.T) {
	old := fitGroundSurface
	fitGroundSurface = func(*l3grid.BackgroundManager, float64) (l3grid.RegionalGroundSurface, error) {
		return l3grid.RegionalGroundSurface{Global: l3grid.GroundSurface{C: -.1}}, nil
	}
	t.Cleanup(func() { fitGroundSurface = old })
	bm := makeTestBgManager(t, "surface-zero")
	bm.Grid.SettlingComplete = true
	settings := &config.L4SurfaceGround{Enabled: true, FloorMetres: 0, CeilingMetres: .15, CellMetres: 10}
	rec := &surfaceRecorder{}
	cb := (&TrackingPipelineConfig{SensorID: "surface-zero", BackgroundManager: bm, Tracker: rec, RemoveGround: true, HeightBandFloor: 99, HeightBandCeiling: 100, SurfaceGround: settings}).NewFrameCallback()
	settings.FloorMetres = 50
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 5; i++ {
		cb(ended(makeStableFrame("surface-zero", now.Add(time.Duration(i)*100*time.Millisecond), 20)))
	}
	for i := 0; i < 10; i++ {
		cb(ended(makeForegroundFrame("surface-zero", now.Add(time.Duration(600+i*100)*time.Millisecond), 20, 5)))
	}
	if rec.detectionUpdates == 0 {
		t.Fatal("zero floor was replaced by the legacy .2 m default, or snapshot mutated")
	}
}
