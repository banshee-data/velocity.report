package perframeeval

import "fmt"

// FirstBodyNs is the capture time of an arm's first solid-body row: where a
// replay's estimates start. A replay writes none before its first confirmed
// track, whatever warm-up it was given, so labels earlier than this read as
// objects nobody found. The arm is read as LoadPhysicalArm reads it, so the
// spec must name one version when the database holds several.
func FirstBodyNs(spec ArmSpec) (int64, error) {
	spec.SolidBodies = true
	arm, err := LoadPhysicalArm(spec)
	if err != nil {
		return 0, err
	}
	if len(arm.Bodies) == 0 {
		return 0, fmt.Errorf("arm %s holds no solid-body row", spec.Label)
	}
	return arm.Bodies[0].TimestampNs, nil
}
