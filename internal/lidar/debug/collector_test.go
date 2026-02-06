package debug

import "testing"

func TestNewDebugCollector_InitiallyDisabled(t *testing.T) {
	collector := NewDebugCollector()

	if collector.IsEnabled() {
		t.Error("Expected collector to be initially disabled")
	}
}

func TestDebugCollector_EnableDisable(t *testing.T) {
	collector := NewDebugCollector()

	// Enable
	collector.SetEnabled(true)
	if !collector.IsEnabled() {
		t.Error("Expected collector to be enabled after SetEnabled(true)")
	}

	// Disable
	collector.SetEnabled(false)
	if collector.IsEnabled() {
		t.Error("Expected collector to be disabled after SetEnabled(false)")
	}
}

func TestDebugCollector_BeginFrame_WhenDisabled(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(false)

	// Should not panic when disabled
	collector.BeginFrame(123)
	frame := collector.Emit()

	if frame != nil {
		t.Error("Expected nil frame when collector is disabled")
	}
}

func TestDebugCollector_BeginFrame_WhenEnabled(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(456)

	frame := collector.Emit()

	if frame == nil {
		t.Fatal("Expected non-nil frame when collector is enabled")
	}
	if frame.FrameID != 456 {
		t.Errorf("Expected FrameID=456, got %d", frame.FrameID)
	}
}

func TestDebugCollector_RecordAssociation_WhenDisabled(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(false)
	collector.BeginFrame(100)

	// Should not panic when disabled
	collector.RecordAssociation(1, "track_1", 5.0, true)

	frame := collector.Emit()
	if frame != nil {
		t.Error("Expected nil frame when disabled")
	}
}

func TestDebugCollector_RecordAssociation_WhenEnabled(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(100)

	collector.RecordAssociation(10, "track_42", 3.5, true)
	collector.RecordAssociation(11, "track_43", 8.2, false)

	frame := collector.Emit()

	if frame == nil {
		t.Fatal("Expected non-nil frame")
	}
	if len(frame.AssociationCandidates) != 2 {
		t.Fatalf("Expected 2 association records, got %d", len(frame.AssociationCandidates))
	}

	// Check first record
	rec1 := frame.AssociationCandidates[0]
	if rec1.ClusterID != 10 || rec1.TrackID != "track_42" {
		t.Errorf("Record 1: expected cluster=10 track=track_42, got cluster=%d track=%s",
			rec1.ClusterID, rec1.TrackID)
	}
	if rec1.MahalanobisDistSquared != 3.5 {
		t.Errorf("Record 1: expected dist=3.5, got %.2f", rec1.MahalanobisDistSquared)
	}
	if !rec1.Accepted {
		t.Error("Record 1: expected accepted=true")
	}

	// Check second record
	rec2 := frame.AssociationCandidates[1]
	if rec2.Accepted {
		t.Error("Record 2: expected accepted=false")
	}
}

func TestDebugCollector_RecordGatingRegion(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(200)

	collector.RecordGatingRegion("track_10", 5.0, 10.0, 2.5, 1.5, 0.78)

	frame := collector.Emit()

	if frame == nil {
		t.Fatal("Expected non-nil frame")
	}
	if len(frame.GatingRegions) != 1 {
		t.Fatalf("Expected 1 gating region, got %d", len(frame.GatingRegions))
	}

	region := frame.GatingRegions[0]
	if region.TrackID != "track_10" {
		t.Errorf("Expected TrackID=track_10, got %s", region.TrackID)
	}
	if region.CenterX != 5.0 || region.CenterY != 10.0 {
		t.Errorf("Expected center=(5.0, 10.0), got (%.1f, %.1f)", region.CenterX, region.CenterY)
	}
	if region.SemiMajorM != 2.5 || region.SemiMinorM != 1.5 {
		t.Errorf("Expected axes=(2.5, 1.5), got (%.1f, %.1f)", region.SemiMajorM, region.SemiMinorM)
	}
	if region.RotationRad != 0.78 {
		t.Errorf("Expected rotation=0.78, got %.2f", region.RotationRad)
	}
}

func TestDebugCollector_RecordInnovation(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(300)

	collector.RecordInnovation("track_5", 10.0, 20.0, 10.5, 20.3, 0.583)

	frame := collector.Emit()

	if frame == nil {
		t.Fatal("Expected non-nil frame")
	}
	if len(frame.Innovations) != 1 {
		t.Fatalf("Expected 1 innovation, got %d", len(frame.Innovations))
	}

	innov := frame.Innovations[0]
	if innov.TrackID != "track_5" {
		t.Errorf("Expected TrackID=track_5, got %s", innov.TrackID)
	}
	if innov.PredictedX != 10.0 || innov.PredictedY != 20.0 {
		t.Errorf("Expected predicted=(10.0, 20.0), got (%.1f, %.1f)",
			innov.PredictedX, innov.PredictedY)
	}
	if innov.MeasuredX != 10.5 || innov.MeasuredY != 20.3 {
		t.Errorf("Expected measured=(10.5, 20.3), got (%.1f, %.1f)",
			innov.MeasuredX, innov.MeasuredY)
	}
	if innov.ResidualMag != 0.583 {
		t.Errorf("Expected residual=0.583, got %.3f", innov.ResidualMag)
	}
}

func TestDebugCollector_RecordPrediction(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(400)

	collector.RecordPrediction("track_7", 15.0, 25.0, 1.5, 2.0)

	frame := collector.Emit()

	if frame == nil {
		t.Fatal("Expected non-nil frame")
	}
	if len(frame.StatePredictions) != 1 {
		t.Fatalf("Expected 1 prediction, got %d", len(frame.StatePredictions))
	}

	pred := frame.StatePredictions[0]
	if pred.TrackID != "track_7" {
		t.Errorf("Expected TrackID=track_7, got %s", pred.TrackID)
	}
	if pred.X != 15.0 || pred.Y != 25.0 {
		t.Errorf("Expected position=(15.0, 25.0), got (%.1f, %.1f)", pred.X, pred.Y)
	}
	if pred.VX != 1.5 || pred.VY != 2.0 {
		t.Errorf("Expected velocity=(1.5, 2.0), got (%.1f, %.1f)", pred.VX, pred.VY)
	}
}

func TestDebugCollector_MultipleRecords(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(500)

	// Record various artifacts
	collector.RecordAssociation(1, "t1", 1.0, true)
	collector.RecordAssociation(2, "t2", 2.0, false)
	collector.RecordGatingRegion("t1", 1.0, 2.0, 3.0, 4.0, 0.5)
	collector.RecordInnovation("t1", 5.0, 6.0, 5.1, 6.1, 0.14)
	collector.RecordPrediction("t2", 7.0, 8.0, 0.5, 0.6)

	frame := collector.Emit()

	if frame == nil {
		t.Fatal("Expected non-nil frame")
	}

	if len(frame.AssociationCandidates) != 2 {
		t.Errorf("Expected 2 association records, got %d", len(frame.AssociationCandidates))
	}
	if len(frame.GatingRegions) != 1 {
		t.Errorf("Expected 1 gating region, got %d", len(frame.GatingRegions))
	}
	if len(frame.Innovations) != 1 {
		t.Errorf("Expected 1 innovation, got %d", len(frame.Innovations))
	}
	if len(frame.StatePredictions) != 1 {
		t.Errorf("Expected 1 prediction, got %d", len(frame.StatePredictions))
	}
}

func TestDebugCollector_EmitClearsState(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(600)

	collector.RecordAssociation(1, "t1", 1.0, true)

	// First emit
	frame1 := collector.Emit()
	if frame1 == nil {
		t.Fatal("Expected first frame")
	}

	// Second emit without BeginFrame should return nil
	frame2 := collector.Emit()
	if frame2 != nil {
		t.Error("Expected nil frame after second Emit without BeginFrame")
	}
}

func TestDebugCollector_Reset(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)
	collector.BeginFrame(700)

	collector.RecordAssociation(1, "t1", 1.0, true)

	// Reset clears without emitting
	collector.Reset()

	frame := collector.Emit()
	if frame != nil {
		t.Error("Expected nil frame after Reset")
	}
}

func TestDebugCollector_RecordWithoutBeginFrame(t *testing.T) {
	collector := NewDebugCollector()
	collector.SetEnabled(true)

	// Record without BeginFrame - should not panic, and should be ignored
	collector.RecordAssociation(1, "t1", 1.0, true)

	// Since we never called BeginFrame, Emit should return nil
	frame := collector.Emit()
	if frame != nil {
		t.Error("Expected nil frame when recording without BeginFrame")
	}
}
