package config

import "testing"

func TestIdentityConfigValidatesQualifiers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value L5Identity
		valid bool
	}{
		{"empty", L5Identity{}, true},
		{"tentative only", L5Identity{TentativePriority: true}, true},
		{"negative", L5Identity{OverlapEndFrames: -1}, false},
		{"body without end", L5Identity{OverlapEndOnBody: true}, false},
		{"older without end", L5Identity{OverlapEndKeepOlder: true}, false},
		{"qualified", L5Identity{OverlapEndFrames: 1, OverlapEndOnBody: true, OverlapEndKeepOlder: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := MustLoadDefaultConfig()
			cfg.L5.CvKfV1.Identity = &tc.value
			if err := cfg.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
