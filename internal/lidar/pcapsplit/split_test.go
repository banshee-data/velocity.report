package pcapsplit

import (
	"strings"
	"testing"
	"time"
)

func periodsFixture() []MotionPeriod {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return []MotionPeriod{
		{Type: MotionLabel, StartSecs: 0, EndSecs: 40, DurationSecs: 40, StartTime: base, EndTime: base.Add(40 * time.Second)},
		{Type: StaticLabel, StartSecs: 40, EndSecs: 130, DurationSecs: 90, StartTime: base.Add(40 * time.Second), EndTime: base.Add(130 * time.Second)},
		{Type: MotionLabel, StartSecs: 130, EndSecs: 170, DurationSecs: 40, StartTime: base.Add(130 * time.Second), EndTime: base.Add(170 * time.Second)},
	}
}

func TestBuildSegments_NamingAndIDs(t *testing.T) {
	segs := BuildSegments(periodsFixture(), "out")
	if len(segs) != 3 {
		t.Fatalf("want 3 segments, got %d", len(segs))
	}
	wantNames := []string{"out-motion-0.pcap", "out-static-0.pcap", "out-motion-1.pcap"}
	for i, w := range wantNames {
		if segs[i].Filename != w {
			t.Errorf("segment %d filename=%s, want %s", i, segs[i].Filename, w)
		}
	}
	// Per-type IDs increment independently.
	if segs[0].ID != 0 || segs[2].ID != 1 {
		t.Errorf("motion IDs: got %d and %d, want 0 and 1", segs[0].ID, segs[2].ID)
	}
	if segs[1].ID != 0 {
		t.Errorf("static ID: got %d, want 0", segs[1].ID)
	}
}

func TestBuildSegments_DefaultPrefix(t *testing.T) {
	segs := BuildSegments(periodsFixture()[:1], "")
	if segs[0].Filename != "out-motion-0.pcap" {
		t.Errorf("empty prefix should default to 'out', got %s", segs[0].Filename)
	}
}

func TestSplitConfigEffectiveOutputPrefix(t *testing.T) {
	cfg := DefaultSplitConfig()
	cfg.PCAPFile = "/tmp/soma2.pcapng"
	if got := cfg.EffectiveOutputPrefix(); got != "soma2" {
		t.Fatalf("EffectiveOutputPrefix() = %q, want soma2", got)
	}
	cfg.OutputPrefix = "manual"
	if got := cfg.EffectiveOutputPrefix(); got != "manual" {
		t.Fatalf("explicit EffectiveOutputPrefix() = %q, want manual", got)
	}
}

func TestDeriveOutputPrefixFallsBackForEmptyNames(t *testing.T) {
	for _, input := range []string{"", ".pcap"} {
		if got := deriveOutputPrefix(input); got != "out" {
			t.Errorf("deriveOutputPrefix(%q) = %q, want out", input, got)
		}
	}
}

func TestSegmentIndexForTime(t *testing.T) {
	segs := BuildSegments(periodsFixture(), "out")
	base := segs[0].StartTime

	cases := []struct {
		name   string
		at     time.Time
		expect int
	}{
		{"before start clamps to 0", base.Add(-5 * time.Second), 0},
		{"inside first", base.Add(10 * time.Second), 0},
		{"exact boundary goes to next", base.Add(40 * time.Second), 1},
		{"inside second", base.Add(100 * time.Second), 1},
		{"inside third", base.Add(150 * time.Second), 2},
		{"after end clamps to last", base.Add(999 * time.Second), 2},
	}
	for _, c := range cases {
		if got := segmentIndexForTime(segs, c.at); got != c.expect {
			t.Errorf("%s: got %d, want %d", c.name, got, c.expect)
		}
	}

	if got := segmentIndexForTime(nil, base); got != -1 {
		t.Errorf("empty segments: got %d, want -1", got)
	}
}

func TestAssignFrameStates(t *testing.T) {
	segs := BuildSegments(periodsFixture(), "out")
	base := segs[0].StartTime
	frames := []FrameMetrics{
		{FrameID: 0, T: base.Add(5 * time.Second)},
		{FrameID: 1, T: base.Add(60 * time.Second)},
		{FrameID: 2, T: base.Add(150 * time.Second)},
	}
	AssignFrameStates(frames, segs)
	want := []string{MotionLabel, StaticLabel, MotionLabel}
	for i, w := range want {
		if frames[i].State != w {
			t.Errorf("frame %d state=%s, want %s", i, frames[i].State, w)
		}
	}
}

func TestFormatSummary(t *testing.T) {
	segs := BuildSegments(periodsFixture(), "out")
	segs[0].PacketCount = 100
	r := Report{
		InputFile:        "/tmp/capture.pcapng",
		ProcessingTimeMs: 1500,
		TotalPackets:     300,
		TotalFrames:      170,
		TotalDurationSec: 170,
		Config:           DefaultSplitConfig(),
		Segments:         segs,
	}
	out := FormatSummary(r)
	for _, want := range []string{
		"PCAP Split Analysis Summary",
		"/tmp/capture.pcapng",
		"2 motion", "1 static",
		"out-motion-0.pcap",
		"100 packets",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}
