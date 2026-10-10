package l5tracks

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

// The tuning file's solid-body block reaches the tracker as the estimator's
// options with the sensor-frame origin and the near-edge switches; a config
// without the block, or with it disabled, gives the tracker the block's
// absence has always meant.
func TestTrackerConfigFromTuningReadsTheSolidBodyBlock(t *testing.T) {
	l5 := *config.MustLoadDefaultConfig().L5.CvKfV1
	plain := TrackerConfigFromTuning(&l5)
	if plain.SolidBody.Enabled || plain.NearEdgeTracking || plain.NearEdgeMedoidGate || plain.SolidBody.OriginSource != "" {
		t.Fatalf("no block gave a solid body: %+v", plain.SolidBody)
	}
	l5.SolidBody = &config.L5SolidBody{
		Enabled: true, FullMembers: true, NearEdgeTracking: true, NearEdgeMedoidGate: true,
		FaceHysteresis: true, CourseAlignedFaces: true, RankOneMedoidScale: 0.25,
		Containment: true, RectangleHeading: true, RectangleCourseFusion: true, RectangleSigmaScale: 1.5,
	}
	got := TrackerConfigFromTuning(&l5)
	want := plain
	want.SolidBody = SolidBodyOptions{
		Enabled: true, OriginSource: OriginTrackingTransformIdentity,
		FaceHysteresis: true, CourseAlignedFaces: true, RankOneMedoidScale: 0.25,
		Containment: true, RectangleHeading: true, RectangleCourseFusion: true, RectangleSigmaScale: 1.5,
	}
	want.NearEdgeTracking, want.NearEdgeMedoidGate = true, true
	if got != want {
		t.Fatalf("the block did not reach the tracker:\n got %+v\nwant %+v", got, want)
	}
	// Disabled, the block's other fields are not read.
	l5.SolidBody.Enabled = false
	if got := TrackerConfigFromTuning(&l5); got != plain {
		t.Fatalf("a disabled block changed the tracker: %+v", got)
	}
}
