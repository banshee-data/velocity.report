package replayeval

import (
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// regionEvidence records the L3 region map into the replay's evidence
// database through BackgroundManager.PersistAt, the server's own grid and
// region serialiser, at two moments: on the first frame after the regions
// are identified (the map the per-region overrides read for the rest of the
// replay), and when the replay ends. Each moment becomes a lidar_bg_snapshot
// row and, once regions exist, a lidar_bg_regions row linked to it, stamped
// with the frame's capture time; l3grid.BuildRegionMap reads the pair back.
//
// The identified map is staged in memory and both are written only by
// finish, after the replay has succeeded, so a failed replay leaves no
// region rows in evidence it did not complete.
//
// It changes nothing the pipeline reads. The manager keeps no store, so it
// never looks for a snapshot to restore; PersistAt only reads the grid, and
// the bookkeeping it updates (SnapshotID, ChangesSinceSnapshot,
// LastSnapshotTime) feeds the live publisher and status endpoints, neither of
// which an offline replay runs.
type regionEvidence struct {
	bg         *l3grid.BackgroundManager
	database   *db.DB
	sourcePath string
	identified *stagedRegionMap
	err        error
}

// stagedRegionMap is the smallest l3grid.RegionStore: it keeps the one grid
// snapshot and region snapshot a PersistAt call writes, for finish to insert.
// Lookups find nothing, though Persist never makes any.
type stagedRegionMap struct {
	grid    *l3grid.BgSnapshot
	regions *l3grid.RegionSnapshot
}

var _ l3grid.RegionStore = (*stagedRegionMap)(nil)

func (s *stagedRegionMap) InsertBgSnapshot(snap *l3grid.BgSnapshot) (int64, error) {
	s.grid = snap
	return 0, nil
}

func (s *stagedRegionMap) InsertRegionSnapshot(snap *l3grid.RegionSnapshot) (int64, error) {
	s.regions = snap
	return 0, nil
}

func (s *stagedRegionMap) GetRegionSnapshotByGridHash(string, string) (*l3grid.RegionSnapshot, error) {
	return nil, nil
}

func (s *stagedRegionMap) GetRegionSnapshotBySourcePath(string, string) (*l3grid.RegionSnapshot, error) {
	return nil, nil
}

func (s *stagedRegionMap) GetLatestRegionSnapshot(string) (*l3grid.RegionSnapshot, error) {
	return nil, nil
}

func (s *stagedRegionMap) GetBgSnapshotByID(int64) (*l3grid.BgSnapshot, error) { return nil, nil }

func newRegionEvidence(bg *l3grid.BackgroundManager, database *db.DB) *regionEvidence {
	return &regionEvidence{bg: bg, database: database, sourcePath: bg.GetSourcePath()}
}

// observe runs after each frame has passed through the pipeline, so the grid
// it stages includes the frame whose processing identified the regions.
// A nil receiver, a replay without an evidence database, does nothing.
func (r *regionEvidence) observe(frameTime time.Time) {
	if r == nil || r.identified != nil || r.err != nil || !r.bg.RegionsIdentified() {
		return
	}
	r.identified, r.err = r.stage(l3grid.SnapshotReasonRegionsIdentified, frameTime)
}

// finish writes the identified map, if regions were identified, and the map
// the replay ended with, and reports the first failure.
func (r *regionEvidence) finish(lastFrameTime time.Time) error {
	if r == nil {
		return nil
	}
	if r.err != nil {
		return r.err
	}
	end, err := r.stage(l3grid.SnapshotReasonReplayEnd, lastFrameTime)
	if err != nil {
		return err
	}
	for _, staged := range []*stagedRegionMap{r.identified, end} {
		if err := r.write(staged); err != nil {
			return err
		}
	}
	return nil
}

func (r *regionEvidence) stage(reason string, at time.Time) (*stagedRegionMap, error) {
	staged := &stagedRegionMap{}
	if err := r.bg.PersistAt(staged, reason, at); err != nil {
		return nil, fmt.Errorf("%s grid snapshot: %w", reason, err)
	}
	if staged.grid == nil {
		return nil, fmt.Errorf("%s: no grid snapshot was produced", reason)
	}
	if r.bg.RegionsIdentified() && staged.regions == nil {
		return nil, fmt.Errorf("%s: regions were identified but no region snapshot was produced", reason)
	}
	return staged, nil
}

// write inserts a staged grid snapshot and links its region snapshot to the
// row it became. The region snapshot carries the replay's capture paths, as
// the server's settle-time persist does, so an evidence database shared by
// several cases says which capture each map came from.
func (r *regionEvidence) write(staged *stagedRegionMap) error {
	if staged == nil {
		return nil
	}
	reason := staged.grid.SnapshotReason
	id, err := r.database.InsertBgSnapshot(staged.grid)
	if err != nil {
		return fmt.Errorf("%s grid snapshot: %w", reason, err)
	}
	if staged.regions == nil {
		return nil
	}
	staged.regions.SnapshotID = id
	staged.regions.SourcePath = r.sourcePath
	if _, err := r.database.InsertRegionSnapshot(staged.regions); err != nil {
		return fmt.Errorf("%s region snapshot: %w", reason, err)
	}
	return nil
}
