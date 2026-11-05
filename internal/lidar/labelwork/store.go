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
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
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

// Store keeps workflow items in lidar_workflow_items, and every saved
// revision in lidar_workflow_events.
type Store struct {
	DB  *sql.DB
	Now func() time.Time
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const itemColumns = `item_key, layer, state, assignee, reviewer, blocker, tbd_reason, note, source_revision,
	revision, updated_by, updated_at_ns, created_at_ns`

func scanItems(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ItemKey, &it.Layer, &it.State, &it.Assignee, &it.Reviewer, &it.Blocker, &it.TBDReason,
			&it.Note, &it.SourceRevision, &it.Revision, &it.UpdatedBy, &it.UpdatedAtNs, &it.CreatedAtNs); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Items returns the items for the keys, or every item when keys is empty,
// bounded.
func (s Store) Items(keys ...string) ([]Item, error) {
	if len(keys) == 0 {
		rows, err := s.DB.Query(`SELECT ` + itemColumns + ` FROM lidar_workflow_items ORDER BY item_key, layer LIMIT 20000`)
		if err != nil {
			return nil, err
		}
		return scanItems(rows)
	}
	var out []Item
	for start := 0; start < len(keys); start += 500 {
		chunk := keys[start:min(start+500, len(keys))]
		args := make([]any, len(chunk))
		for i, k := range chunk {
			args[i] = k
		}
		rows, err := s.DB.Query(`SELECT `+itemColumns+` FROM lidar_workflow_items WHERE item_key IN (?`+
			strings.Repeat(",?", len(chunk)-1)+`) ORDER BY item_key, layer`, args...)
		if err != nil {
			return nil, err
		}
		items, err := scanItems(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	if out == nil {
		out = []Item{}
	}
	return out, nil
}

// Put saves an item. baseRevision is the revision the writer loaded, zero
// for a new item; any other stored revision refuses the write with ErrStale,
// so one reviewer never silently overwrites another.
func (s Store) Put(it Item, baseRevision int) (Item, error) {
	if err := it.Validate(); err != nil {
		return Item{}, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	var current int
	var created int64
	err = tx.QueryRow(`SELECT revision, created_at_ns FROM lidar_workflow_items WHERE item_key = ? AND layer = ?`,
		it.ItemKey, it.Layer).Scan(&current, &created)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		current = 0
	case err != nil:
		return Item{}, err
	}
	if current != baseRevision {
		return Item{}, fmt.Errorf("%w: it is at revision %d, the write was based on %d", ErrStale, current, baseRevision)
	}
	now := s.now().UnixNano()
	it.Revision, it.UpdatedAtNs = current+1, now
	if current == 0 {
		it.CreatedAtNs = now
		_, err = tx.Exec(`INSERT INTO lidar_workflow_items (`+itemColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.ItemKey, it.Layer, it.State, it.Assignee, it.Reviewer, it.Blocker, it.TBDReason, it.Note, it.SourceRevision,
			it.Revision, it.UpdatedBy, it.UpdatedAtNs, it.CreatedAtNs)
	} else {
		it.CreatedAtNs = created
		var res sql.Result
		res, err = tx.Exec(`UPDATE lidar_workflow_items SET state = ?, assignee = ?, reviewer = ?, blocker = ?, tbd_reason = ?,
			note = ?, source_revision = ?, revision = ?, updated_by = ?, updated_at_ns = ?
			WHERE item_key = ? AND layer = ? AND revision = ?`,
			it.State, it.Assignee, it.Reviewer, it.Blocker, it.TBDReason, it.Note, it.SourceRevision, it.Revision,
			it.UpdatedBy, it.UpdatedAtNs, it.ItemKey, it.Layer, current)
		if err == nil {
			if n, _ := res.RowsAffected(); n != 1 {
				err = ErrStale
			}
		}
	}
	if err != nil {
		return Item{}, err
	}
	if _, err := tx.Exec(`INSERT INTO lidar_workflow_events (item_key, layer, revision, state, assignee, reviewer, blocker,
		tbd_reason, note, source_revision, updated_by, updated_at_ns) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.ItemKey, it.Layer, it.Revision, it.State, it.Assignee, it.Reviewer, it.Blocker, it.TBDReason, it.Note,
		it.SourceRevision, it.UpdatedBy, it.UpdatedAtNs); err != nil {
		return Item{}, err
	}
	return it, tx.Commit()
}

// History returns an item's saved revisions, newest first, bounded.
func (s Store) History(itemKey string, limit int) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT item_key, layer, state, assignee, reviewer, blocker, tbd_reason, note, source_revision,
		revision, updated_by, updated_at_ns, updated_at_ns FROM lidar_workflow_events WHERE item_key = ?
		ORDER BY updated_at_ns DESC, layer, revision DESC LIMIT ?`, itemKey, limit)
	if err != nil {
		return nil, err
	}
	return scanItems(rows)
}
