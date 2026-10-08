package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// indexedCapture writes a capture and returns its index row as a probe would
// have left it.
func indexedCapture(t *testing.T, dir, name string, first time.Time, count int64) (sqlite.CaptureFile, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("pcap bytes "+name), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	firstNs, lastNs := first.UnixNano(), first.Add(5*time.Minute).UnixNano()
	return sqlite.CaptureFile{
		RelPath: name, SizeBytes: info.Size(), ModifiedAtNs: info.ModTime().UnixNano(),
		FirstPacketNs: &firstNs, LastPacketNs: &lastNs, PacketCount: &count,
	}, path
}

func TestIndexedExtentsUseTheProbe(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 9, 3, 11, 20, 0, 0, time.UTC)
	a, pa := indexedCapture(t, dir, "a.pcap", start, 1000)
	b, pb := indexedCapture(t, dir, "b.pcap", start.Add(5*time.Minute), 2000)

	got := indexedExtents([]sqlite.CaptureFile{a, b}, []string{pa, pb}, os.Stat)
	if len(got) != 2 {
		t.Fatalf("extents = %+v, want both captures", got)
	}
	if got[1].Path != pb || got[1].PacketCount != 2000 || !got[1].FirstPacket.Equal(start.Add(5*time.Minute)) {
		t.Errorf("second extent = %+v, want b.pcap's probe", got[1])
	}
}

func TestIndexedExtentsCountWhenTheIndexCannotBeTrusted(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 9, 3, 11, 20, 0, 0, time.UTC)
	a, pa := indexedCapture(t, dir, "a.pcap", start, 1000)
	b, pb := indexedCapture(t, dir, "b.pcap", start.Add(5*time.Minute), 2000)
	files, paths := []sqlite.CaptureFile{a, b}, []string{pa, pb}

	unprobed := b
	unprobed.PacketCount = nil
	if got := indexedExtents([]sqlite.CaptureFile{a, unprobed}, paths, os.Stat); got != nil {
		t.Errorf("a capture with no probe: extents = %+v, want nil", got)
	}

	// Rewritten since it was indexed: its probe no longer describes it.
	if err := os.WriteFile(pb, []byte("a longer capture than before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := indexedExtents(files, paths, os.Stat); got != nil {
		t.Errorf("a capture rewritten since its probe: extents = %+v, want nil", got)
	}

	if got := indexedExtents(files, paths[:1], os.Stat); got != nil {
		t.Errorf("paths not matching files: extents = %+v, want nil", got)
	}
}

// clock is a settable time source for motionProgress.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestMotionProgressNamesTheCaptureBeingRead(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)}
	extents := []capseq.Segment{{PacketCount: 100}, {PacketCount: 300}, {PacketCount: 100}}
	m := newMotionProgress(extents, 3, 2*time.Second, c.now)

	p, ok := m.observe(50, 500)
	if !ok || p.Detail != "classifying capture 1 of 3" || p.Current != 50 || p.Total != 500 {
		t.Fatalf("first reading = %+v, %v; want capture 1 of 3 at 50 of 500", p, ok)
	}
	// Within the interval and the same capture: held back.
	c.t = c.t.Add(time.Second)
	if _, ok := m.observe(60, 500); ok {
		t.Error("a reading within the interval in the same capture was passed on")
	}
	// A new capture is passed on at once.
	if p, ok := m.observe(150, 500); !ok || p.Detail != "classifying capture 2 of 3" {
		t.Errorf("entering capture 2 = %+v, %v; want it passed on", p, ok)
	}
	// So is the same capture once the interval has gone by.
	c.t = c.t.Add(3 * time.Second)
	if p, ok := m.observe(200, 500); !ok || p.Current != 200 {
		t.Errorf("after the interval = %+v, %v; want it passed on", p, ok)
	}
	if p, _ := m.observe(450, 500); p.Detail != "classifying capture 3 of 3" {
		t.Errorf("in the last capture = %q", p.Detail)
	}
}

// A slow job-row write must not hold up the reader: sends return at once while
// a write is blocked, superseded updates are dropped, and stop delivers the
// newest before returning.
func TestNewestProgressNeverBlocksTheReader(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var written []int64
	send, stop := newestProgress(func(p capjobs.Progress) {
		if p.Current == 1 {
			close(started)
			<-release // the first write is slow
		}
		written = append(written, p.Current)
	})

	send(capjobs.Progress{Current: 1})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the first update was never written")
	}
	sent := make(chan struct{})
	go func() {
		for i := int64(2); i <= 50; i++ {
			send(capjobs.Progress{Current: i})
		}
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("send blocked behind a slow write")
	}
	close(release)
	stop()

	// Everything sent during the slow write replaced the update before it.
	if len(written) != 2 || written[0] != 1 || written[1] != 50 {
		t.Errorf("written = %v, want [1 50]: the first, then only the newest", written)
	}
}

func TestMotionProgressWithoutExtents(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)}
	m := newMotionProgress(nil, 4, time.Second, c.now)

	// With a total, the capture is estimated from the share read.
	if p, _ := m.observe(600, 1000); p.Detail != "classifying capture 3 of 4" {
		t.Errorf("60%% of 4 captures = %q, want capture 3", p.Detail)
	}
	if got := m.captureAt(1000, 1000); got != 4 {
		t.Errorf("the last packet's capture = %d, want 4", got)
	}
	// With no total there is nothing to estimate from.
	c.t = c.t.Add(2 * time.Second)
	if p, _ := m.observe(10, 0); p.Detail != "classifying 4 captures" || p.Total != 0 {
		t.Errorf("no total = %+v, want the capture count alone", p)
	}
}
