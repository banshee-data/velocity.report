package server

import (
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func TestSurfaceGroundRuntimePatchIsAtomic(t *testing.T) {
	ws := &Server{}
	ws.storeTuningConfig(config.MustLoadDefaultConfig())
	bm := l3grid.NewBackgroundManager("surface-patch", 16, 360, l3grid.DefaultBackgroundConfig().ToBackgroundParams(), nil)
	prefix := "l4.dbscan_xy_v1.surface_ground"
	if err := applyRuntimeTuningPatch(ws, bm, map[string]interface{}{prefix: map[string]interface{}{"enabled": true, "floor_metres": 0., "ceiling_metres": 4.5, "cell_metres": 10.}}); err != nil {
		t.Fatal(err)
	}
	old := *ws.SurfaceGroundFlag().Load()
	if old.FloorMetres != 0 || !old.Enabled || old.CellMetres != 10 {
		t.Fatalf("unexpected runtime block: %+v", old)
	}
	if err := applyRuntimeTuningPatch(ws, bm, map[string]interface{}{prefix + ".cell_metres": -1.}); err == nil {
		t.Fatal("invalid patch accepted")
	}
	if !reflect.DeepEqual(*ws.SurfaceGroundFlag().Load(), old) {
		t.Fatal("failed patch partially published")
	}
	if err := applyRuntimeTuningPatch(ws, bm, map[string]interface{}{prefix + ".enabled": false, prefix + ".cell_metres": 5.}); err != nil {
		t.Fatal(err)
	}
	if ws.SurfaceGroundFlag().Load().Enabled || ws.SurfaceGroundFlag().Load().CellMetres != 5 {
		t.Fatal("runtime disable/cell change not published")
	}
	if ws.snapshotTuningConfig().GetSurfaceGround().CellMetres != 5 {
		t.Fatal("stored config lost settings")
	}
	for _, path := range []string{prefix, prefix + ".enabled", prefix + ".floor_metres", prefix + ".ceiling_metres", prefix + ".cell_metres"} {
		if err := validateRuntimeTuningPath(path); err != nil {
			t.Fatal(path, err)
		}
	}
}
