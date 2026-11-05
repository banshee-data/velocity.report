package config

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestSurfaceGroundValidationAndImmutableSnapshot(t *testing.T) {
	base := MustLoadDefaultConfig()
	fingerprint := base.Fingerprint()
	raw, _ := json.Marshal(base)
	if strings.Contains(string(raw), `"surface_ground"`) {
		t.Fatal("default fingerprint surface changed")
	}
	for _, value := range []L4SurfaceGround{{true, 0, 4.5, 0}, {true, .2, 4.5, 10}, {false, .2, 4.5, 5}} {
		cfg := MustLoadDefaultConfig()
		cfg.L4.DbscanXyV1.SurfaceGround = &value
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
		copy := cfg.GetSurfaceGround()
		copy.CellMetres = 42
		if cfg.GetSurfaceGround().CellMetres == 42 {
			t.Fatal("snapshot aliases the tuning block")
		}
		if cfg.Fingerprint() == fingerprint {
			t.Fatal("enabled settings missing from fingerprint")
		}
	}
	for _, value := range []L4SurfaceGround{{true, -1, 4, 10}, {true, 1, 1, 10}, {true, 0, math.Inf(1), 10}, {true, math.NaN(), 4, 10}, {true, .2, 4, -1}, {true, .2, 4, math.NaN()}, {true, .2, 4, math.Inf(1)}} {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid bounds accepted: %+v", value)
		}
	}
	if MustLoadDefaultConfig().Fingerprint() != fingerprint || base.GetSurfaceGround() != nil {
		t.Fatal("default changed")
	}
}
