package ctl

import "testing"

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  semverVersion
		ok    bool
	}{
		{"0.5.0", semverVersion{0, 5, 0, ""}, true},
		{"0.5.1-pre3", semverVersion{0, 5, 1, "pre3"}, true},
		{"v0.5.1-pre3", semverVersion{0, 5, 1, "pre3"}, true},
		{"1.0.0-rc.1", semverVersion{1, 0, 0, "rc.1"}, true},
		{"2.0.0-alpha.1.beta", semverVersion{2, 0, 0, "alpha.1.beta"}, true},
		{"dev", semverVersion{}, false},
		{"unknown", semverVersion{}, false},
		{"1.2", semverVersion{}, false},
		{"1.2.x", semverVersion{}, false},
	}

	for _, tt := range tests {
		got, ok := parseSemver(tt.input)
		if ok != tt.ok {
			t.Errorf("parseSemver(%q): ok=%v, want %v", tt.input, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("parseSemver(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestIsPrerelease(t *testing.T) {
	stable, _ := parseSemver("0.5.0")
	pre, _ := parseSemver("0.5.1-pre3")

	if stable.isPrerelease() {
		t.Error("0.5.0 should not be a prerelease")
	}
	if !pre.isPrerelease() {
		t.Error("0.5.1-pre3 should be a prerelease")
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// Basic ordering.
		{"0.5.0", "0.5.0", 0},
		{"0.5.1", "0.5.0", 1},
		{"0.5.0", "0.5.1", -1},
		{"0.6.0", "0.5.9", 1},
		{"1.0.0", "0.99.99", 1},

		// Stable vs prerelease of same version: stable wins.
		{"0.5.1", "0.5.1-pre3", 1},
		{"0.5.1-pre3", "0.5.1", -1},

		// Prerelease of higher version vs stable of lower.
		{"0.5.1-pre3", "0.5.0", 1},
		{"0.5.0", "0.5.1-pre3", -1},

		// Prerelease ordering.
		{"0.5.1-pre3", "0.5.1-pre3", 0},
		{"0.5.1-pre5", "0.5.1-pre3", 1},
		{"0.5.1-pre3", "0.5.1-pre5", -1},

		// Numeric prerelease identifiers.
		{"1.0.0-1", "1.0.0-2", -1},
		{"1.0.0-2", "1.0.0-1", 1},

		// Mixed identifiers: numeric < string per semver.
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0-alpha", "1.0.0-1", 1},

		// Multi-part prerelease.
		{"1.0.0-alpha.1", "1.0.0-alpha.2", -1},
		{"1.0.0-alpha.1", "1.0.0-beta.1", -1},

		// Fewer fields < more fields when prefixes match.
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
	}

	for _, tt := range tests {
		a, aOK := parseSemver(tt.a)
		b, bOK := parseSemver(tt.b)
		if !aOK || !bOK {
			t.Fatalf("failed to parse %q or %q", tt.a, tt.b)
		}
		got := compareSemver(a, b)
		if got != tt.want {
			t.Errorf("compareSemver(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
