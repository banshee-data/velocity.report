package api

import (
	"testing"
	"time"
)

func strp(s string) *string   { return &s }
func f64p(f float64) *float64 { return &f }

func TestSceneFromRequestValidation(t *testing.T) {
	valid := sceneRequest{Title: "SoMa 1"}

	cases := []struct {
		name    string
		id      string
		req     sceneRequest
		problem string // substring the message must contain; "" means accepted
	}{
		{"accepts a plain scene", "soma1", valid, ""},
		{"accepts digits and dashes", "s2-sf-2", valid, ""},
		{"rejects uppercase", "SoMa1", valid, "lowercase"},
		{"rejects spaces", "soma 1", valid, "lowercase"},
		{"rejects a path separator", "soma/1", valid, "lowercase"},
		{"rejects a leading dash", "-soma", valid, "lowercase"},
		{"rejects an empty id", "", valid, "lowercase"},
		{"rejects a blank title", "soma1", sceneRequest{Title: "   "}, "Title is required"},
		{
			"rejects latitude without longitude", "soma1",
			sceneRequest{Title: "x", Latitude: f64p(37.7)},
			"together",
		},
		{
			"rejects longitude without latitude", "soma1",
			sceneRequest{Title: "x", Longitude: f64p(-122.4)},
			"together",
		},
		{
			"rejects an out-of-range latitude", "soma1",
			sceneRequest{Title: "x", Latitude: f64p(91), Longitude: f64p(0)},
			"Latitude must be",
		},
		{
			"rejects an out-of-range longitude", "soma1",
			sceneRequest{Title: "x", Latitude: f64p(0), Longitude: f64p(181)},
			"Longitude must be",
		},
		{
			"accepts a complete position", "soma1",
			sceneRequest{Title: "x", Latitude: f64p(37.77493), Longitude: f64p(-122.41942)},
			"",
		},
		{
			"rejects an end before the start", "soma1",
			sceneRequest{
				Title:         "x",
				CapturedStart: strp("2026-09-05T12:00:00Z"),
				CapturedEnd:   strp("2026-09-05T11:00:00Z"),
			},
			"must not be before",
		},
		{
			"rejects an unparseable time", "soma1",
			sceneRequest{Title: "x", CapturedStart: strp("last Tuesday")},
			"not a date and time",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, problem := sceneFromRequest(c.req, c.id)
			if c.problem == "" && problem != "" {
				t.Fatalf("rejected with %q, want acceptance", problem)
			}
			if c.problem != "" && problem == "" {
				t.Fatalf("accepted, want a problem mentioning %q", c.problem)
			}
			if c.problem != "" && !contains(problem, c.problem) {
				t.Errorf("problem %q does not mention %q", problem, c.problem)
			}
		})
	}
}

func TestSceneFromRequestDerivesDuration(t *testing.T) {
	sc, problem := sceneFromRequest(sceneRequest{
		Title:         "x",
		CapturedStart: strp("2026-09-05T12:00:00Z"),
		CapturedEnd:   strp("2026-09-05T12:11:02Z"),
	}, "soma1")
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}
	if sc.DurationSecs == nil {
		t.Fatal("duration not derived from the capture window")
	}
	if got := *sc.DurationSecs; got != 662 {
		t.Errorf("duration %v s, want 662", got)
	}
}

func TestSceneFromRequestLeavesDurationUnsetWithoutBothEnds(t *testing.T) {
	sc, problem := sceneFromRequest(sceneRequest{
		Title: "x", CapturedStart: strp("2026-09-05T12:00:00Z"),
	}, "soma1")
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}
	if sc.DurationSecs != nil {
		t.Errorf("duration = %v with no end; a half-open window has no length", *sc.DurationSecs)
	}
	if sc.CapturedStartNs == nil {
		t.Error("start was dropped")
	}
}

// An editor's datetime-local field emits "YYYY-MM-DDTHH:MM" with no zone or
// seconds; a recording-derived value arrives as RFC 3339. Both must work.
func TestParseSceneTimeAcceptsEditorAndRecordingForms(t *testing.T) {
	want := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC).UnixNano()

	for _, in := range []string{
		"2026-09-05T12:00:00Z",
		"2026-09-05T12:00:00",
		"2026-09-05T12:00",
	} {
		got, problem := parseSceneTime(&in, "start")
		if problem != "" {
			t.Errorf("%q: %s", in, problem)
			continue
		}
		if got == nil || *got != want {
			t.Errorf("%q parsed to %v, want %d", in, got, want)
		}
	}

	// A date alone is a valid, if coarse, answer.
	dateOnly := "2026-09-05"
	if got, problem := parseSceneTime(&dateOnly, "start"); problem != "" || got == nil {
		t.Errorf("date-only value rejected: %s", problem)
	}

	// Blank means "not recorded", not an error.
	blank := "   "
	if got, problem := parseSceneTime(&blank, "start"); problem != "" || got != nil {
		t.Errorf("blank should mean unset, got %v / %q", got, problem)
	}
	if got, problem := parseSceneTime(nil, "start"); problem != "" || got != nil {
		t.Errorf("nil should mean unset, got %v / %q", got, problem)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
