package l4perception

import "testing"

func subsampleFixture(n int) []WorldPoint {
	pts := make([]WorldPoint, n)
	for i := range pts {
		pts[i] = WorldPoint{X: float64(i) * 0.01, Y: float64(i) * 0.02, Z: float64(i%7) * 0.1}
	}
	return pts
}

// The seed was previously taken from the wall clock, so the same capture
// clustered differently on every replay. Roughly 5 % of frames on a busy street
// exceed MaxInputPoints, and the differences cascaded through association into
// track counts and course-error percentiles.
func TestUniformSubsampleIsDeterministic(t *testing.T) {
	pts := subsampleFixture(5000)
	a := uniformSubsample(pts, 800)
	b := uniformSubsample(pts, 800)

	if len(a) != 800 || len(b) != 800 {
		t.Fatalf("lengths %d and %d, want 800", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("subsample differs at %d: %v vs %v", i, a[i], b[i])
		}
	}
}

// Determinism must not come from returning the same subsample for everything:
// different frames still need different draws, which is what the clock was
// there for.
func TestUniformSubsampleVariesWithInput(t *testing.T) {
	a := uniformSubsample(subsampleFixture(5000), 800)

	moved := subsampleFixture(5000)
	moved[1234].X += 0.5
	b := uniformSubsample(moved, 800)

	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("moving one point left the subsample unchanged")
	}
}

func TestUniformSubsampleSeedTracksLength(t *testing.T) {
	if subsampleSeed(subsampleFixture(100)) == subsampleSeed(subsampleFixture(101)) {
		t.Fatal("different point counts produced the same seed")
	}
}

func TestUniformSubsampleSeedIsNonNegative(t *testing.T) {
	// A negative seed is legal for rand.NewSource but makes the value awkward
	// to log and compare, so the top bit is masked off.
	if got := subsampleSeed(subsampleFixture(3)); got < 0 {
		t.Fatalf("seed %d is negative", got)
	}
}

func TestUniformSubsamplePassesThroughWhenUnderCap(t *testing.T) {
	pts := subsampleFixture(10)
	got := uniformSubsample(pts, 20)
	if len(got) != 10 {
		t.Fatalf("length %d, want the input returned unchanged", len(got))
	}
}

// The caller's slice must survive: DBSCAN keeps using it for labels.
func TestUniformSubsampleDoesNotMutateInput(t *testing.T) {
	pts := subsampleFixture(2000)
	before := make([]WorldPoint, len(pts))
	copy(before, pts)

	uniformSubsample(pts, 300)

	for i := range pts {
		if pts[i] != before[i] {
			t.Fatalf("input mutated at index %d", i)
		}
	}
}
