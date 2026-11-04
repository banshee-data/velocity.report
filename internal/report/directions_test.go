package report

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

// Synthetic transits: speeds in mph converted to m/s.

func mph(v float64) float64 { return v / 2.2369362920544 }

func deployment(inbound, outbound []db.DirectionLimit) *db.RadarDirections {
	return &db.RadarDirections{InboundSign: 1, Directions: []db.DirectionConfig{
		{Key: "inbound", Name: "Kirkham St eastbound", Limits: inbound},
		{Key: "outbound", Name: "Kirkham St westbound", Limits: outbound},
	}}
}

func transits(dir string, d *db.RadarDirections, at time.Time, speedsMPH ...float64) []db.DirectionalTransit {
	var out []db.DirectionalTransit
	for i, s := range speedsMPH {
		out = append(out, db.DirectionalTransit{StartUnix: float64(at.Unix() + int64(i)*60), SpeedMPS: mph(s), Direction: dir, Directions: d})
	}
	return out
}

func repeat(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v + float64(i%7)
	}
	return out
}

var (
	sep1  = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	sep30 = time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	sign  = func(v int, from string) db.DirectionLimit {
		return db.DirectionLimit{Value: v, Unit: "mph", Source: "sign", EffectiveFrom: from}
	}
)

func TestTwoDirectionsWithDifferentLimits(t *testing.T) {
	d := deployment([]db.DirectionLimit{sign(25, "2026-01-01")}, []db.DirectionLimit{sign(30, "2026-01-01")})
	all := append(transits("inbound", d, sep1, repeat(22, 60)...), transits("outbound", d, sep1.Add(time.Hour), repeat(27, 60)...)...)
	set := BuildDirections(all, sep1, sep30, "mph", 5, 70)
	if len(set.Directions) != 2 {
		t.Fatalf("directions %+v", set)
	}
	in, out := set.Directions[0], set.Directions[1]
	if in.Name != "Kirkham St eastbound" || in.Count != 60 || in.Segments[0].Limit.Value != 25 {
		t.Fatalf("inbound %+v", in)
	}
	if out.Segments[0].Limit.Value != 30 || out.Segments[0].P98 == nil || out.Segments[0].Max == nil {
		t.Fatalf("outbound %+v", out.Segments[0])
	}
	// 22..28 mph against 25: three of seven values above (26, 27, 28).
	if s := *in.Segments[0].OverLimit; math.Abs(s-100*float64(3*60/7+boolInt(60%7 > 4))/60) > 2 {
		t.Fatalf("inbound share over 25: %v", s)
	}
	if *in.Segments[0].Over10 != 0 {
		t.Fatalf("nobody 10 over: %v", *in.Segments[0].Over10)
	}
	if LimitLabel(in.Segments[0].Limit) != "25 mph (sign)" {
		t.Fatal(LimitLabel(in.Segments[0].Limit))
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestADirectionUnderFiveIsMaskedAndTailsUnderFifty(t *testing.T) {
	d := deployment([]db.DirectionLimit{sign(25, "")}, []db.DirectionLimit{sign(25, "")})
	all := append(transits("inbound", d, sep1, repeat(22, 20)...), transits("outbound", d, sep1, 20, 21, 22, 23)...)
	all = append(all, db.DirectionalTransit{StartUnix: float64(sep1.Unix()), SpeedMPS: 9, Direction: "mixed", Directions: d})
	all = append(all, db.DirectionalTransit{StartUnix: float64(sep1.Unix()), SpeedMPS: 9, Direction: "unknown"})
	set := BuildDirections(all, sep1, sep30, "mph", 5, 70)
	if len(set.Directions) != 1 || set.Masked != 4 || set.Mixed != 1 || set.Unknown != 1 {
		t.Fatalf("set %+v", set)
	}
	seg := set.Directions[0].Segments[0]
	if seg.P50 == nil || seg.P85 == nil || seg.P98 != nil || seg.Max != nil {
		t.Fatalf("20 transits: p98 and max must be masked: %+v", seg)
	}
}

func TestALimitChangedInsideThePeriodSplitsTheDirection(t *testing.T) {
	d := deployment([]db.DirectionLimit{sign(25, "2026-01-01"), sign(20, "2026-09-15")}, nil)
	sep16 := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	all := append(transits("inbound", d, sep1, repeat(22, 30)...), transits("inbound", d, sep16, repeat(22, 30)...)...)
	set := BuildDirections(all, sep1, sep30, "mph", 5, 70)
	segs := set.Directions[0].Segments
	if len(segs) != 2 || segs[0].Limit.Value != 25 || segs[1].Limit.Value != 20 || segs[0].Count != 30 || segs[1].Count != 30 {
		t.Fatalf("segments %+v", segs)
	}
	if !segs[0].From.Equal(sep1) || !segs[1].From.Equal(time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)) || !segs[1].To.Equal(sep30) || !segs[0].To.Equal(segs[1].From) {
		t.Fatalf("windows %v–%v, %v–%v", segs[0].From, segs[0].To, segs[1].From, segs[1].To)
	}
	// The same speeds over a lower limit: more are over it.
	if *segs[1].OverLimit <= *segs[0].OverLimit {
		t.Fatalf("shares %v then %v", *segs[0].OverLimit, *segs[1].OverLimit)
	}
}

// An effective date is a local day: in Los Angeles a transit at 20:00 on
// the 14th is under the old limit, though it is the 15th in UTC.
func TestALimitTakesEffectAtLocalMidnight(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	limits := []db.DirectionLimit{sign(25, "2026-01-01"), sign(20, "2026-09-15")}
	evening := time.Date(2026, 9, 14, 20, 0, 0, 0, la)
	if l := LimitAt(limits, evening); l == nil || l.Value != 25 {
		t.Fatalf("local evening of the 14th: %+v", l)
	}
	if l := LimitAt(limits, evening.UTC()); l == nil || l.Value != 20 {
		t.Fatalf("the same instant read in UTC: %+v", l)
	}
	d := deployment(limits, nil)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, la)
	end := time.Date(2026, 9, 30, 23, 59, 59, 0, la)
	all := append(transits("inbound", d, evening.Add(-3*time.Hour), repeat(22, 30)...), transits("inbound", d, time.Date(2026, 9, 16, 9, 0, 0, 0, la), repeat(22, 30)...)...)
	segs := BuildDirections(all, start, end, "mph", 5, 70).Directions[0].Segments
	if len(segs) != 2 || segs[0].Limit.Value != 25 || segs[0].Count != 30 || !segs[1].From.Equal(time.Date(2026, 9, 15, 0, 0, 0, 0, la)) {
		t.Fatalf("segments %+v", segs)
	}
}

func TestLimitsStatutoryOperatorEndedAndSuppressed(t *testing.T) {
	stat := db.DirectionLimit{Value: 25, Unit: "mph", Source: "statutory"}
	if LimitLabel(&stat) != "25 mph (default, no sign recorded)" || LimitLabel(&db.DirectionLimit{Value: 30, Unit: "km/h", Source: "operator"}) != "30 km/h (operator-supplied)" || LimitLabel(nil) != "no limit recorded" {
		t.Fatal("labels")
	}
	ended := []db.DirectionLimit{{Value: 25, Unit: "mph", Source: "sign", EffectiveFrom: "2026-01-01", EffectiveTo: "2026-06-30"}}
	if LimitAt(ended, time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)) == nil || LimitAt(ended, time.Date(2026, 7, 1, 0, 0, 1, 0, time.UTC)) != nil {
		t.Fatal("end date")
	}
	d := deployment([]db.DirectionLimit{stat}, nil)
	d.FootprintSpansLimitChange = true
	set := BuildDirections(transits("inbound", d, sep1, repeat(22, 10)...), sep1, sep30, "mph", 5, 70)
	seg := set.Directions[0].Segments[0]
	if seg.Suppressed == "" || seg.OverLimit != nil {
		t.Fatalf("suppressed compliance: %+v", seg)
	}
	// No config: the transit has a sign but the deployment names no direction.
	none := BuildDirections([]db.DirectionalTransit{{StartUnix: 1, SpeedMPS: 9, Direction: "inbound"}}, sep1, sep30, "mph", 5, 70)
	if len(none.Directions) != 0 || none.Unknown != 1 {
		t.Fatalf("no deployment directions: %+v", none)
	}
}

func TestSharesAreInTheSignsUnit(t *testing.T) {
	d := deployment([]db.DirectionLimit{{Value: 40, Unit: "km/h", Source: "sign"}}, nil)
	// 45 km/h is 27.96 mph: over a 40 km/h sign by 5.
	var ts []db.DirectionalTransit
	for i := 0; i < 10; i++ {
		ts = append(ts, db.DirectionalTransit{StartUnix: float64(sep1.Unix() + int64(i)), SpeedMPS: 45 / 3.6, Direction: "inbound", Directions: d})
	}
	seg := BuildDirections(ts, sep1, sep30, "mph", 5, 70).Directions[0].Segments[0]
	if *seg.OverLimit != 100 || *seg.Over5 != 0 {
		t.Fatalf("shares over a km/h sign: %v, %v", *seg.OverLimit, *seg.Over5)
	}
}

// directionMockDB answers the direction query as well as the rollups.
type directionMockDB struct {
	*mockDB
	transits []db.DirectionalTransit
	asked    int
}

func (m *directionMockDB) TransitDirections(startUnix, endUnix int64, siteID int, modelVersion string, minSpeed float64) ([]db.DirectionalTransit, error) {
	m.asked++
	return m.transits, nil
}

func TestTheReportCarriesItsDirectionsAndTheMirroredChart(t *testing.T) {
	useMockTypstBinary(t)
	june2 := time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC)
	d := deployment([]db.DirectionLimit{sign(25, "")}, []db.DirectionLimit{{Value: 25, Unit: "mph", Source: "statutory"}})
	m := &directionMockDB{mockDB: &mockDB{}, transits: append(transits("inbound", d, june2, repeat(22, 60)...), transits("outbound", d, june2, repeat(27, 8)...)...)}
	m.transits = append(m.transits, db.DirectionalTransit{StartUnix: float64(june2.Unix()), SpeedMPS: 9, Direction: "mixed", Directions: d})
	cfg := baseTypstConfig(t.TempDir())
	res, err := GenerateTypst(context.Background(), m, cfg)
	if err != nil {
		t.Fatal(err)
	}
	rd, files := readReportDataFromZip(t, res.ZIPPath)
	if m.asked != 1 || len(rd.Directions) != 2 {
		t.Fatalf("asked %d, directions %+v", m.asked, rd.Directions)
	}
	if rd.Directions[0].Name != "Kirkham St eastbound" || rd.Directions[0].Segments[0].Limit != "25 mph (sign)" || rd.Directions[0].Segments[0].P98 == nil {
		t.Fatalf("eastbound %+v", rd.Directions[0])
	}
	west := rd.Directions[1].Segments[0]
	if west.Limit != "25 mph (default, no sign recorded)" || west.Count != 8 || west.P98 != nil || west.MaxSpeed != nil {
		t.Fatalf("westbound %+v", west)
	}
	if rd.Charts.Directions == "" || !strings.Contains(rd.DirectionsNote, "1 transits whose readings disagree") {
		t.Fatalf("chart %q note %q", rd.Charts.Directions, rd.DirectionsNote)
	}
	if _, ok := files["charts/directions.svg"]; !ok {
		t.Fatalf("no mirrored chart in %v", files)
	}

	// Another source has no directions and asks for none.
	m2 := &directionMockDB{mockDB: &mockDB{}, transits: m.transits}
	cfg2 := baseTypstConfig(t.TempDir())
	cfg2.Source = "radar_objects"
	res2, err := GenerateTypst(context.Background(), m2, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	rd2, _ := readReportDataFromZip(t, res2.ZIPPath)
	if m2.asked != 0 || len(rd2.Directions) != 0 {
		t.Fatalf("radar_objects report: asked %d, %d directions", m2.asked, len(rd2.Directions))
	}
}
