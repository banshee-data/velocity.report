package pipeline

import (
	"math"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// FragmentJoinRecord describes one cluster the motion join built from two or
// more DBSCAN components, with what a reader needs to judge whether it joined
// one body or two (l4clus0011's merge reference). It is offline evidence. It
// observes the join and changes no cluster, and it claims nothing about what
// was on the road.
type FragmentJoinRecord struct {
	FrameID        string `json:"frame_id"`
	FrameUnixNanos int64  `json:"frame_unix_nanos"`
	// ClusterID is the joined cluster's ID in the frame's output, the ID its
	// observation carries in the evidence.
	ClusterID int64 `json:"cluster_id"`
	Points    int   `json:"points"`
	// LengthMetres and WidthMetres are the joined cluster's oriented extent.
	LengthMetres float64                 `json:"length_m"`
	WidthMetres  float64                 `json:"width_m"`
	Components   []FragmentJoinComponent `json:"components"`
	// OwnerTrackID is the track whose motion footprint held every component:
	// the body the join attributed the cluster to. OwnerClass and the box are
	// its class and published box when the footprint was built, before any
	// track-conditioned widening.
	OwnerTrackID         string  `json:"owner_track_id"`
	OwnerClass           string  `json:"owner_class"`
	OwnerBoxLengthMetres float64 `json:"owner_box_length_m"`
	OwnerBoxWidthMetres  float64 `json:"owner_box_width_m"`
	// OutsideOwnerBoxShare is the share of the joined returns outside the
	// owner's predicted box (not widened, with the join's margin): what
	// track-conditioned membership admitted beyond the box.
	OutsideOwnerBoxShare float64 `json:"outside_owner_box_share"`
	// Tracks counts the distinct confirmed tracks the components are
	// attributed to. Two or more is a join across bodies the tracker holds
	// apart.
	Tracks int `json:"tracks"`
}

// FragmentJoinComponent is one DBSCAN component of a joined cluster.
type FragmentJoinComponent struct {
	ClusterID int64 `json:"cluster_id"`
	Points    int   `json:"points"`
	// TrackID is the confirmed track whose predicted box holds the largest
	// share of the component's returns, when that share is at least
	// joinRecordAttributionShare; empty when none does. TrackShare is that
	// largest share, attributed or not.
	TrackID    string  `json:"track_id,omitempty"`
	TrackClass string  `json:"track_class,omitempty"`
	TrackShare float64 `json:"track_share"`
}

const (
	// joinRecordAttributionShare is the share of a component's returns a
	// track's predicted box must hold for the component to be the track's.
	joinRecordAttributionShare = 0.5
	// joinRecordMaxStateAgeSecs bounds how far a track's box is projected.
	// A track whose state is older has no predicted box: a long projection
	// would attribute returns to where the body is not.
	joinRecordMaxStateAgeSecs = 1.0
)

// predictedTrackBox is a confirmed track's published box projected to at by
// its velocity, the projection motionFragmentHints gives a footprint before
// any widening. It is false for a track with no usable box or a stale state.
func predictedTrackBox(tr *l5tracks.TrackedObject, at time.Time) (l4perception.Rect, bool) {
	dt := float64(at.UnixNano()-tr.StateUnixNanos) / 1e9
	if tr.StateUnixNanos <= 0 || dt < 0 || dt > joinRecordMaxStateAgeSecs {
		return l4perception.Rect{}, false
	}
	r := l4perception.Rect{
		X: float64(tr.BoxX) + dt*float64(tr.VX), Y: float64(tr.BoxY) + dt*float64(tr.VY),
		HeadingRad: float64(tr.BoxHeadingRad), Length: float64(tr.BoxLength), Width: float64(tr.BoxWidth),
	}
	for _, v := range []float64{r.X, r.Y, r.HeadingRad, r.Length, r.Width} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return l4perception.Rect{}, false
		}
	}
	return r, r.Length > 0 && r.Width > 0
}

// fragmentJoinRecords describes each cluster in after that the join built
// from two or more of before's components. before and beforeTrace are the
// partition JoinMotionFragments was given, after and afterTrace what it
// returned; hints are the footprints it tested and tracks the confirmed
// tracks they came from. Both traces index the same processed points.
func fragmentJoinRecords(before []l4perception.WorldCluster, beforeTrace l4perception.DBSCANTrace,
	after []l4perception.WorldCluster, afterTrace l4perception.DBSCANTrace,
	hints []l4perception.FragmentHint, tracks []*l5tracks.TrackedObject,
	frameID string, at time.Time, margin float64) []FragmentJoinRecord {
	if len(beforeTrace.Labels) != len(afterTrace.Labels) || len(beforeTrace.Labels) != len(beforeTrace.Processed) {
		return nil
	}
	known := make(map[int]bool, len(before))
	for _, c := range before {
		known[int(c.ClusterID)] = true
	}
	output := make(map[int]l4perception.WorldCluster, len(after))
	for _, c := range after {
		output[int(c.ClusterID)] = c
	}
	// Each output label's components, and each component's returns.
	components := make(map[int]map[int]bool)
	members := make(map[int][]l4perception.WorldPoint)
	for i, old := range beforeTrace.Labels {
		if !known[old] {
			continue
		}
		members[old] = append(members[old], beforeTrace.Processed[i])
		joined := afterTrace.Labels[i]
		if _, ok := output[joined]; !ok {
			continue
		}
		if components[joined] == nil {
			components[joined] = make(map[int]bool)
		}
		components[joined][old] = true
	}
	type box struct {
		track *l5tracks.TrackedObject
		rect  l4perception.Rect
	}
	var boxes []box
	byID := make(map[string]box, len(tracks))
	for _, tr := range tracks {
		if r, ok := predictedTrackBox(tr, at); ok {
			b := box{tr, r}
			boxes = append(boxes, b)
			byID[tr.TrackID] = b
		}
	}
	labels := make([]int, 0, len(components))
	for label, parts := range components {
		if len(parts) >= 2 {
			labels = append(labels, label)
		}
	}
	sort.Ints(labels)
	records := make([]FragmentJoinRecord, 0, len(labels))
	for _, label := range labels {
		cluster := output[label]
		ids := make([]int, 0, len(components[label]))
		for id := range components[label] {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		rec := FragmentJoinRecord{FrameID: frameID, FrameUnixNanos: at.UnixNano(), ClusterID: cluster.ClusterID,
			Points: cluster.PointsCount, LengthMetres: float64(cluster.BoundingBoxLength), WidthMetres: float64(cluster.BoundingBoxWidth)}
		var all []l4perception.WorldPoint
		attributed := make(map[string]bool)
		for _, id := range ids {
			pts := members[id]
			all = append(all, pts...)
			comp := FragmentJoinComponent{ClusterID: int64(id), Points: len(pts)}
			for _, b := range boxes {
				inside := 0
				for _, p := range pts {
					if l4perception.FragmentFootprintContains(b.rect, p, margin) {
						inside++
					}
				}
				if share := float64(inside) / float64(len(pts)); share > comp.TrackShare {
					comp.TrackShare, comp.TrackID, comp.TrackClass = share, b.track.TrackID, b.track.ObjectClass
				}
			}
			if comp.TrackShare < joinRecordAttributionShare {
				comp.TrackID, comp.TrackClass = "", ""
			}
			if comp.TrackID != "" {
				attributed[comp.TrackID] = true
			}
			rec.Components = append(rec.Components, comp)
		}
		rec.Tracks = len(attributed)
		// The owner: the footprint the join tested that holds every return.
		for _, h := range hints {
			holds := true
			for _, p := range all {
				if !l4perception.FragmentFootprintContains(h.Rect, p, margin) {
					holds = false
					break
				}
			}
			if holds {
				rec.OwnerTrackID = h.ID
				break
			}
		}
		if b, ok := byID[rec.OwnerTrackID]; ok && rec.OwnerTrackID != "" {
			rec.OwnerClass = b.track.ObjectClass
			rec.OwnerBoxLengthMetres, rec.OwnerBoxWidthMetres = float64(b.track.BoxLength), float64(b.track.BoxWidth)
			outside := 0
			for _, p := range all {
				if !l4perception.FragmentFootprintContains(b.rect, p, margin) {
					outside++
				}
			}
			if len(all) > 0 {
				rec.OutsideOwnerBoxShare = float64(outside) / float64(len(all))
			}
		}
		records = append(records, rec)
	}
	return records
}
