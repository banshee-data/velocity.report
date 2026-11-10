package pipeline

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// joinRecordFixture is two 6-point components along x, 0 to 0.2 m and 1.4 to
// 1.6 m, joined under one footprint that holds both.
func joinRecordFixture(t *testing.T) ([]l4perception.WorldCluster, l4perception.DBSCANTrace, []l4perception.WorldCluster, l4perception.DBSCANTrace, []l4perception.FragmentHint) {
	t.Helper()
	p := l4perception.DefaultDBSCANParams()
	p.Eps, p.MinPts, p.KeepMembers = .25, 2, true
	var pts []l4perception.WorldPoint
	for _, start := range []float64{0, 1.4} {
		for i := 0; i < 3; i++ {
			for j := 0; j < 2; j++ {
				pt := l4perception.WorldPoint{X: start + float64(i)*.1, Y: float64(j) * .1, Z: .8, SensorID: "join", Timestamp: time.Unix(1_700_000_000, 0)}
				pt.SetSourceOrdinal(len(pts))
				pts = append(pts, pt)
			}
		}
	}
	before, trace := l4perception.DBSCANWithTrace(pts, p)
	if len(before) != 2 {
		t.Fatalf("fixture has %d clusters", len(before))
	}
	hints := []l4perception.FragmentHint{{ID: "owner", Rect: l4perception.Rect{X: .8, Y: .05, Length: 2, Width: 1}}}
	after, afterTrace, joined := l4perception.JoinMotionFragments(before, trace, p, hints, 2, .15)
	if joined != 1 || len(after) != 1 {
		t.Fatalf("fixture did not join: %d, %d clusters", joined, len(after))
	}
	return before, trace, after, afterTrace, hints
}

func confirmedAt(id, class string, at time.Time, x, length float64) *l5tracks.TrackedObject {
	return &l5tracks.TrackedObject{TrackID: id, TrackMeasurement: l5tracks.TrackMeasurement{TrackState: l5tracks.TrackConfirmed, ObjectClass: class},
		StateUnixNanos: at.UnixNano(), BoxX: float32(x), BoxY: .05, BoxLength: float32(length), BoxWidth: 1}
}

// A join under one body attributes both components to it; a second track
// whose predicted box holds one component makes it a two-track join; the
// owner's own box, shorter than the widened footprint, leaves the far
// component outside it.
func TestFragmentJoinRecordsAttributeComponentsToConfirmedTracks(t *testing.T) {
	before, trace, after, afterTrace, hints := joinRecordFixture(t)
	at := time.Unix(1_700_000_000, 0)

	t.Run("one body", func(t *testing.T) {
		tracks := []*l5tracks.TrackedObject{confirmedAt("owner", "car", at, .8, 2)}
		got := fragmentJoinRecords(before, trace, after, afterTrace, hints, tracks, "f1", at, .15)
		if len(got) != 1 {
			t.Fatalf("records: %+v", got)
		}
		r := got[0]
		if r.ClusterID != after[0].ClusterID || r.Points != 12 || len(r.Components) != 2 || r.FrameID != "f1" || r.FrameUnixNanos != at.UnixNano() {
			t.Fatalf("record: %+v", r)
		}
		if r.OwnerTrackID != "owner" || r.OwnerClass != "car" || r.OwnerBoxLengthMetres != 2 || r.Tracks != 1 || r.OutsideOwnerBoxShare != 0 {
			t.Fatalf("owner and attribution: %+v", r)
		}
		if math.Abs(r.LengthMetres-float64(after[0].BoundingBoxLength)) > 1e-9 || r.LengthMetres < 1.5 {
			t.Fatalf("extent %.3f against the cluster's %.3f", r.LengthMetres, after[0].BoundingBoxLength)
		}
		for _, c := range r.Components {
			if c.TrackID != "owner" || c.TrackShare != 1 || c.Points != 6 {
				t.Fatalf("component: %+v", c)
			}
		}
	})

	t.Run("two tracks", func(t *testing.T) {
		// The owner's published box covers only the near component; a
		// pedestrian (never a hint) stands on the far one.
		tracks := []*l5tracks.TrackedObject{confirmedAt("owner", "car", at, .1, .6), confirmedAt("walker", "pedestrian", at, 1.5, .6)}
		got := fragmentJoinRecords(before, trace, after, afterTrace, hints, tracks, "f1", at, .15)
		if len(got) != 1 || got[0].Tracks != 2 {
			t.Fatalf("records: %+v", got)
		}
		r := got[0]
		if r.Components[0].TrackID != "owner" || r.Components[1].TrackID != "walker" || r.Components[1].TrackClass != "pedestrian" {
			t.Fatalf("components: %+v", r.Components)
		}
		if math.Abs(r.OutsideOwnerBoxShare-.5) > 1e-9 {
			t.Fatalf("outside the owner's box: %.3f, want 0.5", r.OutsideOwnerBoxShare)
		}
	})

	t.Run("stale and minority", func(t *testing.T) {
		// A state over a second old has no predicted box, and a box holding
		// under half a component does not take it.
		stale := confirmedAt("stale", "car", at.Add(-2*time.Second), 1.5, .6)
		edge := confirmedAt("edge", "car", at, 1.65, .2)
		got := fragmentJoinRecords(before, trace, after, afterTrace, hints, []*l5tracks.TrackedObject{stale, edge}, "f1", at, 0)
		if len(got) != 1 || got[0].Tracks != 0 || got[0].OwnerClass != "" {
			t.Fatalf("records: %+v", got)
		}
		far := got[0].Components[1]
		if far.TrackID != "" || !(far.TrackShare > 0 && far.TrackShare < .5) {
			t.Fatalf("minority box took the component: %+v", far)
		}
	})

	t.Run("no join", func(t *testing.T) {
		if got := fragmentJoinRecords(before, trace, before, trace, hints, nil, "f1", at, .15); len(got) != 0 {
			t.Fatalf("unjoined partition recorded: %+v", got)
		}
		short := trace
		short.Labels = short.Labels[:1]
		if got := fragmentJoinRecords(before, trace, after, short, hints, nil, "f1", at, .15); got != nil {
			t.Fatalf("mismatched traces recorded: %+v", got)
		}
	})
}

func TestPredictedTrackBoxProjectsAndRefuses(t *testing.T) {
	at := time.Unix(1_700_000_000, 0)
	tr := confirmedAt("t", "car", at.Add(-200*time.Millisecond), 1, 4)
	tr.VX = 5
	r, ok := predictedTrackBox(tr, at)
	if !ok || math.Abs(r.X-2) > 1e-6 || r.Length != 4 {
		t.Fatalf("projection: %+v %v", r, ok)
	}
	for name, mutate := range map[string]func(*l5tracks.TrackedObject){
		"future":    func(o *l5tracks.TrackedObject) { o.StateUnixNanos = at.Add(time.Second).UnixNano() },
		"stale":     func(o *l5tracks.TrackedObject) { o.StateUnixNanos = at.Add(-1500 * time.Millisecond).UnixNano() },
		"unset":     func(o *l5tracks.TrackedObject) { o.StateUnixNanos = 0 },
		"no box":    func(o *l5tracks.TrackedObject) { o.BoxLength = 0 },
		"nonfinite": func(o *l5tracks.TrackedObject) { o.VX = float32(math.Inf(1)) },
	} {
		c := *tr
		mutate(&c)
		if _, ok := predictedTrackBox(&c, at); ok {
			t.Fatalf("%s: predicted box accepted", name)
		}
	}
}
