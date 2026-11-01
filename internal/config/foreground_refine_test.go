package config

import (
	"strings"
	"testing"
)

func refineBlock() *L3ForegroundRefine {
	return &L3ForegroundRefine{
		FarRelearnReturns: 10, FarAgreeMetres: 0.3, FarAgreePerMetre: 0.01, FarRelearnSeen: 50,
		WeakMarginMetres: 0.5, WeakSpreads: 2, WeakMarginPerMetre: 0.005,
		GrowRings: 2, GrowBins: 3, GrowTauMetres: 0.4, GrowTauPerMetre: 0.01,
		EnvelopeAttack: 0.3, EnvelopeRelease: 0.005,
		NovelMarginMetres: 0.3, NovelMarginPerMetre: 0.005, NovelMinSeen: 20, NovelMinReturns: 10,
	}
}

func TestForegroundRefineValidate(t *testing.T) {
	var none *L3ForegroundRefine
	if err := none.Validate(); err != nil {
		t.Fatalf("a nil block is off and valid: %v", err)
	}
	if err := refineBlock().Validate(); err != nil {
		t.Fatalf("a good block: %v", err)
	}
	for name, change := range map[string]func(*L3ForegroundRefine){
		"far_relearn_returns":  func(r *L3ForegroundRefine) { r.FarRelearnReturns = 0 },
		"far_relearn_returns ": func(r *L3ForegroundRefine) { r.FarRelearnReturns = 256 },
		"far_relearn_seen":     func(r *L3ForegroundRefine) { r.FarRelearnSeen = 0 },
		"grow_rings":           func(r *L3ForegroundRefine) { r.GrowRings = -1 },
		"novel_min_returns":    func(r *L3ForegroundRefine) { r.NovelMinReturns = 0 },
		"weak_margin_metres":   func(r *L3ForegroundRefine) { r.WeakMarginMetres = -1 },
		"envelope_attack":      func(r *L3ForegroundRefine) { r.EnvelopeAttack = 2 },
		"grow_tau_per_metre":   func(r *L3ForegroundRefine) { r.GrowTauPerMetre = -0.1 },
	} {
		block := refineBlock()
		change(block)
		err := block.Validate()
		if err == nil || !strings.Contains(err.Error(), strings.TrimSpace(name)) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The defaults carry refinement; the block is off by being absent, so a config
// written before it keeps its fingerprint and its behaviour.
func TestForegroundRefineBlockIsOptional(t *testing.T) {
	if MustLoadDefaultConfig().L3.ActiveCommon().ForegroundRefine == nil {
		t.Fatal("the defaults lost their refinement")
	}
	cfg, err := LoadTuningConfig("../../config/experiments/foreground-refine-off.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.L3.ActiveCommon().ForegroundRefine; got != nil {
		t.Fatalf("the off set carries refinement: %+v", got)
	}
}

func TestClusterPositionIsOptionalAndChecked(t *testing.T) {
	if got := MustLoadDefaultConfig().GetClusterCentre(); got != "mean" {
		t.Fatalf("the defaults report %q", got)
	}
	cfg, err := LoadTuningConfig("../../config/experiments/foreground-refine.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetClusterCentre(); got != "" {
		t.Fatalf("a config without the block names a centre: %q", got)
	}
	common := cfg.L4.ActiveCommon()
	common.ClusterPosition = &L4ClusterPosition{Centre: "mean"}
	if err := common.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetClusterCentre(); got != "mean" {
		t.Fatalf("centre %q", got)
	}
	common.ClusterPosition = &L4ClusterPosition{Centre: "centroid"}
	if err := common.Validate(); err == nil || !strings.Contains(err.Error(), "cluster_position") {
		t.Fatalf("an unknown centre: %v", err)
	}
	common.ClusterPosition = nil
}
