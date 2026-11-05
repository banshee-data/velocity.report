package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAssociationClaimRequiresContainedFullMembers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  *L5SolidBody
		valid bool
	}{
		{"absent", nil, false},
		{"disabled", &L5SolidBody{}, false},
		{"no members", &L5SolidBody{Enabled: true, Containment: true}, false},
		{"no containment", &L5SolidBody{Enabled: true, FullMembers: true}, false},
		{"contained", &L5SolidBody{Enabled: true, FullMembers: true, Containment: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := MustLoadDefaultConfig()
			cfg.L5.CvKfV1.Association = &L5Association{ContainmentClaim: true}
			cfg.L5.CvKfV1.SolidBody = tc.body
			if err := cfg.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
	cfg := MustLoadDefaultConfig()
	cfg.L5.CvKfV1.Association = &L5Association{LikelihoodCost: true}
	if err := cfg.Validate(); err != nil {
		t.Fatal("likelihood alone does not require a body", err)
	}
	cfg.L5.CvKfV1.SolidBody = &L5SolidBody{Enabled: true, FullMembers: true, NearEdgeTracking: true}
	if err := cfg.Validate(); err == nil {
		t.Fatal("likelihood cost bypassed the near-edge noise-model refusal")
	}
	cfg.L5.CvKfV1.SolidBody.Enabled = false
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "solid_body:") {
		t.Fatalf("disabled body should report its own validation error: %v", err)
	}
}

func TestAssociationConfigRoundTripAndDefaultOmission(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	plain, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(plain), `"association":`) {
		t.Fatal("absent option changed the default encoding", err)
	}
	defaultFingerprint := cfg.Fingerprint()
	cfg.L5.CvKfV1.Association = &L5Association{LikelihoodCost: true, ContainmentClaim: true}
	cfg.L5.CvKfV1.SolidBody = &L5SolidBody{Enabled: true, FullMembers: true, Containment: true}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back TuningConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Fatal(err)
	}
	if back.Fingerprint() != cfg.Fingerprint() || back.Fingerprint() == defaultFingerprint || !back.L5.CvKfV1.Association.ContainmentClaim || !back.L5.CvKfV1.Association.LikelihoodCost {
		t.Fatal("association parameters lost in round trip or fingerprint")
	}
}

func TestAssociationCardinalityGuardOptionalAndRequiresClaim(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	cfg.L5.CvKfV1.Association = &L5Association{CardinalityGuard: &L5CardinalityGuard{Enabled: true}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("guard accepted without claim")
	}
	cfg.L5.CvKfV1.Association.CardinalityGuard.Enabled = false
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.L5.CvKfV1.Association.ContainmentClaim = true
	cfg.L5.CvKfV1.Association.CardinalityGuard.Enabled = true
	cfg.L5.CvKfV1.SolidBody = &L5SolidBody{Enabled: true, FullMembers: true, Containment: true}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var incomplete TuningConfig
	missing := strings.Replace(string(data), `"cardinality_guard":{"enabled":true}`, `"cardinality_guard":{}`, 1)
	if err := json.Unmarshal([]byte(missing), &incomplete); err == nil || !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("missing enabled key accepted: %v", err)
	}
	var back TuningConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Fatal(err)
	}
	if !back.L5.CvKfV1.Association.CardinalityGuard.Enabled || back.Fingerprint() != cfg.Fingerprint() {
		t.Fatal("guard lost in config encoding")
	}
}
