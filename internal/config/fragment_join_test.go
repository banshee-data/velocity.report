package config

import (
	"math"
	"testing"
)

func TestFragmentJoinRejectsUnboundedEvidence(t *testing.T) {
	valid := L4FragmentJoin{Enabled: true, MaxGapMetres: 2, MarginMetres: .15, MaxAgeSecs: .2, MinSpeedMps: 2, MaxPositionStddevMetres: .75}
	for _, tc := range []struct {
		name   string
		change func(*L4FragmentJoin)
		ok     bool
	}{
		{"valid", func(*L4FragmentJoin) {}, true},
		{"gap", func(j *L4FragmentJoin) { j.MaxGapMetres = 3 }, false},
		{"margin", func(j *L4FragmentJoin) { j.MarginMetres = .5 }, false},
		{"age", func(j *L4FragmentJoin) { j.MaxAgeSecs = 1 }, false},
		{"static", func(j *L4FragmentJoin) { j.MinSpeedMps = 0 }, false},
		{"uncertain", func(j *L4FragmentJoin) { j.MaxPositionStddevMetres = 2 }, false},
		{"nan", func(j *L4FragmentJoin) { j.MaxGapMetres = math.NaN() }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := valid
			tc.change(&j)
			cfg := MustLoadDefaultConfig()
			cfg.L4.DbscanXyV1.FragmentJoin = &j
			if err := cfg.Validate(); (err == nil) != tc.ok {
				t.Fatalf("valid=%v: %v", tc.ok, err)
			}
		})
	}
}

func TestFragmentJoinAccessorKeepsTuningImmutable(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	if cfg.GetFragmentJoin() != nil {
		t.Fatal("defaults enabled joining")
	}
	cfg.L4.DbscanXyV1.FragmentJoin = &L4FragmentJoin{MaxGapMetres: 2}
	copy := cfg.GetFragmentJoin()
	copy.MaxGapMetres = 1
	if cfg.L4.DbscanXyV1.FragmentJoin.MaxGapMetres != 2 {
		t.Fatal("callback mutated the tuning it was created from")
	}
}
