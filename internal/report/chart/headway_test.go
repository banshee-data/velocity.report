package chart

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the chart golden files under testdata")

func f64(v float64) *float64 { return &v }

// headwayTestData is a net time gap distribution shaped like a pooled
// scene: thirteen 0.25 s bins with an open last bin, three band rules,
// five excluded reasons, two of them wholly predicted-only, and a record
// gap. Its bins and excluded time add up to the accounted time.
func headwayTestData() HeadwayHistogramData {
	nanos := []int64{0, 0, 0, 800e6, 1200e6, 1000e6, 1400e6, 1000e6, 600e6, 300e6, 100e6, 0, 0}
	var bins []HeadwayBin
	for k, n := range nanos {
		b := HeadwayBin{Lower: float64(k) * 0.25, Nanos: n, SigmaInsideNanos: n / 2, SigmaOverlapNanos: n + n/2}
		if k < len(nanos)-1 {
			b.Upper = f64(float64(k+1) * 0.25)
		}
		bins = append(bins, b)
	}
	excluded := []HeadwayExcluded{
		{Reason: "model_degraded", Nanos: 200e6, PredictedOnlyNanos: 200e6},
		{Reason: "not_observed", Nanos: 300e6, PredictedOnlyNanos: 300e6},
		{Reason: "no_common_path", Nanos: 700e6},
		{Reason: "extent_not_converged", Nanos: 500e6},
		{Reason: "below_speed_floor", Nanos: 1000e6},
	}
	return HeadwayHistogramData{
		StatusLabel: "PROVISIONAL", Metric: "interaction.following_net_time_gap_s", Unit: "s", Bins: bins,
		Bands:         []HeadwayBand{{2, "2.0 s"}, {1.5, "1.5 s"}, {1, "1.0 s"}},
		BandBenchmark: "no_established_threshold", Excluded: excluded,
		AccountedNanos: 6400e6 + 2700e6, RecordGapNanos: 200e6, Encounters: 6, InBins: 6,
	}
}

// headwaySVGElement is one element with its attributes, its text and the classes
// of every group enclosing it.
type headwaySVGElement struct {
	name    string
	attrs   map[string]string
	text    string
	classes []string
}

func (e headwaySVGElement) in(class string) bool {
	for _, c := range e.classes {
		if c == class {
			return true
		}
	}
	return false
}

func (e headwaySVGElement) num(t *testing.T, name string) float64 {
	t.Helper()
	return parseFloatAttr(t, e.attrs, name)
}

func parseHeadwaySVG(t *testing.T, svg []byte) []headwaySVGElement {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var out []headwaySVGElement
	var groups []string
	var open []int // elements open inside the current group, outermost first
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch x := tok.(type) {
		case xml.StartElement:
			attrs := attrsByName(x.Attr)
			if x.Name.Local == "g" {
				groups = append(groups, attrs["class"])
				continue
			}
			out = append(out, headwaySVGElement{name: x.Name.Local, attrs: attrs, classes: append([]string(nil), groups...)})
			open = append(open, len(out)-1)
		case xml.CharData:
			// A tspan's text belongs to it and to its line.
			for _, i := range open {
				out[i].text += string(x)
			}
		case xml.EndElement:
			if x.Name.Local == "g" {
				if len(groups) > 0 {
					groups = groups[:len(groups)-1]
				}
				continue
			}
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	return out
}

func elementsOf(els []headwaySVGElement, name, class string) []headwaySVGElement {
	var out []headwaySVGElement
	for _, e := range els {
		if e.name == name && (class == "" || e.in(class)) {
			out = append(out, e)
		}
	}
	return out
}

// texts returns the text of every line, or with a class, of every line or
// run carrying it or inside a group carrying it.
func texts(els []headwaySVGElement, class string) []string {
	var out []string
	for _, e := range els {
		switch {
		case class == "" && e.name == "text",
			class != "" && (e.name == "text" || e.name == "tspan") && (e.attrs["class"] == class || e.in(class)):
			out = append(out, strings.TrimSpace(e.text))
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Every bar is a share of the accounted time, so bar heights stand in the
// ratio of their times, bins and excluded columns alike, and the excluded
// columns stack their predicted-only part above their observed part.
func TestRenderHeadwayHistogramDrawsSharesOfAccountedTime(t *testing.T) {
	data := headwayTestData()
	svg, err := RenderHeadwayHistogram(data, DefaultHeadwayHistogramStyle(PaperA4))
	if err != nil {
		t.Fatal(err)
	}
	els := parseHeadwaySVG(t, svg)
	bars := elementsOf(els, "rect", "headway-bins")
	if len(bars) != len(data.Bins) {
		t.Fatalf("%d bin bars, want %d", len(bars), len(data.Bins))
	}
	// Heights per nanosecond agree across every drawn bar.
	perNano := bars[4].num(t, "height") / float64(data.Bins[4].Nanos)
	for i, b := range bars {
		if got, want := b.num(t, "height"), perNano*float64(data.Bins[i].Nanos); math.Abs(got-want) > 1e-3 {
			t.Errorf("bin %d height %.4f, want %.4f", i, got, want)
		}
	}
	columns := elementsOf(els, "rect", "headway-excluded")
	predictedColumns := 0
	for _, x := range data.Excluded {
		if x.PredictedOnlyNanos > 0 {
			predictedColumns++
		}
	}
	if len(columns) != len(data.Excluded)+predictedColumns {
		t.Fatalf("%d excluded rects, want %d", len(columns), len(data.Excluded)+predictedColumns)
	}
	var height float64
	for _, r := range columns {
		height += r.num(t, "height")
	}
	var excluded int64
	for _, x := range data.Excluded {
		excluded += x.Nanos
	}
	if math.Abs(height-perNano*float64(excluded)) > 1e-2 {
		t.Fatalf("excluded columns %.4f px, want %.4f", height, perNano*float64(excluded))
	}
	// One whisker (a stem and two ticks) per bin with any reach.
	reach := 0
	for _, b := range data.Bins {
		if b.SigmaOverlapNanos > b.SigmaInsideNanos {
			reach++
		}
	}
	if got := len(elementsOf(els, "line", "headway-reach")); got != 3*reach {
		t.Fatalf("%d reach lines, want %d", got, 3*reach)
	}
}

// Bands are neutral dashed rules labelled with the threshold only, at the
// bin edge equal to it.
func TestRenderHeadwayHistogramDrawsBandRulesAtEdges(t *testing.T) {
	data := headwayTestData()
	svg, err := RenderHeadwayHistogram(data, DefaultHeadwayHistogramStyle(PaperA4))
	if err != nil {
		t.Fatal(err)
	}
	els := parseHeadwaySVG(t, svg)
	rules := elementsOf(els, "line", "headway-bands")
	if len(rules) != len(data.Bands) {
		t.Fatalf("%d band rules, want %d", len(rules), len(data.Bands))
	}
	bars := elementsOf(els, "rect", "headway-bins")
	for i, band := range data.Bands {
		r := rules[i]
		if r.attrs["stroke"] != "black" || r.attrs["stroke-dasharray"] == "" {
			t.Errorf("band %s is not a neutral dashed rule: %v", band.Label, r.attrs)
		}
		edge := int(band.Threshold / 0.25)
		slot := bars[1].num(t, "x") - bars[0].num(t, "x")
		left := bars[edge].num(t, "x") - (slot-bars[edge].num(t, "width"))/2
		if math.Abs(r.num(t, "x1")-left) > 1e-3 {
			t.Errorf("band %s at x %.4f, want the edge at %.4f", band.Label, r.num(t, "x1"), left)
		}
	}
	if labels := texts(els, "headway-bands"); !equalStrings(labels, []string{"2.0 s", "1.5 s", "1.0 s"}) {
		t.Fatalf("band labels %v", labels)
	}
	data.Bands = append(data.Bands, HeadwayBand{Threshold: 1.1, Label: "1.1 s"})
	svg, _ = RenderHeadwayHistogram(data, DefaultHeadwayHistogramStyle(PaperA4))
	if got := len(elementsOf(parseHeadwaySVG(t, svg), "line", "headway-bands")); got != 3 {
		t.Fatalf("a threshold between edges was drawn: %d rules", got)
	}

	// A metric with no bands (the spatial gap) draws no band group and no
	// band note.
	data.Bands, data.Metric, data.Unit = nil, "interaction.following_spatial_gap_m", "m"
	svg, _ = RenderHeadwayHistogram(data, DefaultHeadwayHistogramStyle(PaperA4))
	if strings.Contains(string(svg), "headway-bands") || strings.Contains(string(svg), "Band rules") {
		t.Fatal("a chart without bands drew band rules or their note")
	}
}

func equalStrings(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

// The chart invents no names: the metric id, the reasons and the band
// benchmark are the caller's, verbatim, and the status is boxed.
func TestRenderHeadwayHistogramDrawsCallerNamesAndStatus(t *testing.T) {
	data := headwayTestData()
	svg, err := RenderHeadwayHistogram(data, DefaultHeadwayHistogramStyle(PaperLetter))
	if err != nil {
		t.Fatal(err)
	}
	els := parseHeadwaySVG(t, svg)
	vocab := texts(els, "vocab")
	want := []string{data.Metric, data.BandBenchmark}
	for _, x := range data.Excluded {
		want = append(want, x.Reason)
	}
	for _, w := range want {
		if !contains(vocab, w) {
			t.Errorf("vocabulary %q is not drawn verbatim; drawn: %v", w, vocab)
		}
	}
	if len(vocab) != len(want) {
		t.Errorf("drawn vocabulary %v, want exactly %v", vocab, want)
	}
	if status := texts(els, "status"); len(status) != 1 || status[0] != "PROVISIONAL" {
		t.Fatalf("status %v", status)
	}
	all := strings.Join(texts(els, ""), "\n")
	for _, w := range []string{"6 encounters, 6 in the bins", "9.1 s accounted", "70% in the bins", "30% beside them",
		"5.5% predicted-only", "Record gaps of 0.2 s", "3+", "1 model_degraded 0.2 s", "5 below_speed_floor 1.0 s",
		"benchmark kind no_established_threshold"} {
		if !strings.Contains(all, w) {
			t.Errorf("chart text lacks %q:\n%s", w, all)
		}
	}
}

// Up to six reasons fit the key's two rows; each further row of three
// grows the canvas rather than running off it.
func TestRenderHeadwayHistogramGrowsForALongKey(t *testing.T) {
	style := DefaultHeadwayHistogramStyle(PaperA4)
	height := func(data HeadwayHistogramData) float64 {
		t.Helper()
		svg, err := RenderHeadwayHistogram(data, style)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`height="([0-9.]+)mm"`).FindSubmatch(svg)
		v, _ := strconv.ParseFloat(string(m[1]), 64)
		return v
	}
	data := headwayTestData()
	base := height(data)
	if math.Abs(base-style.HeightMM) > 1e-3 {
		t.Fatalf("five reasons: %.3f mm, want %.3f", base, style.HeightMM)
	}
	for _, r := range []string{"orientation_unresolved", "trajectory_uncertainty_too_high", "ambiguous_leader"} {
		data.Excluded = append(data.Excluded, HeadwayExcluded{Reason: r, Nanos: 100e6})
		data.AccountedNanos += 100e6
	}
	if got, want := height(data), style.HeightMM+headwayKeyRowPx/pxPerMM; math.Abs(got-want) > 1e-3 {
		t.Fatalf("eight reasons: %.3f mm, want %.3f", got, want)
	}
}

func TestRenderHeadwayHistogramRefusesWhatItCannotDraw(t *testing.T) {
	style := DefaultHeadwayHistogramStyle(PaperA4)
	data := headwayTestData()
	data.StatusLabel = " "
	if _, err := RenderHeadwayHistogram(data, style); !errors.Is(err, ErrHeadwayStatusRequired) {
		t.Fatalf("unlabelled chart: %v", err)
	}
	if _, err := RenderHeadwayMessage("", style, "x"); !errors.Is(err, ErrHeadwayStatusRequired) {
		t.Fatalf("unlabelled message: %v", err)
	}
	data = headwayTestData()
	data.AccountedNanos++
	if _, err := RenderHeadwayHistogram(data, style); err == nil {
		t.Fatal("a chart whose parts do not add up was drawn")
	}
	data = headwayTestData()
	data.Bins = nil
	if _, err := RenderHeadwayHistogram(data, style); err == nil {
		t.Fatal("a chart without bins was drawn")
	}

	// Nothing accounted: a message, still labelled.
	data = headwayTestData()
	for i := range data.Bins {
		data.Bins[i] = HeadwayBin{Lower: data.Bins[i].Lower, Upper: data.Bins[i].Upper}
	}
	data.Excluded, data.AccountedNanos = nil, 0
	svg, err := RenderHeadwayHistogram(data, style)
	if err != nil {
		t.Fatal(err)
	}
	els := parseHeadwaySVG(t, svg)
	if status := texts(els, "status"); len(status) != 1 || status[0] != "PROVISIONAL" ||
		!contains(texts(els, ""), "No following time was accounted in this selection.") ||
		len(elementsOf(els, "rect", "headway-bins")) != 0 {
		t.Fatalf("empty chart texts %v", texts(els, ""))
	}
}

// fontFace is the embedded font every chart carries; golden files elide it.
var fontFace = regexp.MustCompile(`(?m)^<defs><style>@font-face \{[^\n]*</style></defs>$`)

var numberPattern = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

// sameExceptNumbers compares two SVG texts: everything but numbers exactly,
// numbers within tol. Go may fuse a multiply and an add on arm64, which can
// move the fourth decimal of a coordinate against an amd64 golden.
func sameExceptNumbers(want, got []byte, tol float64) string {
	strip := func(b []byte) []byte { return numberPattern.ReplaceAll(b, []byte("#")) }
	if !bytes.Equal(strip(want), strip(got)) {
		w, g := strip(want), strip(got)
		i := 0
		for i < len(w) && i < len(g) && w[i] == g[i] {
			i++
		}
		lo := max(i-40, 0)
		return fmt.Sprintf("text differs near %q", g[lo:min(i+40, len(g))])
	}
	wn, gn := numberPattern.FindAll(want, -1), numberPattern.FindAll(got, -1)
	for i := range wn {
		a, _ := strconv.ParseFloat(string(wn[i]), 64)
		b, _ := strconv.ParseFloat(string(gn[i]), 64)
		if math.Abs(a-b) > tol {
			return fmt.Sprintf("number %d: want %s, got %s", i, wn[i], gn[i])
		}
	}
	return ""
}

// The golden files pin the drawing: a change to the chart is reviewed as a
// diff of them. Regenerate with go test ./internal/report/chart -run Golden
// -update.
func TestRenderHeadwayGolden(t *testing.T) {
	style := DefaultHeadwayHistogramStyle(PaperA4)
	chartSVG, err := RenderHeadwayHistogram(headwayTestData(), style)
	if err != nil {
		t.Fatal(err)
	}
	message, err := RenderHeadwayMessage("PROVISIONAL · SYNTHETIC ORACLE", style,
		"No final-stage following analysis exists for this scene.")
	if err != nil {
		t.Fatal(err)
	}
	for name, svg := range map[string][]byte{
		"headway_histogram.golden.svg": chartSVG,
		"headway_message.golden.svg":   message,
	} {
		got := fontFace.ReplaceAll(svg, []byte("<defs><!-- embedded font elided from the golden file --></defs>"))
		path := filepath.Join("testdata", name)
		if *updateGolden {
			if err := os.MkdirAll("testdata", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (regenerate with go test ./internal/report/chart -run Golden -update)", err)
		}
		if msg := sameExceptNumbers(want, got, 2e-3); msg != "" {
			t.Errorf("%s differs from its golden file: %s\nreview the change, then regenerate with "+
				"go test ./internal/report/chart -run Golden -update", path, msg)
		}
	}
}
