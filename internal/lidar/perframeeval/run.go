package perframeeval

import "fmt"

// Config is one comparison: a reference, the scoring choices, and two arms.
type Config struct {
	Reference ReferenceOptions
	Score     ScoreOptions
	A, B      ArmSpec
	// Physical, when set, also scores each arm against the pack's physical
	// references (PhysicalResult), beside the mask-position comparison.
	Physical *PhysicalOptions
}

// Run loads the reference and both arms, scores each, and compares them. It is
// the whole harness behind the command-line tool, so the tool is flag parsing
// and nothing else.
func Run(cfg Config) (*Comparison, error) {
	if err := cfg.Score.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.Reference.Policy.Validate(); err != nil {
		return nil, err
	}
	if cfg.A.Label == cfg.B.Label {
		return nil, fmt.Errorf("both arms are labelled %q", cfg.A.Label)
	}
	if cfg.A.Kind() != cfg.B.Kind() {
		return nil, fmt.Errorf("arm %s is %s and arm %s is %s: compare estimates with estimates, or runs with runs",
			cfg.A.Label, cfg.A.Kind(), cfg.B.Label, cfg.B.Kind())
	}

	// Refused before anything is scored: a physical comparison asked for on
	// a held-out split, or of analysis runs, is not made at all.
	var physical *PhysicalReference
	if cfg.Physical != nil {
		if cfg.A.Kind() == ArmAnalysisRun {
			return nil, fmt.Errorf("physical scoring needs estimate versions; arms %s and %s are analysis runs", cfg.A.Label, cfg.B.Label)
		}
		pr, err := LoadPhysicalReference(cfg.Reference, *cfg.Physical)
		if err != nil {
			return nil, err
		}
		physical = pr
	}

	ref, err := LoadReference(cfg.Reference)
	if err != nil {
		return nil, err
	}
	a, err := LoadArm(cfg.A)
	if err != nil {
		return nil, err
	}
	b, err := LoadArm(cfg.B)
	if err != nil {
		return nil, err
	}
	ra, err := ScoreArm(ref, a, cfg.Score)
	if err != nil {
		return nil, err
	}
	rb, err := ScoreArm(ref, b, cfg.Score)
	if err != nil {
		return nil, err
	}
	c, err := CompareArms(ra, rb)
	if err != nil {
		return nil, err
	}
	if physical != nil {
		arms := &PhysicalArms{Reference: physical.Identity}
		for _, arm := range []struct {
			spec ArmSpec
			into *PhysicalResult
		}{{cfg.A, &arms.A}, {cfg.B, &arms.B}} {
			loaded, err := LoadPhysicalArm(arm.spec)
			if err != nil {
				return nil, err
			}
			if *arm.into, err = ScorePhysical(physical, loaded); err != nil {
				return nil, err
			}
		}
		c.Physical = arms
		c.Caveats = append(c.Caveats, "The MOT numbers score a visible-mask position, not a body centre; "+
			"body-centre, yaw, dimension, bumper and gap errors are the physical section's, against physical references.")
	}
	return &c, nil
}
