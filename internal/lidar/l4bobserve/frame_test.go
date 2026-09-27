package l4bobserve

import (
	"math"
	"strings"
	"testing"
	"time"
)

// validRecord returns a fresh, independently owned foreground-complete record
// with two clusters and every rejection path the synthetic frame exercises.
func validRecord(t *testing.T) FrameRecord {
	t.Helper()
	b := newTestBuilder(t)
	frame, returns := syntheticFrame(time.Unix(1_700_000_000, 0), 1)
	d := b.BeginFrame(frame)
	runL4(t, d, frame, returns, l4Params{})
	rec, err := d.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Clusters) < 2 || len(rec.Unassigned) == 0 {
		t.Fatalf("fixture lacks clusters or unassigned points: %d, %d", len(rec.Clusters), len(rec.Unassigned))
	}
	return rec
}

func TestFrameValidationRefusesBrokenContracts(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*FrameRecord)
		want   string
	}{
		"unspecified disposition": {func(f *FrameRecord) { f.Disposition = Disposition{} }, "unspecified"},
		"observed while settling": {func(f *FrameRecord) { f.Background = BackgroundSettling }, "disagrees"},
		"failure without reason": {func(f *FrameRecord) {
			f.Disposition = Disposition{Kind: DispositionFailed, Stage: StageL4Cluster}
			f.Payload = PayloadPoints
			f.Clusters, f.Unassigned, f.UnassignedReasons = nil, nil, nil
		}, "stage and reason"},
		"complete without known loss": {func(f *FrameRecord) { f.Completeness = Completeness{State: CompletenessComplete} }, "known loss"},
		"reversed capture":            {func(f *FrameRecord) { f.CaptureEndUnixNanos = f.CaptureStartUnixNanos - 1 }, "starts after"},
		"membership without points": {func(f *FrameRecord) {
			f.Payload = PayloadMembership
			f.Points = RetainedPoints{}
		}, "without its retained domain"},
		"points without flag":      {func(f *FrameRecord) { f.Payload = 0; f.Clusters, f.Unassigned, f.UnassignedReasons = nil, nil, nil }, "without the points payload"},
		"short coordinate column":  {func(f *FrameRecord) { f.Points.Z = f.Points.Z[:1] }, "coordinate columns"},
		"absent field with values": {func(f *FrameRecord) { f.Points.Fields &^= FieldIntensity }, "intensity column"},
		"non-finite coordinate":    {func(f *FrameRecord) { f.Points.Y[2] = math.NaN() }, "non-finite"},
		"time outside capture":     {func(f *FrameRecord) { f.Points.TimeOffsetNanos[0] = -1 }, "outside the frame's capture"},
		"ordinals out of order":    {func(f *FrameRecord) { f.Points.SourceOrdinal[1] = f.Points.SourceOrdinal[0] }, "strictly ascending"},
		"stage arithmetic": {func(f *FrameRecord) {
			f.Stages[1].Rejected = KnownCount(int(f.Stages[1].Rejected.Value) + 1)
		}, "!= output"},
		"duplicate stage":     {func(f *FrameRecord) { f.Stages = append(f.Stages, f.Stages[0]) }, "twice"},
		"unsorted members":    {func(f *FrameRecord) { m := f.Clusters[0].Members; m[0], m[1] = m[1], m[0] }, "sorted and unique"},
		"member out of range": {func(f *FrameRecord) { f.Clusters[0].Members[len(f.Clusters[0].Members)-1] = 9999 }, "outside"},
		"point in two cells": {func(f *FrameRecord) {
			f.Unassigned[0] = f.Clusters[0].Members[0]
		}, "more than one partition cell"},
		"incomplete partition": {func(f *FrameRecord) {
			f.Unassigned, f.UnassignedReasons = f.Unassigned[1:], f.UnassignedReasons[1:]
		}, "partition covers"},
		"summary disagrees":    {func(f *FrameRecord) { f.Clusters[0].Summary.PointsCount++ }, "summary counts"},
		"missing reason":       {func(f *FrameRecord) { f.UnassignedReasons = f.UnassignedReasons[1:] }, "reasons"},
		"unknown reason value": {func(f *FrameRecord) { f.UnassignedReasons[0] = maxRejectionReason + 1 }, "unknown reason"},
		"duplicate cluster":    {func(f *FrameRecord) { f.Clusters[1].ClusterID = f.Clusters[0].ClusterID }, "appears twice"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := validRecord(t)
			tc.mutate(&rec)
			err := rec.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestProfileValidationRequiresItsGuarantees(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*FrameRecord)
		want   string
	}{
		"no L2 count": {func(f *FrameRecord) { f.Stages = f.Stages[1:] }, "L2 return count"},
		"completed w/o cells": {func(f *FrameRecord) {
			f.Payload = PayloadPoints
			f.Clusters, f.Unassigned, f.UnassignedReasons = nil, nil, nil
		}, "requires membership"},
		"missing ordinals": {func(f *FrameRecord) {
			f.Points.Fields &^= FieldSourceOrdinal
			f.Points.SourceOrdinal = nil
		}, "point-source-ordinal"},
		"sampled domain": {func(f *FrameRecord) {
			l3 := f.Stages[1]
			l3.Output = KnownCount(int(l3.Output.Value) + 1)
			l3.Rejected = KnownCount(int(l3.Rejected.Value) - 1)
			f.Stages[1] = l3
		}, "L3 foreground returns"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := validRecord(t)
			tc.mutate(&rec)
			if err := rec.Validate(); err != nil {
				t.Fatalf("mutation should be structurally valid: %v", err)
			}
			err := rec.ValidateFor(ForegroundComplete())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestCountDistinguishesUnknownFromZero(t *testing.T) {
	if (Count{}).Known || (Count{}).String() != "unknown" {
		t.Fatal("zero Count must be unknown")
	}
	if c := KnownCount(0); !c.Known || c.String() != "0" {
		t.Fatalf("known zero = %+v", c)
	}
	if KnownCount(-1).Known {
		t.Fatal("a negative tally became a known count")
	}
	// An unknown rejection count does not falsify a known input/output pair.
	if err := (StageCount{Stage: StageL2Frame, Output: KnownCount(5)}).validate(); err != nil {
		t.Fatal(err)
	}
	if RejectionClusterShape.String() != "cluster-shape" || RejectionReason(200).String() != "rejection(200)" {
		t.Fatal("rejection reason names changed")
	}
	for got, want := range map[string]string{
		DispositionUnsettled.String(): "unsettled", DispositionKind(9).String(): "disposition(9)",
		CompletenessComplete.String(): "complete", CompletenessState(9).String(): "completeness(9)",
		BackgroundSettling.String(): "settling", BackgroundState(9).String(): "background(9)",
	} {
		if got != want {
			t.Fatalf("name %q, want %q", got, want)
		}
	}
}

func TestGapValidation(t *testing.T) {
	valid := []GapRecord{
		{Cause: "unknown loss"},
		{MissingFrames: KnownCount(3), Time: GapTimeBounded, StartUnixNanos: 10, EndUnixNanos: 20, Cause: "l2 queue overflow"},
		{HasSequenceRange: true, FirstSequence: 4, LastSequence: 6, MissingFrames: KnownCount(3), Cause: "salvage"},
	}
	for _, g := range valid {
		if err := g.Validate(); err != nil {
			t.Fatalf("%+v: %v", g, err)
		}
	}
	invalid := []GapRecord{
		{},
		{HasSequenceRange: true, FirstSequence: 6, LastSequence: 4, Cause: "c"},
		{HasSequenceRange: true, FirstSequence: 4, LastSequence: 6, MissingFrames: KnownCount(2), Cause: "c"},
		{FirstSequence: 4, Cause: "c"},
		{Time: GapTimeBounded, StartUnixNanos: 20, EndUnixNanos: 10, Cause: "c"},
		{StartUnixNanos: 1, Cause: "c"},
		{Time: 9, Cause: "c"},
	}
	for _, g := range invalid {
		if err := g.Validate(); err == nil {
			t.Fatalf("accepted %+v", g)
		}
	}
}

func TestStreamValidatorRequiresDenseCoverageAndOrderedStarts(t *testing.T) {
	profile := ForegroundComplete()
	frameAt := func(sequence uint64, start, end int64) FrameRecord {
		return FrameRecord{Sequence: sequence, FrameUnixNanos: start, CaptureStartUnixNanos: start, CaptureEndUnixNanos: end,
			Disposition: Disposition{Kind: DispositionObserved}, Payload: PayloadPoints | PayloadMembership,
			Points: RetainedPoints{Fields: FieldAcquisitionTime | FieldIntensity | FieldChannel | FieldSourceOrdinal},
			Stages: []StageCount{{Stage: StageL2Frame, Output: KnownCount(0)}}}
	}
	v := NewStreamValidator(profile)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(v.AddFrame(frameAt(0, 0, 100)))
	// A rotation seam may split a packet: the next start may precede the
	// previous end, but never the previous start.
	must(v.AddFrame(frameAt(1, 95, 200)))
	must(v.AddGap(GapRecord{MissingFrames: KnownCount(2), Time: GapTimeBounded, StartUnixNanos: 200, EndUnixNanos: 400, Cause: "l2 queue overflow"}))
	must(v.AddFrame(frameAt(2, 400, 500)))
	must(v.AddGap(GapRecord{HasSequenceRange: true, FirstSequence: 3, LastSequence: 4, Cause: "salvage"}))
	must(v.AddFrame(frameAt(5, 600, 700)))
	if v.Frames() != 4 || v.Gaps() != 2 || v.NextSequence() != 6 {
		t.Fatalf("frames=%d gaps=%d next=%d", v.Frames(), v.Gaps(), v.NextSequence())
	}
	for name, add := range map[string]func() error{
		"hole":            func() error { return v.AddFrame(frameAt(7, 800, 900)) },
		"repeat":          func() error { return v.AddFrame(frameAt(5, 800, 900)) },
		"backwards start": func() error { return v.AddFrame(frameAt(6, 599, 900)) },
		"gap overlap": func() error {
			return v.AddGap(GapRecord{HasSequenceRange: true, FirstSequence: 5, LastSequence: 5, Cause: "c"})
		},
		"gap before frame": func() error {
			return v.AddGap(GapRecord{Time: GapTimeBounded, StartUnixNanos: 10, EndUnixNanos: 20, Cause: "c"})
		},
		"invalid record": func() error { f := frameAt(6, 800, 900); f.Stages = nil; return v.AddFrame(f) },
	} {
		if err := add(); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	// Rejections leave the validator where it was.
	must(v.AddFrame(frameAt(6, 800, 900)))
}
