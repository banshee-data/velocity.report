package l5tracks_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// A scripted-cluster scene driver for the reacquisition and identity
// regression cases in identity_regression_test.go, scored by the per-frame
// evaluator.
//
// It sits in the external test package because l8analytics imports
// l5tracks: an internal test could not call ComputeTrackMetrics without an
// import cycle. For the same reason it cannot reuse the ray-cast generator in
// continuity_scene_test.go, and it does not need to. These cases are about
// who takes a cluster when two tracks could, so each frame's clusters are
// written out exactly, including the ones a ray-cast generator does not
// produce: a merge of two bodies into one cluster, and a scrap split off a
// body. The truth is written beside them, so every identity switch and
// fragmentation the evaluator reports is L5's.

const (
	identityFramePeriod = 100 * time.Millisecond
	// identityGateSlackMetres is the per-frame harness's default gate: one
	// metre plus half the reference body's footprint diagonal (the D2 A/B's
	// gate, and lidar-ground-truth-eval perframe's default). A fixed gate
	// wider than the spacing of two bodies cannot see them swap: both
	// hypotheses stay inside both references' gates, and MOT16's matching
	// keeps the old correspondences.
	identityGateSlackMetres = 1.0
)

var identityBase = time.Unix(1_700_000_000, 0)

// sceneObs is one cluster in one frame. owner is the body it belongs to, and
// the body whose label a correct L6 would write back to the track that took
// it; "" is a cluster no body owns.
type sceneObs struct {
	owner                 string
	x, y                  float32
	length, width, height float32
	points                int
}

// sceneTruth is one body's true centre in one frame, and its footprint
// diagonal for the evaluator's gate.
type sceneTruth struct {
	body string
	x, y float32
	diag float32
}

// scriptedFrame is one frame: the clusters the tracker is given and where
// every body the evaluator scores truly is.
type scriptedFrame struct {
	clusters []sceneObs
	truth    []sceneTruth
}

// scriptedScene is a frame sequence and the label a correct L6 gives each
// body.
type scriptedScene struct {
	frames []scriptedFrame
	labels map[string]string
}

func frameTime(frame int) time.Time {
	return identityBase.Add(time.Duration(frame) * identityFramePeriod)
}

func (o sceneObs) cluster(id int64, frame int) l5tracks.WorldCluster {
	return l5tracks.WorldCluster{
		ClusterID: id, SensorID: "identity-scene", TSUnixNanos: frameTime(frame).UnixNano(),
		CentroidX: o.x, CentroidY: o.y,
		BoundingBoxLength: o.length, BoundingBoxWidth: o.width, BoundingBoxHeight: o.height,
		HeightP95: o.height, PointsCount: o.points,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: o.x, CenterY: o.y, Length: o.length, Width: o.width, Height: o.height,
		},
	}
}

// sceneOutcome is what one run of a scene produced.
type sceneOutcome struct {
	metrics  l8analytics.TrackMetrics
	identity l8analytics.IdentityMetrics
	// takers lists, per body, the tracks its clusters went to in order, by
	// creation sequence; one entry is one identity throughout.
	takers map[string][]int64
	// shared lists tracks that took clusters from more than one body.
	shared []int64
	stats  l5tracks.ContinuityStats
}

// runScripted drives a tracker through a scene and scores it. The
// hypothesis is every confirmed track at every frame, observed or coasting,
// keyed by creation sequence so runs compare without the random track IDs.
// That is the population the pipeline persists as estimates and the
// per-frame harness scores.
func runScripted(t *testing.T, cfg l5tracks.TrackerConfig, sc scriptedScene) sceneOutcome {
	t.Helper()
	tk := l5tracks.NewTracker(cfg)
	hyp := map[int64]*l8analytics.TrackSeries{}
	truth := map[string]*l8analytics.TrackSeries{}
	out := sceneOutcome{takers: map[string][]int64{}}
	bodiesOf := map[int64]map[string]bool{}

	for f, frame := range sc.frames {
		now := frameTime(f)
		owners := map[int64]string{}
		clusters := make([]l5tracks.WorldCluster, 0, len(frame.clusters))
		for i, o := range frame.clusters {
			id := int64(f*100 + i + 1)
			owners[id] = o.owner
			clusters = append(clusters, o.cluster(id, f))
		}
		tk.Update(clusters, now)

		ids := make([]string, 0, len(tk.Tracks))
		for id := range tk.Tracks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			tr := tk.Tracks[id]
			if tr.TrackState == l5tracks.TrackDeleted {
				continue
			}
			if tr.LastSupport == l5tracks.SupportObserved {
				if owner := owners[tr.LastClusterID]; owner != "" {
					out.takers[owner] = appendDistinctSeq(out.takers[owner], tr.CreationSequence)
					if bodiesOf[tr.CreationSequence] == nil {
						bodiesOf[tr.CreationSequence] = map[string]bool{}
					}
					bodiesOf[tr.CreationSequence][owner] = true
					if label := sc.labels[owner]; label != "" && tr.TrackState == l5tracks.TrackConfirmed {
						tk.UpdateClassification(id, label, 0.8, "scene")
					}
				}
			}
			if tr.TrackState != l5tracks.TrackConfirmed {
				continue
			}
			s := hyp[tr.CreationSequence]
			if s == nil {
				s = &l8analytics.TrackSeries{ID: fmt.Sprintf("seq-%04d", tr.CreationSequence)}
				hyp[tr.CreationSequence] = s
			}
			s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: now.UnixNano(), X: tr.X, Y: tr.Y})
		}
		for _, b := range frame.truth {
			s := truth[b.body]
			if s == nil {
				s = &l8analytics.TrackSeries{ID: b.body}
				truth[b.body] = s
			}
			s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: now.UnixNano(), X: b.x, Y: b.y,
				FootprintDiagonalMetres: b.diag})
		}
	}

	for seq, bodies := range bodiesOf {
		if len(bodies) > 1 {
			out.shared = append(out.shared, seq)
		}
	}
	sort.Slice(out.shared, func(i, j int) bool { return out.shared[i] < out.shared[j] })
	ref, hs := seriesOf(truth), seriesOf(hyp)
	gate := l8analytics.FootprintGate(identityGateSlackMetres)
	out.metrics = l8analytics.ComputeTrackMetricsGated(ref, hs, gate)
	out.identity = l8analytics.ComputeIdentityMetrics(ref, hs, gate)
	out.stats = tk.ContinuityStats()
	return out
}

func seriesOf[K comparable](m map[K]*l8analytics.TrackSeries) []l8analytics.TrackSeries {
	out := make([]l8analytics.TrackSeries, 0, len(m))
	for _, s := range m {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func appendDistinctSeq(seqs []int64, seq int64) []int64 {
	if len(seqs) == 0 || seqs[len(seqs)-1] != seq {
		return append(seqs, seq)
	}
	return seqs
}

// summary is one line for a log or a failure message.
func (o sceneOutcome) summary() string {
	return fmt.Sprintf("IDSW=%d FM=%d FN=%d FP=%d IDF1=%.3f takers=%v shared=%v",
		o.metrics.IDSwitches, o.metrics.Fragmentations, o.metrics.FN, o.metrics.FP,
		o.identity.IDF1, o.takers, o.shared)
}

// Bodies, as clusters. Point counts only need to clear the tracker's own
// minimums; the scene fixes the geometry.
func carObs(owner string, x, y float32) sceneObs {
	return sceneObs{owner: owner, x: x, y: y, length: 4.5, width: 1.9, height: 1.5, points: 150}
}

func pedestrianObs(owner string, x, y float32) sceneObs {
	return sceneObs{owner: owner, x: x, y: y, length: 0.6, width: 0.5, height: 1.7, points: 30}
}

func cyclistObs(owner string, x, y float32) sceneObs {
	return sceneObs{owner: owner, x: x, y: y, length: 1.8, width: 0.6, height: 1.7, points: 50}
}

// dbscanEps is l4's shipped foreground_dbscan_eps. Two bodies whose boxes
// come closer than this are one cluster.
const dbscanEps = 0.8

// mergedObs is the one cluster DBSCAN makes of two bodies closer than eps:
// the union of their boxes (both head along X here, so axis-aligned) with a
// points-weighted centroid, owned by the body whose label a correct L6 would
// give it, the larger.
func mergedObs(owner string, a, b sceneObs) sceneObs {
	minX, maxX := min(a.x-a.length/2, b.x-b.length/2), max(a.x+a.length/2, b.x+b.length/2)
	minY, maxY := min(a.y-a.width/2, b.y-b.width/2), max(a.y+a.width/2, b.y+b.width/2)
	n := float32(a.points + b.points)
	return sceneObs{owner: owner,
		x: (a.x*float32(a.points) + b.x*float32(b.points)) / n, y: (a.y*float32(a.points) + b.y*float32(b.points)) / n,
		length: maxX - minX, width: maxY - minY, height: max(a.height, b.height), points: a.points + b.points}
}

// clearance is the gap between two axis-aligned boxes.
func clearance(a, b sceneObs) float32 {
	dx := max(0, max(a.x-a.length/2, b.x-b.length/2)-min(a.x+a.length/2, b.x+b.length/2))
	dy := max(0, max(a.y-a.width/2, b.y-b.width/2)-min(a.y+a.width/2, b.y+b.width/2))
	return max(dx, dy)
}

// Truth for the same bodies.
func carAt(body string, x, y float32) sceneTruth { return sceneTruth{body, x, y, 4.88} }

func pedestrianAt(body string, x, y float32) sceneTruth { return sceneTruth{body, x, y, 0.78} }

func cyclistAt(body string, x, y float32) sceneTruth { return sceneTruth{body, x, y, 1.9} }
