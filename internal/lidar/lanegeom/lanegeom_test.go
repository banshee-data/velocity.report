package lanegeom

import (
	"math"
	"testing"
)

func east(id string, y float64) Lane {
	return Lane{ID: id, Points: []Point{{0, y}, {50, y}}, HalfWidth: Corridor(3)}
}

func TestProjectGivesArcLengthLateralAndTangent(t *testing.T) {
	l := Lane{Points: []Point{{0, 0}, {10, 0}, {10, 10}}}
	pr, err := l.Project(Point{12, 4})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(pr.S-14) > 1e-9 || math.Abs(pr.Lateral+2) > 1e-9 || math.Abs(pr.Tangent-math.Pi/2) > 1e-9 {
		t.Fatalf("projection %+v: want s 14, lateral −2 (right of travel), tangent north", pr)
	}
	if pr.Length != 20 || pr.Beyond {
		t.Fatalf("length %v beyond %v", pr.Length, pr.Beyond)
	}
	if pr, _ := l.Project(Point{-3, 0}); !pr.Beyond || pr.S != 0 {
		t.Fatalf("before the start: %+v", pr)
	}
	if _, err := (Lane{Points: []Point{{1, 1}, {1, 1}}}).Project(Point{}); err != ErrDegenerate {
		t.Fatalf("degenerate lane: %v", err)
	}
}

func TestMemberNeedsTheCorridorAndTheHeading(t *testing.T) {
	lanes := []Lane{east("a", 0), east("b", 3.5)}
	if m := Member(lanes, Point{20, 0.4}, 0, -1); m.Lane != 0 {
		t.Fatalf("in lane a: %+v", m)
	}
	if m := Member(lanes, Point{20, 3.2}, 0.3, -1); m.Lane != 1 {
		t.Fatalf("in lane b, 17° off: %+v", m)
	}
	if m := Member(lanes, Point{20, 9}, 0, -1); m.Lane != -1 || m.Reason != ReasonOutside {
		t.Fatalf("outside: %+v", m)
	}
	if m := Member(lanes, Point{20, 0.2}, math.Pi, -1); m.Lane != -1 || m.Reason != ReasonOpposed {
		t.Fatalf("against travel: %+v", m)
	}
	if m := Member(lanes, Point{60, 0}, 0, -1); m.Lane != -1 {
		t.Fatalf("past the end: %+v", m)
	}
	if m := Member(nil, Point{}, 0, -1); m.Reason != ReasonNoLanes {
		t.Fatalf("no lanes: %+v", m)
	}
}

func TestMemberHoldsThePreviousLaneWithHysteresis(t *testing.T) {
	lanes := []Lane{east("a", 0), east("b", 3.5)}
	// Straddling the line between them, nearer b: without history it is b,
	// with a held it stays a while inside a's widened corridor.
	p := Point{20, 2.1}
	if m := Member(lanes, p, 0, -1); m.Lane != 1 {
		t.Fatalf("no history: %+v", m)
	}
	if m := Member(lanes, p, 0, 0); m.Lane != 0 {
		t.Fatalf("held: %+v", m)
	}
	// Well into b, a lets go.
	if m := Member(lanes, Point{20, 3.4}, 0, 0); m.Lane != 1 {
		t.Fatalf("let go: %+v", m)
	}
}

func TestHeadingsAndBearings(t *testing.T) {
	if e := HeadingError(0.1, 2*math.Pi-0.1); math.Abs(e-0.2) > 1e-12 {
		t.Fatalf("wrap: %v", e)
	}
	if h := BearingToHeading(90); math.Abs(h) > 1e-12 {
		t.Fatalf("east: %v", h)
	}
	if b := HeadingToBearing(math.Pi / 2); math.Abs(b) > 1e-9 {
		t.Fatalf("north: %v", b)
	}
	if b := HeadingToBearing(math.Pi); math.Abs(b-270) > 1e-9 {
		t.Fatalf("west: %v", b)
	}
	if RouteDistance(3, 10) != 7 || Corridor(0) != 2 || Corridor(3.4) != 2.2 {
		t.Fatal("route distance or corridor")
	}
}
