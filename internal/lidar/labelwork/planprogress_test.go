package labelwork_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
)

func repoPlans(t *testing.T) map[string]labelwork.Plan {
	t.Helper()
	read := labelwork.ReadPlans(filepath.Join("..", "..", ".."))
	out := map[string]labelwork.Plan{}
	for _, p := range read.Plans {
		out[p.ID] = p
	}
	return out
}

func stepOf(t *testing.T, pp labelwork.PlanProgress, id string) labelwork.StepProgress {
	t.Helper()
	for _, s := range pp.Progress {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no step %s in %s", id, pp.ID)
	return labelwork.StepProgress{}
}

// An unbound plan has no progress, only what to do first.
func TestUnboundPlanSaysHowToStart(t *testing.T) {
	p := repoPlans(t)["label0003"]
	pp := labelwork.Evaluate(p, nil, nil, "", map[string]bool{})
	if pp.StepsDone != 0 || !strings.Contains(pp.Next, "random selector") || !strings.Contains(pp.Next, "held-out role") {
		t.Fatalf("%+v", pp)
	}
	for _, s := range pp.Progress {
		if s.Status != labelwork.StepNotStarted {
			t.Fatalf("%+v", s)
		}
	}
}

// A window selected for tuning can never be label0001's held-out lot.
func TestPlanRefusesATuningWindow(t *testing.T) {
	p := repoPlans(t)["label0001"]
	facts := &labelwork.PlanFacts{HasPack: true, Segments: []labelwork.SegmentFacts{{SegmentID: "s1", Role: "tuning", Finder: "following"}},
		WorkflowDone: map[string]bool{}}
	pp := labelwork.Evaluate(p, &labelwork.PlanBinding{PlanID: "label0001", ItemKey: "label-set:x"}, facts, "", map[string]bool{})
	if s := stepOf(t, pp, "select"); s.Status != labelwork.StepBlocked || !strings.Contains(s.Detail, "tuning") {
		t.Fatalf("%+v", s)
	}
}

// Spans are done only when the operator says every road user is recorded;
// the freeze is done only by a held-out split pinning the current head; and
// label0002 waits on label0001.
func TestPlanStepsFollowTheStores(t *testing.T) {
	plans := repoPlans(t)
	prog := labelwork.MembershipProgress(state())
	facts := &labelwork.PlanFacts{HasPack: true, Progress: &prog, WorkflowDone: map[string]bool{},
		Segments: []labelwork.SegmentFacts{{SegmentID: "s1", Role: "held_out", Finder: "random"}}}
	binding := &labelwork.PlanBinding{PlanID: "label0003", ItemKey: "label-set:x"}
	pp := labelwork.Evaluate(plans["label0003"], binding, facts, "", map[string]bool{})
	if stepOf(t, pp, "select").Status != labelwork.StepDone || stepOf(t, pp, "spans").Status != labelwork.StepInProgress {
		t.Fatalf("%+v", pp.Progress)
	}
	if stepOf(t, pp, "freeze").Status == labelwork.StepDone || pp.Next == "" {
		t.Fatalf("%+v", pp)
	}
	facts.WorkflowDone["spans"] = true
	facts.Frozen = []labelwork.FrozenRef{{Name: "old.json", Roles: []string{"held_out"}, HeadMoved: true, PinnedRevision: 2, HeadRevision: 3}}
	pp = labelwork.Evaluate(plans["label0003"], binding, facts, "", map[string]bool{})
	if stepOf(t, pp, "spans").Status != labelwork.StepDone || stepOf(t, pp, "freeze").Status != labelwork.StepInProgress {
		t.Fatalf("a historical pin is not this revision's freeze: %+v", pp.Progress)
	}
	facts.Frozen = append(facts.Frozen, labelwork.FrozenRef{Name: "tune.json", Roles: []string{"tuning"}})
	if s := stepOf(t, labelwork.Evaluate(plans["label0003"], binding, facts, "", nil), "freeze"); s.Status == labelwork.StepDone {
		t.Fatalf("a tuning split is not a held-out freeze: %+v", s)
	}
	facts.Frozen = append(facts.Frozen, labelwork.FrozenRef{Name: "held.json", Roles: []string{"held_out"}})
	pp = labelwork.Evaluate(plans["label0003"], binding, facts, "", nil)
	if pp.StepsDone != len(pp.Progress) {
		t.Fatalf("%+v", pp.Progress)
	}
	l2 := labelwork.Evaluate(plans["label0002"], nil, nil, "", map[string]bool{"label0001": false})
	if len(l2.Waiting) != 1 || l2.Progress[0].Status != labelwork.StepBlocked {
		t.Fatalf("%+v", l2)
	}
	// label0002's freeze needs the physical references pinned.
	l2 = labelwork.Evaluate(plans["label0002"], binding, facts, "", map[string]bool{"label0001": true})
	if stepOf(t, l2, "freeze").Status == labelwork.StepDone {
		t.Fatalf("a split without the physical pin: %+v", stepOf(t, l2, "freeze"))
	}
}
