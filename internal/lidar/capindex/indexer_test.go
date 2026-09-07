package capindex

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// memStore is an in-memory Store for exercising the Indexer without sqlite.
type memStore struct {
	indexed   []Indexed
	applied   []File
	drift     Drift
	scanState string
	scanErr   string
	probes    map[string]Extent
	failures  map[string]string
	failOn    string // method name to fail
}

func newMemStore() *memStore {
	return &memStore{probes: map[string]Extent{}, failures: map[string]string{}}
}

func (m *memStore) IndexedFiles(string) ([]Indexed, error) {
	if m.failOn == "IndexedFiles" {
		return nil, errors.New("index unreadable")
	}
	return m.indexed, nil
}

func (m *memStore) ApplyScan(_ string, found []File, drift Drift) error {
	if m.failOn == "ApplyScan" {
		return errors.New("write failed")
	}
	m.applied = found
	m.drift = drift
	return nil
}

func (m *memStore) MarkScanned(_, state, scanErr string) error {
	if m.failOn == "MarkScanned" {
		return errors.New("cannot record scan")
	}
	m.scanState, m.scanErr = state, scanErr
	return nil
}

func (m *memStore) RecordProbe(_, relPath string, firstNs, lastNs, count int64, udpPort int) error {
	m.probes[relPath] = Extent{FirstPacketNs: firstNs, LastPacketNs: lastNs,
		PacketCount: uint64(count), UDPPort: udpPort}
	return nil
}

func (m *memStore) RecordProbeFailure(_, relPath, reason string) error {
	m.failures[relPath] = reason
	return nil
}

func (m *memStore) ProbedFiles(string) ([]Probed, error) { return nil, nil }

// probeRoot builds an indexer over a temp dir holding the named captures.
func probeRoot(t *testing.T, store *memStore, probe Prober, names ...string) (*Indexer, string) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		writeCapture(t, dir, n, 4096, 1)
	}
	return &Indexer{RootID: "root-1", RootPath: dir, Store: store, Probe: probe, UDPPort: 2368}, dir
}

func okProbe(start time.Time) Prober {
	var n int64
	return func(string, int) (Extent, error) {
		first := start.Add(time.Duration(n) * 5 * time.Minute)
		n++
		return Extent{
			FirstPacketNs: first.UnixNano(),
			LastPacketNs:  first.Add(5 * time.Minute).UnixNano(),
			PacketCount:   540000,
			UDPPort:       2368,
		}, nil
	}
}

func TestIndexerRefreshScansProbesAndRecords(t *testing.T) {
	store := newMemStore()
	ix, _ := probeRoot(t, store, okProbe(scanBase), "a.pcap", "b.pcap")

	res, err := ix.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if res.State != StateOK {
		t.Errorf("State = %q, want %q", res.State, StateOK)
	}
	if len(res.Drift.Added) != 2 {
		t.Errorf("Added = %d, want 2 on a first scan", len(res.Drift.Added))
	}
	if res.Probed != 2 {
		t.Errorf("Probed = %d, want 2", res.Probed)
	}
	if res.ProbeFailed != 0 {
		t.Errorf("ProbeFailed = %d, want 0", res.ProbeFailed)
	}
	if store.scanState != StateOK {
		t.Errorf("recorded scan state %q, want %q", store.scanState, StateOK)
	}
	if len(store.probes) != 2 {
		t.Errorf("recorded %d probes, want 2", len(store.probes))
	}
}

func TestIndexerRefreshProbesOnlyWhatChanged(t *testing.T) {
	// The whole point of splitting the scan: an unchanged volume costs one walk
	// and no file reads.
	store := newMemStore()
	ix, _ := probeRoot(t, store, okProbe(scanBase), "a.pcap", "b.pcap")

	if _, err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	// Feed the index back what the first pass wrote.
	store.indexed = nil
	for _, f := range store.applied {
		store.indexed = append(store.indexed, Indexed{
			RelPath: f.RelPath, SizeBytes: f.SizeBytes,
			ModifiedAt: f.ModifiedAt, ContentTag: f.ContentTag, Present: true,
		})
	}

	var probeCalls int
	ix.Probe = func(string, int) (Extent, error) {
		probeCalls++
		return Extent{FirstPacketNs: 1, LastPacketNs: 2, PacketCount: 1}, nil
	}
	res, err := ix.Refresh(context.Background())
	if err != nil {
		t.Fatalf("second Refresh: %v", err)
	}
	if res.Drift.Any() {
		t.Errorf("unchanged volume reported drift: %s", res.Drift.Summary())
	}
	if probeCalls != 0 {
		t.Errorf("probed %d files on an unchanged volume, want 0", probeCalls)
	}
	if res.Drift.Unchanged != 2 {
		t.Errorf("Unchanged = %d, want 2", res.Drift.Unchanged)
	}
}

func TestIndexerRecordsAnUnreachableRoot(t *testing.T) {
	// An unmounted volume must be visibly unmounted, not an empty listing that
	// reads as an empty volume.
	store := newMemStore()
	ix := &Indexer{RootID: "root-1", RootPath: "/definitely/not/mounted", Store: store}

	res, err := ix.Refresh(context.Background())
	if !errors.Is(err, ErrRootUnreachable) {
		t.Fatalf("Refresh: %v, want ErrRootUnreachable", err)
	}
	if res.State != StateUnreachable {
		t.Errorf("State = %q, want %q", res.State, StateUnreachable)
	}
	if store.scanState != StateUnreachable {
		t.Errorf("recorded state %q, want %q", store.scanState, StateUnreachable)
	}
	if store.scanErr == "" {
		t.Error("no reason recorded for an unreachable root")
	}
}

func TestIndexerContinuesPastAFailedProbe(t *testing.T) {
	store := newMemStore()
	ix, _ := probeRoot(t, store, nil, "good.pcap", "corrupt.pcap", "empty.pcap")
	ix.Probe = func(path string, _ int) (Extent, error) {
		switch {
		case strings.Contains(path, "corrupt"):
			return Extent{}, errors.New("truncated capture")
		case strings.Contains(path, "empty"):
			// A capture with nothing on the LiDAR port is a failure too, and a
			// more confusing one, so it gets its own reason.
			return Extent{PacketCount: 0}, nil
		default:
			return Extent{FirstPacketNs: 1, LastPacketNs: 2, PacketCount: 10, UDPPort: 2368}, nil
		}
	}

	res, err := ix.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if res.Probed != 1 {
		t.Errorf("Probed = %d, want 1", res.Probed)
	}
	if res.ProbeFailed != 2 {
		t.Errorf("ProbeFailed = %d, want 2", res.ProbeFailed)
	}
	if got := store.failures["corrupt.pcap"]; got != "truncated capture" {
		t.Errorf("corrupt.pcap failure = %q, want the prober's reason", got)
	}
	if got := store.failures["empty.pcap"]; !strings.Contains(got, "no packets") {
		t.Errorf("empty.pcap failure = %q, want it to name the empty match", got)
	}
	// The pass still succeeds: the volume is indexed, two files just need a look.
	if res.State != StateOK {
		t.Errorf("State = %q, want %q despite the failed probes", res.State, StateOK)
	}
}

func TestIndexerHonoursCancellation(t *testing.T) {
	store := newMemStore()
	ctx, cancel := context.WithCancel(context.Background())
	ix, _ := probeRoot(t, store, nil, "a.pcap", "b.pcap", "c.pcap")
	ix.Probe = func(string, int) (Extent, error) {
		cancel()
		return Extent{FirstPacketNs: 1, LastPacketNs: 2, PacketCount: 1}, nil
	}

	if _, err := ix.Refresh(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh: %v, want context.Canceled", err)
	}
	if len(store.probes) > 1 {
		t.Errorf("probed %d files after cancellation, want at most 1", len(store.probes))
	}
}

func TestIndexerSkipsProbingWhenNoProberIsSet(t *testing.T) {
	// Metadata-only indexing is the fast path, and all a listing needs.
	store := newMemStore()
	ix, _ := probeRoot(t, store, nil, "a.pcap")

	res, err := ix.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if res.Probed != 0 || res.ProbeFailed != 0 {
		t.Errorf("Probed = %d, ProbeFailed = %d, want 0 and 0", res.Probed, res.ProbeFailed)
	}
	if res.State != StateOK {
		t.Errorf("State = %q, want %q", res.State, StateOK)
	}
	if len(res.Drift.Added) != 1 {
		t.Errorf("Added = %d, want 1", len(res.Drift.Added))
	}
}

func TestIndexerReportsProgress(t *testing.T) {
	store := newMemStore()
	ix, _ := probeRoot(t, store, okProbe(scanBase), "a.pcap", "b.pcap")
	var seen []int
	ix.OnProgress = func(done, total int, _ string) {
		seen = append(seen, done)
		if total != 2 {
			t.Errorf("progress total = %d, want 2", total)
		}
	}
	if _, err := ix.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	// One call before each file plus a final completion call.
	want := []int{0, 1, 2}
	if len(seen) != len(want) {
		t.Fatalf("progress reported %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("progress %d = %d, want %d", i, seen[i], want[i])
		}
	}
}

func TestIndexerSurfacesStoreFailures(t *testing.T) {
	for _, method := range []string{"IndexedFiles", "ApplyScan"} {
		t.Run(method, func(t *testing.T) {
			store := newMemStore()
			store.failOn = method
			ix, _ := probeRoot(t, store, okProbe(scanBase), "a.pcap")
			if _, err := ix.Refresh(context.Background()); err == nil {
				t.Fatalf("Refresh succeeded despite %s failing", method)
			}
		})
	}
}

func TestDriftSummary(t *testing.T) {
	clean := Drift{Unchanged: 195}
	if got := clean.Summary(); !strings.Contains(got, "no drift") || !strings.Contains(got, "195") {
		t.Errorf("clean summary = %q, want it to say no drift and the count", got)
	}
	dirty := Drift{Added: []File{{}}, Missing: []Indexed{{}}, Changed: []Change{{}}, Unchanged: 192}
	got := dirty.Summary()
	for _, want := range []string{"1 new", "1 missing", "1 changed", "192 unchanged"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q does not contain %q", got, want)
		}
	}
}

func TestExtentDuration(t *testing.T) {
	e := Extent{FirstPacketNs: scanBase.UnixNano(), LastPacketNs: scanBase.Add(5 * time.Minute).UnixNano()}
	if got := e.ExtentDuration(); got != 5*time.Minute {
		t.Errorf("ExtentDuration() = %v, want 5m", got)
	}
}
