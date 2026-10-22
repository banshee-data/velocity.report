package perframeeval

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

func axisInstant(object string, sample int, errDeg, boundDeg, speed float64, est l8behaviour.EstimationState) PhysicalInstant {
	rad := math.Pi / 180
	return PhysicalInstant{
		EpisodeID: "e", ObjectID: object, SampleID: sample,
		Prediction: &PredictedBody{sample: l8behaviour.TrajectorySample{VX: speed, Estimation: est}},
		Comparison: PhysicalComparison{Yaw: &YawComparison{ErrorRad: errDeg * rad, ReferenceBoundRad: boundDeg * rad}},
		Outcomes:   map[PhysicalComponent]Outcome{ComponentYaw: {Category: OutcomeScored}},
	}
}

func axisNear(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestFoldAxisDeg(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ in, want float64 }{
		{0, 0}, {10, 10}, {-10, 10}, {45, 45}, {50, 40}, {90, 0}, {-90, 0}, {100, 10}, {180, 0}, {-135, 45},
	} {
		if got := foldAxisDeg(tt.in * math.Pi / 180); !axisNear(got, tt.want) {
			t.Errorf("foldAxisDeg(%v°) = %v°, want %v°", tt.in, got, tt.want)
		}
	}
}

// A right axis with the wrong length label is a 0° axis error; an error
// inside the reference's bound is zero; the slow and the not established are
// counted out, not pooled.
func TestSummariseAxis(t *testing.T) {
	t.Parallel()
	est := l8behaviour.EstimationEstablished
	instants := []PhysicalInstant{
		axisInstant("car", 1, 90, 2, 5, est),   // wrong label, right axis: 0
		axisInstant("car", 2, 12, 2, 5, est),   // 10 beyond the bound
		axisInstant("car", 3, 1.5, 2, 5, est),  // inside the bound: 0
		axisInstant("truck", 1, 30, 5, 3, est), // 25
		axisInstant("truck", 2, 60, 0, 3, est), // folds to 30
		axisInstant("truck", 3, 40, 0, 1.9, est),
		axisInstant("truck", 4, 40, 0, 6, l8behaviour.EstimationGeometryConverging),
	}
	unscored := axisInstant("car", 9, 40, 0, 5, est)
	unscored.Outcomes[ComponentYaw] = Outcome{Category: "unscored"}
	instants = append(instants, unscored, PhysicalInstant{ObjectID: "car", Outcomes: map[PhysicalComponent]Outcome{}})

	s := summariseAxis(instants)
	if s.Pooled.Poses != 5 {
		t.Fatalf("pooled poses = %d, want 5: %+v", s.Pooled.Poses, s.Poses)
	}
	if s.Excluded[AxisExcludedBelowSpeed] != 1 || s.Excluded[AxisExcludedNotEstablished] != 1 {
		t.Fatalf("excluded = %v, want one below speed and one not established", s.Excluded)
	}
	// Pooled errors sorted: 0, 0, 10, 25, 30. Nearest rank: median rank 3 (10), p90 rank 5 (30).
	if !axisNear(s.Pooled.MedianDeg, 10) || !axisNear(s.Pooled.P90Deg, 30) || !axisNear(s.Pooled.MaxDeg, 30) {
		t.Fatalf("pooled = %+v, want median 10, p90 30, max 30", s.Pooled)
	}
	if len(s.Vehicles) != 2 || s.Vehicles[0].ObjectID != "car" || s.Vehicles[1].ObjectID != "truck" {
		t.Fatalf("vehicles = %+v, want car then truck", s.Vehicles)
	}
	car, truck := s.Vehicles[0], s.Vehicles[1]
	// car: 0, 0, 10. median rank 2 (0), p90 rank 3 (10).
	if car.Poses != 3 || !axisNear(car.MedianDeg, 0) || !axisNear(car.P90Deg, 10) {
		t.Fatalf("car = %+v", car)
	}
	// Setting the truck apart leaves the car's poses: the level is read there.
	if truck.Poses != 2 || truck.Others != car.AxisErrorStats {
		t.Fatalf("truck = %+v, others should equal the car's own %+v", truck, car.AxisErrorStats)
	}
	if !axisNear(truck.MedianDeg, 25) || !axisNear(truck.P90Deg, 30) {
		t.Fatalf("truck = %+v, want median 25 and p90 30", truck)
	}
}

func TestSummariseAxisEmptyMarshals(t *testing.T) {
	t.Parallel()
	s := summariseAxis(nil)
	if s.Pooled != (AxisErrorStats{}) || len(s.Poses) != 0 || len(s.Vehicles) != 0 {
		t.Fatalf("empty summary = %+v", s)
	}
	if _, err := json.Marshal(s); err != nil {
		t.Fatalf("empty summary does not marshal: %v", err)
	}
}
