package l5tracks

import (
	"runtime"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The extent profile (SolidBodyOptions.ExtentProfile) is a shadow of the
// shadow: it reads the solid body and the frame's returns and writes only its
// own grids and the reading's Profile block. These digests were taken on the
// code before the option existed, over every bit the tracked filter and the
// solid body publish (solidBodyFingerprint). Off they must not move, and on
// they must not move either: the profile changes no track and no box.
//
// They are pinned on amd64 only, for the reason the face-state digests are
// (solid_body_face_state_off_test.go).

func extentProfileFingerprintScenarios() []solidBodyFingerprintScenario {
	return []solidBodyFingerprintScenario{
		{
			name:   "profile_face_state_through_truck",
			pass:   throughTruckPass,
			config: faceStateConfig,
			golden: "6f11254aa08a17d9",
		},
		{
			name: "profile_accrual_end_on_truck",
			pass: endOnTruckPass,
			config: func() TrackerConfig {
				cfg := faceStateConfig()
				cfg.SolidBody.ExtentBoundAccrual = true
				return cfg
			},
			golden: "745ced0a8837db7f",
		},
		{
			name:   "profile_shadow_default_pass",
			pass:   l4perception.DefaultSyntheticPass,
			config: solidBodyConfig,
			golden: "69fecbfeb7da6a56",
		},
	}
}

func TestExtentProfileOffLeavesTheShippedArithmetic(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
	scenarios := append(extentProfileFingerprintScenarios(), solidBodyFingerprintScenario{
		name: "profile_course_heading_through_truck",
		pass: throughTruckPass,
		config: func() TrackerConfig {
			cfg := faceStateConfig()
			cfg.SolidBody.CourseHeading = true
			return cfg
		},
		golden: profileCourseHeadingGolden,
	})
	for _, s := range scenarios {
		got := solidBodyFingerprint(t, s.config(), syntheticPassFrames(t, s.pass()))
		if got != s.golden {
			t.Errorf("%s: digest %s, want %s", s.name, got, s.golden)
		}
	}
}

// profileCourseHeadingGolden is the through-truck pass under the face state
// with CourseHeading, the scenario in which the profile admits frames, taken
// on the code before the option existed (ab19e51).
const profileCourseHeadingGolden = "e1ea46a94211a359"
