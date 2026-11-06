package labelworkdb

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/labelwork"
)

// PlanBindings keeps which item each labelling plan is run on, in
// lidar_plan_bindings, with every revision in lidar_plan_binding_events.
type PlanBindings struct {
	DB  *sql.DB
	Now func() time.Time
}

var planIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,63}$`)

// All returns every binding by plan ID.
func (b PlanBindings) All() (map[string]labelwork.PlanBinding, error) {
	rows, err := b.DB.Query(`SELECT plan_id, item_key, note, revision, bound_by, bound_at_ns FROM lidar_plan_bindings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]labelwork.PlanBinding{}
	for rows.Next() {
		var pb labelwork.PlanBinding
		if err := rows.Scan(&pb.PlanID, &pb.ItemKey, &pb.Note, &pb.Revision, &pb.BoundBy, &pb.BoundAtNs); err != nil {
			return nil, err
		}
		out[pb.PlanID] = pb
	}
	return out, rows.Err()
}

// Put binds a plan to an item (an empty key unbinds it) against the revision
// the writer loaded; another stored revision refuses it with ErrStale.
func (b PlanBindings) Put(pb labelwork.PlanBinding, baseRevision int) (labelwork.PlanBinding, error) {
	switch {
	case !planIDPattern.MatchString(pb.PlanID):
		return pb, fmt.Errorf("%w: plan %q is not a plan ID such as label0003", labelwork.ErrInvalid, pb.PlanID)
	case strings.TrimSpace(pb.BoundBy) == "":
		return pb, fmt.Errorf("%w: bound_by is required", labelwork.ErrInvalid)
	case len(pb.ItemKey) > 300 || len(pb.Note) > 2000:
		return pb, fmt.Errorf("%w: item key or note too long", labelwork.ErrInvalid)
	}
	tx, err := b.DB.Begin()
	if err != nil {
		return pb, err
	}
	defer tx.Rollback()
	var current int
	err = tx.QueryRow(`SELECT revision FROM lidar_plan_bindings WHERE plan_id = ?`, pb.PlanID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pb, err
	}
	if current != baseRevision {
		return pb, fmt.Errorf("%w: the binding is at revision %d, the write was based on %d", labelwork.ErrStale, current, baseRevision)
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	pb.Revision, pb.BoundAtNs = current+1, now().UnixNano()
	if _, err := tx.Exec(`INSERT INTO lidar_plan_bindings (plan_id, item_key, note, revision, bound_by, bound_at_ns)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (plan_id) DO UPDATE SET item_key = excluded.item_key, note = excluded.note,
		revision = excluded.revision, bound_by = excluded.bound_by, bound_at_ns = excluded.bound_at_ns`,
		pb.PlanID, pb.ItemKey, pb.Note, pb.Revision, pb.BoundBy, pb.BoundAtNs); err != nil {
		return pb, err
	}
	if _, err := tx.Exec(`INSERT INTO lidar_plan_binding_events (plan_id, revision, item_key, note, bound_by, bound_at_ns)
		VALUES (?, ?, ?, ?, ?, ?)`, pb.PlanID, pb.Revision, pb.ItemKey, pb.Note, pb.BoundBy, pb.BoundAtNs); err != nil {
		return pb, err
	}
	return pb, tx.Commit()
}
