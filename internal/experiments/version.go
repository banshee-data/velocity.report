package experiments

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// ErrSpecMajor means this binary cannot interpret the spec's execution contract.
var ErrSpecMajor = errors.New("unsupported experiment spec major version")
var specVersionPattern = regexp.MustCompile(`^([1-9][0-9]*)\.([0-9]+)$`)

// CheckSpecVersion accepts legacy unversioned specs and the 1.x family.
func CheckSpecVersion(v string) error {
	if v == "" {
		return nil
	}
	m := specVersionPattern.FindStringSubmatch(v)
	if m == nil {
		return fmt.Errorf("spec_version %q: want MAJOR.MINOR", v)
	}
	major, err := strconv.Atoi(m[1])
	if err != nil || major != 1 {
		return fmt.Errorf("%w: %s (supported 1.x)", ErrSpecMajor, v)
	}
	return nil
}

// ValidateSpec checks one spec without letting an unrelated stale experiment
// prevent an offline worker from making progress.
func (s *Set) ValidateSpec(root string, sp *Spec) []error {
	errs := sp.validate(root, s)
	return append(errs, sp.validateFingerprints(root, &fingerprints{root: root, cache: map[string]fpResult{}})...)
}
