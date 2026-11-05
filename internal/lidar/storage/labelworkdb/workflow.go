package labelworkdb

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
)

// Store keeps workflow items in lidar_workflow_items, and every saved
// revision in lidar_workflow_events.
type WorkflowStore struct {
	DB  *sql.DB
	Now func() time.Time
}

func (s WorkflowStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const itemColumns = `item_key, layer, state, assignee, reviewer, blocker, tbd_reason, note, source_revision,
	revision, updated_by, updated_at_ns, created_at_ns`

func scanItems(rows *sql.Rows) ([]labelwork.Item, error) {
	defer rows.Close()
	out := []labelwork.Item{}
	for rows.Next() {
		var it labelwork.Item
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
func (s WorkflowStore) Items(keys ...string) ([]labelwork.Item, error) {
	if len(keys) == 0 {
		rows, err := s.DB.Query(`SELECT ` + itemColumns + ` FROM lidar_workflow_items ORDER BY item_key, layer LIMIT 20000`)
		if err != nil {
			return nil, err
		}
		return scanItems(rows)
	}
	var out []labelwork.Item
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
		out = []labelwork.Item{}
	}
	return out, nil
}

// Put saves an item. baseRevision is the revision the writer loaded, zero
// for a new item; any other stored revision refuses the write with labelwork.ErrStale,
// so one reviewer never silently overwrites another.
func (s WorkflowStore) Put(it labelwork.Item, baseRevision int) (labelwork.Item, error) {
	if err := it.Validate(); err != nil {
		return labelwork.Item{}, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return labelwork.Item{}, err
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
		return labelwork.Item{}, err
	}
	if current != baseRevision {
		return labelwork.Item{}, fmt.Errorf("%w: it is at revision %d, the write was based on %d", labelwork.ErrStale, current, baseRevision)
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
				err = labelwork.ErrStale
			}
		}
	}
	if err != nil {
		return labelwork.Item{}, err
	}
	if _, err := tx.Exec(`INSERT INTO lidar_workflow_events (item_key, layer, revision, state, assignee, reviewer, blocker,
		tbd_reason, note, source_revision, updated_by, updated_at_ns) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.ItemKey, it.Layer, it.Revision, it.State, it.Assignee, it.Reviewer, it.Blocker, it.TBDReason, it.Note,
		it.SourceRevision, it.UpdatedBy, it.UpdatedAtNs); err != nil {
		return labelwork.Item{}, err
	}
	return it, tx.Commit()
}

// History returns an item's saved revisions, newest first, bounded.
func (s WorkflowStore) History(itemKey string, limit int) ([]labelwork.Item, error) {
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
