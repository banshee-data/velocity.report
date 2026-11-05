package labelwork

import (
	"fmt"
	"strings"
)

// Task is one piece of outstanding work: what to do next, why it is
// outstanding, what it waits on, who or what can do it, and where.
type Task struct {
	Layer  string `json:"layer"`
	Action string `json:"action"`
	Reason string `json:"reason"`
	// DependsOn is the dependency or blocker; empty when nothing is.
	DependsOn string `json:"depends_on,omitempty"`
	// Blocked marks a task that cannot start until DependsOn is resolved.
	Blocked bool `json:"blocked"`
	// TBD is the reason a task is TBD, when it is.
	TBD string `json:"tbd,omitempty"`
	// Needs is what does the work: human, mac, cloud, pi.
	Needs []string `json:"needs"`
	// Where is browser, native (the macOS annotation window) or cli.
	Where string `json:"where"`
	// Handoff says how to do a native or CLI task from here.
	Handoff        string `json:"handoff,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`
	Plan           string `json:"plan,omitempty"`
}

// FrozenRef is one frozen split that pins a pack of the row.
type FrozenRef struct {
	Name           string `json:"name"`
	SplitDigest    string `json:"split_digest"`
	Revision       int    `json:"revision"`
	PackDigest     string `json:"pack_digest"`
	PinnedRevision int    `json:"pinned_revision"`
	HeadRevision   int    `json:"head_revision"`
	// Roles are the roles its partitions give this pack's objects.
	Roles []string `json:"roles"`
	// HeadMoved: the label set has been edited since the freeze. The split
	// keeps scoring its pinned revision, which is historical now.
	HeadMoved bool `json:"head_moved"`
	// PhysicalPinned: the split pins the pack's physical references.
	PhysicalPinned bool `json:"physical_pinned"`
}

// PackFacts is one pack of a row.
type PackFacts struct {
	Handle   string            `json:"handle"`
	Digest   string            `json:"pack_digest"`
	Physical *PhysicalProgress `json:"physical,omitempty"`
	Error    string            `json:"error,omitempty"`
}

// RowFacts is what the task derivation reads about one label set (or one
// pack with no label set yet).
type RowFacts struct {
	SourceRevision string
	// Progress is nil when the label set could not be read; ProgressError
	// says why. A row without a label set has neither.
	Progress      *LabelSetProgress
	ProgressError string
	HasLabelSet   bool
	// LabelSetDir is the label set's directory beneath the annotation root.
	LabelSetDir string
	Packs       []PackFacts
	// Cohort is the capture's cohort, or unassigned; CohortReason says why
	// it cannot be used, when it cannot.
	Cohort       string
	CohortReason string
	// Bound is true once the label set has verified a capture file by hash.
	Bound bool
	// SelectionRole is the segment selection's role, when the pack was cut
	// from a selected window.
	SelectionRole string
	Frozen        []FrozenRef
	// EvaluationBlocked is why a development evaluation of the row's capture
	// would be refused now; empty when it would be allowed.
	EvaluationBlocked string
	// Protected marks blind qualification data: aggregate counts only.
	Protected bool
}

// Tasks derives the row's outstanding work from what the stores hold. Plans
// are not evidence: a planned step adds a task, never progress.
func Tasks(f RowFacts) []Task {
	var out []Task
	add := func(t Task) {
		if t.SourceRevision == "" {
			t.SourceRevision = f.SourceRevision
		}
		if t.Needs == nil {
			t.Needs = []string{"human"}
		}
		out = append(out, t)
	}
	switch f.Cohort {
	case "", "unassigned":
		add(Task{Layer: "purpose", Action: "Assign the capture's site and cohort in the cohort manifest",
			Reason:    "the capture has no cohort, so development use is refused and its purpose is TBD",
			DependsOn: "the cohort assignments (site diversity review, custodians) are undecided", Blocked: true,
			TBD: "unassigned_purpose", Where: WhereCLI,
			Handoff: "Add the capture's SHA-256 under its site in the cohort manifest the server reads (--lidar-cohort-manifest)"})
	case "qualification":
		add(Task{Layer: "purpose", Action: "Qualification work needs storage-level access controls",
			Reason:    "blind qualification data is shown here as counts only",
			DependsOn: "storage and backend access controls for qualification data do not exist", Blocked: true,
			TBD: "missing_reference", Where: WhereUnsupported})
	}
	if f.Protected {
		return out
	}
	if f.HasLabelSet && !f.Bound {
		add(Task{Layer: "purpose", Action: "Bind the label set to its capture file",
			Reason:  "without a verified hash the capture is known by name only, and a renamed copy would not be recognised",
			Needs:   []string{"human", "mac"},
			Where:   WhereCLI,
			Handoff: "velocity lidar labels bind (see velocity lidar labels bind -h): it records the file's SHA-256 in the set's applies_to"})
	}
	if f.ProgressError != "" {
		add(Task{Layer: "general", Action: "Repair the label set so it can be read",
			Reason: "the label set could not be read: " + f.ProgressError, TBD: "unavailable_source", Blocked: true,
			DependsOn: "a readable label set", Needs: []string{"human", "mac"}, Where: WhereCLI,
			Handoff: "velocity lidar labels verify --labels " + orPlaceholder(f.LabelSetDir, "<label set dir>")})
		return out
	}
	p := f.Progress
	if !f.HasLabelSet || p == nil || p.Objects == 0 {
		add(Task{Layer: "spans", Action: "Record every road user's span, class and identity",
			Reason: "no road user is recorded yet", Needs: []string{"human", "mac"}, Where: WhereNative,
			Handoff: "Open the pack in VelocityVisualiser's annotation pane; spans first, masks after (label0003)",
			Plan:    "label0003"})
		return out
	}
	add(Task{Layer: "spans", Action: "Confirm every road user in the window has a record",
		Reason: fmt.Sprintf("%d road users are recorded; how many the window holds is not recorded", p.Objects),
		Needs:  []string{"human", "mac"}, Where: WhereNative,
		Handoff: "Step through the window in VelocityVisualiser and mark the spans layer done here when every road user has a record",
		Plan:    "label0003"})
	if p.ProposedObjects > 0 {
		add(Task{Layer: "object_review", Action: fmt.Sprintf("Review %d proposed objects", p.ProposedObjects),
			Reason: "an object counts as truth only once a person reviews it", Where: WhereBrowser})
	}
	if missing := missingSpanFrames(p); missing > 0 {
		add(Task{Layer: "masks", Action: fmt.Sprintf("Mask the %d frames inside spans that have no mask", missing),
			Reason: "a span frame without a mask is either an occlusion to state or a gap", Needs: []string{"human", "mac"},
			Where: WhereNative, Handoff: "VelocityVisualiser annotation pane: propagate or draw the masks", Plan: "label0001"})
	}
	if p.ProposedMasks > 0 {
		add(Task{Layer: "mask_review", Action: fmt.Sprintf("Review %d proposed masks", p.ProposedMasks),
			Reason: "a proposed mask is an ignore region when scored, never truth", Needs: []string{"human", "mac"},
			Where: WhereNative, Handoff: "VelocityVisualiser annotation pane: second-pass review", Plan: "label0001"})
	}
	for _, pk := range f.Packs {
		if pk.Physical == nil {
			continue
		}
		ph := pk.Physical
		switch {
		case !ph.Exists:
			add(Task{Layer: "physical", Action: "Fit and review physical body references for " + pk.Handle,
				Reason: "no physical reference document exists for this pack", DependsOn: "reviewed masks for the objects fitted",
				Blocked: p.TruthMasks == 0, Needs: []string{"human", "mac"}, Where: WhereNative,
				Handoff: "VelocityVisualiser physical pane, or POST /api/lidar/annotations/physical/fit then review", Plan: "label0002"})
		case len(ph.ProposedRecords) > 0:
			add(Task{Layer: "physical", Action: fmt.Sprintf("Review %d proposed physical records in %s", len(ph.ProposedRecords), pk.Handle),
				Reason: "proposed bodies, keyframes and following records are not references until reviewed", Where: WhereBrowser,
				SourceRevision: fmt.Sprintf("physical:%d:%s", ph.Revision, ph.Digest), Plan: "label0002"})
		}
		if ph.Stale > 0 {
			add(Task{Layer: "physical", Action: fmt.Sprintf("Repair %d physical records invalidated by a membership edit in %s", ph.Stale, pk.Handle),
				Reason: "a later label-set save changed the membership they cite; the next save refuses them", Needs: []string{"human", "mac"},
				Where: WhereNative, SourceRevision: fmt.Sprintf("physical:%d:%s", ph.Revision, ph.Digest)})
		}
		if ph.Exists && ph.Following == 0 {
			add(Task{Layer: "following", Action: "Draft following records for " + pk.Handle,
				Reason: "no follower interval has a leader decision", TBD: "missing_reference", Needs: []string{"human", "mac"},
				Where: WhereCLI, Handoff: "velocity lidar annotation-reference draft-following --pack <pack> --follower <id> --leader <id>, then import it reviewed", Plan: "label0002"})
		}
	}
	if len(p.FreezeBlockers) > 0 {
		add(Task{Layer: "freeze", Action: "Finish review before freezing",
			Reason:    fmt.Sprintf("%d of %d objects would refuse a freeze: %s", len(p.FreezeBlockers), p.Objects, listSome(p.FreezeBlockers, 5)),
			DependsOn: "object and mask review", Blocked: true, Where: WhereBrowser})
	} else if len(f.Frozen) == 0 {
		role := "tuning or held_out"
		if f.SelectionRole == "tuning" {
			role = "tuning (a tuning-selected window can never be held out)"
		}
		add(Task{Layer: "freeze", Action: "Preview the freeze and freeze a split",
			Reason: "review is complete for every object and no split pins this revision; role: " + role, Where: WhereBrowser})
	}
	for _, fr := range f.Frozen {
		if fr.HeadMoved {
			add(Task{Layer: "freeze", Action: fmt.Sprintf("Split %s pins revision %d; the head is %d", fr.Name, fr.PinnedRevision, fr.HeadRevision),
				Reason: "the split keeps scoring its pinned revision; scoring the edits takes a superseding split revision",
				Where:  WhereBrowser, SourceRevision: fmt.Sprintf("split:%s", fr.SplitDigest)})
		}
	}
	if len(f.Frozen) > 0 && f.EvaluationBlocked != "" {
		add(Task{Layer: "evaluation", Action: "Development evaluation is refused",
			Reason: f.EvaluationBlocked, DependsOn: "eligibility", Blocked: true, Where: WhereUnsupported})
	}
	return out
}

func missingSpanFrames(p *LabelSetProgress) int {
	n := 0
	for _, sp := range p.Spans {
		if sp.HasSpan && sp.SpanWindows > sp.Masks {
			n += sp.SpanWindows - sp.Masks
		}
	}
	return n
}

func listSome(ids []string, n int) string {
	if len(ids) <= n {
		return strings.Join(ids, ", ")
	}
	return strings.Join(ids[:n], ", ") + fmt.Sprintf(" and %d more", len(ids)-n)
}

func orPlaceholder(v, placeholder string) string {
	if v == "" {
		return placeholder
	}
	return v
}
