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
	// A 1ns interval makes the first packet (after any elapsed time) report.
	p := NewProgressStats(inner, 1000, time.Nanosecond, "scan", &buf)
	time.Sleep(time.Millisecond)
	p.AddPacket(500)
	p.AddPoints(40)
	p.AddDropped()

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
}

func TestProgressStatsNoPercentWithoutSize(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressStats(&recordStats{}, 0 /* unknown size */, time.Nanosecond, "scan", &buf)
	time.Sleep(time.Millisecond)
	p.AddPacket(500)
	if strings.Contains(buf.String(), "%") {
		t.Errorf("fileSize 0 should suppress the percentage: %q", buf.String())
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
