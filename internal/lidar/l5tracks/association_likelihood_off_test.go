package l5tracks

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// RobustLikelihoodAssociationCost is default-off, and off it must leave the
// shipped association arithmetic exactly as it was. Adding it moved the gate
// onto a function that also returns ln det S, for the medoid gate and A2's
// face-residual gate alike, so this test pins both: it drives scripted scenes
// and the synthetic solid-body passes through the real Update path under a
// spread of the existing association options, and hashes every cost the
// assignment saw, every gating ellipse and every tracked state and covariance
// on every frame. The golden digests were taken on the code before the option
// existed.
//
// They are pinned on amd64 only, for the reason solid_body_face_state_off_test
// gives: other targets may fuse multiply-adds, so the same source gives other
// bits there.

// associationDigestCollector collects what association records, with tracks
// named by creation sequence rather than their random public ID. The tracker
// visits tracks in map order outside the assignment, so each frame's records
// are sorted before they are hashed.
type associationDigestCollector struct {
	tracker *Tracker
	records []string
}

func (c *associationDigestCollector) IsEnabled() bool { return true }

func (c *associationDigestCollector) seq(trackID string) int64 {
	if tr, ok := c.tracker.Tracks[trackID]; ok {
		return tr.CreationSequence
	}
	return -1
}

func (c *associationDigestCollector) record(kind int64, trackID string, ints []int64, floats ...float32) {
	h := sha256.New()
	writeInt(h, append([]int64{kind, c.seq(trackID)}, ints...)...)
	writeFloats(h, floats...)
	c.records = append(c.records, string(h.Sum(nil)))
}

func (c *associationDigestCollector) RecordAssociation(clusterID int64, trackID string, cost float32, accepted bool) {
	a := int64(0)
	if accepted {
		a = 1
	}
	c.record(1, trackID, []int64{clusterID, a}, cost)
}

func (c *associationDigestCollector) RecordGatingRegion(trackID string, x, y, major, minor, rot float32) {
	c.record(2, trackID, nil, x, y, major, minor, rot)
}

func (c *associationDigestCollector) RecordInnovation(trackID string, px, py, mx, my, mag float32) {
	c.record(3, trackID, nil, px, py, mx, my, mag)
}

func (c *associationDigestCollector) RecordPrediction(trackID string, x, y, vx, vy float32) {
	c.record(4, trackID, nil, x, y, vx, vy)
}

// flush hashes the frame's records in sorted order.
func (c *associationDigestCollector) flush(h hash.Hash) {
	sort.Strings(c.records)
	for _, r := range c.records {
		h.Write([]byte(r))
	}
	c.records = c.records[:0]
}

// associationScene is a scripted 80-frame scene built to exercise every
// association branch: two pedestrians converging, the second missing six
// frames while it does, so it coasts with inflated covariance into the first's
// neighbourhood; a third born mid-scene; and a vehicle that misses four frames
// and sheds a fragment beside itself.
func associationScene() []syntheticPassFrame {
	start := time.Unix(1_700_000_000, 0)
	var frames []syntheticPassFrame
	for i := 0; i < 80; i++ {
		ts := float32(i) * 0.1
		var cs []WorldCluster
		id := int64(i * 10)
		add := func(c WorldCluster) {
			id++
			c.ClusterID = id
			cs = append(cs, c)
		}
		add(s3Cluster(ts*1.0, 0))
		by := float32(1.5)
		if i >= 30 && i < 50 {
			by = 1.5 - 0.04*float32(i-30)
		} else if i >= 50 {
			by = 0.7
		}
		if i < 35 || i >= 41 {
			add(s3Cluster(ts*1.0, by))
		}
		if i >= 20 {
			add(s3Cluster(5, -3+0.05*float32(i-20)))
		}
		if i < 50 || i >= 54 {
			vx := 20 - 5*ts
			add(associationVehicle(vx, 5, 4.5, 1.8))
		}
		if i >= 45 && i < 48 {
			add(associationVehicle(20-5*ts+2.6, 5.4, 0.3, 0.2))
		}
		frames = append(frames, syntheticPassFrame{at: start.Add(time.Duration(i) * 100 * time.Millisecond), clusters: cs})
	}
	return frames
}

func associationVehicle(x, y, length, width float32) WorldCluster {
	return WorldCluster{
		CentroidX: x, CentroidY: y,
		BoundingBoxLength: length, BoundingBoxWidth: width, BoundingBoxHeight: 1.5,
		PointsCount: 120,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: x, CenterY: y, Length: length, Width: width, Height: 1.5,
		},
	}
}

// associationFingerprint runs frames through a tracker and hashes the
// association record and the tracked state, frame by frame.
func associationFingerprint(cfg TrackerConfig, frames []syntheticPassFrame) string {
	h := sha256.New()
	tracker := NewTracker(cfg)
	collector := &associationDigestCollector{tracker: tracker}
	tracker.DebugCollector = collector
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		collector.flush(h)
		tracks := tracker.GetAllTracks()
		sort.Slice(tracks, func(i, j int) bool { return tracks[i].CreationSequence < tracks[j].CreationSequence })
		for _, tr := range tracks {
			writeInt(h, tr.CreationSequence, int64(tr.Misses), int64(tr.ObservationCount))
			h.Write([]byte(string(tr.TrackState) + "\n"))
			writeFloats(h, tr.X, tr.Y, tr.VX, tr.VY)
			writeFloats(h, tr.P[:]...)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

type associationDigestScenario struct {
	name   string
	frames func(t *testing.T) []syntheticPassFrame
	config func() TrackerConfig
	golden string
}

func scriptedScene(*testing.T) []syntheticPassFrame { return associationScene() }

func associationDigestScenarios() []associationDigestScenario {
	scene := scriptedScene
	pass := func(p func() l4perception.SyntheticPass) func(t *testing.T) []syntheticPassFrame {
		return func(t *testing.T) []syntheticPassFrame { return syntheticPassFrames(t, p()) }
	}
	return []associationDigestScenario{
		{"scene_default", scene, DefaultTrackerConfig, "b305f4eafd3428db"},
		{"scene_likelihood", scene, func() TrackerConfig {
			cfg := DefaultTrackerConfig()
			cfg.LikelihoodAssociationCost = true
			return cfg
		}, "4dbbdd09328ccadc"},
		{"scene_likelihood_adaptive", scene, func() TrackerConfig {
			cfg := DefaultTrackerConfig()
			cfg.LikelihoodAssociationCost = true
			cfg.AdaptiveMeasurementNoise = true
			return cfg
		}, "8b535f07febe89ab"},
		{"scene_tentative_priority_extent", scene, func() TrackerConfig {
			cfg := DefaultTrackerConfig()
			cfg.TentativePriority = true
			cfg.AssociationExtentCostWeight = 1
			return cfg
		}, "b576b4b9bfbd0171"},
		{"scene_cascade_continuity", scene, func() TrackerConfig {
			cfg := DefaultTrackerConfig()
			cfg.CascadedAssociation = true
			cfg.OcclusionContinuity = DefaultOcclusionContinuity()
			cfg.OcclusionContinuity.ContestedRejoin = true
			return cfg
		}, "a4503c5c5a019a69"},
		{"truck_near_edge_tracked", pass(endOnTruckPass), nearEdgeTrackingConfig, "8962df4f854023c0"},
		{"default_pass_near_edge_tracked_claim", pass(l4perception.DefaultSyntheticPass), func() TrackerConfig {
			cfg := nearEdgeTrackingConfig()
			cfg.SolidBody.Containment = true
			cfg.ContainmentClaim = true
			return cfg
		}, "a5cecee4384cea69"},
	}
}

func TestRobustLikelihoodCostOffLeavesTheShippedArithmetic(t *testing.T) {
	if DefaultTrackerConfig().RobustLikelihoodAssociationCost {
		t.Fatal("RobustLikelihoodAssociationCost defaults on; it must be measured against the shipped cost first")
	}
	for _, s := range associationDigestScenarios() {
		got := associationFingerprint(s.config(), s.frames(t))
		t.Logf("%s: digest %s", s.name, got)
		if runtime.GOARCH != "amd64" {
			continue
		}
		if got != s.golden {
			t.Errorf("%s: digest %s, want %s", s.name, got, s.golden)
		}
	}
	if runtime.GOARCH != "amd64" {
		t.Skipf("digests are pinned on amd64; %s may fuse multiply-adds", runtime.GOARCH)
	}
}
