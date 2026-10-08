package server

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// indexedReplayServer is a server whose replay safe directory is also its one
// capture volume, scanned and probed (on port 2368) before it is returned. The
// directory is symlink-resolved, as resolvePCAPPath resolves the paths it
// returns, so on macOS /var and /private/var do not part the two.
func indexedReplayServer(t *testing.T, names ...string) (*Server, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCaptures(t, dir, names...)
	ws := captureAPIServer(t, dir)
	ws.pcapSafeDir = dir
	stubCaptureProber(t)
	payload := doRequest(t, ws, "POST", "/api/lidar/capture/scan", ws.handleCaptureScan)
	if root := payload["roots"].([]any)[0].(map[string]any); root["state"] != "ok" {
		t.Fatalf("scan state = %v (%v)", root["state"], root["error"])
	}
	return ws, dir
}

// countedPackets replaces countPCAPPackets with one that records each file it
// is asked to read and answers from extents, keyed by base name.
func countedPackets(t *testing.T, extents map[string]network.PCAPCountResult) *[]string {
	t.Helper()
	var read []string
	original := countPCAPPackets
	t.Cleanup(func() { countPCAPPackets = original })
	countPCAPPackets = func(path string, _ int) (network.PCAPCountResult, error) {
		read = append(read, filepath.Base(path))
		if res, ok := extents[filepath.Base(path)]; ok {
			return res, nil
		}
		return network.PCAPCountResult{}, errors.New("no count for " + path)
	}
	return &read
}

func TestBuildReplaySequenceTakesExtentsFromTheIndex(t *testing.T) {
	ws, _ := indexedReplayServer(t, "a.pcap", "b.pcap")
	read := countedPackets(t, nil)

	plan, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap"}, 0, -1)
	if err != nil {
		t.Fatalf("buildReplaySequence: %v", err)
	}
	if len(*read) != 0 {
		t.Errorf("read %v to count them, want the index's probe used", *read)
	}
	if len(plan.Steps) != 2 || plan.TotalPackets != 2*540000 {
		t.Errorf("plan = %d steps, %d packets; want 2 steps of the index's 540000", len(plan.Steps), plan.TotalPackets)
	}
}

func TestBuildReplaySequenceCountsWhatTheIndexCannotVouchFor(t *testing.T) {
	ws, dir := indexedReplayServer(t, "a.pcap", "b.pcap")
	// b.pcap is rewritten after its probe, so its indexed extent is stale.
	if err := os.WriteFile(filepath.Join(dir, "b.pcap"), []byte("a different capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	extents := ws.loadIndexedCaptures()
	a, _ := extents.extent(filepath.Join(dir, "a.pcap"), 2368)
	read := countedPackets(t, map[string]network.PCAPCountResult{
		"b.pcap": {Count: 7, FirstTimestampNs: a.LastPacket.UnixNano() + 1_000_000,
			LastTimestampNs: a.LastPacket.UnixNano() + 300_000_000_000},
	})

	if _, err := ws.buildReplaySequence([]string{"a.pcap", "b.pcap"}, 0, -1); err != nil {
		t.Fatalf("buildReplaySequence: %v", err)
	}
	if len(*read) != 1 || (*read)[0] != "b.pcap" {
		t.Errorf("counted %v, want only the rewritten b.pcap", *read)
	}

	// A replay reading another port than the one probed counts every file.
	ws.udpPort = 2369
	reread := countedPackets(t, nil)
	_, _ = ws.buildReplaySequence([]string{"a.pcap"}, 0, -1)
	if len(*reread) != 1 {
		t.Errorf("a capture probed on 2368, replayed on 2369: counted %v, want it counted", *reread)
	}
}

func TestValidateCaseFilesPrefersTheIndexOverProbing(t *testing.T) {
	// Reading a 700 MB capture to learn what the index already knows would make
	// authoring a case from the Captures page a minute-long wait per file.
	ws, _ := indexedReplayServer(t, "a.pcap", "b.pcap")
	probed := 0
	original := probeExtent
	t.Cleanup(func() { probeExtent = original })
	probeExtent = func(string, int) (capindex.Extent, error) {
		probed++
		return capindex.Extent{}, nil
	}

	seq, err := ws.validateCaseFiles([]string{"a.pcap", "b.pcap"})
	if err != nil {
		t.Fatalf("validateCaseFiles: %v", err)
	}
	if len(seq.Segments) != 2 || seq.Segments[0].PacketCount != 540000 {
		t.Errorf("sequence = %+v, want the index's two captures", seq.Segments)
	}
	if probed != 0 {
		t.Errorf("read %d captures to learn what the index already held", probed)
	}
}

// Copies of a capture in two folders share a file name. Only the file the
// index probed is its match; the copy elsewhere is read for itself.
func TestIndexedCapturesMatchTheExactFile(t *testing.T) {
	ws, dir := indexedReplayServer(t, "a.pcap")
	other := filepath.Join(dir, "elsewhere")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	writeCaptures(t, other, "a.pcap")

	index := ws.loadIndexedCaptures()
	if _, ok := index.extent(filepath.Join(dir, "a.pcap"), 0); !ok {
		t.Error("the indexed capture was not found")
	}
	if e, ok := index.extent(filepath.Join(other, "a.pcap"), 0); ok {
		t.Errorf("a copy outside the index matched by name: %+v", e)
	}
}

func TestIndexedCapturesWithoutADatabaseMissEverything(t *testing.T) {
	ws := &Server{}
	if _, ok := ws.loadIndexedCaptures().extent("/any/a.pcap", 0); ok {
		t.Error("a server with no index found a capture in it")
	}
}
