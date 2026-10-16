package pipeline

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// DwellAbsorbAfter is the dwell policy's default T: how long a confirmed
// track must have stood still (l5tracks.TrackedObject.StationaryFor) before
// its object is absorbed into the background. Twenty seconds is the
// beam-wise review's stationary-track threshold (M3, Section 5.1), the
// measure on which the recall retunes made more long stationary tracks. It is
// fixed rather than a tuning key so the option leaves the config fingerprint
// where it was; a replay that tries another T names it as another experiment,
// which sets DwellPolicy.AbsorbAfter.
//
// Its cost is a road user that stands longer than this: a pedestrian waiting
// to cross, or a queue at a long signal, is absorbed and loses its track,
// and is tracked again as a new object when it moves off.
const DwellAbsorbAfter = 20 * time.Second

// dwellBoxMarginMetres grows the matched cluster's box when selecting the
// returns to absorb. The box encloses the cluster's points, but the cluster
// is built from the height-banded, voxelled and possibly subsampled points,
// so a return of the same surfaces can sit just outside it. A fifth of a
// metre is a few beam footprints at road ranges and a quarter of the DBSCAN
// eps that separates one object from the next.
const dwellBoxMarginMetres = 0.2

// DwellPolicy runs the dwell policy (replay experiment dwell_policy): after
// each frame's tracking, a confirmed track observed in the frame that has been
// stationary for the policy's T (AbsorbAfter) has the frame's foreground
// returns inside its cluster's box absorbed into the L3 background
// (l3grid.BackgroundManager.AbsorbReturns), and is then ended with
// l5tracks.ExpiryAbsorbed. From the next frame the object is background: it
// makes no cluster and the track is gone, so neither a coasting box nor a
// fragment of the object carries the track on.
//
// A nil *DwellPolicy in TrackingPipelineConfig, the default, runs nothing. The
// pipeline callback is the policy's only writer; read Report after the last
// frame.
type DwellPolicy struct {
	// AbsorbAfter is the policy's T. Zero is DwellAbsorbAfter, the 20 s of
	// replay experiment dwell_policy; dwell_policy_t40 and dwell_policy_t60
	// set 40 s and 60 s. The stationary speed and box margin are the same at
	// every T. Set it before the first frame and leave it.
	AbsorbAfter time.Duration

	// Stationary is the drift rule of the stationary clock the policy reads.
	// The clock is L5's, so the pipeline sets the tracker's
	// l5tracks.TrackerConfig.Stationary to it when the frame callback is
	// built, before the first frame: the rule the report names is the rule
	// the tracker ran. The zero value is the shipped rule, 1 m from the
	// run's first estimate; dwell_policy_t40_drift2m sets 2 m and
	// dwell_policy_t40_median measures from the run's running median.
	Stationary l5tracks.StationaryRule

	report DwellReport
}

// Validate refuses a policy that would run as something other than what its
// report names: a drift rule the stationary clock does not know, which the
// clock would run as the shipped rule (l5tracks.StationaryRule.Validate), or
// a negative T, which would run as the default. The pipeline calls it before
// applying the policy (TrackingPipelineConfig.BuildFrameCallback).
func (d *DwellPolicy) Validate() error {
	if d.AbsorbAfter < 0 {
		return fmt.Errorf("dwell policy: absorb-after %v is negative", d.AbsorbAfter)
	}
	if err := d.Stationary.Validate(); err != nil {
		return fmt.Errorf("dwell policy: %w", err)
	}
	return nil
}

// absorbAfter is the T the policy runs with: AbsorbAfter when it is set,
// DwellAbsorbAfter otherwise.
func (d *DwellPolicy) absorbAfter() time.Duration {
	if d.AbsorbAfter > 0 {
		return d.AbsorbAfter
	}
	return DwellAbsorbAfter
}

// DwellReport is what a dwell policy did over a run.
type DwellReport struct {
	// The policy's constants, so a report says what it ran with.
	// AbsorbAfterSeconds is the T it ran at, not the default, and
	// StationaryDriftMetres and StationaryAnchor the clock's drift rule
	// (l5tracks.StationaryRule) as the tracker ran it.
	// StationaryMedianWindowFrames is the running median's window, written
	// only when the anchor is the running median.
	AbsorbAfterSeconds           float64 `json:"absorb_after_seconds"`
	StationarySpeedMps           float64 `json:"stationary_speed_mps"`
	StationaryDriftMetres        float64 `json:"stationary_drift_metres"`
	StationaryAnchor             string  `json:"stationary_anchor"`
	StationaryMedianWindowFrames int     `json:"stationary_median_window_frames,omitempty"`
	BoxMarginMetres              float64 `json:"box_margin_metres"`

	TracksAbsorbed  int `json:"tracks_absorbed"`
	CellsAbsorbed   int `json:"cells_absorbed"`
	ReturnsAbsorbed int `json:"returns_absorbed"`
	// Skipped counts due tracks the policy could not absorb: no matched
	// cluster with a box, or no foreground return inside it. Such a track
	// is left to run on, and is due again on its next observed frame.
	Skipped int `json:"skipped"`
	// Absorptions lists each absorption in the order made. Tracks are named
	// by creation sequence, not by their random public identity, so two
	// replays of a capture write the same list.
	Absorptions []DwellAbsorption `json:"absorptions"`
}

// DwellAbsorption is one absorbed track.
type DwellAbsorption struct {
	CreationSequence  int64   `json:"creation_sequence"`
	FrameUnixNanos    int64   `json:"frame_unix_nanos"`
	StationarySeconds float64 `json:"stationary_seconds"`
	X                 float32 `json:"x"`
	Y                 float32 `json:"y"`
	ObjectClass       string  `json:"object_class,omitempty"`
	Returns           int     `json:"returns"`
	Cells             int     `json:"cells"`
}

// Report returns a copy of what the policy has done so far.
func (d *DwellPolicy) Report() DwellReport {
	r := d.report
	r.AbsorbAfterSeconds = d.absorbAfter().Seconds()
	r.StationarySpeedMps = l5tracks.StationarySpeedMps
	r.StationaryDriftMetres = float64(d.Stationary.Drift())
	r.StationaryAnchor = string(d.Stationary.AnchorMode())
	if d.Stationary.AnchorMode() == l5tracks.StationaryAnchorMedian {
		r.StationaryMedianWindowFrames = l5tracks.StationaryMedianWindowFrames
	}
	r.BoxMarginMetres = dwellBoxMarginMetres
	r.Absorptions = append([]DwellAbsorption{}, d.report.Absorptions...)
	return r
}

// absorbDue runs the policy for one frame. confirmed is the frame's confirmed
// tracks after L5; clusters and foreground are the frame's L4 clusters and L3
// foreground returns. It changes nothing until a track falls due, and on the
// frames where none does it allocates nothing.
func (d *DwellPolicy) absorbDue(bm *l3grid.BackgroundManager, tracker l5tracks.TrackerInterface,
	confirmed []*l5tracks.TrackedObject, clusters []l4perception.WorldCluster,
	foreground []l2frames.PointPolar, sensorID string, at time.Time) {
	atNanos := at.UnixNano()
	after := d.absorbAfter()
	var due []*l5tracks.TrackedObject
	for _, track := range confirmed {
		// Misses is zero only on a frame the track was observed, so its
		// cluster is this frame's and its returns are in foreground.
		if track.Misses == 0 && track.StationaryFor(atNanos) >= after {
			due = append(due, track)
		}
	}
	if len(due) == 0 {
		return
	}
	// GetConfirmedTracks walks a map; creation order makes the absorptions,
	// and so the report, the same on every replay.
	sort.Slice(due, func(i, j int) bool { return due[i].CreationSequence < due[j].CreationSequence })

	clusterOf := make(map[string]int, len(due))
	for i, id := range tracker.GetLastAssociations() {
		if id != "" {
			clusterOf[id] = i
		}
	}
	// The pipeline clusters in the frame TransformToWorld gives with no
	// pose, so the same call puts the returns in the boxes' frame.
	world := l4perception.TransformToWorld(foreground, nil, sensorID)

	for _, track := range due {
		i, ok := clusterOf[track.TrackID]
		if !ok || i >= len(clusters) || clusters[i].OBB == nil {
			d.report.Skipped++
			diagf("[Dwell] track_id=%s due but has no matched cluster box this frame; left to run", track.TrackID)
			continue
		}
		box := clusters[i].OBB
		var returns []l2frames.PointPolar
		for j := range world {
			if insideBox(box, world[j], dwellBoxMarginMetres) {
				returns = append(returns, foreground[j])
			}
		}
		if len(returns) == 0 {
			d.report.Skipped++
			diagf("[Dwell] track_id=%s due but no foreground return lies in its box; left to run", track.TrackID)
			continue
		}
		cells := bm.AbsorbReturns(returns, at)
		stationary := track.StationaryFor(atNanos)
		tracker.EndAbsorbed(track.TrackID, at)
		d.report.TracksAbsorbed++
		d.report.CellsAbsorbed += cells
		d.report.ReturnsAbsorbed += len(returns)
		d.report.Absorptions = append(d.report.Absorptions, DwellAbsorption{
			CreationSequence: track.CreationSequence, FrameUnixNanos: atNanos,
			StationarySeconds: stationary.Seconds(), X: track.X, Y: track.Y,
			ObjectClass: track.ObjectClass, Returns: len(returns), Cells: cells,
		})
		diagf("[Dwell] absorbed track_id=%s seq=%d after %.1fs stationary at (%.2f, %.2f): returns=%d cells=%d",
			track.TrackID, track.CreationSequence, stationary.Seconds(), track.X, track.Y, len(returns), cells)
	}
}

// insideBox reports whether p lies in the oriented box grown by margin on
// every side. The box spans its length along HeadingRad and its width across
// it, centred on (CenterX, CenterY), and Height upwards from CenterZ, which
// EstimateOBBFromCluster sets to the cluster's lowest point.
func insideBox(box *l4perception.OrientedBoundingBox, p l4perception.WorldPoint, margin float64) bool {
	dz := p.Z - float64(box.CenterZ)
	if dz < -margin || dz > float64(box.Height)+margin {
		return false
	}
	dx := p.X - float64(box.CenterX)
	dy := p.Y - float64(box.CenterY)
	sin, cos := math.Sincos(float64(box.HeadingRad))
	along := dx*cos + dy*sin
	across := -dx*sin + dy*cos
	return math.Abs(along) <= float64(box.Length)/2+margin &&
		math.Abs(across) <= float64(box.Width)/2+margin
}
