package l9endpoints

import (
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

func TestDebugProtoRoundTrip(t *testing.T) {
	if DebugToProto(nil) != nil || DebugFromProto(nil) != nil {
		t.Fatal("invented absent diagnostics")
	}
	d := &DebugOverlaySet{FrameID: 7, TimestampNanos: 123,
		AssociationCandidates: []AssociationCandidate{{ClusterID: 4, TrackID: "a", Distance: 2.5, Accepted: true}, {ClusterID: 5, TrackID: "a", Distance: 8}},
		GatingEllipses:        []GatingEllipse{{TrackID: "a", CenterX: 1, CenterY: 2, SemiMajor: 3, SemiMinor: 4, RotationRad: 0.5}},
		Residuals:             []InnovationResidual{{TrackID: "a", PredictedX: 1, PredictedY: 2, MeasuredX: 3, MeasuredY: 4, ResidualMagnitude: 5}},
		Predictions:           []StatePrediction{{TrackID: "a", X: 1, Y: 2, VX: 3, VY: 4}}}
	if got := DebugFromProto(DebugToProto(d)); !reflect.DeepEqual(got, d) {
		t.Fatalf("lost diagnostics: %+v", got)
	}
	got := DebugFromProto(&pb.DebugOverlaySet{AssociationCandidates: []*pb.AssociationCandidate{nil}, GatingEllipses: []*pb.GatingEllipse{nil}, Residuals: []*pb.InnovationResidual{nil}, Predictions: []*pb.StatePrediction{nil}})
	if len(got.Residuals) != 1 || got.Residuals[0] != (InnovationResidual{}) {
		t.Fatal("nil entry not safely decoded")
	}
}
