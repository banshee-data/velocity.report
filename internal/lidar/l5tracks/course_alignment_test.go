package l5tracks

import (
	"math"
	"testing"
)

func TestFoldAxisAngleDeg(t *testing.T) {
	const rad = math.Pi / 180
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"aligned", 0, 0},
		{"small positive", 10 * rad, 10},
		{"small negative", -10 * rad, 10},
		// An OBB is symmetric: pointing backwards is the same box.
		{"reversed is aligned", 180 * rad, 0},
		{"near reversed", 170 * rad, 10},
		// A length/width swap is the worst case.
		{"perpendicular", 90 * rad, 90},
		{"perpendicular negative", -90 * rad, 90},
		{"270 folds to perpendicular", 270 * rad, 90},
		{"beyond a full turn", 370 * rad, 10},
		{"two full turns", 730 * rad, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FoldAxisAngleDeg(c.in)
			if math.Abs(got-c.want) > 1e-6 {
				t.Fatalf("FoldAxisAngleDeg(%v rad) = %v, want %v", c.in/rad, got, c.want)
			}
			if got < 0 || got > 90 {
				t.Fatalf("result %v outside [0, 90]", got)
			}
		})
	}
}

// A track below the minimum speed has no meaningful course, so sampling it
// must not manufacture a perfect score.
func TestSampleCourseAlignmentIgnoresSlowTracks(t *testing.T) {
	tr := &TrackedObject{VX: 0.5, VY: 0, OBBHeadingRad: math.Pi / 2}
	tr.SampleCourseAlignment()
	if tr.CourseAlignmentCount != 0 {
		t.Fatalf("slow track recorded %d samples, want 0", tr.CourseAlignmentCount)
	}
	if _, ok := tr.CourseAlignmentPercentileDeg(50); ok {
		t.Fatal("percentile reported as valid with no samples")
	}
}

func TestSampleCourseAlignmentRecordsAlignedTrack(t *testing.T) {
	// Travelling along +X at 10 m/s with the box pointing the same way.
	tr := &TrackedObject{VX: 10, VY: 0, OBBHeadingRad: 0}
	for i := 0; i < 10; i++ {
		tr.SampleCourseAlignment()
	}
	if tr.CourseAlignmentCount != 10 {
		t.Fatalf("recorded %d samples, want 10", tr.CourseAlignmentCount)
	}
	p50, ok := tr.CourseAlignmentPercentileDeg(50)
	if !ok {
		t.Fatal("percentile not valid despite samples")
	}
	if p50 > CourseAlignmentBinWidthDeg {
		t.Fatalf("aligned track p50 = %v deg, want <= %v", p50, CourseAlignmentBinWidthDeg)
	}
}

// The failure the metric exists to catch: a box locked across the direction of
// travel. Heading jitter is zero here, which is exactly why jitter alone is not
// enough.
func TestSampleCourseAlignmentCatchesPerpendicularLock(t *testing.T) {
	tr := &TrackedObject{VX: 10, VY: 0, OBBHeadingRad: math.Pi / 2}
	for i := 0; i < 10; i++ {
		tr.SampleCourseAlignment()
	}
	p50, ok := tr.CourseAlignmentPercentileDeg(50)
	if !ok {
		t.Fatal("percentile not valid despite samples")
	}
	if p50 < 90-CourseAlignmentBinWidthDeg {
		t.Fatalf("perpendicular box p50 = %v deg, want >= %v", p50, 90-CourseAlignmentBinWidthDeg)
	}
	if tr.HeadingJitterCount != 0 {
		t.Fatal("test precondition: this track has no heading jitter samples")
	}
}

// A box pointing backwards along the course occupies the same space, so it is
// aligned. This is the folding that separates box orientation from arrow
// direction.
func TestSampleCourseAlignmentTreatsReversedBoxAsAligned(t *testing.T) {
	tr := &TrackedObject{VX: 10, VY: 0, OBBHeadingRad: math.Pi}
	tr.SampleCourseAlignment()
	p50, _ := tr.CourseAlignmentPercentileDeg(50)
	if p50 > CourseAlignmentBinWidthDeg {
		t.Fatalf("reversed box p50 = %v deg, want <= %v", p50, CourseAlignmentBinWidthDeg)
	}
}

func TestCourseAlignmentPercentileOrdering(t *testing.T) {
	// Nine aligned samples and one perpendicular: the median stays low while
	// the p90 picks up the outlier.
	tr := &TrackedObject{VX: 10, VY: 0}
	tr.OBBHeadingRad = 0
	for i := 0; i < 9; i++ {
		tr.SampleCourseAlignment()
	}
	tr.OBBHeadingRad = math.Pi / 2
	tr.SampleCourseAlignment()

	p50, _ := tr.CourseAlignmentPercentileDeg(50)
	p90, _ := tr.CourseAlignmentPercentileDeg(90)
	if p50 > CourseAlignmentBinWidthDeg {
		t.Fatalf("p50 = %v deg, want <= %v", p50, CourseAlignmentBinWidthDeg)
	}
	if p90 < p50 {
		t.Fatalf("p90 %v below p50 %v", p90, p50)
	}
}

// Every sample must land in a bin, including the exact 90 degree boundary,
// which would otherwise index one past the end of the histogram.
func TestSampleCourseAlignmentBinBounds(t *testing.T) {
	for deg := 0; deg <= 360; deg++ {
		tr := &TrackedObject{VX: 10, VY: 0}
		tr.OBBHeadingRad = float32(float64(deg) * math.Pi / 180)
		tr.SampleCourseAlignment()
		if tr.CourseAlignmentCount != 1 {
			t.Fatalf("heading %d deg: recorded %d samples, want 1", deg, tr.CourseAlignmentCount)
		}
		var total uint32
		for _, n := range tr.CourseAlignmentHist {
			total += n
		}
		if total != 1 {
			t.Fatalf("heading %d deg: histogram holds %d entries, want 1", deg, total)
		}
	}
}

func TestGetTrackingMetricsPoolsCourseAlignment(t *testing.T) {
	tr := NewTracker(DefaultTrackerConfig())

	aligned := &TrackedObject{TrackID: "aligned", TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}, VX: 10, VY: 0}
	for i := 0; i < 20; i++ {
		aligned.SampleCourseAlignment()
	}
	crossed := &TrackedObject{TrackID: "crossed", TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}, VX: 10, VY: 0, OBBHeadingRad: math.Pi / 2}
	for i := 0; i < 20; i++ {
		crossed.SampleCourseAlignment()
	}
	tr.Tracks["aligned"] = aligned
	tr.Tracks["crossed"] = crossed

	m := tr.GetTrackingMetrics()
	if m.CourseAlignmentSamples != 40 {
		t.Fatalf("pooled samples = %d, want 40", m.CourseAlignmentSamples)
	}
	// Half the samples are perpendicular, so the p90 must be at the top.
	if m.CourseAlignmentP90Deg < 90-CourseAlignmentBinWidthDeg {
		t.Fatalf("pooled p90 = %v deg, want >= %v", m.CourseAlignmentP90Deg, 90-CourseAlignmentBinWidthDeg)
	}
}

// A track with no velocity/displacement alignment samples still contributes
// course alignment. Without this the worst boxes would be silently dropped.
func TestCourseAlignmentSurvivesMissingVelocityAlignment(t *testing.T) {
	tr := NewTracker(DefaultTrackerConfig())
	bad := &TrackedObject{TrackID: "bad", TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}, VX: 10, VY: 0, OBBHeadingRad: math.Pi / 2}
	for i := 0; i < 5; i++ {
		bad.SampleCourseAlignment()
	}
	if bad.AlignmentSampleCount != 0 {
		t.Fatal("test precondition: track must have no velocity alignment samples")
	}
	tr.Tracks["bad"] = bad

	m := tr.GetTrackingMetrics()
	if m.CourseAlignmentSamples != 5 {
		t.Fatalf("pooled samples = %d, want 5: the early continue dropped the track", m.CourseAlignmentSamples)
	}
}

// Deleted tracks carry frozen state and are excluded from live metrics, so
// their stale course alignment must not be pooled.
func TestCourseAlignmentExcludesDeletedTracks(t *testing.T) {
	tr := NewTracker(DefaultTrackerConfig())
	dead := &TrackedObject{TrackID: "dead", TrackMeasurement: TrackMeasurement{TrackState: TrackDeleted}, VX: 10, VY: 0, OBBHeadingRad: math.Pi / 2}
	for i := 0; i < 5; i++ {
		dead.SampleCourseAlignment()
	}
	tr.Tracks["dead"] = dead

	m := tr.GetTrackingMetrics()
	if m.CourseAlignmentSamples != 0 {
		t.Fatalf("pooled samples = %d, want 0 for a deleted track", m.CourseAlignmentSamples)
	}
}
