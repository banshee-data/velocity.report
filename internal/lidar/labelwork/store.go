// Package labelwork is the labelling workspace's model: the workflow state an
// operator keeps per item and progress layer, the progress measures read from
// the canonical annotation stores, and the outstanding work derived from them.
//
// Workflow state is not annotation truth. A workflow item's "done" never
// makes an annotation reviewed, and nothing here writes a label set or a
// physical-reference document: progress is always read from those stores, so
// it is what they hold and survives a reload. Annotation review statuses are
// proposed, reviewed and rejected; open, in progress, blocked, TBD and done
// are workflow states and stay apart from them.
package labelwork

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Layers are the progress layers a workflow item can be kept for, in the
// order the workspace lists them.
var Layers = []string{
	"purpose", "spans", "object_review", "masks", "mask_review", "physical",
	"occlusion", "following", "freeze", "evaluation", "general",
}

// States are the workflow states.
var States = []string{"open", "in_progress", "blocked", "tbd", "done"}

// TBDReasons are why something is TBD. TBD always carries one; it never
// means "safe for tuning".
var TBDReasons = []string{
	"unassigned_purpose", "missing_reference", "undecided_criterion",
	"unavailable_source", "missing_required_case", "other",
}

// Item is one workflow record: an item and a layer.
type Item struct {
	ItemKey string `json:"item_key"`
	Layer   string `json:"layer"`
	State   string `json:"state"`
	// Assignee and Reviewer are names as the operator gives them. The server
	// has no sign-in, so they are statements, not authenticated identities.
	Assignee  string `json:"assignee"`
	Reviewer  string `json:"reviewer"`
	Blocker   string `json:"blocker"`
	TBDReason string `json:"tbd_reason"`
	Note      string `json:"note"`
	// SourceRevision is the annotation revision the record was written
	// against, such as "label-set:3:sha256:…": a reader can tell a note
	// about an older head from one about the current.
	SourceRevision string `json:"source_revision"`
	Revision       int    `json:"revision"`
	UpdatedBy      string `json:"updated_by"`
	UpdatedAtNs    int64  `json:"updated_at_ns,string"`
	CreatedAtNs    int64  `json:"created_at_ns,string"`
}

// ErrStale is a write whose base revision is not the stored one: another
// reviewer saved first. The caller reloads and reconciles.
var ErrStale = errors.New("the workflow item changed since it was loaded")

// ErrInvalid is a write that breaks a rule.
var ErrInvalid = errors.New("invalid workflow item")

var itemKeyPattern = regexp.MustCompile(`^(label-set|pack|capture|segment|split|plan):[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,255}$`)

const maxNote = 8000

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Validate refuses what the table would, with a reason a person can act on.
func (it Item) Validate() error {
	switch {
	case !itemKeyPattern.MatchString(it.ItemKey):
		return fmt.Errorf("%w: item key %q is not <kind>:<id> for a label-set, pack, capture, segment, split or plan", ErrInvalid, it.ItemKey)
	case !oneOf(it.Layer, Layers):
		return fmt.Errorf("%w: layer %q is not one of %s", ErrInvalid, it.Layer, strings.Join(Layers, ", "))
	case !oneOf(it.State, States):
		return fmt.Errorf("%w: state %q is not one of %s", ErrInvalid, it.State, strings.Join(States, ", "))
	case it.State == "tbd" && !oneOf(it.TBDReason, TBDReasons):
		return fmt.Errorf("%w: TBD needs a reason: one of %s", ErrInvalid, strings.Join(TBDReasons, ", "))
	case it.State != "tbd" && it.TBDReason != "":
		return fmt.Errorf("%w: only a TBD item has a TBD reason", ErrInvalid)
	case it.State == "tbd" && it.TBDReason == "other" && strings.TrimSpace(it.Note) == "":
		return fmt.Errorf("%w: a TBD reason of other needs a note saying what is undecided", ErrInvalid)
	case it.State == "blocked" && strings.TrimSpace(it.Blocker) == "":
		return fmt.Errorf("%w: a blocked item names its blocker", ErrInvalid)
	case strings.TrimSpace(it.UpdatedBy) == "":
		return fmt.Errorf("%w: updated_by is required", ErrInvalid)
	case len(it.Note) > maxNote || len(it.Blocker) > maxNote:
		return fmt.Errorf("%w: note and blocker are limited to %d bytes", ErrInvalid, maxNote)
	case len(it.Assignee) > 200 || len(it.Reviewer) > 200 || len(it.SourceRevision) > 300:
		return fmt.Errorf("%w: a name or revision is too long", ErrInvalid)
	}
	return nil
}
