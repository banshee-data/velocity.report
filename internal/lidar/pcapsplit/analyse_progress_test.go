//go:build pcap

package pcapsplit

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

func TestAnalyseContextStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AnalyseContext(ctx, fixtureConfig(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AnalyseContext on a cancelled context = %v, want context.Canceled", err)
	}
}

func TestAnalyseReportsProgressAgainstTheKnownExtent(t *testing.T) {
	cfg := fixtureConfig(t)
	count, err := network.CountPCAPPackets(cfg.PCAPFile, cfg.UDPPort)
	if err != nil {
		t.Fatalf("CountPCAPPackets: %v", err)
	}
	cfg.Extents = []capseq.Segment{{
		Path:        cfg.PCAPFile,
		FirstPacket: time.Unix(0, count.FirstTimestampNs),
		LastPacket:  time.Unix(0, count.LastTimestampNs),
		PacketCount: count.Count,
	}}
	var calls int
	var lastCurrent, lastTotal uint64
	cfg.OnProgress = func(current, total uint64) {
		calls++
		lastCurrent, lastTotal = current, total
	}
	if _, err := Analyse(cfg); err != nil {
		t.Fatalf("Analyse: %v", err)
	}
	if calls == 0 {
		t.Fatal("OnProgress was never called")
	}
	if lastTotal != count.Count {
		t.Errorf("progress total = %d, want the extent's %d packets", lastTotal, count.Count)
	}
	if lastCurrent == 0 || lastCurrent > lastTotal {
		t.Errorf("last progress = %d of %d, want some of the capture read", lastCurrent, lastTotal)
	}
}

// Captures the index has probed are joined from their stored extents. Neither
// path below exists, so counting them would fail: a sequence means the extents
// were used and nothing was read.
func TestSequenceForAnalysisJoinsKnownExtentsWithoutReading(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pcap"), filepath.Join(dir, "b.pcap")
	start := time.Date(2026, 9, 3, 11, 20, 0, 0, time.UTC)
	extents := []capseq.Segment{
		{Path: a, FirstPacket: start, LastPacket: start.Add(5 * time.Minute), PacketCount: 1000},
		{Path: b, FirstPacket: start.Add(5*time.Minute + time.Millisecond),
			LastPacket: start.Add(10 * time.Minute), PacketCount: 1000},
	}
	cfg := SplitConfig{PCAPFile: a, PCAPFiles: []string{a, b}, UDPPort: 2369, Extents: extents}

	seq, err := sequenceForAnalysis(cfg, cfg.PCAPFiles)
	if err != nil {
		t.Fatalf("sequenceForAnalysis: %v", err)
	}
	if !seq.Continuous() || seq.Start != start {
		t.Errorf("sequence = %+v, want a continuous one starting %v", seq, start)
	}

	// Extents that do not name the files in order are not trusted: the
	// captures are counted, which here fails on the missing files.
	cfg.Extents = []capseq.Segment{extents[1], extents[0]}
	if _, err := sequenceForAnalysis(cfg, cfg.PCAPFiles); err == nil || !strings.Contains(err.Error(), "probing") {
		t.Errorf("reordered extents: err = %v, want the files counted", err)
	}
	cfg.Extents = extents[:1]
	if _, err := sequenceForAnalysis(cfg, cfg.PCAPFiles); err == nil || !strings.Contains(err.Error(), "probing") {
		t.Errorf("a partial set of extents: err = %v, want the files counted", err)
	}
}
