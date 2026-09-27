package chart

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"testing"
)

// svgElement is one parsed element: its name, attributes and direct text.
type svgElement struct {
	name  string
	attrs map[string]string
	text  string
}

func parseSVG(t *testing.T, svg []byte) []svgElement {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var out []svgElement
	var stack []int
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse svg: %v", err)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			e := svgElement{name: tk.Name.Local, attrs: map[string]string{}}
			for _, a := range tk.Attr {
				e.attrs[a.Name.Local] = a.Value
			}
			out = append(out, e)
			stack = append(stack, len(out)-1)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				out[stack[len(stack)-1]].text += string(tk)
			}
		}
	}
	return out
}

func textsWithClass(els []svgElement, class string) []string {
	var out []string
	for _, e := range els {
		if e.name == "text" && e.attrs["class"] == class {
			out = append(out, e.text)
		}
	}
	return out
}

func hasText(els []svgElement, want string) bool {
	for _, e := range els {
		if e.name == "text" && e.text == want {
			return true
		}
	}
	return false
}

func sampleDistribution() FollowingDistributionData {
	return FollowingDistributionData{
		Status: "SYNTHETIC ORACLE",
		Metric: "interaction.following_net_time_gap_s",
		Unit:   "s",
		Bins: []DistributionBin{
			{Lower: 0, Upper: 0.5, Share: 0},
			{Lower: 0.5, Upper: 1, Share: 0.25},
			{Lower: 1, Upper: 1.5, Share: 0.25},
			{Lower: 1.5, Upper: 2, Share: 0.2},
			{Lower: 2, Overflow: true, Share: 0.1},
		},
		Thresholds: []Threshold{{Value: 2, Label: "2 s"}, {Value: 1.5, Label: "1.5 s"}, {Value: 1, Label: "1 s"}},
		Excluded:   []ExcludedShare{{Label: "not_observed", Share: 0.15}, {Label: "below_speed_floor", Share: 0.05}},
	}
}

func TestRenderFollowingDistributionDrawsBinsThresholdsAndExcludedShare(t *testing.T) {
	d := sampleDistribution()
	svg, err := RenderFollowingDistribution(d, DefaultFollowingDistributionStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderFollowingDistribution: %v", err)
	}
	els := parseSVG(t, svg)

	var bars, excluded, rules int
	for _, e := range els {
		switch {
		case e.name == "rect" && strings.Contains(e.attrs["fill"], ColourFollowingTimeGap):
			bars++
		case e.name == "rect" && e.attrs["fill"] == ColourFollowingSuppressed:
			excluded++
		case e.name == "line" && e.attrs["stroke"] == ColourFollowingThreshold:
			rules++
		}
	}
	if bars != 4 {
		t.Errorf("bars = %d, want 4 (one per non-empty bin; an empty bin draws nothing)", bars)
	}
	if excluded != 2 {
		t.Errorf("excluded columns = %d, want 2", excluded)
	}
	if rules != 3 {
		t.Errorf("threshold rules = %d, want 3", rules)
	}
	if got := textsWithClass(els, TextClassVocab); strings.Join(got, ",") !=
		"interaction.following_net_time_gap_s,not_observed,below_speed_floor" {
		t.Errorf("vocab texts = %v", got)
	}
	if got := textsWithClass(els, TextClassStatus); len(got) != 1 || got[0] != "SYNTHETIC ORACLE" {
		t.Errorf("status texts = %v, want the status once", got)
	}
	for _, want := range []string{"25.0%", "15.0%", "5.0%", "2+", "suppressed", "2 s", "1.5 s"} {
		if !hasText(els, want) {
			t.Errorf("chart has no text %q", want)
		}
	}
}

func TestRenderFollowingDistributionWithNoTimeSaysSo(t *testing.T) {
	d := sampleDistribution()
	for i := range d.Bins {
		d.Bins[i].Share = 0
	}
	d.Excluded = nil
	svg, err := RenderFollowingDistribution(d, DefaultFollowingDistributionStyle(PaperLetter))
	if err != nil {
		t.Fatalf("RenderFollowingDistribution: %v", err)
	}
	els := parseSVG(t, svg)
	if !hasText(els, "No accounted time") {
		t.Error("an empty distribution must say so")
	}
	if len(textsWithClass(els, TextClassStatus)) != 1 {
		t.Error("an empty distribution still carries its status")
	}
}

func TestRenderFollowingDistributionRejectsBadInput(t *testing.T) {
	style := DefaultFollowingDistributionStyle(PaperA4)
	cases := map[string]func(*FollowingDistributionData){
		"no status":        func(d *FollowingDistributionData) { d.Status = "" },
		"no metric":        func(d *FollowingDistributionData) { d.Metric = "" },
		"no bins":          func(d *FollowingDistributionData) { d.Bins = nil },
		"gap between bins": func(d *FollowingDistributionData) { d.Bins[2].Lower = 1.1 },
		"overflow not last": func(d *FollowingDistributionData) {
			d.Bins[1].Overflow = true
		},
		"shares not whole":  func(d *FollowingDistributionData) { d.Excluded[0].Share = 0.2 },
		"share over one":    func(d *FollowingDistributionData) { d.Bins[1].Share = 1.5 },
		"threshold outside": func(d *FollowingDistributionData) { d.Thresholds[0].Value = 9 },
		"unlabelled excluded": func(d *FollowingDistributionData) {
			d.Excluded[0].Label = ""
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := sampleDistribution()
			mutate(&d)
			if _, err := RenderFollowingDistribution(d, style); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// sampleEncounter is a pair observed for 1.5 s, hidden for 0.5 s (predicted
// only), then observed again for 1.9 s.
func sampleEncounter() FollowingEncounterData {
	d := FollowingEncounterData{
		Status:              "SYNTHETIC ORACLE",
		Title:               "E2 trk_leader -> trk_follower",
		GapMetric:           "interaction.following_spatial_gap_m",
		TimeGapMetric:       "interaction.following_net_time_gap_s",
		PredictedMetric:     "interaction.following_predicted_gap_m",
		PredictedVisibility: "review_only",
		DurationS:           3.9,
		BreakAfterS:         0.15,
		Spans:               []FollowingSpan{{Start: 1.5, End: 1.8, Label: "not_observed"}, {Start: 1.8, End: 2.0, Label: "model_degraded"}},
		Thresholds:          []Threshold{{Value: 2, Label: "2 s"}, {Value: 1.5, Label: "1.5 s"}, {Value: 1, Label: "1 s"}},
	}
	for k := 0; k < 40; k++ {
		ts := float64(k) / 10
		if k >= 15 && k <= 19 {
			d.Predicted = append(d.Predicted, FollowingPredictedPoint{T: ts, Value: 15.75, Sigma: 0.25 + 0.025*float64(k-14), CoastAgeS: float64(k-14) / 10})
			continue
		}
		d.Gap = append(d.Gap, FollowingSeriesPoint{T: ts, Value: 15.75, Sigma: 0.25})
		d.TimeGap = append(d.TimeGap, FollowingSeriesPoint{T: ts, Value: 1.575, Sigma: 0.04})
	}
	return d
}

func polylineXs(t *testing.T, points string) (lo, hi float64) {
	t.Helper()
	lo, hi = 1e18, -1e18
	for _, p := range strings.Fields(points) {
		xy := strings.SplitN(p, ",", 2)
		x, err := strconv.ParseFloat(xy[0], 64)
		if err != nil {
			t.Fatalf("parse point %q: %v", p, err)
		}
		lo, hi = min(lo, x), max(hi, x)
	}
	return lo, hi
}

func TestRenderFollowingEncounterNeverBridgesUnsupportedTime(t *testing.T) {
	svg, err := RenderFollowingEncounter(sampleEncounter(), DefaultFollowingEncounterStyle(PaperA4))
	if err != nil {
		t.Fatalf("RenderFollowingEncounter: %v", err)
	}
	els := parseSVG(t, svg)

	var spans [][2]float64
	for _, e := range els {
		if e.name == "rect" && e.attrs["fill"] == ColourFollowingSuppressed && e.attrs["fill-opacity"] == "0.6" {
			x, _ := strconv.ParseFloat(e.attrs["x"], 64)
			w, _ := strconv.ParseFloat(e.attrs["width"], 64)
			if w > 12.5 { // the legend swatch is 12 px
				spans = append(spans, [2]float64{x, x + w})
			}
		}
	}
	if len(spans) != 4 { // two intervals, shaded on both panels
		t.Fatalf("shaded spans = %d, want 4", len(spans))
	}

	var observed, timeGap, predicted int
	for _, e := range els {
		if e.name != "polyline" {
			continue
		}
		lo, hi := polylineXs(t, e.attrs["points"])
		switch {
		case e.attrs["stroke"] == ColourFollowingObserved:
			observed++
		case e.attrs["stroke"] == ColourFollowingTimeGap:
			timeGap++
		case e.attrs["stroke"] == ColourFollowingPredicted:
			predicted++
			if e.attrs["stroke-dasharray"] == "" {
				t.Error("the predicted gap must be dashed")
			}
			continue
		}
		for _, s := range spans {
			if lo < s[0] && hi > s[1] {
				t.Errorf("a %s line spans the unsupported interval %v", e.attrs["stroke"], s)
			}
		}
	}
	if observed != 2 || timeGap != 2 {
		t.Errorf("observed lines = %d, time-gap lines = %d; each must break into two runs", observed, timeGap)
	}
	if predicted != 1 {
		t.Errorf("predicted lines = %d, want 1", predicted)
	}

	var hollow int
	for _, e := range els {
		if e.name == "circle" && e.attrs["fill"] == "white" && e.attrs["stroke"] == ColourFollowingPredicted {
			hollow++
		}
	}
	if hollow != 5 {
		t.Errorf("hollow predicted markers = %d, want 5", hollow)
	}
	if !hasText(els, "coast age 0.1 to 0.5 s") {
		t.Error("the predicted run must state its coast age")
	}
	if got := textsWithClass(els, TextClassRef); len(got) != 1 || got[0] != "E2 trk_leader -> trk_follower" {
		t.Errorf("ref texts = %v", got)
	}
	want := []string{
		"interaction.following_spatial_gap_m", "interaction.following_predicted_gap_m", "review_only",
		"interaction.following_net_time_gap_s", "not_observed", "model_degraded",
	}
	if got := textsWithClass(els, TextClassVocab); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("vocab texts = %v, want %v", got, want)
	}
}

func TestRenderFollowingEncounterWithoutSupportedValuesSaysSo(t *testing.T) {
	d := sampleEncounter()
	d.Gap, d.TimeGap, d.Predicted = nil, nil, nil
	d.Spans = []FollowingSpan{{Start: 0, End: 2, Label: "ambiguous_leader"}}
	svg, err := RenderFollowingEncounter(d, DefaultFollowingEncounterStyle(PaperLetter))
	if err != nil {
		t.Fatalf("RenderFollowingEncounter: %v", err)
	}
	els := parseSVG(t, svg)
	n := 0
	for _, e := range els {
		if e.name == "text" && e.text == "no supported value" {
			n++
		}
		if e.name == "polyline" {
			t.Error("no line may be drawn without a supported value")
		}
	}
	if n != 2 {
		t.Errorf("'no supported value' appears %d times, want once per panel", n)
	}
}

func TestRenderFollowingEncounterRejectsBadInput(t *testing.T) {
	style := DefaultFollowingEncounterStyle(PaperA4)
	cases := map[string]func(*FollowingEncounterData){
		"no status":         func(d *FollowingEncounterData) { d.Status = "" },
		"no title":          func(d *FollowingEncounterData) { d.Title = "" },
		"no visibility":     func(d *FollowingEncounterData) { d.PredictedVisibility = "" },
		"no break interval": func(d *FollowingEncounterData) { d.BreakAfterS = 0 },
		"unordered gap":     func(d *FollowingEncounterData) { d.Gap[1].T = d.Gap[0].T },
		"negative sigma":    func(d *FollowingEncounterData) { d.TimeGap[0].Sigma = -1 },
		"unlabelled span":   func(d *FollowingEncounterData) { d.Spans[0].Label = "" },
		"reversed span":     func(d *FollowingEncounterData) { d.Spans[0].End = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := sampleEncounter()
			mutate(&d)
			if _, err := RenderFollowingEncounter(d, style); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestFollowingChartsAreDeterministic(t *testing.T) {
	a, err := RenderFollowingEncounter(sampleEncounter(), DefaultFollowingEncounterStyle(PaperA4))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RenderFollowingEncounter(sampleEncounter(), DefaultFollowingEncounterStyle(PaperA4))
	if !bytes.Equal(a, b) {
		t.Error("encounter chart differs between identical renders")
	}
	c, err := RenderFollowingDistribution(sampleDistribution(), DefaultFollowingDistributionStyle(PaperA4))
	if err != nil {
		t.Fatal(err)
	}
	d, _ := RenderFollowingDistribution(sampleDistribution(), DefaultFollowingDistributionStyle(PaperA4))
	if !bytes.Equal(c, d) {
		t.Error("distribution chart differs between identical renders")
	}
}

func TestFormatAxisNumberAndOnStep(t *testing.T) {
	for v, want := range map[float64]string{0: "0", 0.30000000000000004: "0.3", 1.5: "1.5", 2: "2", -0.0000000001: "0"} {
		if got := formatAxisNumber(v); got != want {
			t.Errorf("formatAxisNumber(%v) = %q, want %q", v, got, want)
		}
	}
	if !onStep(1.5, 0.5) || onStep(1.25, 0.5) || onStep(1, 0) {
		t.Error("onStep misjudged a multiple")
	}
}

func TestCanvasPolygon(t *testing.T) {
	c := NewCanvas(10, 10)
	c.Polygon(nil, "")
	c.Polygon([][2]float64{{0, 0}, {1, 0}, {1, 1}}, `fill="red"`)
	out := string(c.Bytes())
	if strings.Count(out, "<polygon") != 1 || !strings.Contains(out, `points="0.0000,0.0000 1.0000,0.0000 1.0000,1.0000" fill="red"`) {
		t.Errorf("polygon output = %s", out)
	}
}
