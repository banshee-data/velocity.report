//go:build pcap

package lidarbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func dumpBuilder(t *testing.T, path string) *analysisFrameBuilder {
	t.Helper()
	fb := &analysisFrameBuilder{cfg: Config{ClustersOutput: path}}
	if err := fb.openClusterDump(); err != nil {
		t.Fatalf("openClusterDump: %v", err)
	}
	return fb
}

func readRecords(t *testing.T, path string) []clusterRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading dump: %v", err)
	}
	var recs []clusterRecord
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var rec clusterRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("parsing %q: %v", line, err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func TestClusterDumpNotAskedForWritesNothing(t *testing.T) {
	dir := t.TempDir()
	fb := &analysisFrameBuilder{cfg: Config{}}
	if err := fb.openClusterDump(); err != nil {
		t.Fatalf("openClusterDump: %v", err)
	}
	if fb.clusterOut != nil {
		t.Error("a run that asked for no dump opened one")
	}

	// The no-dump path is the one every perf-gate run takes, so it has to
	// survive being handed clusters.
	fb.writeClusters([]l4perception.WorldCluster{{PointsCount: 3}})
	fb.closeClusterDump()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("temp dir has %d entries, want none", len(entries))
	}
}

func TestClusterDumpRecordsGeometryPerFrame(t *testing.T) {
	// A nested directory: the sweep names a file under a per-run directory
	// that does not exist yet.
	path := filepath.Join(t.TempDir(), "run-07", "clusters.jsonl")
	fb := dumpBuilder(t, path)

	start := time.Unix(1700000000, 250)
	fb.frameCount, fb.frameStartTime = 4, start
	fb.writeClusters([]l4perception.WorldCluster{
		{CentroidX: 1.5, CentroidY: -2.25, CentroidZ: 0.5,
			BoundingBoxLength: 4, BoundingBoxWidth: 1.75, BoundingBoxHeight: 1.5, PointsCount: 120},
		{CentroidX: 30, CentroidY: 8, CentroidZ: -1,
			BoundingBoxLength: 0.5, BoundingBoxWidth: 0.5, BoundingBoxHeight: 0.25, PointsCount: 6},
	})
	fb.frameCount, fb.frameStartTime = 5, start.Add(100*time.Millisecond)
	fb.writeClusters([]l4perception.WorldCluster{{CentroidX: 2, PointsCount: 118}})
	fb.closeClusterDump()

	recs := readRecords(t, path)
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3", len(recs))
	}
	want := clusterRecord{
		Frame: 4, TSNs: start.UnixNano(), X: 1.5, Y: -2.25, Z: 0.5,
		Length: 4, Width: 1.75, Height: 1.5, Points: 120,
	}
	if recs[0] != want {
		t.Errorf("first record = %+v, want %+v", recs[0], want)
	}
	if recs[1].Frame != 4 || recs[1].Points != 6 {
		t.Errorf("second record = %+v, want frame 4 with 6 points", recs[1])
	}
	if recs[2].Frame != 5 || recs[2].TSNs != start.Add(100*time.Millisecond).UnixNano() {
		t.Errorf("third record = %+v, want frame 5 at the later timestamp", recs[2])
	}
}

func TestClusterDumpFrameWithNoClustersWritesNoLine(t *testing.T) {
	// Absence has to be readable as absence: a scorer counts lines per frame,
	// so an empty frame must contribute none rather than a null record.
	path := filepath.Join(t.TempDir(), "clusters.jsonl")
	fb := dumpBuilder(t, path)
	fb.frameCount = 9
	fb.writeClusters(nil)
	fb.closeClusterDump()

	if recs := readRecords(t, path); len(recs) != 0 {
		t.Errorf("got %d records, want none", len(recs))
	}
}

func TestClusterDumpClosedByFinalise(t *testing.T) {
	// The dump is buffered, so a run that never closed it would leave the last
	// frames of every sweep run missing.
	path := filepath.Join(t.TempDir(), "clusters.jsonl")
	fb := dumpBuilder(t, path)
	fb.writeClusters([]l4perception.WorldCluster{{PointsCount: 11}})
	fb.finalise()

	if fb.clusterOut != nil || fb.clusterFile != nil {
		t.Error("finalise left the dump open")
	}
	if recs := readRecords(t, path); len(recs) != 1 {
		t.Fatalf("got %d records after finalise, want 1", len(recs))
	}
	// Closing twice happens when finalise runs after an error path already
	// closed the dump.
	fb.closeClusterDump()
}

func TestClusterDumpUnwritablePathFailsBeforeReplay(t *testing.T) {
	// Discovering this after an hour of replay would waste the whole run.
	fb := &analysisFrameBuilder{cfg: Config{
		ClustersOutput: filepath.Join(t.TempDir(), "clusters.jsonl", "nested.jsonl"),
	}}
	if err := os.WriteFile(filepath.Dir(fb.cfg.ClustersOutput), []byte("x"), 0o600); err != nil {
		t.Fatalf("writing blocking file: %v", err)
	}
	if err := fb.openClusterDump(); err == nil {
		t.Error("openClusterDump accepted a path under a regular file")
	}
}

func TestClusterDumpStopsAfterWriteFailure(t *testing.T) {
	// A full disk mid-sweep must not take the benchmark down with it: the
	// measurement is still worth reporting.
	path := filepath.Join(t.TempDir(), "clusters.jsonl")
	fb := dumpBuilder(t, path)
	if err := fb.clusterFile.Close(); err != nil {
		t.Fatalf("closing underlying file: %v", err)
	}
	big := make([]l4perception.WorldCluster, 4096)
	fb.writeClusters(big)
	if fb.clusterOut != nil {
		t.Error("the dump kept writing after a failure")
	}
	fb.clusterFile = nil
	fb.closeClusterDump()
}
