package l5tracks

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// pieceCluster is a small cluster, 0.8 m square, at (x, y): a piece of a
// body, or a small road user.
func pieceCluster(id int64, x, y float64) WorldCluster {
	pts := visibleFaces(x, y, 0, 0.8, 0.8, 8)
	obb := l4perception.EstimateOBBFromCluster(pts)
	cx, cy := medoid(pts)
	return WorldCluster{ClusterID: id, SensorID: "box-test", CentroidX: float32(cx), CentroidY: float32(cy),
		BoundingBoxLength: obb.Length, BoundingBoxWidth: obb.Width, BoundingBoxHeight: 1.0,
		PointsCount: len(pts), OBB: &obb, Members: pts}
}

func birthGateConfig() TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.OverlapEndFrames = 1
	cfg.OverlapEndOnBody = true
	cfg.OverlapEndKeepOlder = true
	cfg.BirthGateOnBody = true
	return cfg
}

// runPiece drives a car on cluster 1 along x, 8 m to the sensor's side so
// its long face is observed, and from frame from on a piece at
// (x+along, 8+lateral) moving with it; along(i) and lateral(i) may
// move the piece. It returns the tracker and the id of the track the piece
// seeded, or "".
func runPiece(t *testing.T, cfg TrackerConfig, from, end int, along, lateral func(i int) float64) (*Tracker, string) {
	t.Helper()
	tracker := NewTracker(cfg)
	start := time.Unix(1_700_000_000, 0)
	piece := ""
	for i := 0; i <= end; i++ {
		x := 10 + 0.5*float64(i)
		clusters := []WorldCluster{carCluster(1, x, 8, 0)}
		if i >= from {
			clusters = append(clusters, pieceCluster(2, x+along(i), 8+lateral(i)))
		}
		tracker.Update(clusters, start.Add(time.Duration(i)*100*time.Millisecond))
		if piece == "" {
			for id, tr := range tracker.Tracks {
				if tr.LastClusterID == 2 && tr.ObservationCount > 0 {
					piece = id
				}
			}
		}
	}
	return tracker, piece
}

func constant(v float64) func(int) float64 { return func(int) float64 { return v } }

func TestBirthGateRefusesAPieceInsideALiveBody(t *testing.T) {
	// Without the gate, a piece inside the car's returns confirms a second
	// track: a duplicate.
	off := birthGateConfig()
	off.BirthGateOnBody = false
	tracker, piece := runPiece(t, off, 6, 20, constant(1.2), constant(0))
	if piece == "" || tracker.Tracks[piece].TrackState != TrackConfirmed {
		t.Fatalf("setup: without the gate the piece did not confirm a track: %+v", tracker.Tracks[piece])
	}
	tracker, piece = runPiece(t, birthGateConfig(), 6, 20, constant(1.2), constant(0))
	if piece == "" {
		t.Fatal("the piece seeded no track")
	}
	tr := tracker.Tracks[piece]
	if tr.TrackState == TrackConfirmed {
		t.Fatal("a track born inside a live body confirmed")
	}
	if tr.TrackState != TrackDeleted || tr.ExpiryReason != ExpiryBirthGated {
		t.Fatalf("the gated track was not ended as birth_gated: state %v reason %v", tr.TrackState, tr.ExpiryReason)
	}
	if ExpiryBirthGated.String() != "birth_gated" {
		t.Errorf("token %q", ExpiryBirthGated.String())
	}
	if got := len(tracker.GetConfirmedTracks()); got != 1 {
		t.Errorf("%d confirmed tracks, want the car's alone", got)
	}
}

func TestBirthGateSparesANeighbour(t *testing.T) {
	// A small road user 3 m beyond the car, beyond its returns and the
	// quarter-metre margin, confirms as it would without the gate.
	tracker, piece := runPiece(t, birthGateConfig(), 6, 20, constant(0), constant(3))
	if piece == "" || tracker.Tracks[piece].TrackState != TrackConfirmed {
		t.Fatalf("a neighbour beside the car did not confirm: %+v", tracker.Tracks[piece])
	}
}

func TestBirthGateLetsATrackThatLeavesTheBodyConfirm(t *testing.T) {
	// The piece starts on the car's returns for two frames, under MaxMisses,
	// then moves off across the road: once outside the body its hits count.
	lateral := func(i int) float64 {
		if i < 8 {
			return 0
		}
		return 3
	}
	cfg := birthGateConfig()
	if cfg.MaxMisses <= 2 {
		t.Fatalf("setup: MaxMisses %d leaves no room for two gated frames", cfg.MaxMisses)
	}
	tracker, piece := runPiece(t, cfg, 6, 24, constant(1.2), lateral)
	if piece == "" {
		t.Fatal("the piece seeded no track")
	}
	confirmed := 0
	for _, tr := range tracker.GetConfirmedTracks() {
		if tr.LastClusterID == 2 {
			confirmed++
		}
	}
	if confirmed != 1 {
		t.Fatalf("%d confirmed tracks on the piece after it left the body, want 1", confirmed)
	}
}
