package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

const testFiveMin = 5 * time.Minute

// sequenceServer returns a Server whose PCAP safe directory holds the named
// files, so resolvePCAPPath accepts them.
func sequenceServer(t *testing.T, names ...string) *Server {
	t.Helper()
	// On macOS t.TempDir() sits under /var, a symlink to /private/var. The
	// safe-directory check compares the candidate's symlink-resolved path
	// against the configured directory, so an unresolved fixture root would
	// make every file look like an escape attempt.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving fixture directory: %v", err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", name, err)
		}
	}
	return &Server{pcapSafeDir: dir, udpPort: 2368}
}

// stubCounts installs a countPCAPPackets returning the extent recorded for each
// file's base name, and restores the real one afterwards.
func stubCounts(t *testing.T, extents map[string]network.PCAPCountResult, failures map[string]error) {
	t.Helper()
	original := countPCAPPackets
	t.Cleanup(func() { countPCAPPackets = original })
	countPCAPPackets = func(path string, _ int) (network.PCAPCountResult, error) {
		name := filepath.Base(path)
		if err, ok := failures[name]; ok {
			return network.PCAPCountResult{}, err
		}
		res, ok := extents[name]
		if !ok {
			return network.PCAPCountResult{}, errors.New("no stub extent for " + name)
		}
		return res, nil
	}
}

// extent builds a count result running from base+offset for dur.
func extent(base time.Time, offset, dur time.Duration, packets uint64) network.PCAPCountResult {
	return network.PCAPCountResult{
		Count:            packets,
		FirstTimestampNs: base.Add(offset).UnixNano(),
		LastTimestampNs:  base.Add(offset + dur).UnixNano(),
	}
}

// statusOf extracts the HTTP status a switchError carries.
func statusOf(t *testing.T, err error) int {
	t.Helper()
	var se *switchError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a switchError", err)
	}
	return se.status
}

func TestBuildReplaySequenceRejectsAnEmptyList(t *testing.T) {
	ws := sequenceServer(t)
	_, err := ws.buildReplaySequence(nil, 0, -1)
	if err == nil {
		t.Fatal("empty file list accepted, want an error")
	}
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", got, http.StatusBadRequest)
	}
}

func TestBuildReplaySequencePlansAContinuousRun(t *testing.T) {
	ws := sequenceServer(t, "a.pcap", "b.pcap", "c.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{
		// a → b seamless (4ms), b → c acceptable (300ms).
		"a.pcap": extent(base, 0, testFiveMin, 1000),
		"b.pcap": extent(base, testFiveMin+4*time.Millisecond, testFiveMin, 1100),
		"c.pcap": extent(base, 2*testFiveMin+304*time.Millisecond, testFiveMin, 1200),
	}, nil)

	plan, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap", "c.pcap"}, 0, -1)
	if err != nil {
		t.Fatalf("buildReplaySequence: %v", err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("planned %d steps, want 3", len(plan.Steps))
	}
	if plan.TotalPackets != 3300 {
		t.Errorf("TotalPackets = %d, want 3300", plan.TotalPackets)
	}
	if plan.JoinsCrossed != 2 {
		t.Errorf("JoinsCrossed = %d, want 2", plan.JoinsCrossed)
	}
	// Only the b → c join is non-seamless, so only one revolution is dropped.
	if plan.FrameDrops != 1 {
		t.Errorf("FrameDrops = %d, want 1", plan.FrameDrops)
	}
	if want := 304 * time.Millisecond; plan.Lost != want {
		t.Errorf("Lost = %v, want %v", plan.Lost, want)
	}
	if plan.FirstTimestampNs != base.UnixNano() {
		t.Errorf("FirstTimestampNs = %d, want %d", plan.FirstTimestampNs, base.UnixNano())
	}
	wantLast := base.Add(3*testFiveMin + 304*time.Millisecond).UnixNano()
	if plan.LastTimestampNs != wantLast {
		t.Errorf("LastTimestampNs = %d, want %d", plan.LastTimestampNs, wantLast)
	}
	// Steps must be resolved to absolute paths the reader can open.
	for _, step := range plan.Steps {
		if !filepath.IsAbs(step.Path) {
			t.Errorf("step path %q is not absolute", step.Path)
		}
	}
}

func TestBuildReplaySequenceOrdersFilesByPacketTime(t *testing.T) {
	// A caller listing files out of order must still get a correct plan; the
	// sequence is defined by packet time, not by argument order.
	ws := sequenceServer(t, "a.pcap", "b.pcap", "c.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{
		"a.pcap": extent(base, 0, testFiveMin, 1000),
		"b.pcap": extent(base, testFiveMin, testFiveMin, 1000),
		"c.pcap": extent(base, 2*testFiveMin, testFiveMin, 1000),
	}, nil)

	plan, err := ws.buildReplaySequence([]string{"c.pcap", "a.pcap", "b.pcap"}, 0, -1)
	if err != nil {
		t.Fatalf("buildReplaySequence: %v", err)
	}
	for i, want := range []string{"a.pcap", "b.pcap", "c.pcap"} {
		if got := filepath.Base(plan.Steps[i].Path); got != want {
			t.Errorf("step %d = %q, want %q", i, got, want)
		}
	}
}

func TestBuildReplaySequenceRefusesABrokenJoin(t *testing.T) {
	ws := sequenceServer(t, "a.pcap", "b.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{
		"a.pcap": extent(base, 0, testFiveMin, 1000),
		// 30 seconds missing: far beyond the one-second bound.
		"b.pcap": extent(base, testFiveMin+30*time.Second, testFiveMin, 1000),
	}, nil)

	_, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap"}, 0, -1)
	if err == nil {
		t.Fatal("broken join accepted, want an error")
	}
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", got, http.StatusBadRequest)
	}
	// The operator has to be able to see which join failed and by how much.
	for _, want := range []string{"a.pcap", "b.pcap", "broken", "30s"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestBuildReplaySequenceRefusesOverlappingFiles(t *testing.T) {
	ws := sequenceServer(t, "a.pcap", "b.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{
		"a.pcap": extent(base, 0, testFiveMin, 1000),
		"b.pcap": extent(base, testFiveMin-time.Second, testFiveMin, 1000),
	}, nil)

	_, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap"}, 0, -1)
	if err == nil {
		t.Fatal("overlapping files accepted, want an error")
	}
	if !strings.Contains(err.Error(), "overlap") {
		t.Errorf("error %q does not name the overlap", err)
	}
}

func TestBuildReplaySequenceRejectsAnEmptyCapture(t *testing.T) {
	// A file with nothing on the LiDAR port has no extent to sequence. The
	// message must say that rather than complain about unset packet times.
	ws := sequenceServer(t, "a.pcap", "b.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{
		"a.pcap": extent(base, 0, testFiveMin, 1000),
		"b.pcap": {Count: 0},
	}, nil)

	_, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap"}, 0, -1)
	if err == nil {
		t.Fatal("empty capture accepted, want an error")
	}
	for _, want := range []string{"b.pcap", "no packets", "2368"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestBuildReplaySequenceSurfacesAProbeFailure(t *testing.T) {
	ws := sequenceServer(t, "a.pcap", "b.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t,
		map[string]network.PCAPCountResult{"a.pcap": extent(base, 0, testFiveMin, 1000)},
		map[string]error{"b.pcap": errors.New("truncated capture")})

	_, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap"}, 0, -1)
	if err == nil {
		t.Fatal("probe failure swallowed, want an error")
	}
	for _, want := range []string{"b.pcap", "truncated capture"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestBuildReplaySequenceRejectsAnUnknownFile(t *testing.T) {
	ws := sequenceServer(t, "a.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{"a.pcap": extent(base, 0, testFiveMin, 1000)}, nil)

	_, err := ws.buildReplaySequence([]string{"a.pcap", "absent.pcap"}, 0, -1)
	if err == nil {
		t.Fatal("unknown file accepted, want an error")
	}
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Errorf("status = %d, want %d", got, http.StatusNotFound)
	}
}

func TestBuildReplaySequenceRejectsAPathEscapingTheSafeDir(t *testing.T) {
	// The safe directory is a containment boundary, and a multi-file request
	// must not become a way around it.
	ws := sequenceServer(t, "a.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{"a.pcap": extent(base, 0, testFiveMin, 1000)}, nil)

	_, err := ws.buildReplaySequence([]string{"a.pcap", "../../etc/passwd"}, 0, -1)
	if err == nil {
		t.Fatal("path escaping the safe directory accepted, want an error")
	}
}

func TestBuildReplaySequenceHonoursTheWindow(t *testing.T) {
	ws := sequenceServer(t, "a.pcap", "b.pcap", "c.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{
		"a.pcap": extent(base, 0, testFiveMin, 1000),
		"b.pcap": extent(base, testFiveMin, testFiveMin, 1000),
		"c.pcap": extent(base, 2*testFiveMin, testFiveMin, 1000),
	}, nil)

	// The window is measured from the start of the sequence, not of any file:
	// 200s in, running 350s, so the tail of a and most of b.
	plan, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap", "c.pcap"}, 200, 350)
	if err != nil {
		t.Fatalf("buildReplaySequence: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("planned %d steps, want 2 (a and b only)", len(plan.Steps))
	}
	if got := filepath.Base(plan.Steps[0].Path); got != "a.pcap" {
		t.Errorf("first step = %q, want a.pcap", got)
	}
	if plan.Steps[0].StartSecs != 200 {
		t.Errorf("first step StartSecs = %v, want 200", plan.Steps[0].StartSecs)
	}
	if plan.Steps[0].DurationSecs != -1 {
		t.Errorf("first step DurationSecs = %v, want -1 (to end of file)", plan.Steps[0].DurationSecs)
	}
	// 350s of window, 100s of it in a, so 250s of b — stopping short of b's
	// end, which is what makes this an explicit duration rather than -1.
	if got := plan.Steps[1].DurationSecs; got != 250 {
		t.Errorf("second step DurationSecs = %v, want 250", got)
	}
}

func TestBuildReplaySequenceAcceptsASingleFile(t *testing.T) {
	ws := sequenceServer(t, "a.pcap")
	base := time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)
	stubCounts(t, map[string]network.PCAPCountResult{"a.pcap": extent(base, 0, testFiveMin, 1000)}, nil)

	plan, err := ws.buildReplaySequence([]string{"a.pcap"}, 0, -1)
	if err != nil {
		t.Fatalf("buildReplaySequence: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("planned %d steps, want 1", len(plan.Steps))
	}
	if plan.JoinsCrossed != 0 || plan.FrameDrops != 0 {
		t.Errorf("JoinsCrossed = %d, FrameDrops = %d, want 0 and 0",
			plan.JoinsCrossed, plan.FrameDrops)
	}
}
