package pipeline

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// estimateIdentityConfig carries every identity the estimate sink requires.
func estimateIdentityConfig() *TrackingPipelineConfig {
	return &TrackingPipelineConfig{
		ObservationSourceID:      "source/v1/test",
		ObservationCalibrationID: "calibration/v1/test",
		StateEstimatorID:         "cv_kf_v1",
		StateObservationModelID:  string(l5tracks.MeasurementMedoidV0),
		StateParameterHash:       "sha256:params",
	}
}

// trackedVehicle drives a tracker over a car-sized cluster moving at 10 m/s
// and returns a snapshot of its track after the last update, which carries a
// valid residual.
func trackedVehicle(t *testing.T, solidBody bool) (*l5tracks.TrackedObject, int64) {
	t.Helper()
	cfg := l5tracks.DefaultTrackerConfig()
	cfg.SolidBody = l5tracks.SolidBodyOptions{Enabled: solidBody, OriginSource: "test: sensor frame"}
	tracker := l5tracks.NewTracker(cfg)
	base := time.Unix(1_700_000_000, 0)
	var at time.Time
	for i := 0; i < 3; i++ {
		x := float32(-10 + i)
		cluster := l4perception.WorldCluster{
			ClusterID: int64(i + 1), CentroidX: x, CentroidY: 5, PointsCount: 50,
			BoundingBoxLength: 4.5, BoundingBoxWidth: 1.8, BoundingBoxHeight: 1.5,
			OBB: &l4perception.OrientedBoundingBox{CenterX: x, CenterY: 5, Length: 4.5, Width: 1.8, Height: 1.5},
		}
		at = base.Add(time.Duration(i) * 100 * time.Millisecond)
		tracker.Update([]l4perception.WorldCluster{cluster}, at)
	}
	active := tracker.GetActiveTracks()
	if len(active) != 1 || !active[0].LastResidual.Valid {
		t.Fatalf("want one track with a valid residual, got %d", len(active))
	}
	return active[0], at.UnixNano()
}

func TestOnlineStateEstimateFilesTheSolidBodyBesideThePointEstimate(t *testing.T) {
	cfg := estimateIdentityConfig()
	track, frame := trackedVehicle(t, true)
	pair, err := onlineStateEstimate(cfg, track, frame)
	if err != nil {
		t.Fatal(err)
	}
	sb := pair.SolidBody
	if sb == nil {
		t.Fatal("a track with a solid body produced no solid-body record")
	}
	reading, _ := track.SolidBody()
	if !reflect.DeepEqual(sb.Reading, reading) {
		t.Fatalf("the filed reading is not the track's")
	}
	// The same evidence, estimator, parameters and stage as the point
	// estimate; its own observation model and its own key.
	e := pair.Estimate
	if sb.ObservationID != e.ObservationID || sb.SourceID != e.SourceID || sb.CalibrationID != e.CalibrationID ||
		sb.FrameUnixNanos != e.FrameUnixNanos || sb.EstimatorID != e.EstimatorID || sb.ParamHash != e.ParamHash ||
		sb.Stage != e.Stage || sb.CreationSequence != e.CreationSequence || sb.TrackID != e.TrackID {
		t.Fatalf("solid body identity %+v disagrees with its point estimate %+v", sb, e)
	}
	if sb.ObservationModelID != string(l5tracks.MeasurementNearEdgeCandidateV1) {
		t.Errorf("observation model %q", sb.ObservationModelID)
	}
	if sb.EstimateID == e.EstimateID || !strings.HasPrefix(sb.EstimateID, "solid_body/") {
		t.Errorf("solid-body estimate ID %q can collide with the point estimate's %q", sb.EstimateID, e.EstimateID)
	}
}

// The online row states what the track says its position refers to and what
// the instant rested on, whatever the measurement source would suggest.
func TestOnlineStateEstimateStatesTheTracksReferenceAndSupport(t *testing.T) {
	cfg := estimateIdentityConfig()
	track, frame := trackedVehicle(t, false)
	pair, err := onlineStateEstimate(cfg, track, frame)
	if err != nil {
		t.Fatal(err)
	}
	if e := pair.Estimate; e.Reference != l5tracks.ReferenceClusterMedoid || e.Support != l5tracks.SupportObserved ||
		e.MeasurementSource != string(l5tracks.MeasurementMedoidV0) {
		t.Fatalf("a medoid track's row states %s and %q from %s", e.Reference, e.Support, e.MeasurementSource)
	}
	// Another statement from the track is carried as it is.
	stated := *track
	stated.LastMeasurementSource = l5tracks.MeasurementOBBCentreV1
	stated.LastSupport = l5tracks.SupportClusterSplit
	pair, err = onlineStateEstimate(cfg, &stated, frame)
	if err != nil {
		t.Fatal(err)
	}
	if e := pair.Estimate; e.Reference != stated.PositionReference() || e.Reference != l5tracks.ReferenceVisibleOBBCentre ||
		e.Support != l5tracks.SupportClusterSplit {
		t.Fatalf("the row states %s and %q, the track %s and %q", e.Reference, e.Support, stated.PositionReference(), stated.LastSupport)
	}
}

func TestOnlineStateEstimateHasNoSolidBodyWithTheOptionOff(t *testing.T) {
	track, frame := trackedVehicle(t, false)
	pair, err := onlineStateEstimate(estimateIdentityConfig(), track, frame)
	if err != nil {
		t.Fatal(err)
	}
	if pair.SolidBody != nil {
		t.Fatal("a solid-body record appeared with the option off")
	}
}

type pointOnlySink struct{ inserted int }

func (s *pointOnlySink) Insert(sqlite.TrackEstimate, sqlite.TrackResidual) error {
	s.inserted++
	return nil
}

type solidBodyCapableSink struct {
	pointOnlySink
	bodies []sqlite.TrackSolidBody
}

func (s *solidBodyCapableSink) InsertSolidBody(sb sqlite.TrackSolidBody) error {
	s.bodies = append(s.bodies, sb)
	return nil
}

func TestPersistOnlineStateEstimateNeverDropsASolidBodySilently(t *testing.T) {
	track, frame := trackedVehicle(t, true)

	cfg := estimateIdentityConfig()
	pointOnly := &pointOnlySink{}
	cfg.StateEstimateSink = pointOnly
	if err := persistOnlineStateEstimate(cfg, track, frame); err == nil || !strings.Contains(err.Error(), "solid-body") {
		t.Fatalf("a sink that cannot hold a solid body was accepted: %v", err)
	}

	capable := &solidBodyCapableSink{}
	cfg.StateEstimateSink = capable
	if err := persistOnlineStateEstimate(cfg, track, frame); err != nil {
		t.Fatal(err)
	}
	if capable.inserted != 1 || len(capable.bodies) != 1 {
		t.Fatalf("point estimates %d, solid bodies %d; want one of each", capable.inserted, len(capable.bodies))
	}

	// With the option off the point-only sink is exactly as sufficient as
	// it always was.
	plain, frame := trackedVehicle(t, false)
	cfg.StateEstimateSink = pointOnly
	if err := persistOnlineStateEstimate(cfg, plain, frame); err != nil {
		t.Fatalf("option off: %v", err)
	}
}
