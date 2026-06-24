package network

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

type recordStats struct{ packets, points, dropped int }

func (r *recordStats) AddPacket(int)   { r.packets++ }
func (r *recordStats) AddDropped()     { r.dropped++ }
func (r *recordStats) AddPoints(n int) { r.points += n }
func (r *recordStats) LogStats(bool)   {}

func TestProgressStatsDisabledReturnsInner(t *testing.T) {
	inner := &recordStats{}
	if got := NewProgressStats(inner, 100, 0, "x", io.Discard); got != inner {
		t.Fatal("interval <= 0 must return the inner stats undecorated")
	}
}

func TestProgressStatsForwardsAndReports(t *testing.T) {
	inner := &recordStats{}
	var buf bytes.Buffer
	// A 1ns interval makes the next ObserveProgress report.
	p := NewProgressStats(inner, 1000, time.Nanosecond, "scan", &buf)
	p.AddPacket(500)
	p.AddPoints(40)
	p.AddDropped()
	time.Sleep(time.Millisecond)
	p.(ProgressObserver).ObserveProgress(time.Unix(1000, 0), 600)

	if inner.packets != 1 || inner.points != 40 || inner.dropped != 1 {
		t.Fatalf("inner stats not forwarded: %+v", inner)
	}
	out := buf.String()
	if !strings.Contains(out, "[scan]") || !strings.Contains(out, "packets") {
		t.Fatalf("missing progress line: %q", out)
	}
	if !strings.Contains(out, "~50.0%") { // 500 payload bytes of a 1000-byte file
		t.Errorf("expected ~50%% with fileSize 1000: %q", out)
	}
	if !strings.Contains(out, "600 rpm") {
		t.Errorf("expected rpm in line: %q", out)
	}
}

func TestProgressStatsReportsPointsPerFrame(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressStats(&recordStats{}, 1000, time.Nanosecond, "scan", &buf).(*ProgressStats)
	t0 := time.Unix(1000, 0)
	p.AddPacket(100)
	p.ObserveProgress(t0, 600) // seed the capture baseline (and an empty first report)
	buf.Reset()

	// 600 rpm over 10 s of capture is 100 frames; add 7,000,000 points.
	for range 100 {
		p.AddPoints(70_000)
	}
	time.Sleep(time.Millisecond)
	p.ObserveProgress(t0.Add(10*time.Second), 600)

	out := buf.String()
	if !strings.Contains(out, "70000 pts/frame") {
		t.Errorf("expected 70000 pts/frame (7M pts / 100 frames): %q", out)
	}
}

func TestProgressStatsNoPercentWithoutSize(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressStats(&recordStats{}, 0 /* unknown size */, time.Nanosecond, "scan", &buf)
	p.AddPacket(500)
	time.Sleep(time.Millisecond)
	p.(ProgressObserver).ObserveProgress(time.Unix(1000, 0), 600)
	if strings.Contains(buf.String(), "%") {
		t.Errorf("fileSize 0 should suppress the percentage: %q", buf.String())
	}
}

func TestPacketOnlyProgressStatsSuppressesPointAndRPMColumns(t *testing.T) {
	var buf bytes.Buffer
	p := NewPacketOnlyProgressStats(&recordStats{}, 1000, time.Nanosecond, "write", &buf)
	p.AddPacket(500)
	time.Sleep(time.Millisecond)
	p.(ProgressObserver).ObserveProgress(time.Unix(1000, 0), 0)

	out := buf.String()
	if !strings.Contains(out, "[write]") || !strings.Contains(out, "packets") {
		t.Fatalf("missing packet-only progress line: %q", out)
	}
	for _, forbidden := range []string{"points", "rpm", "pts/frame"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("packet-only progress line should omit %q: %q", forbidden, out)
		}
	}
	if !strings.Contains(out, "elapsed") || !strings.Contains(out, "pkt/s") {
		t.Fatalf("packet-only line missing elapsed/rate columns: %q", out)
	}
}

func TestProgressFormatHelpers(t *testing.T) {
	if got := commaInt(1234567); got != "1,234,567" {
		t.Errorf("commaInt = %q", got)
	}
	if got := compactInt(308_000_000); got != "308M" {
		t.Errorf("compactInt(M) = %q", got)
	}
	if got := compactInt(12_500); got != "12.5K" {
		t.Errorf("compactInt(K) = %q", got)
	}
	if got := mmss(90 * time.Second); got != "01:30" {
		t.Errorf("mmss = %q", got)
	}
	if got := mmss(3725 * time.Second); got != "1:02:05" {
		t.Errorf("mmss(hours) = %q", got)
	}
}
