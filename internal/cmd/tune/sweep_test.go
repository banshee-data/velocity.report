package tune

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/server"
)

// fakeMonitor stands in for the lidar monitor HTTP server that the sweep
// applet drives. It answers every endpoint internal/lidar/server.Client
// touches, so Main can be exercised end-to-end without a sensor, a PCAP, or
// a running server.
type fakeMonitor struct {
	mu sync.Mutex
	// hits counts requests per URL path so tests can assert the applet
	// actually drove the sweep loop rather than bailing out early.
	hits map[string]int
	// buckets is echoed from /api/lidar/acceptance as BucketsMeters.
	buckets []float64
	// pcapOKBudget is the number of /api/lidar/pcap/start calls answered 200
	// before the endpoint starts returning 500. Negative means "always OK".
	pcapOKBudget int
	// failParams makes /api/lidar/params return 500.
	failParams bool
	// trackingMetrics is returned from /api/lidar/tracks/metrics.
	trackingMetrics map[string]any
}

func newFakeMonitor() *fakeMonitor {
	return &fakeMonitor{
		hits:         map[string]int{},
		buckets:      []float64{10, 20, 30},
		pcapOKBudget: -1,
		trackingMetrics: map[string]any{
			"active_tracks":           float64(4),
			"total_alignment_samples": float64(120),
			"mean_alignment_rad":      0.25,
			"mean_alignment_deg":      14.3,
			"total_misaligned":        float64(3),
			"misalignment_ratio":      0.025,
		},
	}
}

func (f *fakeMonitor) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func (f *fakeMonitor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	failPCAP := false
	if r.URL.Path == "/api/lidar/pcap/start" && f.pcapOKBudget >= 0 {
		if f.pcapOKBudget == 0 {
			failPCAP = true
		} else {
			f.pcapOKBudget--
		}
	}
	failParams := f.failParams
	buckets, tracking := f.buckets, f.trackingMetrics
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/lidar/acceptance":
		// Serves double duty: bucket discovery and per-iteration sampling.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"BucketsMeters":   buckets,
			"AcceptCounts":    []any{float64(5), float64(6), float64(7)},
			"RejectCounts":    []any{float64(1), float64(2), float64(3)},
			"Totals":          []any{float64(6), float64(8), float64(10)},
			"AcceptanceRates": []any{0.83, 0.75, 0.7},
		})
	case "/api/lidar/pcap/start":
		if failPCAP {
			http.Error(w, "replay unavailable", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "started"})
	case "/api/lidar/params":
		if failParams {
			http.Error(w, "params rejected", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	case "/api/lidar/tracks/metrics":
		_ = json.NewEncoder(w).Encode(tracking)
	case "/api/lidar/grid_status":
		_ = json.NewEncoder(w).Encode(map[string]any{"settled": true, "SettlingComplete": true})
	case "/api/lidar/grid_reset", "/api/lidar/acceptance/reset":
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "reset"})
	case "/api/lidar/data_source":
		_ = json.NewEncoder(w).Encode(map[string]any{"source": "live", "done": true})
	default:
		http.NotFound(w, r)
	}
}

// startMonitor spins up the fake monitor and chdirs into a temp directory so
// the CSVs Main writes land somewhere disposable.
func startMonitor(t *testing.T) (*fakeMonitor, string) {
	t.Helper()
	mon := newFakeMonitor()
	srv := httptest.NewServer(mon)
	t.Cleanup(srv.Close)
	t.Chdir(t.TempDir())
	return mon, srv.URL
}

// fastArgs are the timing flags that keep a sweep sub-second.
func fastArgs(monitorURL string, extra ...string) []string {
	args := []string{
		"-monitor", monitorURL,
		"-iterations", "1",
		"-interval", "1ms",
		"-settle-time", "1ms",
		"-pcap-settle", "1ms",
	}
	return append(args, extra...)
}

// readCSV returns the parsed rows of a CSV written into the working directory.
func readCSV(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	defer f.Close()
	// The sweep writers emit a variable column count per bucket set, so the
	// reader must not enforce a fixed record length.
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return rows
}

func TestMainMultiModeWritesSummaryAndRawCSVs(t *testing.T) {
	mon, url := startMonitor(t)

	code := Main(fastArgs(url,
		"-mode", "multi",
		"-noise", "0.01",
		"-closeness", "2.0",
		"-neighbours", "1",
		"-output", "sweep.csv",
	))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}

	summary := readCSV(t, "sweep.csv")
	if len(summary) != 2 {
		t.Fatalf("summary rows = %d, want 2 (header + 1 combo)", len(summary))
	}
	// The raw file is derived by swapping the .csv suffix.
	if rows := readCSV(t, "sweep-raw.csv"); len(rows) < 2 {
		t.Errorf("raw rows = %d, want at least 2 (header + 1 sample)", len(rows))
	}

	if got := mon.count("/api/lidar/params"); got != 1 {
		t.Errorf("params calls = %d, want 1 (one per combination)", got)
	}
	if got := mon.count("/api/lidar/grid_reset"); got != 1 {
		t.Errorf("grid_reset calls = %d, want 1", got)
	}
}

func TestMainDefaultsOutputFilenameWhenFlagOmitted(t *testing.T) {
	_, url := startMonitor(t)

	code := Main(fastArgs(url, "-mode", "multi", "-noise", "0.01",
		"-closeness", "2.0", "-neighbours", "1"))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}

	// Without -output the applet synthesises sweep-<mode>-<timestamp>.csv.
	matches, err := filepath.Glob("sweep-multi-*.csv")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	var summaries []string
	for _, m := range matches {
		if !strings.HasSuffix(m, "-raw.csv") {
			summaries = append(summaries, m)
		}
	}
	if len(summaries) != 1 {
		t.Fatalf("summary files = %v, want exactly one sweep-multi-*.csv", summaries)
	}
}

func TestMainSingleVariableModesSweepOnlyTheirOwnAxis(t *testing.T) {
	// Each single-variable mode holds the other two axes at their fixed
	// values, so the combination count is the swept axis's length alone.
	tests := []struct {
		name     string
		extra    []string
		wantRows int // header + one row per combination
	}{
		{
			name: "noise",
			extra: []string{"-mode", "noise",
				"-noise-start", "0.01", "-noise-end", "0.02", "-noise-step", "0.01"},
			wantRows: 3,
		},
		{
			name: "closeness",
			extra: []string{"-mode", "closeness",
				"-closeness-start", "1.5", "-closeness-end", "2.5", "-closeness-step", "0.5"},
			wantRows: 4,
		},
		{
			name: "neighbour",
			extra: []string{"-mode", "neighbour",
				"-neighbour-start", "0", "-neighbour-end", "1", "-neighbour-step", "1"},
			wantRows: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mon, url := startMonitor(t)
			args := append(fastArgs(url, "-output", "s.csv"), tc.extra...)

			if code := Main(args); code != 0 {
				t.Fatalf("Main() = %d, want 0", code)
			}
			if rows := readCSV(t, "s.csv"); len(rows) != tc.wantRows {
				t.Errorf("summary rows = %d, want %d", len(rows), tc.wantRows)
			}
			if got := mon.count("/api/lidar/params"); got != tc.wantRows-1 {
				t.Errorf("params calls = %d, want %d", got, tc.wantRows-1)
			}
		})
	}
}

func TestMainSeedFlagTogglesPerCombination(t *testing.T) {
	// -seed toggle alternates SeedFromFirst across combinations; capture the
	// posted bodies to confirm the alternation actually reaches the monitor.
	var mu sync.Mutex
	var seeds []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/lidar/params" {
			// BackgroundParams marshals nested under l3.ema_baseline_v1, so
			// decode through the real type rather than a flat struct.
			var params server.BackgroundParams
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Errorf("decoding params body: %v", err)
			}
			mu.Lock()
			seeds = append(seeds, params.SeedFromFirst)
			mu.Unlock()
		}
		newFakeMonitor().ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Chdir(t.TempDir())

	code := Main(fastArgs(srv.URL,
		"-mode", "noise",
		"-noise-start", "0.01", "-noise-end", "0.02", "-noise-step", "0.01",
		"-seed", "toggle",
		"-output", "s.csv",
	))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seeds) != 2 {
		t.Fatalf("captured %d params posts, want 2", len(seeds))
	}
	if seeds[0] == seeds[1] {
		t.Errorf("seed values = %v, want alternating across combinations", seeds)
	}
}

func TestMainSeedFlagFixedValues(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"nonsense", true}, // unrecognised values fall back to seeding
	} {
		t.Run(tc.flag, func(t *testing.T) {
			var mu sync.Mutex
			var seeds []bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/lidar/params" {
					// BackgroundParams marshals nested under l3.ema_baseline_v1,
					// so decode through the real type rather than a flat struct.
					var params server.BackgroundParams
					if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
						t.Errorf("decoding params body: %v", err)
					}
					mu.Lock()
					seeds = append(seeds, params.SeedFromFirst)
					mu.Unlock()
				}
				newFakeMonitor().ServeHTTP(w, r)
			}))
			t.Cleanup(srv.Close)
			t.Chdir(t.TempDir())

			code := Main(fastArgs(srv.URL, "-mode", "multi",
				"-noise", "0.01", "-closeness", "2.0", "-neighbours", "1",
				"-seed", tc.flag, "-output", "s.csv"))
			if code != 0 {
				t.Fatalf("Main() = %d, want 0", code)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(seeds) != 1 {
				t.Fatalf("captured %d params posts, want 1", len(seeds))
			}
			if seeds[0] != tc.want {
				t.Errorf("seed = %v, want %v", seeds[0], tc.want)
			}
		})
	}
}

func TestMainEmptyParamListsFallBackToBuiltinDefaults(t *testing.T) {
	_, url := startMonitor(t)

	// A range whose step overshoots the end yields no values, so Main must
	// substitute its built-in 3x3x3 default grid.
	code := Main(fastArgs(url,
		"-mode", "multi",
		"-noise-start", "1", "-noise-end", "0", "-noise-step", "1",
		"-closeness-start", "1", "-closeness-end", "0", "-closeness-step", "1",
		"-neighbour-start", "1", "-neighbour-end", "0", "-neighbour-step", "1",
		"-output", "s.csv",
	))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}
	// 3 noise x 3 closeness x 3 neighbour = 27 combinations, plus header.
	if rows := readCSV(t, "s.csv"); len(rows) != 28 {
		t.Errorf("summary rows = %d, want 28 (header + 27 default combos)", len(rows))
	}
}

func TestMainContinuesToNextComboWhenSetParamsFails(t *testing.T) {
	mon, url := startMonitor(t)
	mon.mu.Lock()
	mon.failParams = true
	mon.mu.Unlock()

	code := Main(fastArgs(url, "-mode", "multi",
		"-noise", "0.01,0.02", "-closeness", "2.0", "-neighbours", "1",
		"-output", "s.csv"))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0 (param failures are skipped, not fatal)", code)
	}

	// Both combos attempted, but neither produced a summary row.
	if got := mon.count("/api/lidar/params"); got != 2 {
		t.Errorf("params calls = %d, want 2", got)
	}
	if rows := readCSV(t, "s.csv"); len(rows) != 1 {
		t.Errorf("summary rows = %d, want 1 (header only)", len(rows))
	}
}

func TestMainPCAPModeReplaysPerCombination(t *testing.T) {
	mon, url := startMonitor(t)

	code := Main(fastArgs(url, "-mode", "multi",
		"-pcap", "golden.pcap",
		"-noise", "0.01", "-closeness", "2.0", "-neighbours", "1",
		"-output", "s.csv"))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}

	// One priming replay before the loop, then one per combination.
	if got := mon.count("/api/lidar/pcap/start"); got != 2 {
		t.Errorf("pcap/start calls = %d, want 2 (prime + 1 combo)", got)
	}
	// The combination still sampled, so a summary row was written.
	if rows := readCSV(t, "s.csv"); len(rows) != 2 {
		t.Errorf("summary rows = %d, want 2 (header + 1 combo)", len(rows))
	}
}

func TestMainPCAPReplayFailureRetriesThenSkipsCombo(t *testing.T) {
	// Slow by construction: the in-loop retry path sleeps a hard-coded 5s
	// between attempts, so this case is skipped under -short.
	if testing.Short() {
		t.Skip("covers the 5s hard-coded PCAP retry backoff")
	}

	mon, url := startMonitor(t)
	// Let the priming replay succeed — it is fatal on failure — then fail
	// every in-loop attempt so the retry-then-skip branch runs.
	mon.mu.Lock()
	mon.pcapOKBudget = 1
	mon.mu.Unlock()

	code := Main(fastArgs(url, "-mode", "multi",
		"-pcap", "golden.pcap",
		"-noise", "0.01", "-closeness", "2.0", "-neighbours", "1",
		"-output", "s.csv"))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}
	if rows := readCSV(t, "s.csv"); len(rows) != 1 {
		t.Errorf("summary rows = %d, want 1 (header only; combo skipped)", len(rows))
	}
	// Priming attempt plus two in-loop attempts (initial + one retry).
	if got := mon.count("/api/lidar/pcap/start"); got != 3 {
		t.Errorf("pcap/start calls = %d, want 3 (prime + attempt + retry)", got)
	}
}

func TestMainTrackingModeWritesAlignmentMetrics(t *testing.T) {
	mon, url := startMonitor(t)

	code := Main(fastArgs(url,
		"-mode", "tracking",
		"-pcap", "golden.pcap",
		"-gating-start", "16", "-gating-end", "16", "-gating-step", "4",
		"-pnoise-pos-start", "0.05", "-pnoise-pos-end", "0.05", "-pnoise-pos-step", "0.05",
		"-mnoise-start", "0.1", "-mnoise-end", "0.1", "-mnoise-step", "0.1",
		"-output", "tracking.csv",
	))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}

	rows := readCSV(t, "tracking.csv")
	if len(rows) != 2 {
		t.Fatalf("tracking rows = %d, want 2 (header + 1 combo)", len(rows))
	}
	if rows[0][0] != "gating_distance_sq" {
		t.Errorf("first header column = %q, want %q", rows[0][0], "gating_distance_sq")
	}
	// Columns are gating, pnoise, mnoise, tracks, samples, rad, deg, misaligned, ratio.
	if got, want := rows[1][3], "4"; got != want {
		t.Errorf("active_tracks column = %q, want %q", got, want)
	}
	if got, want := rows[1][6], "14.3000"; got != want {
		t.Errorf("mean_alignment_deg column = %q, want %q", got, want)
	}
	if got := mon.count("/api/lidar/tracks/metrics"); got != 1 {
		t.Errorf("tracks/metrics calls = %d, want 1", got)
	}
}

func TestMainTrackingModeSkipsComboWhenReplayFails(t *testing.T) {
	mon, url := startMonitor(t)
	// Tracking mode has no priming replay, so every attempt can fail.
	mon.mu.Lock()
	mon.pcapOKBudget = 0
	mon.mu.Unlock()

	code := Main(fastArgs(url,
		"-mode", "tracking", "-pcap", "golden.pcap",
		"-gating-start", "16", "-gating-end", "16", "-gating-step", "4",
		"-pnoise-pos-start", "0.05", "-pnoise-pos-end", "0.05", "-pnoise-pos-step", "0.05",
		"-mnoise-start", "0.1", "-mnoise-end", "0.1", "-mnoise-step", "0.1",
		"-output", "tracking.csv",
	))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}
	if rows := readCSV(t, "tracking.csv"); len(rows) != 1 {
		t.Errorf("tracking rows = %d, want 1 (header only; combo skipped)", len(rows))
	}
	// Replay is attempted before metrics, so metrics must never be fetched.
	if got := mon.count("/api/lidar/tracks/metrics"); got != 0 {
		t.Errorf("tracks/metrics calls = %d, want 0", got)
	}
}

func TestMainTrackingModeDefaultsOutputFilename(t *testing.T) {
	_, url := startMonitor(t)

	code := Main(fastArgs(url,
		"-mode", "tracking", "-pcap", "golden.pcap",
		"-gating-start", "16", "-gating-end", "16", "-gating-step", "4",
		"-pnoise-pos-start", "0.05", "-pnoise-pos-end", "0.05", "-pnoise-pos-step", "0.05",
		"-mnoise-start", "0.1", "-mnoise-end", "0.1", "-mnoise-step", "0.1",
	))
	if code != 0 {
		t.Fatalf("Main() = %d, want 0", code)
	}
	matches, err := filepath.Glob("sweep-tracking-*.csv")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("tracking output files = %v, want exactly one", matches)
	}
}

func TestParseParamListPrefersExplicitListOverRange(t *testing.T) {
	got := parseParamList("0.005,0.01,0.02", 1, 5, 1)
	want := []float64{0.005, 0.01, 0.02}
	if len(got) != len(want) {
		t.Fatalf("parseParamList() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseParamList() = %v, want %v", got, want)
		}
	}
}

func TestParseParamListGeneratesRangeWhenListEmpty(t *testing.T) {
	got := parseParamList("", 1, 3, 1)
	if len(got) != 3 {
		t.Fatalf("parseParamList(\"\", 1, 3, 1) = %v, want 3 values", got)
	}
}

func TestParseIntParamListPrefersExplicitListOverRange(t *testing.T) {
	got := parseIntParamList("0,2,4", 1, 5, 1)
	want := []int{0, 2, 4}
	if len(got) != len(want) {
		t.Fatalf("parseIntParamList() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseIntParamList() = %v, want %v", got, want)
		}
	}
}

func TestParseIntParamListGeneratesRangeWhenListEmpty(t *testing.T) {
	got := parseIntParamList("", 0, 2, 1)
	if len(got) != 3 {
		t.Fatalf("parseIntParamList(\"\", 0, 2, 1) = %v, want 3 values", got)
	}
}

func TestToFloat64(t *testing.T) {
	// JSON decoding yields float64, but the helper also accepts the integer
	// types a Go caller might pass, and degrades to 0 for anything else.
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{"float64", float64(2.5), 2.5},
		{"int", int(3), 3},
		{"int64", int64(4), 4},
		{"string is not numeric", "5", 0},
		{"nil missing key", nil, 0},
		{"bool", true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := toFloat64(tc.in); got != tc.want {
				t.Errorf("toFloat64(%#v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
