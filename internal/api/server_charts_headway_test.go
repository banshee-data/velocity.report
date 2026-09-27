package api

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// svgText is the text an SVG shows: its lines, the vocabulary runs among
// them, and the classes of the groups drawn. The embedded font is data, not
// text, and is never read.
type svgText struct {
	lines, vocab, groups []string
}

func readSVGText(t *testing.T, svg []byte) svgText {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var out svgText
	var line, run *strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch x := tok.(type) {
		case xml.StartElement:
			class := ""
			for _, a := range x.Attr {
				if a.Name.Local == "class" {
					class = a.Value
				}
			}
			switch x.Name.Local {
			case "g":
				if class != "" {
					out.groups = append(out.groups, class)
				}
			case "text":
				line = &strings.Builder{}
				if class == "vocab" {
					run = &strings.Builder{}
				}
			case "tspan":
				if class == "vocab" {
					run = &strings.Builder{}
				}
			}
		case xml.CharData:
			if line != nil {
				line.Write(x)
			}
			if run != nil {
				run.Write(x)
			}
		case xml.EndElement:
			switch x.Name.Local {
			case "tspan", "text":
				if run != nil {
					out.vocab = append(out.vocab, run.String())
					run = nil
				}
				if x.Name.Local == "text" && line != nil {
					out.lines = append(out.lines, line.String())
					line = nil
				}
			}
		}
	}
	return out
}

func getChart(t *testing.T, server *Server, q url.Values) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/charts/histogram?"+q.Encode(), nil)
	w := httptest.NewRecorder()
	server.ServeMux().ServeHTTP(w, req)
	if w.Code == http.StatusOK && w.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("content type %q", w.Header().Get("Content-Type"))
	}
	return w.Code, w.Body.Bytes()
}

// checkServedChart holds a served chart to the surface rules: the status
// label drawn, every vocabulary run a registered name, and no verdict word
// in any text.
func checkServedChart(t *testing.T, svg []byte, status string) svgText {
	t.Helper()
	text := readSVGText(t, svg)
	for _, line := range text.lines {
		if verdictWords.MatchString(line) {
			t.Errorf("chart text %q contains a verdict word", line)
		}
	}
	found := false
	for _, line := range text.lines {
		found = found || line == status
	}
	if !found {
		t.Errorf("status %q not drawn; lines %q", status, text.lines)
	}
	for _, token := range text.vocab {
		_, metric := l8behaviour.LookupMetric(l8behaviour.MetricID(token))
		_, reasonErr := l8behaviour.ParseSuppressionReason(token)
		_, benchmarkErr := l8behaviour.ParseBenchmarkKind(token)
		if !metric && reasonErr != nil && benchmarkErr != nil {
			t.Errorf("chart draws %q as vocabulary, which no registry defines", token)
		}
	}
	return text
}

func TestChartHistogramDrawsASceneHeadway(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	f := seedHeadway(t, dbInst)
	direct, err := l8behaviour.AggregateFollowing(f.final)
	if err != nil {
		t.Fatal(err)
	}

	code, svg := getChart(t, server, url.Values{"kind": {"headway"}, "scene": {"headway-a"}})
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, svg)
	}
	text := checkServedChart(t, svg, l8behaviour.StatusSyntheticOracle.Label())
	want := map[string]bool{string(l8behaviour.MetricFollowingNetTimeGap): true, "no_established_threshold": true}
	for _, r := range direct.ExcludedReasons() {
		want[r.String()] = true
	}
	got := map[string]bool{}
	for _, v := range text.vocab {
		got[v] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("chart does not draw %q; vocabulary %v", w, text.vocab)
		}
	}
	for _, g := range []string{"headway-bins", "headway-reach", "headway-bands", "headway-excluded", "status"} {
		if !strings.Contains(strings.Join(text.groups, " "), g) {
			t.Errorf("chart has no %s group", g)
		}
	}

	// The spatial gap has no bands; a metric outside the distribution, or an
	// alias of one, is refused.
	code, svg = getChart(t, server, url.Values{"kind": {"headway"}, "scene": {"headway-a"},
		"metric": {string(l8behaviour.MetricFollowingSpatialGap)}})
	if code != http.StatusOK || strings.Contains(strings.Join(checkServedChart(t, svg, "PROVISIONAL · SYNTHETIC ORACLE").groups, " "), "headway-bands") {
		t.Fatalf("spatial gap chart: %d", code)
	}
	for _, metric := range []string{"thw", string(l8behaviour.MetricFollowingValidTime)} {
		if code, _ := getChart(t, server, url.Values{"kind": {"headway"}, "scene": {"headway-a"}, "metric": {metric}}); code != http.StatusBadRequest {
			t.Errorf("metric %s: %d", metric, code)
		}
	}
}

// A selection with nothing to draw is a labelled message chart; a malformed
// one is a JSON error, as for the scene API.
func TestChartHistogramHeadwayStates(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	seedHeadway(t, dbInst)

	for name, c := range map[string]struct {
		q    url.Values
		line string
	}{
		"no window":      {url.Values{"scene": {"headway-none"}}, "This scene has no capture window"},
		"no encounters":  {url.Values{"scene": {"headway-empty"}}, "No following encounters are stored"},
		"two sources":    {url.Values{"scene": {"headway-twin"}}, "2 analyses cover this capture window"},
		"no online data": {url.Values{"scene": {"headway-a"}, "stage": {"online"}}, "No following analysis at stage online"},
	} {
		c.q.Set("kind", "headway")
		code, svg := getChart(t, server, c.q)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, code, svg)
		}
		text := checkServedChart(t, svg, "PROVISIONAL")
		if !strings.Contains(strings.Join(text.lines, "\n"), c.line) {
			t.Errorf("%s: lines %q", name, text.lines)
		}
	}
	for name, c := range map[string]struct {
		q    url.Values
		code int
	}{
		"no scene":      {url.Values{"kind": {"headway"}}, http.StatusBadRequest},
		"bad scene id":  {url.Values{"kind": {"headway"}, "scene": {"Not/A/Scene"}}, http.StatusBadRequest},
		"unknown scene": {url.Values{"kind": {"headway"}, "scene": {"no-such-scene"}}, http.StatusNotFound},
		"bad stage":     {url.Values{"kind": {"headway"}, "scene": {"headway-a"}, "stage": {"latest"}}, http.StatusBadRequest},
	} {
		if code, body := getChart(t, server, c.q); code != c.code || !strings.Contains(string(body), `"error"`) {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	// Any other kind is the speed histogram, exactly as before: it still
	// asks for a site.
	if code, body := getChart(t, server, url.Values{"kind": {"speed"}, "scene": {"headway-a"}}); code != http.StatusBadRequest ||
		!strings.Contains(string(body), "site_id") {
		t.Fatalf("kind=speed: %d %s", code, body)
	}
}
