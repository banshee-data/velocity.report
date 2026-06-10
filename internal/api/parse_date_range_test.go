package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// rfc3339Unix is a test helper: parse an RFC3339 string and return its unix sec.
func rfc3339Unix(t *testing.T, s string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad test fixture %q: %v", s, err)
	}
	return parsed.Unix()
}

func TestParseInstant(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string // RFC3339; empty when wantErr
		wantErr bool
	}{
		{"UTC Z form", "2026-06-09T00:00:00Z", "2026-06-09T00:00:00Z", false},
		{"negative offset", "2026-06-09T00:00:00-07:00", "2026-06-09T07:00:00Z", false},
		{"positive offset", "2026-06-09T00:00:00+12:00", "2026-06-08T12:00:00Z", false},
		{"fractional seconds (from JS toISOString)", "2026-06-09T07:00:00.000Z", "2026-06-09T07:00:00Z", false},
		{"bare date is rejected (needs time + offset)", "2026-06-09", "", true},
		{"naive datetime without offset is rejected", "2026-06-09T00:00:00", "", true},
		{"garbage is rejected", "not-a-date", "", true},
		{"unix seconds are rejected", "1717977600", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInstant("start", tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != rfc3339Unix(t, tt.want) {
				t.Errorf("parseInstant(%q) = %d (%s), want %s",
					tt.value, got, time.Unix(got, 0).UTC(), tt.want)
			}
		})
	}
}

func TestParseDateRange_ISOInstants(t *testing.T) {
	t.Run("instants define the window; tz is display-only", func(t *testing.T) {
		q := url.Values{}
		q.Set("start", "2026-06-09T00:00:00-07:00")
		q.Set("end", "2026-06-09T23:59:59-07:00")

		// The unix window must be identical regardless of the display tz.
		for _, tz := range []string{"America/Los_Angeles", "UTC", "Asia/Tokyo"} {
			qq := url.Values{}
			qq.Set("start", q.Get("start"))
			qq.Set("end", q.Get("end"))
			qq.Set("tz", tz)

			startUnix, endUnix, loc, err := parseDateRange(qq, "UTC")
			if err != nil {
				t.Fatalf("tz=%s: unexpected error: %v", tz, err)
			}
			if want := rfc3339Unix(t, "2026-06-09T07:00:00Z"); startUnix != want {
				t.Errorf("tz=%s: startUnix = %d, want %d", tz, startUnix, want)
			}
			if want := rfc3339Unix(t, "2026-06-10T06:59:59Z"); endUnix != want {
				t.Errorf("tz=%s: endUnix = %d, want %d", tz, endUnix, want)
			}
			if loc.String() != tz {
				t.Errorf("loc = %q, want %q (display tz)", loc.String(), tz)
			}
		}
	})

	t.Run("tz falls back to the server default for display", func(t *testing.T) {
		q := url.Values{}
		q.Set("start", "2026-06-09T00:00:00Z")
		q.Set("end", "2026-06-09T23:59:59Z")
		_, _, loc, err := parseDateRange(q, "America/New_York")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loc.String() != "America/New_York" {
			t.Errorf("loc = %q, want America/New_York", loc.String())
		}
	})
}

func TestParseDateRange_Errors(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
	}{
		{"missing start", url.Values{"end": []string{"2026-06-09T00:00:00Z"}}},
		{"missing end", url.Values{"start": []string{"2026-06-09T00:00:00Z"}}},
		{
			name: "bare YYYY-MM-DD is no longer accepted",
			query: url.Values{
				"start": []string{"2026-06-09"},
				"end":   []string{"2026-06-10"},
			},
		},
		{
			name: "invalid display tz",
			query: url.Values{
				"start": []string{"2026-06-09T00:00:00Z"},
				"end":   []string{"2026-06-09T23:59:59Z"},
				"tz":    []string{"Not/A/Zone"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := parseDateRange(tt.query, "UTC"); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestResolveTimezone(t *testing.T) {
	tests := []struct {
		name     string
		tz       string
		fallback string
		want     string
		wantErr  bool
	}{
		{"explicit tz wins", "America/New_York", "UTC", "America/New_York", false},
		{"falls back when empty", "", "America/Los_Angeles", "America/Los_Angeles", false},
		{"UTC when both empty", "", "", "UTC", false},
		{"invalid tz errors", "Not/A/Zone", "UTC", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := resolveTimezone(tt.tz, tt.fallback)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && loc.String() != tt.want {
				t.Errorf("loc = %q, want %q", loc.String(), tt.want)
			}
		})
	}
}

// parseLocalDateRange still backs the report-generation path (calendar dates),
// so it keeps its own coverage.
func TestParseLocalDateRange(t *testing.T) {
	utc, _ := time.LoadLocation("UTC")

	t.Run("inclusive end-of-day", func(t *testing.T) {
		startUnix, endUnix, err := parseLocalDateRange("2026-06-09", "2026-06-09", utc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := rfc3339Unix(t, "2026-06-09T00:00:00Z"); startUnix != want {
			t.Errorf("startUnix = %d, want %d", startUnix, want)
		}
		if want := rfc3339Unix(t, "2026-06-09T23:59:59Z"); endUnix != want {
			t.Errorf("endUnix = %d, want %d", endUnix, want)
		}
	})

	t.Run("rejects non-date input", func(t *testing.T) {
		if _, _, err := parseLocalDateRange("2026-06-09T00:00:00Z", "2026-06-09T00:00:00Z", utc); err == nil {
			t.Fatal("expected datetime input to be rejected by the calendar-date parser")
		}
	})
}

// TestShowRadarObjectStats_AcceptsISO verifies the radar_stats handler responds
// 200 to the canonical ISO 8601 instant contract.
func TestShowRadarObjectStats_AcceptsISO(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/radar_stats?start=2024-12-03T00:00:00Z&end=2024-12-03T23:59:59Z&tz=UTC&group=1h",
		nil,
	)
	w := httptest.NewRecorder()

	server.showRadarObjectStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}
