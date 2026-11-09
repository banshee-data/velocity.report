package perframeeval

import (
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

const bsFrame = int64(100_000_000)

// bsReference is two objects, a on y=0 and b on y=20, moving along +x at a
// metre a frame for ten frames.
func bsReference() *Reference {
	ep := EpisodeReference{EpisodeID: "ep", spans: []timeSpan{{0, 9 * bsFrame}}}
	for i := 0; i < 10; i++ {
		ep.Frames = append(ep.Frames, int64(i)*bsFrame)
	}
	for _, o := range []struct {
		id string
		y  float32
	}{{"a", 0}, {"b", 20}} {
		ep.Series = append(ep.Series, bsTrack(o.id, o.y, 0, 9))
	}
	// An object every point of which is ignored, as the parked car is: never
	// drawn.
	parked := bsTrack("parked", 40, 0, 9)
	for i := range parked.Points {
		parked.Points[i].Ignore = true
	}
	ep.Series = append(ep.Series, parked)
	return &Reference{Episodes: []EpisodeReference{ep}}
}

func bsTrack(id string, y float32, from, to int) l8analytics.TrackSeries {
	s := l8analytics.TrackSeries{ID: id}
	for i := from; i <= to; i++ {
		s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: int64(i) * bsFrame, X: float32(i), Y: y})
	}
	return s
}

func bsScore() ScoreOptions {
	o := DefaultScoreOptions()
	o.Gate = l8analytics.FixedGate(1)
	return o
}

// perfect follows both objects with one track each.
func perfect() Hypothesis {
	return Hypothesis{Identity: ArmIdentity{Label: "perfect"}, Series: []l8analytics.TrackSeries{
		bsTrack("pa", 0, 0, 9), bsTrack("pb", 20, 0, 9),
	}}
}

// switching follows a with one track and b with two, one switch on b, and
// carries a stray track on nobody's returns.
func switching() Hypothesis {
	return Hypothesis{Identity: ArmIdentity{Label: "switching"}, Series: []l8analytics.TrackSeries{
		bsTrack("sa", 0, 0, 9), bsTrack("sb1", 20, 0, 4), bsTrack("sb2", 20, 5, 9), bsTrack("stray", 100, 0, 9),
	}}
}

func TestBootstrapIntervalsByHand(t *testing.T) {
	res, err := Bootstrap(bsReference(), [2]Hypothesis{perfect(), switching()}, bsScore(), BootstrapOptions{Draws: 400, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Objects, []string{"a", "b"}) {
		t.Fatalf("objects %v", res.Objects)
	}
	// The perfect arm scores 1 and no switch on every draw.
	for name, p := range map[string]Percentiles{"hota": res.A.Spread.HOTA, "idf1": res.A.Spread.IDF1} {
		if p.P5 != 1 || p.P95 != 1 {
			t.Errorf("perfect arm %s %+v, want 1 throughout", name, p)
		}
	}
	if res.A.Spread.IDSwitches.P95 != 0 {
		t.Errorf("perfect arm switches %+v", res.A.Spread.IDSwitches)
	}
	// The switching arm: a draw of two objects holds b zero, one or two
	// times, with probability 1/4, 1/2, 1/4, and has that many switches.
	if res.B.Point.IDSwitches != 1 {
		t.Errorf("switching arm point switches %d, want 1", res.B.Point.IDSwitches)
	}
	if s := res.B.Spread.IDSwitches; s.P5 != 0 || s.P50 != 1 || s.P95 != 2 {
		t.Errorf("switching arm switches %+v, want 0, 1, 2", s)
	}
	// The stray track is attributed to nobody and is no false positive.
	if res.B.TracksUnattributed != 1 || res.B.TracksAttributed != 3 || res.B.Point.FP != 0 {
		t.Errorf("attribution: attributed %d unattributed %d fp %d", res.B.TracksAttributed, res.B.TracksUnattributed, res.B.Point.FP)
	}
	// The switching arm is never better than the perfect one, so the paired
	// IDF1 delta is never positive.
	if res.Delta.IDF1.P95 > 0 {
		t.Errorf("idf1 delta %+v: switching above perfect on some draw", res.Delta.IDF1)
	}
}

func TestBootstrapAgainstItselfIsZero(t *testing.T) {
	h := switching()
	other := switching()
	other.Identity.Label = "again"
	res, err := Bootstrap(bsReference(), [2]Hypothesis{h, other}, bsScore(), BootstrapOptions{Draws: 200, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	zero := Percentiles{}
	for name, p := range map[string]Percentiles{
		"hota": res.Delta.HOTA, "idf1": res.Delta.IDF1, "switches": res.Delta.IDSwitches, "fragmentations": res.Delta.Fragmentations,
	} {
		if p != zero {
			t.Errorf("delta of an arm against itself, %s: %+v", name, p)
		}
	}
	for name, stands := range res.DeltaStands {
		if stands {
			t.Errorf("a zero delta %s is reported as standing", name)
		}
	}
}

func TestBootstrapIsDeterministicForASeed(t *testing.T) {
	opts := BootstrapOptions{Draws: 100, Seed: 11}
	a, err := Bootstrap(bsReference(), [2]Hypothesis{perfect(), switching()}, bsScore(), opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Bootstrap(bsReference(), [2]Hypothesis{perfect(), switching()}, bsScore(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two runs with one seed differ")
	}
	if _, err := Bootstrap(bsReference(), [2]Hypothesis{perfect(), switching()}, bsScore(), BootstrapOptions{}); err == nil {
		t.Fatal("zero draws were accepted")
	}
}

// shortened follows a whole and b only on frames 0 to 6, so b loses three
// matched frames to false negatives.
func shortened() Hypothesis {
	return Hypothesis{Identity: ArmIdentity{Label: "shortened"}, Series: []l8analytics.TrackSeries{
		bsTrack("ta", 0, 0, 9), bsTrack("tb", 20, 0, 6),
	}}
}

// The false negatives' paired spread, and each object's own point, show a
// loss on one object that the pooled figures carry at half weight.
func TestBootstrapReportsFalseNegativesAndEachObject(t *testing.T) {
	res, err := Bootstrap(bsReference(), [2]Hypothesis{perfect(), shortened()}, bsScore(), BootstrapOptions{Draws: 400, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.B.Point.FN != 3 || res.A.Point.FN != 0 {
		t.Fatalf("point false negatives: a %d b %d, want 0 and 3", res.A.Point.FN, res.B.Point.FN)
	}
	// A draw holds b zero, one or two times: 0, 3 or 6 more missed points.
	if d := res.Delta.FN; d.P5 != 0 || d.P50 != 3 || d.P95 != 6 {
		t.Errorf("false-negative delta %+v, want 0, 3, 6", d)
	}
	if res.DeltaStands["fn"] {
		t.Error("a delta whose 5th percentile is zero is reported as standing")
	}
	want := map[string][2]int{"a": {10, 10}, "b": {10, 7}}
	for i, arm := range []BootstrapArm{res.A, res.B} {
		if len(arm.ByObject) != len(res.Objects) {
			t.Fatalf("arm %s: %d objects scored alone, want %d", arm.Label, len(arm.ByObject), len(res.Objects))
		}
		for j, o := range arm.ByObject {
			if o.Object != res.Objects[j] {
				t.Errorf("arm %s object %d is %s, want %s", arm.Label, j, o.Object, res.Objects[j])
			}
			if o.Point.Matches != want[o.Object][i] {
				t.Errorf("arm %s object %s: %d matched frames, want %d", arm.Label, o.Object, o.Point.Matches, want[o.Object][i])
			}
		}
	}
}
