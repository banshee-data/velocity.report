package l9endpoints

import "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"

// DebugToProto preserves optional diagnostics for both storage and streaming.
// Distance is the tracker's squared Mahalanobis distance, despite the legacy
// protobuf field's shorter name. No schema change is required.
func DebugToProto(d *DebugOverlaySet) *pb.DebugOverlaySet {
	if d == nil {
		return nil
	}
	p := &pb.DebugOverlaySet{FrameId: d.FrameID, TimestampNs: d.TimestampNanos}
	for _, v := range d.AssociationCandidates {
		p.AssociationCandidates = append(p.AssociationCandidates, &pb.AssociationCandidate{ClusterId: v.ClusterID, TrackId: v.TrackID, Distance: v.Distance, Accepted: v.Accepted})
	}
	for _, v := range d.GatingEllipses {
		p.GatingEllipses = append(p.GatingEllipses, &pb.GatingEllipse{TrackId: v.TrackID, CenterX: v.CenterX, CenterY: v.CenterY, SemiMajor: v.SemiMajor, SemiMinor: v.SemiMinor, RotationRad: v.RotationRad})
	}
	for _, v := range d.Residuals {
		p.Residuals = append(p.Residuals, &pb.InnovationResidual{TrackId: v.TrackID, PredictedX: v.PredictedX, PredictedY: v.PredictedY, MeasuredX: v.MeasuredX, MeasuredY: v.MeasuredY, ResidualMagnitude: v.ResidualMagnitude})
	}
	for _, v := range d.Predictions {
		p.Predictions = append(p.Predictions, &pb.StatePrediction{TrackId: v.TrackID, X: v.X, Y: v.Y, Vx: v.VX, Vy: v.VY})
	}
	return p
}

// DebugFromProto restores diagnostics from a recording. Protobuf getters also
// tolerate absent entries in a caller-built message.
func DebugFromProto(p *pb.DebugOverlaySet) *DebugOverlaySet {
	if p == nil {
		return nil
	}
	d := &DebugOverlaySet{FrameID: p.FrameId, TimestampNanos: p.TimestampNs}
	for _, v := range p.AssociationCandidates {
		d.AssociationCandidates = append(d.AssociationCandidates, AssociationCandidate{ClusterID: v.GetClusterId(), TrackID: v.GetTrackId(), Distance: v.GetDistance(), Accepted: v.GetAccepted()})
	}
	for _, v := range p.GatingEllipses {
		d.GatingEllipses = append(d.GatingEllipses, GatingEllipse{TrackID: v.GetTrackId(), CenterX: v.GetCenterX(), CenterY: v.GetCenterY(), SemiMajor: v.GetSemiMajor(), SemiMinor: v.GetSemiMinor(), RotationRad: v.GetRotationRad()})
	}
	for _, v := range p.Residuals {
		d.Residuals = append(d.Residuals, InnovationResidual{TrackID: v.GetTrackId(), PredictedX: v.GetPredictedX(), PredictedY: v.GetPredictedY(), MeasuredX: v.GetMeasuredX(), MeasuredY: v.GetMeasuredY(), ResidualMagnitude: v.GetResidualMagnitude()})
	}
	for _, v := range p.Predictions {
		d.Predictions = append(d.Predictions, StatePrediction{TrackID: v.GetTrackId(), X: v.GetX(), Y: v.GetY(), VX: v.GetVx(), VY: v.GetVy()})
	}
	return d
}
