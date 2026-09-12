package analysis

import (
	"math"
	"testing"
)

func TestFoldAxisAngleDegAnalysis(t *testing.T) {
	const rad = math.Pi / 180
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"aligned", 0, 0},
		{"ten degrees", 10 * rad, 10},
		{"negative ten", -10 * rad, 10},
		{"reversed box is aligned", 180 * rad, 0},
		{"perpendicular is worst", 90 * rad, 90},
		{"beyond a full turn", 370 * rad, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := foldAxisAngleDeg(c.in)
			if math.Abs(got-c.want) > 1e-6 {
				t.Fatalf("foldAxisAngleDeg(%v deg) = %v, want %v", c.in/rad, got, c.want)
			}
		})
	}
}

// The offline fold must agree with the tracker's exported one, or a run will
// report different numbers live and on replay.
func TestOfflineFoldMatchesTrackerFold(t *testing.T) {
	for deg := -720; deg <= 720; deg += 7 {
		r := float64(deg) * math.Pi / 180
		if got, want := foldAxisAngleDeg(r), trackerFoldReference(r); math.Abs(got-want) > 1e-9 {
			t.Fatalf("%d deg: offline %v, tracker %v", deg, got, want)
		}
	}
}

// trackerFoldReference mirrors l5tracks.FoldAxisAngleDeg. It is duplicated
// rather than imported to keep analysis free of a dependency on l5tracks.
func trackerFoldReference(diffRad float64) float64 {
	d := math.Mod(math.Abs(diffRad)*180/math.Pi, 360)
	if d > 180 {
		d = 360 - d
	}
	if d > 90 {
		d = 180 - d
	}
	return d
}

func TestCourseAlignmentMetricsSkipsSlowAndDeletedFrames(t *testing.T) {
	const perpendicular = float32(math.Pi / 2)

	// Six frames: two too slow, two deleted, two live and fast. Only the last
	// pair may be counted, and both are perpendicular.
	obb := []float32{0, 0, perpendicular, perpendicular, perpendicular, perpendicular}
	course := []float32{0, 0, 0, 0, 0, 0}
	speeds := []float32{0.1, 1.0, 10, 10, 10, 10}
	live := []bool{true, true, false, false, true, true}

	p50, p90, n := courseAlignmentMetrics(obb, course, speeds, live, CourseAlignmentMinSpeedMps)
	if n != 2 {
		t.Fatalf("counted %d samples, want 2", n)
	}
	if math.Abs(float64(p50)-90) > 1e-4 || math.Abs(float64(p90)-90) > 1e-4 {
		t.Fatalf("p50=%v p90=%v, want both 90", p50, p90)
	}
}

func TestCourseAlignmentMetricsNoQualifyingFrames(t *testing.T) {
	obb := []float32{1, 2, 3}
	course := []float32{0, 0, 0}
	speeds := []float32{0.1, 0.2, 0.3}
	live := []bool{true, true, true}

	p50, p90, n := courseAlignmentMetrics(obb, course, speeds, live, CourseAlignmentMinSpeedMps)
	if n != 0 || p50 != 0 || p90 != 0 {
		t.Fatalf("got p50=%v p90=%v n=%d, want all zero", p50, p90, n)
	}
}

func TestCourseAlignmentMetricsRejectsRaggedInput(t *testing.T) {
	_, _, n := courseAlignmentMetrics(
		[]float32{0, 0},
		[]float32{0},
		[]float32{10, 10},
		[]bool{true, true},
		CourseAlignmentMinSpeedMps,
	)
	if n != 0 {
		t.Fatalf("ragged input produced %d samples, want 0", n)
	}
}

func TestCourseAlignmentMetricsPercentileSpread(t *testing.T) {
	// Nine aligned frames, one perpendicular.
	obb := make([]float32, 10)
	course := make([]float32, 10)
	speeds := make([]float32, 10)
	live := make([]bool, 10)
	for i := range obb {
		speeds[i] = 10
		live[i] = true
	}
	obb[9] = float32(math.Pi / 2)

	p50, p90, n := courseAlignmentMetrics(obb, course, speeds, live, CourseAlignmentMinSpeedMps)
	if n != 10 {
		t.Fatalf("counted %d samples, want 10", n)
	}
	if p50 > 1 {
		t.Fatalf("p50 = %v, want ~0", p50)
	}
	if p90 < p50 {
		t.Fatalf("p90 %v below p50 %v", p90, p50)
	}
}

func TestPercentileSorted(t *testing.T) {
	one := []float64{42}
	if got := percentileSorted(one, 50); got != 42 {
		t.Fatalf("single element p50 = %v, want 42", got)
	}
	ten := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if got := percentileSorted(ten, 0); got != 0 {
		t.Fatalf("p0 = %v, want 0", got)
	}
	if got := percentileSorted(ten, 100); got != 9 {
		t.Fatalf("p100 = %v, want 9", got)
	}
	if got := percentileSorted(ten, 50); got < 4 || got > 5 {
		t.Fatalf("p50 = %v, want 4 or 5", got)
	}
}
