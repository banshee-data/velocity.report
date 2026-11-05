package labelwork

import (
	"fmt"
	"strings"
)

// A labelling plan (data/experiments/label/*.md) says what is to be done on
// one window. The operator binds the plan to the workspace item it is run on;
// each step's status is then read from that item's stores, never from the
// plan. A plan with no window bound has no progress, only a next action.

// Step statuses.
const (
	StepDone       = "done"
	StepInProgress = "in_progress"
	StepNotStarted = "not_started"
	StepBlocked    = "blocked"
	StepUntracked  = "untracked"
)

// PlanBinding is which item a plan is run on.
type PlanBinding struct {
	PlanID    string `json:"plan_id"`
	ItemKey   string `json:"item_key"`
	Note      string `json:"note"`
	Revision  int    `json:"revision"`
	BoundBy   string `json:"bound_by"`
	BoundAtNs int64  `json:"bound_at_ns,string"`
}

// PlanFacts is what the step rules read about the bound item.
type PlanFacts struct {
	ItemKey  string
	Title    string
	Stage    string
	HasPack  bool
	Packs    []PackFacts
	Progress *LabelSetProgress
	// Segments are the item's recorded selections: role and finder.
	Segments []SegmentFacts
	Frozen   []FrozenRef
	// WorkflowDone are the layers whose workflow item the operator has
	// marked done: a statement, needed where no total is recorded.
	WorkflowDone map[string]bool
	Protected    bool
}

// SegmentFacts is one recorded selection of the bound item.
type SegmentFacts struct {
	SegmentID string
	Role      string
	Finder    string
	Capture   string
}

// StepProgress is one plan step as the stores show it.
type StepProgress struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Status string  `json:"status"`
	Detail string  `json:"detail"`
	Next   string  `json:"next,omitempty"`
	Where  string  `json:"where"`
	Counts []Count `json:"counts"`
	Note   string  `json:"note,omitempty"`
}

// PlanProgress is one plan with its binding and its steps' progress.
type PlanProgress struct {
	Plan
	Binding    *PlanBinding   `json:"binding,omitempty"`
	BoundTitle string         `json:"bound_title,omitempty"`
	BoundStage string         `json:"bound_stage,omitempty"`
	BoundError string         `json:"bound_error,omitempty"`
	Waiting    []string       `json:"waiting_on"`
	Progress   []StepProgress `json:"progress"`
	StepsDone  int            `json:"steps_done"`
	Next       string         `json:"next"`
	Where      string         `json:"where"`
}

// planNeeds are a plan's selection requirements, read from its steps' notes:
// a window drawn by lot is a random-finder, held-out selection.
type planNeeds struct {
	heldOut, random, physical bool
}

func needsOf(p Plan) planNeeds {
	var n planNeeds
	for _, s := range p.Steps {
		note := strings.ToLower(s.Note)
		if strings.Contains(note, "held_out") || strings.Contains(note, "by lot") || strings.Contains(note, "held-out") {
			n.heldOut = true
		}
		if strings.Contains(note, "by lot") || strings.Contains(note, "random") {
			n.random = true
		}
		if s.ID == "fit" {
			n.physical = true
		}
	}
	if strings.Contains(strings.ToLower(p.Title), "held-out") || strings.Contains(strings.ToLower(p.Title), "held out") {
		n.heldOut = true
	}
	return n
}

// Evaluate derives a plan's step progress. facts is nil when no item is
// bound, or the bound item no longer exists (boundErr says which). done says
// which other plans are complete, for depends_on.
func Evaluate(p Plan, binding *PlanBinding, facts *PlanFacts, boundErr string, done map[string]bool) PlanProgress {
	out := PlanProgress{Plan: p, Binding: binding, BoundError: boundErr, Waiting: []string{}, Progress: []StepProgress{}}
	for _, d := range p.DependsOn {
		if !done[d] {
			out.Waiting = append(out.Waiting, d)
		}
	}
	need := needsOf(p)
	if facts != nil {
		out.BoundTitle, out.BoundStage = facts.Title, facts.Stage
	}
	previousDone := true
	for _, step := range p.Steps {
		sp := evaluateStep(step, need, facts)
		if facts != nil && sp.Status == StepNotStarted && !previousDone && step.ID == "freeze" {
			sp.Status, sp.Detail = StepBlocked, "the steps before it are not done"
		}
		if len(out.Waiting) > 0 && sp.Status != StepDone {
			sp.Status = StepBlocked
			sp.Detail = strings.TrimSpace("Waits on " + strings.Join(out.Waiting, ", ") + ". " + sp.Detail)
			sp.Next = ""
		}
		if sp.Status == StepDone {
			out.StepsDone++
		}
		previousDone = previousDone && sp.Status == StepDone
		out.Progress = append(out.Progress, sp)
	}
	if len(out.Waiting) > 0 {
		out.Next = "Finish " + strings.Join(out.Waiting, ", ") + " first: this plan runs on its frozen window."
		out.Where = WhereUnsupported
		return out
	}
	for _, sp := range out.Progress {
		if sp.Status != StepDone {
			out.Next, out.Where = sp.Next, sp.Where
			if out.Next == "" {
				out.Next = sp.Detail
			}
			break
		}
	}
	if out.Next == "" && len(out.Progress) > 0 {
		out.Next = "Every step is done in the stores."
	}
	return out
}

func evaluateStep(step PlanStep, need planNeeds, f *PlanFacts) StepProgress {
	sp := StepProgress{ID: step.ID, Kind: step.Kind, Note: step.Note, Counts: []Count{}, Where: WhereNative}
	if f == nil {
		sp.Status = StepNotStarted
		if step.ID == "select" || strings.HasSuffix(step.Kind, ":capture") {
			sp.Where = WhereBrowser
			sp.Next = selectNext(need)
			sp.Detail = "no window is bound to this plan"
		}
		return sp
	}
	if f.Protected {
		sp.Status, sp.Where = StepBlocked, WhereUnsupported
		sp.Detail = "the bound item is blind qualification data: progress is not shown and work on it is refused"
		return sp
	}
	p := f.Progress
	objects, reviewed, masks, reviewedMasks, proposedMasks := 0, 0, 0, 0, 0
	masked, spanFrames := 0, 0
	if p != nil {
		objects, reviewed, masks, reviewedMasks, proposedMasks = p.Objects, p.ReviewedObjects, p.Masks, p.ReviewedMasks, p.ProposedMasks
		for _, s := range p.Spans {
			if s.HasSpan {
				masked += s.Masks
				spanFrames += s.SpanWindows
			}
		}
	}
	switch step.ID {
	case "select":
		sp.Where = WhereBrowser
		evaluateSelect(&sp, need, f)
	case "spans":
		sp.Counts = []Count{
			{Name: "road users recorded", Done: objects, Unit: "objects"},
			{Name: "objects reviewed", Done: reviewed, Total: known(objects), Unit: "objects"},
		}
		switch {
		case !f.HasPack:
			sp.Status, sp.Detail, sp.Next = StepBlocked, "the window has no pack yet", "Queue the clip's pack on the Segments page"
		case objects == 0:
			sp.Status, sp.Detail = StepNotStarted, "no road user is recorded"
			sp.Next = "Open the pack in VelocityVisualiser and record each road user's first and last frame, class and identity"
		case f.WorkflowDone["spans"]:
			sp.Status, sp.Detail = StepDone, fmt.Sprintf("%d road users recorded; the operator marked every one recorded", objects)
		default:
			sp.Status = StepInProgress
			sp.Detail = fmt.Sprintf("%d road users recorded; how many the window holds is not recorded", objects)
			sp.Next = "Finish the spans in VelocityVisualiser, then mark the spans layer done in this item's Workflow"
		}
	case "propose":
		sp.Counts = []Count{
			{Name: "frames masked within spans", Done: masked, Total: known(spanFrames), Unit: "frames"},
			{Name: "masks recorded", Done: masks, Unit: "masks"},
		}
		switch {
		case !f.HasPack:
			sp.Status, sp.Detail, sp.Next = StepBlocked, "the window has no pack yet", "Queue the clip's pack on the Segments page"
		case masks == 0:
			sp.Status, sp.Detail = StepNotStarted, "no mask is recorded"
			sp.Next = "Propose and correct masks in VelocityVisualiser (Propose objects, then propagate)"
		case masked >= spanFrames && f.WorkflowDone["masks"]:
			sp.Status, sp.Detail = StepDone, "every frame inside every span has a mask, and the operator marked masks done"
		default:
			sp.Status = StepInProgress
			sp.Detail = fmt.Sprintf("%d of %d frames inside spans masked", masked, spanFrames)
			sp.Next = "Mask the remaining frames in VelocityVisualiser, then mark the masks layer done in Workflow"
		}
	case "review":
		sp.Counts = []Count{
			{Name: "masks reviewed", Done: reviewedMasks, Total: known(masks), Unit: "masks"},
			{Name: "objects reviewed", Done: reviewed, Total: known(objects), Unit: "objects"},
		}
		switch {
		case masks == 0 && objects == 0:
			sp.Status, sp.Detail = StepNotStarted, "nothing to review yet"
		case reviewedMasks == masks && reviewed == objects && masks > 0:
			sp.Status, sp.Detail = StepDone, "every object and every mask is reviewed"
		default:
			sp.Status = StepInProgress
			sp.Detail = fmt.Sprintf("%d proposed masks and %d unreviewed objects left", proposedMasks, objects-reviewed)
			sp.Next = "Second-pass review in VelocityVisualiser (masks); objects can be reviewed here"
		}
	case "fit":
		best := PhysicalProgress{}
		for _, pk := range f.Packs {
			if pk.Physical != nil && pk.Physical.ReviewedBodies >= best.ReviewedBodies {
				best = *pk.Physical
			}
		}
		sp.Counts = []Count{
			{Name: "reviewed bodies", Done: best.ReviewedBodies, Total: known(reviewed), Unit: "reviewed objects"},
			{Name: "keyframes reviewed", Done: best.ReviewedKeyframes, Total: known(best.Keyframes), Unit: "keyframes"},
		}
		switch {
		case reviewed == 0:
			sp.Status, sp.Detail = StepBlocked, "no reviewed object to fit"
		case best.ReviewedBodies >= reviewed && best.Stale == 0:
			sp.Status, sp.Detail = StepDone, "every reviewed object has a reviewed body"
		case best.Bodies == 0:
			sp.Status, sp.Detail = StepNotStarted, "no physical reference is recorded"
			sp.Next = "Fit each reviewed vehicle in VelocityVisualiser's physical pane and review each pose"
		default:
			sp.Status = StepInProgress
			sp.Detail = fmt.Sprintf("%d of %d reviewed objects have a reviewed body; %d stale records", best.ReviewedBodies, reviewed, best.Stale)
			sp.Next = "Fit and review the remaining bodies in VelocityVisualiser"
		}
	case "following":
		total, rev := 0, 0
		for _, pk := range f.Packs {
			if pk.Physical != nil && pk.Physical.Following > total {
				total, rev = pk.Physical.Following, pk.Physical.ReviewedFollowing
			}
		}
		sp.Where = WhereCLI
		sp.Counts = []Count{{Name: "following records reviewed", Done: rev, Total: known(total), Unit: "records"}}
		switch {
		case total == 0:
			sp.Status, sp.Detail = StepNotStarted, "no following record is drafted"
			sp.Next = "velocity lidar annotation-reference draft-following, then import the records reviewed"
		case rev == total && f.WorkflowDone["following"]:
			sp.Status, sp.Detail = StepDone, "every record is reviewed and the operator marked following done"
		default:
			sp.Status = StepInProgress
			sp.Detail = fmt.Sprintf("%d of %d records reviewed; how many follower intervals need one is not recorded", rev, total)
			sp.Next = "Draft and review the remaining following records, then mark following done in Workflow"
		}
	case "freeze":
		sp.Where = WhereBrowser
		evaluateFreeze(&sp, need, f)
	default:
		sp.Status, sp.Where = StepUntracked, WhereUnsupported
		sp.Detail = "no store records this step: keep its state in the item's Workflow"
	}
	return sp
}

func selectNext(need planNeeds) string {
	how := "Choose a run on the Segments page"
	if need.random {
		how += ", draw a window with the random selector"
	}
	if need.heldOut {
		how += " in the held-out role"
	}
	return how + ", make its clip and queue its pack, then bind the window to this plan here"
}

func evaluateSelect(sp *StepProgress, need planNeeds, f *PlanFacts) {
	if len(f.Segments) == 0 {
		if f.HasPack {
			sp.Status = StepBlocked
			sp.Detail = "the bound item has no recorded selection, so how its window was chosen cannot be confirmed"
			sp.Next = "Bind the plan to a window chosen on the Segments page instead"
		} else {
			sp.Status, sp.Detail, sp.Next = StepNotStarted, "nothing selected", selectNext(need)
		}
		return
	}
	s := f.Segments[0]
	switch {
	case need.heldOut && s.Role != "held_out":
		sp.Status = StepBlocked
		sp.Detail = fmt.Sprintf("segment %s was selected for %s; this plan needs a held-out window, which a tuning selection can never become", s.SegmentID, s.Role)
		sp.Next = selectNext(need)
	case need.random && s.Finder != "random":
		sp.Status = StepBlocked
		sp.Detail = fmt.Sprintf("segment %s was chosen by the %s finder; this plan needs a draw by lot (random)", s.SegmentID, s.Finder)
		sp.Next = selectNext(need)
	case !f.HasPack:
		sp.Status = StepInProgress
		sp.Detail = fmt.Sprintf("segment %s (%s, %s) is selected; its pack is not cut yet", s.SegmentID, s.Role, s.Finder)
		sp.Next = "Queue the clip's pack on the Segments page"
	default:
		sp.Status = StepDone
		sp.Detail = fmt.Sprintf("segment %s drawn by the %s finder in the %s role, pack cut", s.SegmentID, s.Finder, s.Role)
	}
}

func evaluateFreeze(sp *StepProgress, need planNeeds, f *PlanFacts) {
	var current, historical []FrozenRef
	for _, fr := range f.Frozen {
		if need.heldOut && !containsString(fr.Roles, "held_out") {
			continue
		}
		if need.physical && !fr.PhysicalPinned {
			continue
		}
		if fr.HeadMoved {
			historical = append(historical, fr)
		} else {
			current = append(current, fr)
		}
	}
	what := "a split"
	if need.heldOut {
		what = "a held-out split"
	}
	if need.physical {
		what += " pinning the physical references"
	}
	switch {
	case len(current) > 0:
		sp.Status, sp.Detail = StepDone, fmt.Sprintf("split %s pins the current revision", current[0].Name)
	case len(historical) > 0:
		sp.Status = StepInProgress
		sp.Detail = fmt.Sprintf("split %s pins revision %d; the head has moved to %d", historical[0].Name, historical[0].PinnedRevision, historical[0].HeadRevision)
		sp.Next = "Preview and freeze a superseding revision from this item's Freeze section"
	case f.Progress != nil && len(f.Progress.FreezeBlockers) > 0:
		sp.Status = StepNotStarted
		sp.Detail = fmt.Sprintf("%d objects would refuse a freeze", len(f.Progress.FreezeBlockers))
		sp.Next = "Finish review, then preview and freeze " + what + " from this item's Freeze section"
	default:
		sp.Status = StepNotStarted
		sp.Detail = "no " + strings.TrimPrefix(what, "a ") + " pins this window"
		sp.Next = "Preview and freeze " + what + " from this item's Freeze section"
	}
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
