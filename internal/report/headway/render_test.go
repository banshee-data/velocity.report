package headway

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/typst"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

var update = flag.Bool("update", false, "rewrite the oracle golden files under testdata/oracle")

const goldenDir = "testdata/oracle"

// fontFace is the embedded font every chart carries. The goldens elide it:
// it is the same bytes in every chart and would make each file unreviewable.
var fontFace = regexp.MustCompile(`(?m)^<defs><style>@font-face \{[^\n]*</style></defs>$`)

func normaliseSVG(svg []byte) []byte {
	return fontFace.ReplaceAll(svg, []byte("<defs><!-- embedded font elided from the golden file --></defs>"))
}

func assembled(t *testing.T, r Report) (Report, []typst.Asset) {
	t.Helper()
	out, charts, err := Assemble(r, chart.PaperA4)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	return out, charts
}

var numberPattern = regexp.MustCompile(`-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?`)

// diffWithTolerance compares two texts: everything but numbers must match
// exactly, and numbers must agree within abs or rel. The tolerance exists
// for one reason: Go may fuse a multiply and an add on arm64, which moves
// the last bit of a Monte Carlo draw, so an interval bound can differ from
// an amd64 golden by an ulp. Everything else in the oracle is exact binary
// arithmetic and matches digit for digit.
func diffWithTolerance(want, got []byte, abs, rel float64) string {
	wi, gi := numberPattern.FindAllIndex(want, -1), numberPattern.FindAllIndex(got, -1)
	wp, gp := 0, 0
	for k := 0; ; k++ {
		var wEnd, gEnd int
		if k < len(wi) {
			wEnd = wi[k][0]
		} else {
			wEnd = len(want)
		}
		if k < len(gi) {
			gEnd = gi[k][0]
		} else {
			gEnd = len(got)
		}
		if !bytes.Equal(want[wp:wEnd], got[gp:gEnd]) {
			return fmt.Sprintf("text differs near byte %d: want %q, got %q", wp, clip(want[wp:wEnd]), clip(got[gp:gEnd]))
		}
		if k >= len(wi) || k >= len(gi) {
			if len(wi) != len(gi) {
				return fmt.Sprintf("want %d numbers, got %d", len(wi), len(gi))
			}
			return ""
		}
		ws, gs := string(want[wi[k][0]:wi[k][1]]), string(got[gi[k][0]:gi[k][1]])
		if ws != gs {
			a, _ := strconv.ParseFloat(ws, 64)
			b, _ := strconv.ParseFloat(gs, 64)
			if !(math.Abs(a-b) <= math.Max(abs, rel*math.Max(math.Abs(a), math.Abs(b)))) {
				return fmt.Sprintf("number near byte %d: want %s, got %s", wi[k][0], ws, gs)
			}
		}
		wp, gp = wi[k][1], gi[k][1]
	}
}

func clip(b []byte) string {
	if len(b) > 80 {
		return string(b[:80]) + "..."
	}
	return string(b)
}

func compareGolden(t *testing.T, name string, got []byte, abs, rel float64) {
	t.Helper()
	path := filepath.Join(goldenDir, name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with go test ./internal/report/headway -run Golden -update)", path, err)
	}
	if msg := diffWithTolerance(want, got, abs, rel); msg != "" {
		t.Errorf("%s differs from its golden file: %s\n"+
			"review the change, then regenerate with go test ./internal/report/headway -run Golden -update", path, msg)
	}
}

// TestOracleGolden pins the rendered contract: the data.json every template
// and chart reads, and every chart. A change here is a change to what the
// report says, and is reviewed as a diff of these files.
func TestOracleGolden(t *testing.T) {
	r, charts := assembled(t, oracleReport(t))
	data, err := typst.MarshalData(r)
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "data.json", data, 1e-12, 1e-9)

	want := map[string]bool{}
	for _, a := range charts {
		if !fontFace.Match(a.Data) {
			t.Errorf("%s does not embed its font", a.Name)
		}
		want[filepath.Base(a.Name)] = true
		compareGolden(t, a.Name, normaliseSVG(a.Data), 2e-4, 0)
	}
	entries, err := os.ReadDir(filepath.Join(goldenDir, "charts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !want[e.Name()] {
			if *update {
				_ = os.Remove(filepath.Join(goldenDir, "charts", e.Name()))
				continue
			}
			t.Errorf("golden chart %s is no longer rendered", e.Name())
		}
	}
	if len(charts) != len(r.Aggregates)+len(r.Encounters) || len(charts) != 10 {
		t.Errorf("rendered %d charts, want one per aggregate and encounter (10)", len(charts))
	}
}

func TestDiffWithTolerance(t *testing.T) {
	for _, c := range []struct {
		want, got string
		ok        bool
	}{
		{`x="1.0000" y="2.5000"`, `x="1.0001" y="2.5000"`, true},
		{`x="1.0000"`, `x="1.0010"`, false},
		{`id E1`, `id E2`, false},
		{`a 1 b`, `a 1 c`, false},
		{`a 1 2`, `a 1`, false},
		{`hash 2789569b98`, `hash 2789569b99`, false},
	} {
		if got := diffWithTolerance([]byte(c.want), []byte(c.got), 2e-4, 0) == ""; got != c.ok {
			t.Errorf("diff(%q, %q) ok = %v, want %v", c.want, c.got, got, c.ok)
		}
	}
}

// registeredToken reports whether s is a token of any closed vocabulary the
// report prints: the l8behaviour vocabularies, the report status and the
// value-block names.
func registeredToken(s string) bool {
	parsers := []func(string) error{
		func(s string) error { _, err := l8behaviour.ParseSuppressionReason(s); return err },
		func(s string) error { _, err := l8behaviour.ParseSupportState(s); return err },
		func(s string) error { _, err := l8behaviour.ParseEstimateStage(s); return err },
		func(s string) error { _, err := l8behaviour.ParseEstimationState(s); return err },
		func(s string) error { _, err := l8behaviour.ParseEndpointSource(s); return err },
		func(s string) error { _, err := l8behaviour.ParseMotionClass(s); return err },
		func(s string) error { _, err := l8behaviour.ParseReferencePoint(s); return err },
		func(s string) error { _, err := l8behaviour.ParseBeliefProvenance(s); return err },
		func(s string) error { _, err := l8behaviour.ParsePathExtremity(s); return err },
		func(s string) error { _, err := l8behaviour.ParsePathCondition(s); return err },
		func(s string) error { _, err := l8behaviour.ParseCandidateDisposition(s); return err },
		func(s string) error { _, err := l8behaviour.ParseUncertaintyKind(s); return err },
		func(s string) error { _, err := l8behaviour.ParsePropagationMethod(s); return err },
		func(s string) error { _, err := l8behaviour.ParseBenchmarkKind(s); return err },
		func(s string) error { _, err := l8behaviour.ParseVisibility(s); return err },
		func(s string) error { _, err := ParseStatus(s); return err },
	}
	for _, p := range parsers {
		if p(s) == nil {
			return true
		}
	}
	return s == string(ValueBlockMeasurements) || s == string(ValueBlockProvisional)
}

func registeredMetric(s string) bool {
	_, ok := l8behaviour.LookupMetric(l8behaviour.MetricID(s))
	return ok
}

var (
	metricPattern    = regexp.MustCompile(`interaction\.[a-z0-9_]+`)
	snakeCasePattern = regexp.MustCompile(`\b[a-z][a-z0-9]*(?:_[a-z0-9]+)+\b`)
)

// tokenKeys are the data.json fields that hold a vocabulary token, and
// metricKeys the ones that hold a metric id.
var (
	tokenKeys = map[string]bool{
		"reason": true, "rate_reason": true, "condition": true, "path_conditions": true, "estimate_stage": true,
		"support": true, "extremity": true, "role": true, "motion_class": true, "visibility": true,
		"benchmark": true, "duration_benchmark": true, "rate_benchmark": true, "kind": true, "method": true,
		"status": true, "value_block": true,
	}
	metricKeys = map[string]bool{"metric": true, "name": true, "duration_metric": true, "rate_metric": true}
)

// walkJSON visits every string in a decoded JSON value with the key it sits
// under (an array element inherits its array's key) and its parent key.
func walkJSON(v any, key, parent string, visit func(key, parent, s string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			walkJSON(child, k, key, visit)
		}
	case []any:
		for _, child := range x {
			walkJSON(child, key, parent, visit)
		}
	case string:
		visit(key, parent, x)
	}
}

// TestRenderedNamesAreRegistered is the report-side registry check: every
// metric id and vocabulary token the report writes, in data.json, on a
// chart or in the template, is a registered id or token, and the template
// names none itself.
func TestRenderedNamesAreRegistered(t *testing.T) {
	for _, r := range []Report{oracleReport(t), provisionalReport(t)} {
		rendered, charts := assembled(t, r)
		data, err := typst.MarshalData(rendered)
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		walkJSON(doc, "", "", func(key, parent, s string) {
			switch {
			case metricKeys[key] && !registeredMetric(s):
				t.Errorf("%s: %s %q is not a registered metric", r.Status, key, s)
			case tokenKeys[key] && !registeredToken(s):
				t.Errorf("%s: %s %q is not a registered token", r.Status, key, s)
			case key == "source" && (parent == "leader_trailing_sources" || parent == "follower_leading_sources" ||
				parent == "leader" || parent == "follower") && !registeredToken(s):
				t.Errorf("%s: endpoint source %q is not registered", r.Status, s)
			}
			for _, m := range metricPattern.FindAllString(s, -1) {
				if !registeredMetric(m) {
					t.Errorf("%s: %s mentions unregistered metric %q", r.Status, key, m)
				}
			}
			if key == "display" || strings.HasSuffix(key, "_display") {
				for _, tok := range snakeCasePattern.FindAllString(s, -1) {
					if !registeredToken(tok) {
						t.Errorf("%s: %s %q prints unregistered token %q", r.Status, key, s, tok)
					}
				}
			}
		})

		for _, a := range charts {
			for _, txt := range svgTexts(t, a.Data) {
				switch txt.class {
				case chart.TextClassVocab:
					for _, tok := range strings.Fields(txt.text) {
						if !registeredMetric(tok) && !registeredToken(tok) {
							t.Errorf("%s %s: vocabulary text %q is not registered", r.Status, a.Name, tok)
						}
					}
				case chart.TextClassStatus:
					if txt.text != r.StatusLabel {
						t.Errorf("%s: status text %q, want %q", a.Name, txt.text, r.StatusLabel)
					}
				case chart.TextClassRef:
					if !chartRef(r, txt.text) {
						t.Errorf("%s: reference %q names no encounter of the report", a.Name, txt.text)
					}
				default:
					if metricPattern.MatchString(txt.text) || snakeCasePattern.MatchString(txt.text) {
						t.Errorf("%s: unclassed text %q carries an identifier the check cannot see", a.Name, txt.text)
					}
				}
			}
		}
	}

	tpl := headwayTemplate(t)
	var names []string
	for _, d := range l8behaviour.FollowingMetrics() {
		names = append(names, string(d.ID))
	}
	for _, r := range l8behaviour.SuppressionReasons() {
		names = append(names, r.String())
	}
	for _, c := range l8behaviour.PathConditions() {
		names = append(names, c.String())
	}
	for _, s := range Statuses() {
		names = append(names, s.String(), s.Label())
	}
	names = append(names, string(ValueBlockMeasurements), string(ValueBlockProvisional))
	for _, n := range names {
		if strings.Contains(tpl, `"`+n+`"`) || strings.Contains(tpl, "["+n+"]") {
			t.Errorf("headway.typ names %q itself; every name must come from data.json", n)
		}
	}
	if metricPattern.MatchString(tpl) {
		t.Errorf("headway.typ mentions a metric id: %v", metricPattern.FindAllString(tpl, -1))
	}
}

func provisionalReport(t *testing.T) Report {
	t.Helper()
	r, err := Build(fieldInput(t, l8behaviour.StageFixedLag))
	if err != nil {
		t.Fatalf("provisional Build: %v", err)
	}
	return r
}

// chartRef reports whether a chart title names an encounter and its two
// tracks, as encounterChart writes it.
func chartRef(r Report, s string) bool {
	for _, e := range r.Encounters {
		if s == e.ID+" "+e.Leader.TrackID+" -> "+e.Follower.TrackID {
			return true
		}
	}
	return false
}

type svgText struct{ class, text string }

func svgTexts(t *testing.T, svg []byte) []svgText {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var out []svgText
	var cur *svgText
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("parse svg: %v", err)
		}
		switch x := tok.(type) {
		case xml.StartElement:
			if x.Name.Local == "text" {
				cur = &svgText{}
				for _, a := range x.Attr {
					if a.Name.Local == "class" {
						cur.class = a.Value
					}
				}
			}
		case xml.CharData:
			if cur != nil {
				cur.text += string(x)
			}
		case xml.EndElement:
			if x.Name.Local == "text" && cur != nil {
				out = append(out, *cur)
				cur = nil
			}
		}
	}
}

func headwayTemplate(t *testing.T) string {
	t.Helper()
	src, err := typst.SourcesFor(typst.EntryHeadway)
	if err != nil {
		t.Fatal(err)
	}
	return string(src[typst.EntryHeadway])
}

// verdictPattern is language the report must never use (Sections 1 and
// 8.3): no verdict, score, category or trait of a road user, in any form,
// including in negation.
var verdictPattern = regexp.MustCompile(`(?i)tailgat|aggress|driver|risk|score|verdict|unsafe|danger|violat|offend|propensity|profil`)

// TestNoVerdictLanguage scans everything a reader can see: the data, every
// chart's text, the template and the archive README.
func TestNoVerdictLanguage(t *testing.T) {
	for _, r := range []Report{oracleReport(t), provisionalReport(t)} {
		rendered, charts := assembled(t, r)
		data, _ := typst.MarshalData(rendered)
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		walkJSON(doc, "", "", func(key, _, s string) {
			if verdictPattern.MatchString(s) {
				t.Errorf("data.json %s: %q", key, s)
			}
		})
		for _, a := range charts {
			for _, txt := range svgTexts(t, a.Data) {
				if verdictPattern.MatchString(txt.text) {
					t.Errorf("%s: %q", a.Name, txt.text)
				}
			}
		}
	}
	for name, body := range map[string]string{"headway.typ": headwayTemplate(t), "README.md": zipReadme} {
		if m := verdictPattern.FindAllString(body, -1); len(m) > 0 {
			t.Errorf("%s uses %v", name, m)
		}
	}
}

// TestStatusOnEverySurface: the label is on every chart once, in the data,
// and on every page (header, footer, background mark, banner and document
// title all read it from data.json).
func TestStatusOnEverySurface(t *testing.T) {
	for _, r := range []Report{oracleReport(t), provisionalReport(t)} {
		rendered, charts := assembled(t, r)
		if rendered.StatusLabel != rendered.Status.Label() {
			t.Fatalf("assembled report label %q", rendered.StatusLabel)
		}
		for _, a := range charts {
			n := 0
			for _, txt := range svgTexts(t, a.Data) {
				if txt.class == chart.TextClassStatus && txt.text == r.StatusLabel {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%s %s carries its status %d times, want once", r.Status, a.Name, n)
			}
		}
	}
	tpl := headwayTemplate(t)
	if n := strings.Count(tpl, "data.status_label"); n < 5 {
		t.Errorf("headway.typ reads the status label %d times; want the header, footer, background, banner and title", n)
	}
	between := func(from, to string) string {
		i, j := strings.Index(tpl, from), strings.Index(tpl, to)
		if i < 0 || j < i {
			t.Fatalf("headway.typ has no %s ... %s region", from, to)
		}
		return tpl[i:j]
	}
	for name, region := range map[string]string{
		"header":     between("header:", "footer:"),
		"footer":     between("footer:", "background:"),
		"background": between("background:", "#show: apply-styles"),
	} {
		if !strings.Contains(region, "data.status_label") && !strings.Contains(region, "status-box") {
			t.Errorf("headway.typ page %s does not show the status", name)
		}
	}
}

func TestAssembleLeavesTheCallersReportAlone(t *testing.T) {
	r := oracleReport(t)
	rendered, _ := assembled(t, r)
	if r.Encounters[0].Chart != "" || r.Aggregates[0].Chart != "" || r.Paper != "" {
		t.Error("Assemble wrote chart paths into the caller's report")
	}
	if rendered.Encounters[0].Chart != "/charts/encounter_E1.svg" || rendered.Aggregates[0].Chart != "/charts/aggregate_A1.svg" ||
		rendered.Paper != "a4" {
		t.Errorf("assembled paths: %q, %q, paper %q", rendered.Encounters[0].Chart, rendered.Aggregates[0].Chart, rendered.Paper)
	}
	letter, _, err := Assemble(r, chart.PaperLetter)
	if err != nil || letter.Paper != "us-letter" {
		t.Errorf("letter paper = %q (%v)", letter.Paper, err)
	}
	r.Status = StatusPromoted
	if _, _, err := Assemble(r, chart.PaperA4); err == nil {
		t.Error("Assemble must refuse a report that does not validate")
	}
}

// minimalPDF is a structurally valid PDF with an Info dictionary, enough for
// the metadata stamp to update, standing in for typst's output.
func minimalPDF() []byte {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = pdf.Len()
		pdf.WriteString(obj)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	fmt.Fprintf(&pdf, "%010d %05d f \n", 0, 65535)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&pdf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R /Info 3 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.Bytes()
}

func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = body
	}
	return out
}

// TestGenerateWithMockTypst drives Generate end to end with a stand-in typst
// on PATH: the headway entry is compiled, the PDF is stamped with its
// status, and the archive holds exactly the headway template set, the data
// and the charts.
func TestGenerateWithMockTypst(t *testing.T) {
	if typstbin.Embedded() {
		t.Skip("an embedded typst takes precedence over PATH")
	}
	bin := t.TempDir()
	stdin := filepath.Join(bin, "stdin.typ")
	script := "#!/bin/sh\ncat > " + stdin + "\ncat <<'EOF'\n" + string(minimalPDF()) + "EOF\n"
	if err := os.WriteFile(filepath.Join(bin, "typst"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// First on PATH, so it wins over any real typst; the rest of PATH stays
	// for the shell tools the script uses.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(typstbin.EnvNoDownload, "1")

	out := t.TempDir()
	res, err := Generate(oracleReport(t), Options{Paper: chart.PaperA4, OutputDir: out})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if filepath.Base(res.PDFPath) != "headway_synthetic_oracle_report.pdf" ||
		filepath.Base(res.ZIPPath) != "headway_synthetic_oracle_report_sources.zip" {
		t.Errorf("outputs %s and %s are not named for their status", res.PDFPath, res.ZIPPath)
	}
	if got, _ := os.ReadFile(stdin); string(got) != `#include "/headway.typ"` {
		t.Errorf("typst compiled %q, want the headway entry", got)
	}
	pdf, err := os.ReadFile(res.PDFPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("status:synthetic_oracle")) || !bytes.Contains(pdf, []byte("contract:headway_report_v1")) {
		t.Error("the PDF metadata does not carry the status and contract")
	}

	files := readZip(t, res.ZIPPath)
	var names []string
	for name := range files {
		if !strings.HasPrefix(name, "fonts/") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	want := []string{"README.md", "charts/aggregate_A1.svg", "charts/aggregate_A2.svg"}
	for i := 1; i <= 8; i++ {
		want = append(want, fmt.Sprintf("charts/encounter_E%d.svg", i))
	}
	want = append(want, "data.json", "headway.typ", "preamble.typ", "sections.typ")
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("archive holds %v, want %v", names, want)
	}
	rendered, _ := assembled(t, oracleReport(t))
	wantData, _ := typst.MarshalData(rendered)
	if !bytes.Equal(files["data.json"], wantData) {
		t.Error("the archive's data.json is not the rendered report")
	}

	if _, err := Generate(oracleReport(t), Options{}); err == nil {
		t.Error("Generate must require an output directory")
	}
}

// TestGenerateCompilesThePDF compiles the oracle with the real typst, then
// recompiles the source archive on its own, as its README tells a reviewer
// to. It is skipped when typst is neither embedded nor on PATH.
func TestGenerateCompilesThePDF(t *testing.T) {
	t.Setenv(typstbin.EnvNoDownload, "1")
	typstPath, err := exec.LookPath("typst")
	if !typstbin.Embedded() && err != nil {
		t.Skip("typst not embedded or on PATH; run make install-typst and add bin/ to PATH")
	}
	out := t.TempDir()
	res, err := Generate(oracleReport(t), Options{Paper: chart.PaperLetter, OutputDir: out})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	pdf, err := os.ReadFile(res.PDFPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdf) < 50_000 {
		t.Fatalf("PDF is %d bytes starting %q", len(pdf), pdf[:min(8, len(pdf))])
	}
	if typstPath == "" {
		return // embedded only: nothing on PATH to recompile the archive with
	}
	src := t.TempDir()
	for name, body := range readZip(t, res.ZIPPath) {
		p := filepath.Join(src, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(typstPath, "compile", "--font-path", "fonts", "--ignore-system-fonts", "headway.typ", "recompiled.pdf")
	cmd.Dir = src
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the source archive does not recompile: %v\n%s", err, msg)
	}
}
