package annotation

import (
	"strings"
	"testing"
)

// draftScene is a pack of ten samples of ten returns, each object holding one
// return of its own, with a mix of objects:
//
//	car_a    reviewed car, reviewed complete masks, samples 0 to 9
//	car_late reviewed car, samples 6 to 9 only
//	car_early reviewed car, samples 0 to 3 only
//	truck_p  reviewed truck with one proposed mask
//	van_u    reviewed van with a reviewed mask of unstated completeness
//	bus_n    proposed bus
//	noise_n  reviewed noise
//	gone     rejected car
func draftScene(t *testing.T) (*Pack, *Sidecar) {
	t.Helper()
	p := synthPack(t, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
	s := NewSidecar(p)
	s.Change = Provenance{Author: "test", Operation: "draft"}
	add := func(o Object, from, to int, tweak func(*FrameMask)) {
		point := len(s.Objects) // each object holds a return of its own
		s.Objects = append(s.Objects, o)
		for i := from; i <= to; i++ {
			m := mask(o.ObjectID, i, point)
			if tweak != nil {
				tweak(&m)
			}
			s.Masks = append(s.Masks, m)
		}
	}
	add(reviewedObject("car_a", "car"), 0, 9, nil)
	add(reviewedObject("car_late", "car"), 6, 9, nil)
	add(reviewedObject("car_early", "car"), 0, 3, nil)
	add(reviewedObject("truck_p", "truck"), 0, 9, func(m *FrameMask) {
		if m.SampleID == 4 {
			m.Status = StatusProposed
		}
	})
	add(reviewedObject("van_u", "van"), 0, 9, func(m *FrameMask) {
		if m.SampleID == 2 {
			m.Completeness = MaskUnreviewed
		}
	})
	add(Object{ObjectID: "bus_n", Class: "bus", Confidence: 1, Status: StatusProposed}, 0, 9, func(m *FrameMask) { m.Status = StatusProposed })
	add(reviewedObject("noise_n", "noise"), 0, 9, nil)
	add(Object{ObjectID: "gone", Class: "car", Status: StatusRejected}, 0, 1, func(m *FrameMask) { m.Status = StatusRejected })
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	return p, s
}

func draftOptions() DraftSplitOptions {
	return DraftSplitOptions{SplitName: "tuning", Role: SplitRoleTuning, Policy: DefaultReferencePolicy()}
}

func skippedBecause(r *DraftSplitReport) map[string]string {
	out := map[string]string{}
	for _, s := range r.Skipped {
		out[s.ObjectID] = s.Reason
	}
	return out
}

func TestDraftSplitTakesWhatFreezingWouldAcceptAndSaysWhatItLeftOut(t *testing.T) {
	p, s := draftScene(t)
	o := draftOptions()
	o.FirstSample, o.Note = 4, "after the warm-up"
	m, report, err := DraftSplitManifest(p, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Splits[0].ObjectIDs, ","); got != "car_a,car_late" {
		t.Fatalf("objects %s", got)
	}
	ep := m.Episodes[0]
	if ep.EpisodeID != "tuning-from-4" || ep.Split != "tuning" || len(ep.FrameIntervals) != 1 ||
		ep.FrameIntervals[0] != (FrameInterval{FirstSample: 4, LastSample: 9}) {
		t.Fatalf("episode %+v", ep)
	}
	if m.SidecarRevision != s.Revision || m.PackDigest != p.Manifest.PackDigest || m.Note != "after the warm-up" || m.Splits[0].Role != SplitRoleTuning {
		t.Fatalf("manifest %+v", m)
	}
	// It binds, and survives the load a result goes through.
	if err := m.ValidateAgainst(p, s); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"car_early": DraftSkipNoScoredInSpan, "truck_p": DraftSkipProposedMasks, "van_u": DraftSkipUnstatedMasks,
		"bus_n": DraftSkipNotReviewed, "noise_n": DraftSkipNotRoadUser,
	}
	got := skippedBecause(report)
	for id, reason := range want {
		if got[id] != reason {
			t.Fatalf("%s skipped as %q, want %q (all: %v)", id, got[id], reason, got)
		}
	}
	if _, listed := got["gone"]; listed {
		t.Fatalf("a rejected object is not a candidate, and was listed: %v", got)
	}
	if len(report.Included) != 2 || report.Included[0].ObjectID != "car_a" || report.Included[0].ScoredMasks != 6 ||
		report.Included[1].FirstSample != 6 || report.Included[1].LastSample != 9 || report.ScoredMasks != 10 ||
		report.SidecarRevision != s.Revision || report.FirstSample != 4 || report.LastSample != 9 {
		t.Fatalf("report %+v", report)
	}
}

func TestDraftSplitWholePackAndHeldOutRoleAndEpisodeName(t *testing.T) {
	p, s := draftScene(t)
	o := draftOptions()
	o.Role, o.SplitName, o.EpisodeID = SplitRoleHeldOut, "hold", "ep-all"
	m, report, err := DraftSplitManifest(p, s, o)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(m.Splits[0].ObjectIDs, ",") != "car_a,car_early,car_late" || m.Splits[0].Role != SplitRoleHeldOut ||
		m.Episodes[0].EpisodeID != "ep-all" || m.Episodes[0].FrameIntervals[0].LastSample != 9 || report.LastSample != 9 {
		t.Fatalf("manifest %+v report %+v", m, report)
	}
}

func TestDraftSplitRefusesWhatItCannotDraft(t *testing.T) {
	p, s := draftScene(t)
	cases := map[string]func(*DraftSplitOptions){
		"no name":       func(o *DraftSplitOptions) { o.SplitName = "" },
		"bad role":      func(o *DraftSplitOptions) { o.Role = "screen" },
		"negative":      func(o *DraftSplitOptions) { o.FirstSample = -1 },
		"inverted":      func(o *DraftSplitOptions) { o.FirstSample, o.LastSample = 5, 3 },
		"past the pack": func(o *DraftSplitOptions) { o.LastSample = 99 },
		"bad policy":    func(o *DraftSplitOptions) { o.Policy.Position = "nowhere" },
	}
	for name, tweak := range cases {
		o := draftOptions()
		tweak(&o)
		if _, _, err := DraftSplitManifest(p, s, o); err == nil {
			t.Fatalf("%s: drafted", name)
		}
	}
	// A window nobody qualifies in names every skipped object.
	o := draftOptions()
	o.FirstSample, o.LastSample = 4, 5
	s2 := NewSidecar(p)
	s2.Objects = []Object{reviewedObject("only_noise", "noise")}
	s2.Masks = []FrameMask{mask("only_noise", 4, 0)}
	_, report, err := DraftSplitManifest(p, s2, o)
	if err == nil || !strings.Contains(err.Error(), "no object qualifies") || report == nil || skippedBecause(report)["only_noise"] != DraftSkipNotRoadUser {
		t.Fatalf("got %v / %+v", err, report)
	}
	// A pack with no samples.
	if _, _, err := DraftSplitManifest(&Pack{}, s, draftOptions()); err == nil {
		t.Fatal("an empty pack drafted")
	}
}

func TestFirstSampleAtOrAfterFindsWhereAWarmUpEnds(t *testing.T) {
	p := synthPack(t, 4, 4, 4, 4, 4)
	for i := range p.Samples {
		p.Samples[i].TimestampNs = 1_000_000_000 + int64(i)*100_000_000
	}
	for seconds, want := range map[float64]int{0: 0, 0.1: 1, 0.15: 2, 0.4: 4} {
		if got, err := FirstSampleAtOrAfter(p, seconds); err != nil || got != want {
			t.Fatalf("%g s: sample %d, %v; want %d", seconds, got, err, want)
		}
	}
	if _, err := FirstSampleAtOrAfter(p, 5); err == nil || !strings.Contains(err.Error(), "ends") {
		t.Fatalf("past the end: %v", err)
	}
	if _, err := FirstSampleAtOrAfter(p, -1); err == nil {
		t.Fatal("a negative time")
	}
	if _, err := FirstSampleAtOrAfter(&Pack{}, 0); err == nil {
		t.Fatal("an empty pack")
	}
}
