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
		"promoted_learn_share": func(r *L3ForegroundRefine) { r.PromotedLearnShare = 1.5 },
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

// The block is off by being absent, so the defaults' fingerprint does not move
// with it, and the experiment set that carries it validates.
func TestForegroundRefineBlockIsOptional(t *testing.T) {
	if got := MustLoadDefaultConfig().L3.ActiveCommon().ForegroundRefine; got != nil {
		t.Fatalf("the defaults carry refinement on: %+v", got)
	}
	cfg, err := LoadTuningConfig("../../config/experiments/foreground-refine.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.L3.ActiveCommon().ForegroundRefine == nil {
		t.Fatal("the experiment set lost its block")
	}
}
