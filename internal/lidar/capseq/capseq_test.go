package capseq

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// base is an arbitrary fixed capture start; using a constant keeps the expected
// offsets in the tests readable.
var base = time.Date(2026, 9, 1, 17, 42, 0, 0, time.UTC)

// seg builds a segment starting at base+offset and running for dur.
func seg(path string, offset, dur time.Duration) Segment {
	return Segment{
		Path:        path,
		FirstPacket: base.Add(offset),
		LastPacket:  base.Add(offset + dur),
		PacketCount: 1000,
	}
}

// fiveMin is the field capture's rolling file length.
const fiveMin = 5 * time.Minute

func TestGradeGap(t *testing.T) {
	tol := DefaultTolerances()
	tests := []struct {
		name string
		gap  time.Duration
		want SeamGrade
	}{
		{"exactly abutting", 0, SeamSeamless},
		{"one millisecond", time.Millisecond, SeamSeamless},
		{"at the seamless bound", 10 * time.Millisecond, SeamSeamless},
		{"just past seamless", 10*time.Millisecond + time.Microsecond, SeamAcceptable},
		{"half a second", 500 * time.Millisecond, SeamAcceptable},
		{"at the max gap", time.Second, SeamAcceptable},
		{"just past the max gap", time.Second + time.Microsecond, SeamBroken},
		{"ten seconds", 10 * time.Second, SeamBroken},
		{"small overlap inside tolerance", -5 * time.Millisecond, SeamSeamless},
		{"overlap at the bound", -10 * time.Millisecond, SeamSeamless},
		{"overlap past the bound", -11 * time.Millisecond, SeamOverlap},
		{"whole revolution overlap", -50 * time.Millisecond, SeamOverlap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := GradeGap(tc.gap, tol); got != tc.want {
				t.Errorf("GradeGap(%v) = %q, want %q", tc.gap, got, tc.want)
			}
		})
	}
}

func TestGradeGapZeroTolerancesUseDefaults(t *testing.T) {
	// A zero Tolerances must not grade every join as broken; it falls back to
	// the field defaults.
	if got := GradeGap(5*time.Millisecond, Tolerances{}); got != SeamSeamless {
		t.Errorf("zero tolerances graded a 5ms gap as %q, want %q", got, SeamSeamless)
	}
	if got := GradeGap(2*time.Second, Tolerances{}); got != SeamBroken {
		t.Errorf("zero tolerances graded a 2s gap as %q, want %q", got, SeamBroken)
	}
}

func TestSeamGradeReplayable(t *testing.T) {
	for _, tc := range []struct {
		grade SeamGrade
		want  bool
	}{
		{SeamSeamless, true},
		{SeamAcceptable, true},
		{SeamBroken, false},
		{SeamOverlap, false},
	} {
		if got := tc.grade.Replayable(); got != tc.want {
			t.Errorf("%q.Replayable() = %v, want %v", tc.grade, got, tc.want)
		}
	}
}

func TestBuildRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		segs []Segment
	}{
		{"empty", nil},
		{"empty path", []Segment{{FirstPacket: base, LastPacket: base.Add(time.Minute)}}},
		{"unset first packet", []Segment{{Path: "a.pcap", LastPacket: base}}},
		{"unset last packet", []Segment{{Path: "a.pcap", FirstPacket: base}}},
		{"inverted extent", []Segment{{Path: "a.pcap", FirstPacket: base.Add(time.Minute), LastPacket: base}}},
		{"duplicate path", []Segment{seg("a.pcap", 0, fiveMin), seg("a.pcap", fiveMin, fiveMin)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build(tc.segs, DefaultTolerances()); err == nil {
				t.Fatal("Build accepted malformed input, want an error")
			}
		})
	}
}

func TestBuildEmptyReturnsErrNoSegments(t *testing.T) {
	_, err := Build(nil, DefaultTolerances())
	if !errors.Is(err, ErrNoSegments) {
		t.Fatalf("Build(nil) error = %v, want ErrNoSegments", err)
	}
}

func TestBuildSingleSegmentHasNoSeams(t *testing.T) {
	s, err := Build([]Segment{seg("a.pcap", 0, fiveMin)}, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(s.Seams) != 0 {
		t.Errorf("single segment produced %d seams, want 0", len(s.Seams))
	}
	if s.Worst != SeamSeamless {
		t.Errorf("Worst = %q, want %q", s.Worst, SeamSeamless)
	}
	if !s.Continuous() {
		t.Error("single segment is not Continuous")
	}
	if s.Span != fiveMin || s.Covered != fiveMin {
		t.Errorf("Span = %v, Covered = %v, want both %v", s.Span, s.Covered, fiveMin)
	}
	if s.Lost != 0 {
		t.Errorf("Lost = %v, want 0", s.Lost)
	}
}

func TestBuildOrdersUnsortedInput(t *testing.T) {
	// Presented newest-first, as a directory walk might hand them over.
	segs := []Segment{
		seg("c.pcap", 2*fiveMin, fiveMin),
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []string{"a.pcap", "b.pcap", "c.pcap"}
	for i, w := range want {
		if s.Segments[i].Path != w {
			t.Errorf("Segments[%d].Path = %q, want %q", i, s.Segments[i].Path, w)
		}
	}
	if s.Worst != SeamSeamless {
		t.Errorf("Worst = %q, want %q for abutting files", s.Worst, SeamSeamless)
	}
	if s.PacketCount != 3000 {
		t.Errorf("PacketCount = %d, want 3000", s.PacketCount)
	}
}

func TestBuildTieBreaksOnPath(t *testing.T) {
	// Two files claiming the same first packet must order deterministically.
	segs := []Segment{seg("b.pcap", 0, fiveMin), seg("a.pcap", 0, fiveMin)}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if s.Segments[0].Path != "a.pcap" {
		t.Errorf("Segments[0].Path = %q, want %q", s.Segments[0].Path, "a.pcap")
	}
}

func TestBuildGradesAndAccumulatesGaps(t *testing.T) {
	// a → b is seamless (5ms), b → c is acceptable (400ms).
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin+5*time.Millisecond, fiveMin),
		seg("c.pcap", 2*fiveMin+405*time.Millisecond, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(s.Seams) != 2 {
		t.Fatalf("got %d seams, want 2", len(s.Seams))
	}
	if s.Seams[0].Grade != SeamSeamless {
		t.Errorf("seam 0 grade = %q, want %q", s.Seams[0].Grade, SeamSeamless)
	}
	if s.Seams[0].Before != "a.pcap" || s.Seams[0].After != "b.pcap" {
		t.Errorf("seam 0 joins %q → %q, want a.pcap → b.pcap", s.Seams[0].Before, s.Seams[0].After)
	}
	if s.Seams[1].Grade != SeamAcceptable {
		t.Errorf("seam 1 grade = %q, want %q", s.Seams[1].Grade, SeamAcceptable)
	}
	if s.Worst != SeamAcceptable {
		t.Errorf("Worst = %q, want %q", s.Worst, SeamAcceptable)
	}
	if want := 405 * time.Millisecond; s.Lost != want {
		t.Errorf("Lost = %v, want %v", s.Lost, want)
	}
	if want := 3 * fiveMin; s.Covered != want {
		t.Errorf("Covered = %v, want %v", s.Covered, want)
	}
	if want := 3*fiveMin + 405*time.Millisecond; s.Span != want {
		t.Errorf("Span = %v, want %v", s.Span, want)
	}
	if !s.Continuous() {
		t.Error("sequence with a seamless and an acceptable seam is not Continuous")
	}
}

func TestBuildDetectsBrokenAndOverlap(t *testing.T) {
	t.Run("broken", func(t *testing.T) {
		segs := []Segment{
			seg("a.pcap", 0, fiveMin),
			seg("b.pcap", fiveMin+30*time.Second, fiveMin),
		}
		s, err := Build(segs, DefaultTolerances())
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if s.Worst != SeamBroken {
			t.Errorf("Worst = %q, want %q", s.Worst, SeamBroken)
		}
		if s.Continuous() {
			t.Error("sequence with a 30s gap reports Continuous")
		}
		if got := s.BrokenSeams(); len(got) != 1 {
			t.Errorf("BrokenSeams returned %d, want 1", len(got))
		}
	})

	t.Run("overlap", func(t *testing.T) {
		// b starts a full second before a ends.
		segs := []Segment{
			seg("a.pcap", 0, fiveMin),
			seg("b.pcap", fiveMin-time.Second, fiveMin),
		}
		s, err := Build(segs, DefaultTolerances())
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if s.Worst != SeamOverlap {
			t.Errorf("Worst = %q, want %q", s.Worst, SeamOverlap)
		}
		if s.Continuous() {
			t.Error("overlapping sequence reports Continuous")
		}
	})
}

func TestBuildWorstIsTheWeakestSeam(t *testing.T) {
	// A broken join anywhere dominates, even surrounded by seamless ones.
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin, fiveMin),
		seg("c.pcap", 2*fiveMin+5*time.Second, fiveMin),
		seg("d.pcap", 3*fiveMin+5*time.Second, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if s.Worst != SeamBroken {
		t.Errorf("Worst = %q, want %q", s.Worst, SeamBroken)
	}
}

func TestOffsetOf(t *testing.T) {
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin, fiveMin),
		seg("c.pcap", 2*fiveMin, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for i, want := range []time.Duration{0, fiveMin, 2 * fiveMin} {
		if got := s.OffsetOf(i); got != want {
			t.Errorf("OffsetOf(%d) = %v, want %v", i, got, want)
		}
	}
	// Out-of-range indices return zero rather than panicking.
	if got := s.OffsetOf(-1); got != 0 {
		t.Errorf("OffsetOf(-1) = %v, want 0", got)
	}
	if got := s.OffsetOf(99); got != 0 {
		t.Errorf("OffsetOf(99) = %v, want 0", got)
	}
}

// nearly compares seconds with a tolerance that ignores float representation
// noise but would still catch an off-by-one-packet error.
func nearly(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func TestPlanWholeSequence(t *testing.T) {
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin, fiveMin),
		seg("c.pcap", 2*fiveMin, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	steps, err := s.Plan(0, -1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}
	for i, st := range steps {
		nearly(t, "steps["+st.Path+"].StartSecs", st.StartSecs, 0)
		if st.DurationSecs != -1 {
			t.Errorf("steps[%d].DurationSecs = %v, want -1 (to end of file)", i, st.DurationSecs)
		}
		if st.DropFrameAtStart {
			t.Errorf("steps[%d] drops a frame across a seamless join", i)
		}
		if st.PacketCount != 1000 {
			t.Errorf("steps[%d].PacketCount = %d, want 1000", i, st.PacketCount)
		}
	}
}

func TestPlanDropsFrameOnlyAtNonSeamlessJoins(t *testing.T) {
	// a → b seamless, b → c acceptable.
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin+2*time.Millisecond, fiveMin),
		seg("c.pcap", 2*fiveMin+402*time.Millisecond, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	steps, err := s.Plan(0, -1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	want := []bool{false, false, true}
	for i, st := range steps {
		if st.DropFrameAtStart != want[i] {
			t.Errorf("steps[%d] (%s).DropFrameAtStart = %v, want %v",
				i, st.Path, st.DropFrameAtStart, want[i])
		}
	}
}

func TestPlanFirstStepNeverDropsAFrame(t *testing.T) {
	// The window starts inside b, so b is the first step. Even though the a → b
	// join is merely acceptable, there is no in-flight revolution to discard.
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin+500*time.Millisecond, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	steps, err := s.Plan(fiveMin.Seconds()+30, -1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Path != "b.pcap" {
		t.Fatalf("first step is %q, want b.pcap", steps[0].Path)
	}
	if steps[0].DropFrameAtStart {
		t.Error("the first step of a plan dropped a frame")
	}
}

func TestPlanWindowInsideOneFile(t *testing.T) {
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin, fiveMin),
		seg("c.pcap", 2*fiveMin, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// 60s of b, starting 60s into b.
	steps, err := s.Plan(fiveMin.Seconds()+60, 60)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Path != "b.pcap" {
		t.Errorf("step path = %q, want b.pcap", steps[0].Path)
	}
	nearly(t, "StartSecs", steps[0].StartSecs, 60)
	nearly(t, "DurationSecs", steps[0].DurationSecs, 60)
}

func TestPlanWindowSpanningThreeFiles(t *testing.T) {
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin, fiveMin),
		seg("c.pcap", 2*fiveMin, fiveMin),
		seg("d.pcap", 3*fiveMin, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Start 200s into a, run for 600s: tail of a, all of b, head of c.
	steps, err := s.Plan(200, 600)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps (%v), want 3", len(steps), stepPaths(steps))
	}

	nearly(t, "a.StartSecs", steps[0].StartSecs, 200)
	if steps[0].DurationSecs != -1 {
		t.Errorf("a.DurationSecs = %v, want -1 (runs to end of file)", steps[0].DurationSecs)
	}

	nearly(t, "b.StartSecs", steps[1].StartSecs, 0)
	if steps[1].DurationSecs != -1 {
		t.Errorf("b.DurationSecs = %v, want -1 (runs to end of file)", steps[1].DurationSecs)
	}

	// The window ends 800s in; c starts at 600s, so 200s of c.
	nearly(t, "c.StartSecs", steps[2].StartSecs, 0)
	nearly(t, "c.DurationSecs", steps[2].DurationSecs, 200)
}

func stepPaths(steps []ReadStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Path
	}
	return out
}

func TestPlanClampsOversizedWindow(t *testing.T) {
	segs := []Segment{seg("a.pcap", 0, fiveMin), seg("b.pcap", fiveMin, fiveMin)}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Ask for an hour from a sequence that is ten minutes long.
	steps, err := s.Plan(0, 3600)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	for i, st := range steps {
		if st.DurationSecs != -1 {
			t.Errorf("steps[%d].DurationSecs = %v, want -1 after clamping to the sequence end", i, st.DurationSecs)
		}
	}
}

func TestPlanTreatsNegativeStartAsZero(t *testing.T) {
	s, err := Build([]Segment{seg("a.pcap", 0, fiveMin)}, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	steps, err := s.Plan(-30, -1)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	nearly(t, "StartSecs", steps[0].StartSecs, 0)
}

func TestPlanRejectsEmptyWindow(t *testing.T) {
	s, err := Build([]Segment{seg("a.pcap", 0, fiveMin)}, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, tc := range []struct {
		name              string
		start, durationSe float64
	}{
		{"zero duration", 0, 0},
		{"window starts past the end", 3600, -1},
		{"infinite start", math.Inf(1), -1},
		{"not-a-number start", math.NaN(), -1},
		{"not-a-number duration", 0, math.NaN()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Plan(tc.start, tc.durationSe); !errors.Is(err, ErrEmptyWindow) {
				t.Fatalf("Plan error = %v, want ErrEmptyWindow", err)
			}
		})
	}
}

func TestPlanClampsInfiniteDurationToSequenceEnd(t *testing.T) {
	s, err := Build([]Segment{seg("a.pcap", 0, fiveMin)}, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	steps, err := s.Plan(30, math.Inf(1))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(steps))
	}
	if steps[0].DurationSecs != -1 {
		t.Errorf("DurationSecs = %v, want -1 (clamped to file end)", steps[0].DurationSecs)
	}
}

func TestPlanRefusesBrokenSequence(t *testing.T) {
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin+30*time.Second, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_, err = s.Plan(0, -1)
	if !errors.Is(err, ErrNotContinuous) {
		t.Fatalf("Plan error = %v, want ErrNotContinuous", err)
	}
	// The message must name the offending join so the operator can act on it.
	for _, want := range []string{"a.pcap", "b.pcap", string(SeamBroken)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestPlanRefusesOverlappingSequence(t *testing.T) {
	segs := []Segment{
		seg("a.pcap", 0, fiveMin),
		seg("b.pcap", fiveMin-time.Second, fiveMin),
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := s.Plan(0, -1); !errors.Is(err, ErrNotContinuous) {
		t.Fatalf("Plan error = %v, want ErrNotContinuous", err)
	}
}

func TestPlanOnEmptySequence(t *testing.T) {
	// A hand-built zero-value Sequence must not panic.
	s := &Sequence{}
	if _, err := s.Plan(0, -1); !errors.Is(err, ErrNoSegments) {
		t.Fatalf("Plan error = %v, want ErrNoSegments", err)
	}
}

// TestPlanRealisticSiteVisit exercises the shape the field workflow actually
// produces: seven rolling five-minute files, a clean roll-over between each,
// and a replay case covering a parked stretch that begins inside file 2 and
// ends inside file 5.
func TestPlanRealisticSiteVisit(t *testing.T) {
	var segs []Segment
	for i := range 7 {
		// 3 ms of roll-over cost per file, which stays inside the seamless bound.
		offset := time.Duration(i) * (fiveMin + 3*time.Millisecond)
		segs = append(segs, seg(fileName(i), offset, fiveMin))
	}
	s, err := Build(segs, DefaultTolerances())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if s.Worst != SeamSeamless {
		t.Fatalf("Worst = %q, want %q for 3ms roll-overs", s.Worst, SeamSeamless)
	}
	if want := 18 * time.Millisecond; s.Lost != want {
		t.Errorf("Lost = %v, want %v across six joins", s.Lost, want)
	}

	// Parked stretch: from 120s into file 2 to 90s into file 5.
	start := 2*(fiveMin+3*time.Millisecond) + 120*time.Second
	end := 5*(fiveMin+3*time.Millisecond) + 90*time.Second
	steps, err := s.Plan(start.Seconds(), (end - start).Seconds())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("got %d steps (%v), want 4 (files 2-5)", len(steps), stepPaths(steps))
	}
	if steps[0].Path != fileName(2) || steps[3].Path != fileName(5) {
		t.Errorf("steps span %v, want files 2 through 5", stepPaths(steps))
	}
	nearly(t, "first step StartSecs", steps[0].StartSecs, 120)
	nearly(t, "last step DurationSecs", steps[3].DurationSecs, 90)
	for i, st := range steps {
		if st.DropFrameAtStart {
			t.Errorf("steps[%d] (%s) drops a frame across a seamless join", i, st.Path)
		}
	}
}

func fileName(i int) string {
	return "soma1-0901-17" + string(rune('0'+i)) + "0.pcap"
}
