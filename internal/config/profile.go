package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// EngineNone disables a layer. It is an ordinary value of the `engine`
// selector each layer already carries, because "what runs at this layer" is
// the question that selector exists to answer, and "nothing" is a legitimate
// answer to it.
//
// A disabled layer needs no parameter block: the codec requires only the
// selected engine's block, so a config that switches L4 off stops carrying
// nine clustering parameters that have no effect on it.
const EngineNone = "none"

// Profile is a *derived* label for how far up the layer stack a config runs.
// It is not stored anywhere: the engine selectors are the configuration, and
// this is read off them. Nothing can disagree with the config, because there
// is nothing else that could.
//
// The label exists because baseline filenames, CI gate lists and operators all
// need a short name for a depth — not because the depth needs a second home.
type Profile string

const (
	// ProfileL3Only runs the L3 background model, settling and region
	// identification, and stops. Foreground points are extracted but never
	// clustered. For background and settling tuning in isolation, sensor
	// health checks, and thermally constrained hardware.
	ProfileL3Only Profile = "l3-only"

	// ProfileDetect adds L4: world transform, ground removal, clustering. It
	// holds no Kalman state and persists no track, which is what
	// distinguishes it — the CPU saving over full is under 1%.
	ProfileDetect Profile = "detect"

	// ProfileFull runs the whole stack: L3 foreground, L4 clustering, L5
	// tracking, L6 classification. This is what ships.
	ProfileFull Profile = "full"

	// DefaultFrameBudgetMs is the per-frame wall-clock ceiling. Beyond it a
	// frame is counted as over budget — "alarm" or lag territory — because
	// the pipeline is no longer keeping up with a 10 Hz sensor with margin
	// for jitter. It is deliberately just under the 100 ms frame interval.
	DefaultFrameBudgetMs = 98.0
)

// orderedProfiles lists the profiles by increasing depth.
var orderedProfiles = []Profile{ProfileL3Only, ProfileDetect, ProfileFull}

// profileTopLayer maps each profile to the highest layer it runs. L6 has no
// engine selector of its own, so it always follows L5 — see the backlog entry
// on aligning the config schema with the layer model.
var profileTopLayer = map[Profile]int{
	ProfileL3Only: 3,
	ProfileDetect: 4,
	ProfileFull:   6,
}

// KnownProfiles returns the supported profile labels in depth order.
func KnownProfiles() []Profile {
	out := make([]Profile, len(orderedProfiles))
	copy(out, orderedProfiles)
	return out
}

// ParseProfile resolves a profile label, for CLI flags and CI gate lists.
func ParseProfile(name string) (Profile, error) {
	p := Profile(strings.TrimSpace(name))
	if _, ok := profileTopLayer[p]; !ok {
		return "", fmt.Errorf("must be one of %s, got %q", profileNames(), name)
	}
	return p, nil
}

// RunsLayer reports whether the profile runs the given layer number, so stage
// boundaries read as `if !profile.RunsLayer(4) { stop here }` rather than as a
// list of profile names that has to be revisited whenever one is added.
func (p Profile) RunsLayer(layer int) bool {
	return layer <= p.TopLayer()
}

// TopLayer returns the highest layer number the profile runs. An unrecognised
// label reports the full depth, matching the permissive direction taken
// elsewhere: a misread label must not silently switch the pipeline off.
func (p Profile) TopLayer() int {
	if depth, ok := profileTopLayer[p]; ok {
		return depth
	}
	return profileTopLayer[ProfileFull]
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

// Profile derives the depth label from the engine selectors. This is the only
// place that mapping lives, and it reads the same configuration the pipeline
// gates on.
func (c *TuningConfig) Profile() Profile {
	switch {
	case c.L4.Engine == EngineNone:
		return ProfileL3Only
	case c.L5.Engine == EngineNone:
		return ProfileDetect
	default:
		return ProfileFull
	}
}

// ApplyProfile switches layers off to reach the requested depth.
//
// It can only reduce depth. Raising it would need parameter blocks the config
// does not carry — a config with `l4.engine: "none"` holds no clustering
// parameters to turn back on — and inventing defaults there would produce a
// run whose tuning nobody chose. A caller wanting more depth should point at a
// config that has it.
func (c *TuningConfig) ApplyProfile(p Profile) error {
	if _, ok := profileTopLayer[p]; !ok {
		return fmt.Errorf("unknown profile %q, must be one of %s", p, profileNames())
	}
	if current := c.Profile(); p.TopLayer() > current.TopLayer() {
		return fmt.Errorf(
			"cannot raise the profile from %s to %s: this config carries no engine block for the layers %s would run",
			current, p, p)
	}

	if !p.RunsLayer(4) {
		c.L4.Engine = EngineNone
		c.L4.DbscanXyV1 = nil
		c.L4.TwoStageMahalanobisV2 = nil
		c.L4.HdbscanAdaptiveV1 = nil
	}
	if !p.RunsLayer(5) {
		c.L5.Engine = EngineNone
		c.L5.CvKfV1 = nil
		c.L5.ImmCvCaV2 = nil
		c.L5.ImmCvCaRtsEvalV2 = nil
	}
	return nil
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

// ProfileConfigPath is the on-disk location of the tuning config for a named
// profile. These live under config/profiles/ rather than beside
// tuning.defaults.json because they are derived rather than authored: each is
// the defaults with layers switched off, and a test asserts exactly that. The
// key-order tooling treats config/tuning*.json as full configs that must carry
// every key, which a config with disabled layers deliberately does not.
func ProfileConfigPath(p Profile) string {
	if p == ProfileFull {
		return DefaultConfigPath
	}
	return fmt.Sprintf("config/profiles/%s.json", p)
}
