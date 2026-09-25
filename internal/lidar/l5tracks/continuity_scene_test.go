package l5tracks

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// A deterministic cluster-level scene generator with known ground truth, for
// the occlusion-continuity scenarios in continuity_scenario_test.go.
//
// It works at the tracker's boundary: bodies move on physically defined
// paths, a sensor at the origin sees them, and each visible body becomes one
// WorldCluster. Occlusion is ray geometry, not a schedule: a body is hidden
// where the rays to it pass through a nearer body first. So "the object was
// behind the van" is a fact the generator derives, and the tracker's
// explanation of the same absence is checked against it, not against a
// hand-written expectation (state-estimation plan Section 16.3).
//
// It is deliberately simpler than l4perception's ray-cast pass generator,
// which models rings and faces to study measurement bias. Here the question is
// continuity, so a body is a rectangle sampled on a grid, its cluster is the
// visible part, and the only noise is a small, seeded centroid jitter.

const (
	sceneFramePeriod = 0.1
	// sceneMinClusterPoints is l4's foreground_min_cluster_points: fewer
	// returns than this and DBSCAN reports nothing.
	sceneMinClusterPoints = 5
	sceneCentroidNoise    = 0.03
	sceneGridLength       = 7
	sceneGridWidth        = 3
)

// bodyPose is a body's true state at an instant.
type bodyPose struct {
	x, y, heading, speed float64
	present              bool
}

// sceneBody is one road user or obstacle.
type sceneBody struct {
	name          string
	label         string // what a correct L6 would label it; "" for none
	length, width float64
	height        float64
	// points is the return count when wholly visible at 10 m; it falls with
	// range squared unless fixedPoints holds it at every range.
	points      float64
	fixedPoints bool
	// dropout is the chance a frame's cluster falls below the minimum anyway:
	// the sparse-returns case.
	dropout float64
	path    func(t float64) bodyPose
}

// segment is one piece of a kinematic path: for its duration the body
// accelerates at accel (speed never below zero) and turns at yawRate.
type segment struct {
	duration, accel, yawRate float64
}

// kinematicPath integrates segments from an initial pose at 1 ms, which is
// exact enough that the path, not the integrator, is what a test measures.
// After the last segment the body holds its speed and heading.
func kinematicPath(x0, y0, heading0, speed0 float64, segments []segment) func(float64) bodyPose {
	const step = 0.001
	var poses []bodyPose
	x, y, h, v := x0, y0, heading0, speed0
	poses = append(poses, bodyPose{x, y, h, v, true})
	for _, s := range segments {
		for n := 0; n < int(math.Round(s.duration/step)); n++ {
			v = math.Max(0, v+s.accel*step)
			h += s.yawRate * step
			x += v * math.Cos(h) * step
			y += v * math.Sin(h) * step
			poses = append(poses, bodyPose{x, y, h, v, true})
		}
	}
	return func(t float64) bodyPose {
		i := int(math.Round(t / step))
		if i < len(poses) {
			return poses[i]
		}
		last := poses[len(poses)-1]
		dt := t - float64(len(poses)-1)*step
		last.x += last.speed * math.Cos(last.heading) * dt
		last.y += last.speed * math.Sin(last.heading) * dt
		return last
	}
}

// stationary is a body that never moves.
func stationary(x, y, heading float64) func(float64) bodyPose {
	return func(float64) bodyPose { return bodyPose{x: x, y: y, heading: heading, present: true} }
}

// presentBetween limits a path to [from, to).
func presentBetween(path func(float64) bodyPose, from, to float64) func(float64) bodyPose {
	return func(t float64) bodyPose {
		p := path(t)
		p.present = t >= from && t < to
		return p
	}
}

// Why a body produced no cluster, or a reduced one.
const (
	whySeen        = ""
	whyOccluded    = "occluded"
	whySparse      = "sparse"
	whyOutOfRange  = "out_of_range"
	whyAbsent      = "absent"
	whyPartialView = "partial"
)

// bodyView is what the sensor made of one body at one instant.
type bodyView struct {
	pose            bodyPose
	visibleFraction float64
	cluster         *WorldCluster
	why             string
}

// scene is a set of bodies seen by a sensor at the origin.
type scene struct {
	bodies   []*sceneBody
	maxRange float64 // zero: unlimited
	rng      *rand.Rand
}

// segmentHitsBody reports whether the segment from the sensor to p passes
// through body b's footprint before reaching p (Liang-Barsky in b's frame).
func segmentHitsBody(px, py float64, b bodyPose, length, width float64) bool {
	c, s := math.Cos(-b.heading), math.Sin(-b.heading)
	toLocal := func(x, y float64) (float64, float64) {
		dx, dy := x-b.x, y-b.y
		return dx*c - dy*s, dx*s + dy*c
	}
	ox, oy := toLocal(0, 0)
	ex, ey := toLocal(px, py)
	dx, dy := ex-ox, ey-oy
	u0, u1 := 0.0, 1.0
	for _, edge := range [][3]float64{
		{-dx, ox + length/2, 0}, {dx, length/2 - ox, 0},
		{-dy, oy + width/2, 0}, {dy, width/2 - oy, 0},
	} {
		p, q := edge[0], edge[1]
		if p == 0 {
			if q < 0 {
				return false
			}
			continue
		}
		r := q / p
		if p < 0 {
			u0 = math.Max(u0, r)
		} else {
			u1 = math.Min(u1, r)
		}
		if u0 > u1 {
			return false
		}
	}
	return u0 < 1-1e-9
}

// render produces one frame: every present body's view, and the clusters in
// body order. Cluster IDs are unique across frames so a track's
// LastClusterID identifies what it took.
func (sc *scene) render(frame int, nowNanos int64, t float64) ([]bodyView, []WorldCluster) {
	views := make([]bodyView, len(sc.bodies))
	poses := make([]bodyPose, len(sc.bodies))
	for i, b := range sc.bodies {
		poses[i] = b.path(t)
	}
	var clusters []WorldCluster
	for i, b := range sc.bodies {
		p := poses[i]
		v := bodyView{pose: p}
		if !p.present {
			v.why = whyAbsent
			views[i] = v
			continue
		}
		cosH, sinH := math.Cos(p.heading), math.Sin(p.heading)
		var visible, total, occluded int
		minA, maxA, minB, maxB := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		var sumX, sumY float64
		for a := 0; a < sceneGridLength; a++ {
			for w := 0; w < sceneGridWidth; w++ {
				total++
				la := -b.length/2 + (float64(a)+0.5)*b.length/sceneGridLength
				lw := -b.width/2 + (float64(w)+0.5)*b.width/sceneGridWidth
				x, y := p.x+la*cosH-lw*sinH, p.y+la*sinH+lw*cosH
				if sc.maxRange > 0 && math.Hypot(x, y) > sc.maxRange {
					continue
				}
				hidden := false
				for j, other := range sc.bodies {
					if j != i && poses[j].present && segmentHitsBody(x, y, poses[j], other.length, other.width) {
						hidden = true
						break
					}
				}
				if hidden {
					occluded++
					continue
				}
				visible++
				sumX, sumY = sumX+x, sumY+y
				minA, maxA = math.Min(minA, la), math.Max(maxA, la)
				minB, maxB = math.Min(minB, lw), math.Max(maxB, lw)
			}
		}
		v.visibleFraction = float64(visible) / float64(total)
		rangeM := math.Hypot(p.x, p.y)
		scale := 100 / (rangeM * rangeM)
		if b.fixedPoints {
			scale = 1
		}
		count := int(b.points * scale * v.visibleFraction)
		switch {
		case visible == 0 && occluded > 0:
			v.why = whyOccluded
		case visible == 0:
			v.why = whyOutOfRange
		case count < sceneMinClusterPoints && occluded > 0:
			v.why = whyOccluded
		case count < sceneMinClusterPoints:
			v.why = whySparse
		case b.dropout > 0 && sc.rng.Float64() < b.dropout:
			v.why = whySparse
		}
		if v.why == whySeen {
			if visible < total {
				v.why = whyPartialView
			}
			cellL, cellW := b.length/sceneGridLength, b.width/sceneGridWidth
			visL, visW := maxA-minA+cellL, maxB-minB+cellW
			midA, midB := (maxA+minA)/2, (maxB+minB)/2
			cx := sumX/float64(visible) + sc.rng.NormFloat64()*sceneCentroidNoise
			cy := sumY/float64(visible) + sc.rng.NormFloat64()*sceneCentroidNoise
			cluster := WorldCluster{
				ClusterID: int64(frame*100 + i), SensorID: "scene", TSUnixNanos: nowNanos,
				CentroidX: float32(cx), CentroidY: float32(cy),
				BoundingBoxLength: float32(visL), BoundingBoxWidth: float32(visW), BoundingBoxHeight: float32(b.height),
				HeightP95: float32(b.height), PointsCount: count,
				OBB: &l4perception.OrientedBoundingBox{
					CenterX: float32(p.x + midA*cosH - midB*sinH), CenterY: float32(p.y + midA*sinH + midB*cosH),
					Length: float32(visL), Width: float32(visW), Height: float32(b.height),
					HeadingRad: float32(p.heading),
				},
			}
			clusters = append(clusters, cluster)
			v.cluster = &clusters[len(clusters)-1]
		}
		views[i] = v
	}
	// Re-point views at the final slice, which append may have moved.
	for i := range views {
		if views[i].cluster != nil {
			for k := range clusters {
				if clusters[k].ClusterID == views[i].cluster.ClusterID {
					views[i].cluster = &clusters[k]
				}
			}
		}
	}
	return views, clusters
}

// targetSample is one instant of the target body against the hypothesis
// following it.
type targetSample struct {
	t       float64
	why     string // whySeen, whyPartialView, or why it produced no cluster
	trackID string // the track following the target, "" before one exists
	state   TrackState
	support ObservationSupport
	exist   ExistenceState
	reason  ExpiryReason
	coast   float32
	// err is the distance from the track's position to the true centre, and
	// d2 the squared Mahalanobis distance of the truth under the track's
	// position covariance: within the 99% ellipse when d2 <= chi2_2(0.99).
	err, d2 float64
}

// sceneRun is the outcome of one run.
type sceneRun struct {
	tk      *Tracker
	samples []targetSample
	// identities are the distinct tracks the target's clusters went to, in
	// order. One entry means identity was preserved throughout.
	identities []string
	// distractorTo are the tracks the distractor's clusters went to, and
	// othersTo every track any body but the target's clusters went to.
	distractorTo []string
	othersTo     map[string]bool
	// tracks keeps every track pointer seen, since deleted tracks leave the
	// tracker's map after the grace period.
	tracks map[string]*TrackedObject
}

// chi2TwoDOF99 is the 99% quantile of chi-squared with two degrees of
// freedom: the "3-sigma" ellipse for a planar position.
const chi2TwoDOF99 = 9.21

func mahalanobis2(track *TrackedObject, x, y float64) float64 {
	ex, ey := x-float64(track.X), y-float64(track.Y)
	a, b, c, d := float64(track.P[0]), float64(track.P[1]), float64(track.P[4]), float64(track.P[5])
	det := a*d - b*c
	if det <= 0 {
		return math.Inf(1)
	}
	return (ex*(d*ex-b*ey) + ey*(-c*ex+a*ey)) / det
}

// runScene drives the tracker through a scene. With classify set, a correct
// L6 labels each confirmed track after its body, as the pipeline would write
// it back through UpdateClassification.
func runScene(t *testing.T, cfg TrackerConfig, sc *scene, target, distractor string, duration float64, classify bool) sceneRun {
	t.Helper()
	run := sceneRun{tk: NewTracker(cfg), tracks: map[string]*TrackedObject{}, othersTo: map[string]bool{}}
	targetIdx, distractorIdx := -1, -1
	for i, b := range sc.bodies {
		switch b.name {
		case target:
			targetIdx = i
		case distractor:
			distractorIdx = i
		}
	}
	if targetIdx < 0 {
		t.Fatalf("scene has no body %q", target)
	}
	var following *TrackedObject
	frames := int(math.Round(duration / sceneFramePeriod))
	for k := 0; k <= frames; k++ {
		now := float64(k) * sceneFramePeriod
		nowNanos := tdAt(1 + now).UnixNano()
		views, clusters := sc.render(k, nowNanos, now)
		run.tk.Update(clusters, tdAt(1+now))

		byCluster := map[int64]*TrackedObject{}
		for id, tr := range run.tk.Tracks {
			run.tracks[id] = tr
			if tr.TrackState != TrackDeleted && tr.LastSupport == SupportObserved {
				byCluster[tr.LastClusterID] = tr
			}
		}
		if classify {
			for i, v := range views {
				if v.cluster == nil || sc.bodies[i].label == "" {
					continue
				}
				if tr := byCluster[v.cluster.ClusterID]; tr != nil && tr.TrackState == TrackConfirmed {
					run.tk.UpdateClassification(tr.TrackID, sc.bodies[i].label, 0.9, "scene")
				}
			}
		}
		for i, v := range views {
			if i == targetIdx || v.cluster == nil {
				continue
			}
			if tr := byCluster[v.cluster.ClusterID]; tr != nil {
				run.othersTo[tr.TrackID] = true
				if i == distractorIdx {
					run.distractorTo = appendDistinct(run.distractorTo, tr.TrackID)
				}
			}
		}

		tv := views[targetIdx]
		if tv.cluster != nil {
			if tr := byCluster[tv.cluster.ClusterID]; tr != nil {
				following = tr
				run.identities = appendDistinct(run.identities, tr.TrackID)
			}
		}
		s := targetSample{t: now, why: tv.why}
		if following != nil {
			s.trackID, s.state, s.support, s.exist = following.TrackID, following.TrackState, following.LastSupport, following.Existence
			s.reason, s.coast = following.ExpiryReason, following.CoastAgeSecs
			s.err = math.Hypot(float64(following.X)-tv.pose.x, float64(following.Y)-tv.pose.y)
			s.d2 = mahalanobis2(following, tv.pose.x, tv.pose.y)
		}
		run.samples = append(run.samples, s)
	}
	return run
}

func appendDistinct(ids []string, id string) []string {
	if len(ids) == 0 || ids[len(ids)-1] != id {
		return append(ids, id)
	}
	return ids
}

// logSamples writes the target's timeline, for reading a failure.
func (r sceneRun) logSamples(t *testing.T) {
	t.Helper()
	short := map[string]string{}
	for _, s := range r.samples {
		if s.trackID != "" && short[s.trackID] == "" {
			short[s.trackID] = fmt.Sprintf("T%d", len(short)+1)
		}
		t.Logf("t=%4.1f %-12s %-3s %-9s %-17s %-20s %-17s coast=%.2f err=%.2f d2=%.2f",
			s.t, s.why, short[s.trackID], s.state, s.support, s.exist, s.reason, s.coast, s.err, s.d2)
	}
}
