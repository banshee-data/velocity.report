package config

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestTrackGuardValidate(t *testing.T) {
	var none *L3TrackGuard
	if err := none.Validate(); err != nil {
		t.Fatalf("a nil block is the guard off: %v", err)
	}
	ok := L3TrackGuard{HoldTracks: true, HoldMarginMetres: 0.3, HoldCooldown: "1s", HoldMinSpeedMps: 1,
		EdgeFollowCluster: true, EdgeMarginMetres: 0.2}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid block was refused: %v", err)
	}
	for name, tc := range map[string]struct {
		edit func(*L3TrackGuard)
		want string
	}{
		"negative hold margin": {func(g *L3TrackGuard) { g.HoldMarginMetres = -0.1 }, "hold_margin_metres"},
		"NaN edge margin":      {func(g *L3TrackGuard) { g.EdgeMarginMetres = math.NaN() }, "edge_margin_metres"},
		"huge edge margin":     {func(g *L3TrackGuard) { g.EdgeMarginMetres = 6 }, "edge_margin_metres"},
		"negative speed":       {func(g *L3TrackGuard) { g.HoldMinSpeedMps = -1 }, "hold_min_speed_mps"},
		"NaN speed":            {func(g *L3TrackGuard) { g.HoldMinSpeedMps = math.NaN() }, "hold_min_speed_mps"},
		"unparsable cool-down": {func(g *L3TrackGuard) { g.HoldCooldown = "soon" }, "hold_cooldown"},
		"empty cool-down":      {func(g *L3TrackGuard) { g.HoldCooldown = "" }, "hold_cooldown"},
		"negative cool-down":   {func(g *L3TrackGuard) { g.HoldCooldown = "-1s" }, "hold_cooldown"},
		"long cool-down":       {func(g *L3TrackGuard) { g.HoldCooldown = "2m" }, "hold_cooldown"},
	} {
		t.Run(name, func(t *testing.T) {
			g := ok
			tc.edit(&g)
			err := g.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error naming %s", err, tc.want)
			}
			cfg := MustLoadDefaultConfig()
			cfg.L3.EmaBaselineV1.TrackGuard = &g
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "track_guard") {
				t.Fatalf("the whole config validated with a bad guard: %v", err)
			}
		})
	}
}

func TestDefaultsCarryTheTrackGuardOn(t *testing.T) {
	g := MustLoadDefaultConfig().L3.ActiveCommon().TrackGuard
	if g == nil || !g.HoldTracks || !g.EdgeFollowCluster || g.HoldCooldown != "1s" ||
		g.HoldMarginMetres != 0.3 || g.EdgeMarginMetres != 0.2 || g.HoldMinSpeedMps != 1 {
		t.Fatalf("defaults' track guard %+v", g)
	}
}

// The block is optional: a config written before it loads with the guard
// off. Present, it is strict: every key, and no others.
func TestTrackGuardBlockIsOptionalButWhole(t *testing.T) {
	base := mustMarshalJSON(t, MustLoadDefaultConfig())
	edit := func(f func(guard map[string]interface{}, l3 map[string]interface{})) []byte {
		var raw map[string]interface{}
		if err := json.Unmarshal(base, &raw); err != nil {
			t.Fatal(err)
		}
		l3 := raw["l3"].(map[string]interface{})["ema_baseline_v1"].(map[string]interface{})
		f(l3["track_guard"].(map[string]interface{}), l3)
		return mustMarshalJSON(t, raw)
	}

	cfg, err := ParseTuningConfig(edit(func(_, l3 map[string]interface{}) { delete(l3, "track_guard") }))
	if err != nil {
		t.Fatalf("a config without the block was refused: %v", err)
	}
	if cfg.L3.ActiveCommon().TrackGuard != nil {
		t.Fatal("a config without the block must run the guard off")
	}
	if _, err := ParseTuningConfig(edit(func(g, _ map[string]interface{}) { delete(g, "hold_min_speed_mps") })); err == nil ||
		!strings.Contains(err.Error(), "hold_min_speed_mps") {
		t.Fatalf("a block missing a key was accepted: %v", err)
	}
	if _, err := ParseTuningConfig(edit(func(g, _ map[string]interface{}) { g["hold_everything"] = true })); err == nil ||
		!strings.Contains(err.Error(), "hold_everything") {
		t.Fatalf("a block with an unknown key was accepted: %v", err)
	}
}
