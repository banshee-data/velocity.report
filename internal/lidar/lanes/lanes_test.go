package lanes

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanegeom"
)

// Synthetic streets on flat ground, identity pose unless a test turns it.

var identity = Pose{Rotation: [4]float64{1, 0, 0, 0}}

func lane(id, row string, x0, y0, x1, y1 float64, tags map[string]string) Feature {
	t := map[string]string{"lane_type": "general", "width_m": "3.0", "street": "KIRKHAM ST"}
	for k, v := range tags {
		t[k] = v
	}
	return Feature{FeatureID: id, RowID: row, Kind: KindTravelLane, XYZ: []float64{x0, y0, 21, x1, y1, 21}, Tags: t, EditedAt: "2026-09-01T00:00:00Z"}
}

func limit(v string, from string) map[string]string {
	return map[string]string{"speed_limit": v, "speed_limit_unit": "mph", "speed_limit_source": "sign", "speed_limit_jurisdiction": "US-CA", "speed_limit_effective_from": from}
}

func request(id string, features ...Feature) Request {
	return Request{RequestID: id, Writer: WriterPainter, SensorID: "s1", RecordID: "gms:rec", Pose: identity, Author: "operator", Features: features}
}

func snap(t *testing.T, at time.Time, r Request) Snapshot {
	t.Helper()
	s, err := Build("808580f47", r, at, "")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var (
	jan   = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	june  = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	sept  = time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC)
	later = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
)

func eastHeading() float64 { return 0 }
func westHeading() float64 { return math.Pi }

func TestBuildPlacesLanesAndDerivesDirections(t *testing.T) {
	s := snap(t, sept, request("r1",
		lane("gms:e1", "row1", 0, -2, 50, -2, limit("25", "2026-01-01")),
		lane("gms:e2", "row2", 0, -5.5, 50, -5.5, limit("25", "2026-01-01")),
		lane("gms:w1", "row3", 50, 2, 0, 2, limit("30", "2026-01-01")),
		Feature{FeatureID: "gms:s1", RowID: "row4", Kind: "surface", XYZ: []float64{0, 0, 0, 1, 0, 0, 1, 1, 0}, Tags: map[string]string{"surface": "sidewalk"}},
	))
	if len(s.Lanes) != 3 || len(s.Features) != 4 {
		t.Fatalf("lanes %d features %d", len(s.Lanes), len(s.Features))
	}
	if len(s.Directions) != 2 {
		t.Fatalf("directions %+v", s.Directions)
	}
	byID := map[string]Direction{}
	for _, d := range s.Directions {
		byID[d.ID] = d
	}
	e, w := byID["kirkham-st:e"], byID["kirkham-st:w"]
	if e.Name != "Kirkham St eastbound" || len(e.Lanes) != 2 || e.Limit == nil || e.Limit.Value != 25 {
		t.Fatalf("eastbound %+v", e)
	}
	if w.Name != "Kirkham St westbound" || w.Limit == nil || w.Limit.Value != 30 {
		t.Fatalf("westbound %+v", w)
	}
	for _, l := range s.Lanes {
		if l.Direction == "" {
			t.Fatalf("lane %s has no direction", l.FeatureID)
		}
	}
	if s.SnapshotID == "" || s.FeaturesDigest == "" {
		t.Fatal("ids")
	}
}

func TestDirectionLimitOnlyWhenLanesAgreeAndNamesFromTheTag(t *testing.T) {
	s := snap(t, sept, request("r1",
		lane("gms:a", "r", 0, -2, 50, -2, limit("25", "2026-01-01")),
		lane("gms:b", "r", 0, -5, 50, -5, mergeTags(limit("30", "2026-01-01"), map[string]string{"direction_name": "Kirkham eastbound kerbside"})),
	))
	if len(s.Directions) != 1 || s.Directions[0].Limit != nil || s.Directions[0].Name != "Kirkham eastbound kerbside" {
		t.Fatalf("direction %+v", s.Directions)
	}
}

func mergeTags(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func TestThePoseTurnsSiteLanesIntoTheSensorFrame(t *testing.T) {
	// The sensor at (10, 20), turned 90° anticlockwise: its +Y points west.
	half := math.Sqrt(0.5)
	r := request("r1", lane("gms:a", "r", 10, 30, 10, 40, nil))
	r.Pose = Pose{Translation: [3]float64{10, 20, 21}, Rotation: [4]float64{half, 0, 0, half}}
	s := snap(t, sept, r)
	p := s.Lanes[0].Sensor
	// (10, 30) is 10 m north of the sensor; the sensor's +X points north,
	// so it lies at (10, 0) in the sensor frame.
	if math.Abs(p[0].X-10) > 1e-9 || math.Abs(p[0].Y) > 1e-9 {
		t.Fatalf("sensor point %+v, want (10, 0)", p[0])
	}
	if b := r.Pose.SensorYBearing(); math.Abs(b-270) > 1e-9 {
		t.Fatalf("sensor +Y bears %v", b)
	}
}

func TestRequestsAreValidated(t *testing.T) {
	bad := []Request{
		{},
		{RequestID: "r", Writer: "web", SensorID: "s", RecordID: "x", Pose: identity, Author: "a"},
		{RequestID: "r", Writer: WriterPull, SensorID: "s", Pose: identity, Author: "a"},
		{RequestID: "r", Writer: WriterPull, SensorID: "s", RecordID: "x", Pose: Pose{Rotation: [4]float64{2, 0, 0, 0}}, Author: "a"},
		{RequestID: "r", Writer: WriterPull, SensorID: "s", RecordID: "x", Pose: identity},
		request("r", Feature{FeatureID: "", RowID: "x"}),
		request("r", lane("gms:a", "r", 0, 0, 1, 0, nil), lane("gms:a", "r", 0, 0, 1, 0, nil)),
	}
	for i, r := range bad {
		if err := r.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("request %d accepted: %v", i, err)
		}
	}
}

// The posted-limits plan's fixtures.

func TestALaneSplitAtASignAttributesEachSideToItsLimit(t *testing.T) {
	h := NewHistory([]Snapshot{snap(t, sept, request("r1",
		lane("gms:a", "r", 0, -2, 25, -2, limit("25", "2026-01-01")),
		lane("gms:b", "r", 25, -2, 50, -2, limit("35", "2026-01-01")),
	))})
	a := h.Attribute(lanegeom.Point{X: 10, Y: -2}, eastHeading(), sept, "")
	b := h.Attribute(lanegeom.Point{X: 40, Y: -2}, eastHeading(), sept, "")
	if a.FeatureID != "gms:a" || a.Limit.Value != 25 || b.FeatureID != "gms:b" || b.Limit.Value != 35 {
		t.Fatalf("split: %+v / %+v", a, b)
	}
	if a.DirectionID != b.DirectionID || a.DirectionName != "Kirkham St eastbound" {
		t.Fatalf("one direction: %q %q", a.DirectionID, b.DirectionID)
	}
}

func TestOppositeDirectionsWithDifferentLimits(t *testing.T) {
	h := NewHistory([]Snapshot{snap(t, sept, request("r1",
		lane("gms:e", "r", 0, -2, 50, -2, limit("25", "2026-01-01")),
		lane("gms:w", "r", 50, 2, 0, 2, limit("30", "2026-01-01")),
	))})
	e := h.Attribute(lanegeom.Point{X: 20, Y: -2}, eastHeading(), sept, "")
	w := h.Attribute(lanegeom.Point{X: 20, Y: 2}, westHeading(), sept, "")
	if e.Limit.Value != 25 || w.Limit.Value != 30 || e.DirectionID == w.DirectionID {
		t.Fatalf("e %+v w %+v", e, w)
	}
}

func TestOutsideEveryCorridorWrongWayAndBeforeAnySnapshot(t *testing.T) {
	h := NewHistory([]Snapshot{snap(t, sept, request("r1", lane("gms:e", "r", 0, -2, 50, -2, limit("25", "2026-09-01"))))})
	if a := h.Attribute(lanegeom.Point{X: 20, Y: 10}, eastHeading(), sept, ""); a.Reason != ReasonNoLane {
		t.Fatalf("outside: %+v", a)
	}
	if a := h.Attribute(lanegeom.Point{X: 20, Y: -2}, westHeading(), sept, ""); a.Reason != ReasonWrongWay {
		t.Fatalf("opposed: %+v", a)
	}
	// Before the lane's limit took effect, and with no other version, there
	// is nothing in force.
	if a := h.Attribute(lanegeom.Point{X: 20, Y: -2}, eastHeading(), june, ""); a.Reason != ReasonNoSnapshot {
		t.Fatalf("before any version: %+v", a)
	}
	if a := NewHistory(nil).Attribute(lanegeom.Point{}, 0, sept, ""); a.Reason != ReasonNoSnapshot {
		t.Fatalf("no snapshots: %+v", a)
	}
	// A lane without a limit is found, with the reason.
	h2 := NewHistory([]Snapshot{snap(t, sept, request("r1", lane("gms:n", "r", 0, -2, 50, -2, nil)))})
	if a := h2.Attribute(lanegeom.Point{X: 20, Y: -2}, eastHeading(), sept, ""); a.FeatureID != "gms:n" || a.Reason != ReasonNoLimit {
		t.Fatalf("no limit: %+v", a)
	}
}

func TestALimitRaisedBetweenTwoSnapshotsIsAttributedByMeasurementTime(t *testing.T) {
	first := snap(t, june, request("r1", lane("gms:e", "row-25", 0, -2, 50, -2, limit("25", "2026-01-01"))))
	raised := lane("gms:e", "row-30", 0, -2, 50, -2, limit("30", "2026-08-01"))
	raised.EditedAt = "2026-09-05T00:00:00Z"
	second := snap(t, later, request("r2", raised))
	h := NewHistory([]Snapshot{second, first}) // in any order
	july := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	if a := h.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, july, ""); a.Limit.Value != 25 || a.RowID != "row-25" {
		t.Fatalf("before the date: %+v", a)
	}
	if a := h.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, august, ""); a.Limit.Value != 30 || a.RowID != "row-30" {
		t.Fatalf("after the date: %+v", a)
	}
	if len(h.Versions) != 2 || len(h.Snapshots) != 2 {
		t.Fatalf("versions %d snapshots %d", len(h.Versions), len(h.Snapshots))
	}
}

func TestALateEnteredChangeReattributesAnEarlierTime(t *testing.T) {
	// A report run in July read only the first snapshot and attributed June
	// to 25. The change to 20 from June 1 is entered in September; a run
	// after that attributes the same June measurement to 20.
	first := snap(t, jan, request("r1", lane("gms:e", "row-25", 0, -2, 50, -2, limit("25", "2026-01-01"))))
	before := NewHistory([]Snapshot{first})
	if a := before.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, june, ""); a.Limit.Value != 25 {
		t.Fatalf("before entry: %+v", a)
	}
	late := snap(t, sept, request("r2", lane("gms:e", "row-20", 0, -2, 50, -2, limit("20", "2026-06-01"))))
	after := NewHistory([]Snapshot{first, late})
	if a := after.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, june, ""); a.Limit.Value != 20 || a.SnapshotID != late.SnapshotID {
		t.Fatalf("after entry: %+v", a)
	}
}

func TestALaneRemovedFromALaterSnapshotEndsThere(t *testing.T) {
	first := snap(t, june, request("r1", lane("gms:e", "r", 0, -2, 50, -2, limit("25", "2026-01-01")), lane("gms:w", "r", 50, 2, 0, 2, limit("25", "2026-01-01"))))
	second := snap(t, sept, request("r2", lane("gms:w", "r", 50, 2, 0, 2, limit("25", "2026-01-01"))))
	h := NewHistory([]Snapshot{first, second})
	if a := h.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, june.Add(time.Hour), ""); a.FeatureID != "gms:e" {
		t.Fatalf("while held: %+v", a)
	}
	if a := h.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, later, ""); a.Reason != ReasonNoLane {
		t.Fatalf("after removal: %+v", a)
	}
	// An explicit end date ends it too.
	ended := lane("gms:x", "r", 0, -2, 50, -2, mergeTags(limit("25", "2026-01-01"), map[string]string{"speed_limit_effective_to": "2026-06-30"}))
	h2 := NewHistory([]Snapshot{snap(t, sept, request("r3", ended))})
	if a := h2.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, time.Date(2026, 6, 30, 23, 0, 0, 0, time.UTC), ""); a.FeatureID != "gms:x" {
		t.Fatalf("on its last day: %+v", a)
	}
	if a := h2.Attribute(lanegeom.Point{X: 20, Y: -2}, 0, time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC), ""); a.Reason != ReasonNoSnapshot {
		t.Fatalf("after its end: %+v", a)
	}
}

func instantsAlong(y, x0, x1, speed float64, t0 time.Time, n int) []Instant {
	var out []Instant
	for i := 0; i < n; i++ {
		f := float64(i) / float64(n-1)
		vx := speed
		if x1 < x0 {
			vx = -speed
		}
		out = append(out, Instant{TimeNs: t0.Add(time.Duration(i) * 100 * time.Millisecond).UnixNano(), X: x0 + f*(x1-x0), Y: y, VX: vx, SpeedMps: speed})
	}
	return out
}

func TestATrackTakesTheLaneOfMostOfItsMovingInstants(t *testing.T) {
	h := NewHistory([]Snapshot{snap(t, jan, request("r1",
		lane("gms:a", "r", 0, -2, 25, -2, limit("25", "2026-01-01")),
		lane("gms:b", "r", 25, -2, 50, -2, limit("35", "2026-01-01")),
	))})
	// 30 instants: 20 in a, 10 in b, the fastest in b.
	ins := instantsAlong(-2, 1, 24, 10, sept, 20)
	ins = append(ins, instantsAlong(-2, 26, 49, 15, sept.Add(3*time.Second), 10)...)
	tr := h.AttributeTrack(ins)
	if tr.FeatureID != "gms:a" || tr.Reason != "" {
		t.Fatalf("track %+v", tr)
	}
	if tr.MaxSpeedMps != 15 || tr.Limit == nil || tr.Limit.Value != 35 {
		t.Fatalf("its maximum is in b, against b's limit: %+v", tr)
	}
	if len(tr.PerLane) != 2 || tr.PerLane[0].MaxSpeedMps != 10 || tr.PerLane[1].MaxSpeedMps != 15 {
		t.Fatalf("per lane %+v", tr.PerLane)
	}
}

func TestTrackReasons(t *testing.T) {
	h := NewHistory([]Snapshot{snap(t, jan, request("r1",
		lane("gms:a", "r", 0, -2, 25, -2, limit("25", "2026-01-01")),
		lane("gms:b", "r", 25, -2, 50, -2, limit("35", "2026-01-01")),
	))})
	half := append(instantsAlong(-2, 1, 24, 10, sept, 10), instantsAlong(-2, 26, 49, 10, sept.Add(2*time.Second), 10)...)
	if tr := h.AttributeTrack(half); tr.Reason != ReasonAmbiguous || tr.FeatureID != "" {
		t.Fatalf("half and half: %+v", tr)
	}
	if tr := h.AttributeTrack(instantsAlong(-2, 49, 1, 10, sept, 20)); tr.Reason != ReasonWrongWay {
		t.Fatalf("against: %+v", tr)
	}
	if tr := h.AttributeTrack(instantsAlong(20, 1, 49, 10, sept, 20)); tr.Reason != ReasonNoLane {
		t.Fatalf("off the street: %+v", tr)
	}
	if tr := h.AttributeTrack(instantsAlong(-2, 1, 2, 0.5, sept, 20)); tr.Reason != ReasonStationary {
		t.Fatalf("parked: %+v", tr)
	}
	// Hysteresis: a track drifting across the line between two parallel
	// lanes stays in the one it started in.
	h2 := NewHistory([]Snapshot{snap(t, jan, request("r1",
		lane("gms:in", "r", 0, -2, 50, -2, limit("25", "2026-01-01")),
		lane("gms:out", "r", 0, -5.2, 50, -5.2, limit("25", "2026-01-01")),
	))})
	var drift []Instant
	for i := 0; i < 20; i++ {
		y := -2 - 1.8*float64(i)/19 // ends 0.3 m past the midline
		drift = append(drift, Instant{TimeNs: sept.Add(time.Duration(i) * 100 * time.Millisecond).UnixNano(), X: float64(5 + i), Y: y, VX: 10, SpeedMps: 10})
	}
	if tr := h2.AttributeTrack(drift); tr.FeatureID != "gms:in" {
		t.Fatalf("drift: %+v", tr)
	}
}

func TestBoundsAndNames(t *testing.T) {
	if Bound(0) != "N" || Bound(311) != "NW" || Bound(-90) != "W" || BoundWord("SE") != "southeastbound" || TitleCase("COLUMBUS AVE") != "Columbus Ave" {
		t.Fatal("bounds or names")
	}
	if ParseLimit(map[string]string{"speed_limit": "91", "speed_limit_unit": "mph"}) != nil || ParseLimit(map[string]string{"speed_limit": "25"}) != nil {
		t.Fatal("bad limits parsed")
	}
	if l := ParseLimit(map[string]string{"speed_limit": "40", "speed_limit_unit": "km/h", "speed_limit_jurisdiction": "Spain"}); l == nil || l.Jurisdiction != "" {
		t.Fatalf("km/h limit %+v", l)
	}
	if Digest([]Feature{{FeatureID: "b"}, {FeatureID: "a"}}) != Digest([]Feature{{FeatureID: "a"}, {FeatureID: "b"}}) {
		t.Fatal("digest depends on order")
	}
}

func TestFromGeoJSONReadsStreetUseAsReturned(t *testing.T) {
	data := []byte(`{"type":"FeatureCollection","features":[
	 {"type":"Feature","id":"gms:a","geometry":{"type":"LineString","coordinates":[[0,-2,21],[50,-2,21.5]]},
	  "properties":{"layer":"street_use","id":"gms:a","kind":"travel_lane","author":"op","edited_at":"2026-09-01T00:00:00Z","row_id":"row1",
	   "lane_type":"general","speed_limit":"25","speed_limit_unit":"mph","street":"KIRKHAM ST"}},
	 {"type":"Feature","id":"gms:s","geometry":{"type":"Polygon","coordinates":[[[0,0,1],[1,0,1],[1,1,1],[0,0,1]]]},
	  "properties":{"layer":"street_use","id":"gms:s","kind":"surface","row_id":"row2","surface":"sidewalk"}},
	 {"type":"Feature","id":"x","geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]},"properties":{"layer":"kerbs"}}]}`)
	fs, err := FromGeoJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 {
		t.Fatalf("features %d", len(fs))
	}
	a := fs[0]
	if a.FeatureID != "gms:a" || a.RowID != "row1" || a.Kind != KindTravelLane || a.Author != "op" || len(a.XYZ) != 6 || a.XYZ[5] != 21.5 {
		t.Fatalf("lane %+v", a)
	}
	if a.Tags["speed_limit"] != "25" || a.Tags["layer"] != "" || a.Tags["row_id"] != "" {
		t.Fatalf("tags %+v", a.Tags)
	}
	if s := fs[1]; !s.Closed || len(s.XYZ) != 9 || s.Tags["surface"] != "sidewalk" {
		t.Fatalf("surface %+v", s)
	}
	if _, err := FromGeoJSON([]byte(`nope`)); err == nil {
		t.Fatal("bad JSON accepted")
	}
}
