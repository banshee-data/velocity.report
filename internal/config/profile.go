package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Profile names how far up the layer stack the pipeline runs. It is a closed
// set rather than a cross-product of per-layer switches: independent depth
// toggles would be eight combinations, most of them meaningless, and every one
// of them a support surface. The per-layer `engine` selectors stay orthogonal
// to this — a profile says which layers run, an engine says which algorithm
// runs at a layer.
type Profile string

const (
	// ProfileL3Only runs the L3 background model, settling and region
	// identification, and stops. Foreground points are extracted but never
	// clustered. For background and settling tuning in isolation, sensor
	// health checks, and thermally constrained hardware.
	ProfileL3Only Profile = "l3-only"

	// ProfileDetect adds the L4 world transform, ground removal and
	// clustering. It holds no Kalman state and performs no track
	// persistence, which is what distinguishes it from `track` — the CPU
	// saving over `full` is under 1% and is not the reason it exists.
	ProfileDetect Profile = "detect"

	// ProfileTrack adds L5 Kalman tracking but not L6 classification. It is
	// a diagnostic profile for isolating classifier cost and behaviour; it
	// holds the same tracker state as `full`.
	ProfileTrack Profile = "track"

	// ProfileFull runs the whole stack: L3 foreground, L4 clustering, L5
	// tracking, L6 classification. This is what ships and it is the default
	// when no profile is configured.
	ProfileFull Profile = "full"

	// DefaultProfile applies when `pipeline.profile` is absent, so every
	// config written before profiles existed keeps its current behaviour.
	DefaultProfile = ProfileFull

	// DefaultFrameBudgetMs is the per-frame wall-clock ceiling. Beyond it a
	// frame is counted as over budget — "alarm" or lag territory — because
	// the pipeline is no longer keeping up with a 10 Hz sensor with margin
	// for jitter. It is deliberately just under the 100 ms frame interval.
	DefaultFrameBudgetMs = 98.0
)

// orderedProfiles lists the profiles by increasing depth. Order matters: it is
// what makes RunsLayer a comparison rather than a switch.
var orderedProfiles = []Profile{ProfileL3Only, ProfileDetect, ProfileTrack, ProfileFull}

// profileDepth maps each profile to the highest layer it runs.
var profileDepth = map[Profile]int{
	ProfileL3Only: 3,
	ProfileDetect: 4,
	ProfileTrack:  5,
	ProfileFull:   6,
}

// KnownProfiles returns the supported profile names in depth order.
func KnownProfiles() []Profile {
	out := make([]Profile, len(orderedProfiles))
	copy(out, orderedProfiles)
	return out
}

// ParseProfile resolves a configured profile name, treating empty as the
// default so pre-profile configs keep working.
func ParseProfile(name string) (Profile, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return DefaultProfile, nil
	}
	p := Profile(trimmed)
	if _, ok := profileDepth[p]; !ok {
		return "", fmt.Errorf("must be one of %s, got %q", profileNames(), trimmed)
	}
	return p, nil
}

// RunsLayer reports whether the profile runs the given layer number, so stage
// boundaries read as `if !profile.RunsLayer(4) { stop here }` rather than as a
// list of profile names that has to be updated whenever one is added.
func (p Profile) RunsLayer(layer int) bool {
	depth, ok := profileDepth[p]
	if !ok {
		depth = profileDepth[DefaultProfile]
	}
	return layer <= depth
}

// TopLayer returns the highest layer number the profile runs, for logging and
// diagnostics. An unknown profile reports the default's depth, matching
// RunsLayer.
func (p Profile) TopLayer() int {
	if depth, ok := profileDepth[p]; ok {
		return depth
	}
	return profileDepth[DefaultProfile]
}

// String satisfies fmt.Stringer.
func (p Profile) String() string { return string(p) }

func profileNames() string {
	names := make([]string, 0, len(orderedProfiles))
	for _, p := range orderedProfiles {
		names = append(names, string(p))
	}
	return strings.Join(names, ", ")
}

// GetProfile returns the resolved pipeline profile. An unparseable value
// cannot reach here: Validate rejects it at load.
func (c *TuningConfig) GetProfile() Profile {
	p, err := ParseProfile(c.Pipeline.Profile)
	if err != nil {
		return DefaultProfile
	}
	return p
}

// GetFrameBudgetMs returns the per-frame wall-clock ceiling in milliseconds,
// falling back to the default when unset.
func (c *TuningConfig) GetFrameBudgetMs() float64 {
	if c.Pipeline.FrameBudgetMs <= 0 {
		return DefaultFrameBudgetMs
	}
	return c.Pipeline.FrameBudgetMs
}

// Fingerprint returns a stable short hash of the whole resolved tuning config.
// Two runs with the same fingerprint measured the same configured workload;
// two runs with different fingerprints are not comparable, whatever their
// numbers say. Marshalling is canonical because Go emits struct fields in
// declaration order, and map ordering is not reachable from these types.
func (c *TuningConfig) Fingerprint() string {
	data, err := json.Marshal(c)
	if err != nil {
		// Marshalling a struct of scalars cannot fail; a sentinel keeps this
		// total rather than propagating an impossible error to every caller.
		return "unfingerprintable"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

// ProfileConfigPath is the conventional on-disk location of the tuning config
// for a named profile. The profile configs are byte-identical to
// tuning.defaults.json apart from `pipeline.profile`, which a test enforces.
func ProfileConfigPath(p Profile) string {
	if p == DefaultProfile {
		return DefaultConfigPath
	}
	return fmt.Sprintf("config/tuning.profile-%s.json", p)
}
