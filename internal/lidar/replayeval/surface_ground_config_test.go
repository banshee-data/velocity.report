package replayeval

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

func TestSurfaceGroundTuningAliasAndFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		file string
		cell float64
	}{{ExperimentSurfaceGround, "surface-ground-10m.json", 10}, {ExperimentSurfaceGroundGlobal, "surface-ground-global.json", 0}, {ExperimentSurfaceGround5m, "surface-ground-5m.json", 5}} {
		cfg := config.MustLoadDefaultConfig()
		got, err := surfaceGroundFor(Config{}, cfg, []string{tc.name})
		if err != nil {
			t.Fatal(err)
		}
		want := &config.L4SurfaceGround{Enabled: true, FloorMetres: .2, CeilingMetres: 4.5, CellMetres: tc.cell}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %+v", tc.name, got)
		}
		cfg, err = config.LoadTuningConfig(filepath.Join("..", "..", "..", "config", "experiments", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		file, err := surfaceGroundFor(Config{}, cfg, nil)
		if err != nil || !reflect.DeepEqual(file, want) {
			t.Fatal(file, err)
		}
		both, err := surfaceGroundFor(Config{}, cfg, []string{tc.name})
		if err != nil || !reflect.DeepEqual(both, want) {
			t.Fatal(both, err)
		}
	}
	base := config.MustLoadDefaultConfig()
	if got, err := surfaceGroundFor(Config{}, base, nil); err != nil || got != nil {
		t.Fatal("default altered", got, err)
	}
	flags, err := surfaceGroundFor(Config{UseSurfaceGround: true}, base, nil)
	if err != nil || flags.CellMetres != 0 || flags.FloorMetres != .2 || flags.CeilingMetres != 4.5 {
		t.Fatal(flags, err)
	}
	for _, aliases := range [][]string{{ExperimentSurfaceGround, ExperimentSurfaceGround5m}, {ExperimentSurfaceGround, ExperimentSurfaceGroundGlobal}} {
		if _, err := surfaceGroundFor(Config{}, base, aliases); err == nil {
			t.Fatal("conflicting aliases accepted")
		}
	}
	if _, err := surfaceGroundFor(Config{UseSurfaceGround: true}, base, []string{ExperimentSurfaceGround}); err == nil {
		t.Fatal("conflicting flags accepted")
	}
	base.L4.DbscanXyV1.SurfaceGround = &config.L4SurfaceGround{Enabled: true, FloorMetres: 0, CeilingMetres: 4, CellMetres: 5}
	if _, err := surfaceGroundFor(Config{}, base, []string{ExperimentSurfaceGround}); err == nil {
		t.Fatal("conflicting file accepted")
	}
	if got, err := surfaceGroundFor(Config{}, base, nil); err != nil || got.FloorMetres != 0 {
		t.Fatal("explicit zero floor lost", err)
	}
	if _, err := surfaceGroundFor(Config{UseSurfaceGround: true, SurfaceGroundFloor: math.NaN()}, config.MustLoadDefaultConfig(), nil); err == nil {
		t.Fatal("nonfinite flags accepted")
	}
}
