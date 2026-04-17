package ctl

import (
	"strconv"
	"strings"
)

// semverVersion represents a parsed semantic version.
type semverVersion struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string // empty for stable releases
}

// parseSemver parses a version string like "0.5.1-pre3" into components.
// Returns ok=false for unparseable versions (e.g. "dev").
func parseSemver(v string) (semverVersion, bool) {
	v = strings.TrimPrefix(v, "v")

	var sv semverVersion

	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		sv.Prerelease = v[idx+1:]
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return sv, false
	}

	var err error
	sv.Major, err = strconv.Atoi(parts[0])
	if err != nil {
		return sv, false
	}
	sv.Minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return sv, false
	}
	sv.Patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return sv, false
	}

	return sv, true
}

// isPrerelease returns true if this version has a prerelease suffix.
func (v semverVersion) isPrerelease() bool {
	return v.Prerelease != ""
}

// compareSemver returns -1, 0, or 1 for a < b, a == b, a > b.
// Per semver: 1.0.0-alpha < 1.0.0.
func compareSemver(a, b semverVersion) int {
	if c := intCmp(a.Major, b.Major); c != 0 {
		return c
	}
	if c := intCmp(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := intCmp(a.Patch, b.Patch); c != 0 {
		return c
	}

	// Same major.minor.patch: stable > prerelease per semver.
	if a.Prerelease == "" && b.Prerelease == "" {
		return 0
	}
	if a.Prerelease == "" {
		return 1
	}
	if b.Prerelease == "" {
		return -1
	}

	return comparePrerelease(a.Prerelease, b.Prerelease)
}

// comparePrerelease compares prerelease identifiers per semver §11:
// split by ".", numeric identifiers compared as integers, string
// identifiers compared lexically, numeric < string, fewer fields < more.
func comparePrerelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	n := len(aParts)
	if len(bParts) < n {
		n = len(bParts)
	}

	for i := 0; i < n; i++ {
		aNum, aErr := strconv.Atoi(aParts[i])
		bNum, bErr := strconv.Atoi(bParts[i])

		switch {
		case aErr == nil && bErr == nil:
			if c := intCmp(aNum, bNum); c != 0 {
				return c
			}
		case aErr == nil:
			return -1 // numeric < string
		case bErr == nil:
			return 1 // string > numeric
		default:
			if aParts[i] < bParts[i] {
				return -1
			}
			if aParts[i] > bParts[i] {
				return 1
			}
		}
	}

	return intCmp(len(aParts), len(bParts))
}

func intCmp(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
