package perframeeval

import "fmt"

// Config is one comparison: a reference, the scoring choices, and two arms.
type Config struct {
	Reference ReferenceOptions
	Score     ScoreOptions
	A, B      ArmSpec
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
	return &c, nil
}
