package l8analytics

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// Synthetic fixtures only: hand-made tracks on a made-up plane.

const laneSec = int64(1_000_000_000)

// straightTrack is a track moving along +Y (bearing 0) at x, from y0, one
// instant every 0.1 s, n instants, at speed m/s. Every instant has a 4 x 2 m
// box when boxed is true.
func straightTrack(id, class string, x, y0, speed float64, n int, t0 int64, boxed bool) LaneFieldTrack {
	tr := LaneFieldTrack{TrackID: id, Class: class}
	for i := 0; i < n; i++ {
		y := y0 + speed*0.1*float64(i)
		in := LaneFieldInstant{TimeNs: t0 + int64(i)*laneSec/10, X: x, Y: y, VX: 0, VY: speed, SpeedMps: speed}
		if boxed {
			in.BoxX, in.BoxY, in.BoxHeadingRad, in.BoxLength, in.BoxWidth = x, y, math.Pi/2, 4, 2
		}
		tr.Instants = append(tr.Instants, in)
	}
	return tr
}

// parkedTrack sits still at (x, y) for n instants with a box.
func parkedTrack(id string, x, y float64, n int, t0 int64) LaneFieldTrack {
	tr := LaneFieldTrack{TrackID: id, Class: "car"}
	for i := 0; i < n; i++ {
		tr.Instants = append(tr.Instants, LaneFieldInstant{
			TimeNs: t0 + int64(i)*laneSec/10, X: x, Y: y, SpeedMps: 0.1, VX: 0.1,
			BoxX: x, BoxY: y, BoxHeadingRad: 0, BoxLength: 4.5, BoxWidth: 1.8,
		})
	}
	return tr
}

func build(t *testing.T, tracks []LaneFieldTrack, part LaneFieldPart) LaneField {
	t.Helper()
	f, err := BuildLaneField(tracks, LaneFieldMeta{SensorID: "s1", CaptureRef: "cap"}, part, DefaultLaneFieldOptions())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// cellAt finds a group's cell holding the sensor-frame point.
func cellAt(f LaneField, group string, x, y float64) (tracks, boxTracks uint32, heading []uint32, speedSum float32, ok bool) {
	for _, g := range f.Groups {
		if g.ClassGroup != group {
			continue
		}
		for c := range g.IX {
			cx := f.Header.Origin[0] + float64(g.IX[c])*f.Header.CellM
			cy := f.Header.Origin[1] + float64(g.IY[c])*f.Header.CellM
			if math.Abs(cx-x) <= f.Header.CellM/2 && math.Abs(cy-y) <= f.Header.CellM/2 {
				return g.Tracks[c], g.BoxTracks[c], g.Heading[8*c : 8*c+8], g.SpeedSumMps[c], true
			}
		}
	}
	return 0, 0, nil, 0, false
}

func TestLaneFieldTrackCountsOncePerCell(t *testing.T) {
	// Five cars each crawl 0.02 m per instant: many instants per cell.
	var tracks []LaneFieldTrack
	for i := 0; i < 5; i++ {
		tracks = append(tracks, straightTrack(fmt.Sprintf("t%d", i), "car", 0.1, 0.01, 2.5, 40, 0, false))
	}
	f := build(t, tracks, LaneFieldPartAll)
	n, _, heading, speed, ok := cellAt(f, LaneFieldVehicle, 0.125, 0.125)
	if !ok || n != 5 {
		t.Fatalf("cell (0.125, 0.125) holds %d tracks (present %v), want 5", n, ok)
	}
	if heading[0] != 5 {
		t.Fatalf("heading %v, want 5 in bin 0 (bearing 0, along +Y)", heading)
	}
	if math.Abs(float64(speed)-12.5) > 1e-4 {
		t.Fatalf("speed sum %v, want 5 × 2.5", speed)
	}
	if f.Header.TrackCount != 5 {
		t.Fatalf("track_count %d, want 5", f.Header.TrackCount)
	}
}

func TestLaneFieldParkedCarNamesNoDirection(t *testing.T) {
	// Five moving cars set the vehicle floor aside; the parked cars must
	// themselves pass it, so each is given one fast instant at its start.
	var tracks []LaneFieldTrack
	for i := 0; i < 5; i++ {
		p := parkedTrack(fmt.Sprintf("p%d", i), 10.1, 10.1, 20, 0)
		p.Instants[0].SpeedMps = 3 // a lifetime maximum over the floor
		p.Instants[0].VX = 0
		p.Instants[0].X, p.Instants[0].Y = 30, 30 // that instant elsewhere
		tracks = append(tracks, p)
	}
	f := build(t, tracks, LaneFieldPartAll)
	n, bt, heading, _, ok := cellAt(f, LaneFieldVehicle, 10.125, 10.125)
	if !ok || n != 5 || bt != 5 {
		t.Fatalf("parked cell: tracks %d, box %d (present %v); want 5 and 5", n, bt, ok)
	}
	for b, h := range heading {
		if h != 0 {
			t.Fatalf("parked cell counts %d in heading bin %d; a stopped car names no direction", h, b)
		}
	}
}

func TestLaneFieldVehicleNeverMovingIsLeftOut(t *testing.T) {
	var tracks []LaneFieldTrack
	for i := 0; i < 6; i++ {
		tracks = append(tracks, parkedTrack(fmt.Sprintf("p%d", i), 1, 1, 30, 0))
	}
	f := build(t, tracks, LaneFieldPartAll)
	if f.CellCount() != 0 || f.Header.TrackCount != 0 {
		t.Fatalf("cars parked through the run painted %d cells from %d tracks", f.CellCount(), f.Header.TrackCount)
	}
	// A pedestrian standing still is still a pedestrian where it stood.
	for i := range tracks {
		tracks[i].Class = "pedestrian"
	}
	if f = build(t, tracks, LaneFieldPartAll); f.CellCount() == 0 {
		t.Fatal("still pedestrians: the vehicle floor must not apply to them")
	}
}

func TestLaneFieldKBoundary(t *testing.T) {
	mk := func(n int) []LaneFieldTrack {
		var out []LaneFieldTrack
		for i := 0; i < n; i++ {
			out = append(out, straightTrack(fmt.Sprintf("t%d", i), "car", 0.1, 0.01, 2.5, 20, 0, false))
		}
		return out
	}
	if f := build(t, mk(4), LaneFieldPartAll); f.CellCount() != 0 || len(f.Groups) != 0 {
		t.Fatalf("four tracks emitted %d cells; a cell under k must be absent", f.CellCount())
	}
	if f := build(t, mk(4), LaneFieldPartAll); f.Header.NX != 0 || f.Header.NY != 0 {
		t.Fatalf("an empty field's grid is %d by %d, want 0 by 0", f.Header.NX, f.Header.NY)
	}
	f := build(t, mk(5), LaneFieldPartAll)
	if _, _, _, _, ok := cellAt(f, LaneFieldVehicle, 0.125, 0.125); !ok {
		t.Fatal("five tracks: the cell must be present")
	}
	for _, g := range f.Groups {
		for c := range g.IX {
			if g.Tracks[c] < 5 && g.BoxTracks[c] < 5 {
				t.Fatalf("cell %d of %s holds %d and %d by box, under k", c, g.ClassGroup, g.Tracks[c], g.BoxTracks[c])
			}
		}
	}
}

func TestLaneFieldBoxFootprintCoversItsCorners(t *testing.T) {
	// One instant each, box 1 m along +X by 0.5 m, centred on (0.5, 0.25):
	// it covers x in [0, 1] and y in [0, 0.5], so the centres at x 0.125 ..
	// 0.875 and y 0.125, 0.375: eight cells.
	in := LaneFieldInstant{BoxX: 0.5, BoxY: 0.25, BoxHeadingRad: 0, BoxLength: 1, BoxWidth: 0.5}
	got := laneBoxCells(in, 0.25)
	if len(got) != 8 {
		t.Fatalf("axis box covers %d cells, want 8: %v", len(got), got)
	}
	for _, k := range got {
		if k.ix < 0 || k.ix > 3 || k.iy < 0 || k.iy > 1 {
			t.Fatalf("cell %v is outside the box", k)
		}
	}
	// Turned 90°, the same box covers 2 columns by 4 rows about its centre.
	in.BoxHeadingRad = math.Pi / 2
	in.BoxX, in.BoxY = 0.25, 0.5
	got = laneBoxCells(in, 0.25)
	if len(got) != 8 {
		t.Fatalf("turned box covers %d cells, want 8: %v", len(got), got)
	}
	for _, k := range got {
		if k.ix < 0 || k.ix > 1 || k.iy < 0 || k.iy > 3 {
			t.Fatalf("turned: cell %v is outside the box", k)
		}
	}
	// At 45° a 2 m by 0.2 m box covers cells on its diagonal and not the
	// corners of its bounding square.
	in = LaneFieldInstant{BoxX: 0, BoxY: 0, BoxHeadingRad: math.Pi / 4, BoxLength: 2, BoxWidth: 0.2}
	for _, k := range laneBoxCells(in, 0.25) {
		cx, cy := (float64(k.ix)+0.5)*0.25, (float64(k.iy)+0.5)*0.25
		if math.Abs(cx-cy) > 0.2 {
			t.Fatalf("diagonal box covers off-diagonal cell centre (%v, %v)", cx, cy)
		}
	}
}

func TestLaneFieldBoxTracksGiveWidth(t *testing.T) {
	var tracks []LaneFieldTrack
	for i := 0; i < 5; i++ {
		tracks = append(tracks, straightTrack(fmt.Sprintf("t%d", i), "car", 0.1, 0, 10, 30, 0, true))
	}
	f := build(t, tracks, LaneFieldPartAll)
	// 0.9 m to the side of the centre line: inside the 2 m box, no centre.
	n, bt, _, _, ok := cellAt(f, LaneFieldVehicle, -0.875, 5.125)
	if !ok || n != 0 || bt != 5 {
		t.Fatalf("side cell: tracks %d, box %d (present %v); want 0 and 5", n, bt, ok)
	}
	// An unconverged box (zero width) counts in tracks only.
	for i := range tracks {
		for j := range tracks[i].Instants {
			tracks[i].Instants[j].BoxWidth = 0
		}
	}
	f = build(t, tracks, LaneFieldPartAll)
	if _, _, _, _, ok := cellAt(f, LaneFieldVehicle, -0.875, 5.125); ok {
		t.Fatal("unconverged boxes still swept the side cell")
	}
}

func TestLaneFieldHalvesPartitionTheWhole(t *testing.T) {
	// Ten cars across 20 s, each 2 s long, so some straddle the middle.
	var tracks []LaneFieldTrack
	for i := 0; i < 10; i++ {
		tracks = append(tracks, straightTrack(fmt.Sprintf("t%02d", i), "car", 0.1, 0, 5, 20, int64(i)*2*laneSec, true))
	}
	meta := LaneFieldMeta{SensorID: "s1", CaptureRef: "cap", WindowStartNs: 1, WindowEndNs: 20 * laneSec}
	fields, err := BuildLaneFields(tracks, meta, DefaultLaneFieldOptions())
	if err != nil {
		t.Fatal(err)
	}
	all, first, second := fields[0], fields[1], fields[2]
	if first.Header.WindowEnd != second.Header.WindowStart || all.Header.WindowStart != first.Header.WindowStart || all.Header.WindowEnd != second.Header.WindowEnd {
		t.Fatalf("windows do not partition: all %s–%s, first %s–%s, second %s–%s",
			all.Header.WindowStart, all.Header.WindowEnd, first.Header.WindowStart, first.Header.WindowEnd, second.Header.WindowStart, second.Header.WindowEnd)
	}
	// Each instant is in exactly one half: with k = 1 a cell's tracks in the
	// two halves add to at least the whole's, and to exactly it where no
	// track straddles that cell's instants across the middle.
	opts := DefaultLaneFieldOptions()
	opts.K = 1
	mid := meta.WindowStartNs + (meta.WindowEndNs-meta.WindowStartNs)/2
	nFirst, nSecond, nAll := 0, 0, 0
	for _, tr := range tracks {
		for _, in := range tr.Instants {
			nAll++
			if in.TimeNs < mid {
				nFirst++
			} else {
				nSecond++
			}
		}
	}
	if nFirst+nSecond != nAll || nFirst == 0 || nSecond == 0 {
		t.Fatalf("instants %d + %d != %d", nFirst, nSecond, nAll)
	}
	a, _ := BuildLaneField(tracks, meta, LaneFieldPartAll, opts)
	h1, _ := BuildLaneField(tracks, meta, LaneFieldPartFirstHalf, opts)
	h2, _ := BuildLaneField(tracks, meta, LaneFieldPartSecondHalf, opts)
	if h1.Header.TrackCount+h2.Header.TrackCount < a.Header.TrackCount || h1.Header.TrackCount == 0 || h2.Header.TrackCount == 0 {
		t.Fatalf("track counts: halves %d + %d, whole %d", h1.Header.TrackCount, h2.Header.TrackCount, a.Header.TrackCount)
	}
	sum := func(f LaneField) uint32 {
		var s uint32
		for _, g := range f.Groups {
			for _, n := range g.Tracks {
				s += n
			}
		}
		return s
	}
	if sum(h1)+sum(h2) < sum(a) {
		t.Fatalf("halves count %d + %d track-cells, fewer than the whole's %d", sum(h1), sum(h2), sum(a))
	}
}

func TestLaneFieldHeadingBinsClockwiseFromY(t *testing.T) {
	cases := []struct {
		vx, vy float64
		bin    int
	}{{0, 1, 0}, {1, 1, 1}, {1, 0, 2}, {1, -1, 3}, {0, -1, 4}, {-1, -1, 5}, {-1, 0, 6}, {-1, 1, 7}}
	for _, c := range cases {
		// Nudge clockwise off the bin edge so the expected bin is unambiguous.
		b := math.Atan2(c.vx, c.vy) + 0.01
		if got := laneHeadingBin(b); got != c.bin {
			t.Errorf("velocity (%v, %v): bin %d, want %d", c.vx, c.vy, got, c.bin)
		}
	}
}

func TestLaneFieldClassGroupsAndShortTracks(t *testing.T) {
	var tracks []LaneFieldTrack
	for i := 0; i < 5; i++ {
		tracks = append(tracks,
			straightTrack(fmt.Sprintf("b%d", i), "bus", 0.1, 0, 5, 20, 0, false),
			straightTrack(fmt.Sprintf("c%d", i), "cyclist", 5.1, 0, 4, 20, 0, false),
			straightTrack(fmt.Sprintf("u%d", i), "dynamic", 9.1, 0, 4, 20, 0, false),
			straightTrack(fmt.Sprintf("s%d", i), "car", 12.1, 0, 4, 9, 0, false), // nine instants
		)
	}
	f := build(t, tracks, LaneFieldPartAll)
	var groups []string
	for _, g := range f.Groups {
		groups = append(groups, g.ClassGroup)
	}
	if strings.Join(groups, ",") != "vehicle,cyclist" {
		t.Fatalf("groups %v, want vehicle then cyclist", groups)
	}
	if _, _, _, _, ok := cellAt(f, LaneFieldVehicle, 12.125, 0.125); ok {
		t.Fatal("tracks of nine instants counted")
	}
	if f.Header.TrackCount != 10 {
		t.Fatalf("track_count %d, want 10 (buses and cyclists)", f.Header.TrackCount)
	}
}

func TestLaneFieldDeterministicAndRoundTrips(t *testing.T) {
	var tracks []LaneFieldTrack
	for i := 0; i < 12; i++ {
		tracks = append(tracks, straightTrack(fmt.Sprintf("t%02d", i), "car", 0.1+0.03*float64(i), 0, 3+0.7*float64(i), 25, int64(i)*laneSec, true))
	}
	meta := LaneFieldMeta{SensorID: "s1", CaptureRef: "cap", PipelineBuild: "v0+abc", TuningFingerprint: "f00d"}
	want, err := BuildLaneField(tracks, meta, LaneFieldPartAll, DefaultLaneFieldOptions())
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := want.JSON()
	r := rand.New(rand.NewSource(7))
	for trial := 0; trial < 5; trial++ {
		shuffled := make([]LaneFieldTrack, len(tracks))
		for i, p := range r.Perm(len(tracks)) {
			tr := tracks[p]
			ins := append([]LaneFieldInstant(nil), tr.Instants...)
			r.Shuffle(len(ins), func(a, b int) { ins[a], ins[b] = ins[b], ins[a] })
			shuffled[i] = LaneFieldTrack{TrackID: tr.TrackID, Class: tr.Class, Instants: ins}
		}
		got, _ := BuildLaneField(shuffled, meta, LaneFieldPartAll, DefaultLaneFieldOptions())
		gotJSON, _ := got.JSON()
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("trial %d: shuffled input changed the field", trial)
		}
	}
	var back LaneField
	if err := json.Unmarshal(wantJSON, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, want) {
		t.Fatal("the JSON does not round-trip")
	}
	// The input is not mutated by sorting.
	if tracks[0].Instants[0].TimeNs != 0 {
		t.Fatal("BuildLaneField reordered the caller's instants")
	}
}

// The JSON keys and their order are Gerardus's upload-lane-field contract.
func TestLaneFieldJSONShape(t *testing.T) {
	var tracks []LaneFieldTrack
	for i := 0; i < 5; i++ {
		tracks = append(tracks, straightTrack(fmt.Sprintf("t%d", i), "car", 0.1, 0, 5, 12, 0, false))
	}
	f := build(t, tracks, LaneFieldPartAll)
	b, _ := f.JSON()
	s := string(b)
	order := []string{`{"header":{"l16_token":""`, `"sensor_id":"s1"`, `"record_id":""`, `"capture_ref":"cap"`,
		`"window_start":"`, `"window_end":"`, `"window_part":"all"`, `"cell_m":0.25`, `"origin":[0.125,`, `"nx":1`,
		`"ny":`, `"k":5`, `"pipeline_build":""`, `"tuning_fingerprint":""`, `"track_count":5`,
		`"groups":[{"class_group":"vehicle","ix":[`, `"iy":[`, `"tracks":[`, `"box_tracks":[`, `"heading":[`, `"speed_sum_mps":[`}
	at := 0
	for _, part := range order {
		i := strings.Index(s[at:], part)
		if i < 0 {
			t.Fatalf("%s not found after byte %d in %s", part, at, s)
		}
		at += i + len(part)
	}
	if !strings.HasSuffix(s, "]}]}\n") {
		t.Fatalf("unexpected ending: %q", s[len(s)-10:])
	}
	// An empty field is a header and an empty list, never null.
	e, _ := BuildLaneField(nil, LaneFieldMeta{SensorID: "s1"}, LaneFieldPartFirstHalf, DefaultLaneFieldOptions())
	eb, _ := e.JSON()
	if !strings.HasSuffix(string(eb), `"groups":[]}`+"\n") || !strings.Contains(string(eb), `"window_part":"first_half"`) {
		t.Fatalf("empty field: %s", eb)
	}
}

func TestLaneFieldRejectsBadOptions(t *testing.T) {
	if _, err := BuildLaneField(nil, LaneFieldMeta{}, "middle", DefaultLaneFieldOptions()); err == nil {
		t.Fatal("unknown part accepted")
	}
	if _, err := BuildLaneField(nil, LaneFieldMeta{}, LaneFieldPartAll, LaneFieldOptions{}); err == nil {
		t.Fatal("zero options accepted")
	}
	if p, err := ParseLaneFieldPart(""); err != nil || p != LaneFieldPartAll {
		t.Fatal("empty part must read as all")
	}
	if f, err := BuildLaneField(nil, LaneFieldMeta{}, "", DefaultLaneFieldOptions()); err != nil || f.Header.WindowPart != string(LaneFieldPartAll) {
		t.Fatalf("an empty part builds all: %v %q", err, f.Header.WindowPart)
	}
}
