package sweep

import (
	"testing"
)

func TestComputeAgreement_EmptyLabels(t *testing.T) {
	result := ComputeAgreement(nil)
	if result.TotalTracks != 0 {
		t.Errorf("expected TotalTracks=0, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 0 {
		t.Errorf("expected MultiLabelledCount=0, got %d", result.MultiLabelledCount)
	}
	if result.AgreementRate != 0 {
		t.Errorf("expected AgreementRate=0, got %f", result.AgreementRate)
	}
}

func TestComputeAgreement_SingleLabeller(t *testing.T) {
	labels := []LabelRecord{
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track2", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track3", Label: "pedestrian", LabellerID: "alice"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 3 {
		t.Errorf("expected TotalTracks=3, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 0 {
		t.Errorf("expected MultiLabelledCount=0 (all single-labeller), got %d", result.MultiLabelledCount)
	}
	if result.DisagreementCount != 0 {
		t.Errorf("expected DisagreementCount=0, got %d", result.DisagreementCount)
	}
	if result.AgreementRate != 0 {
		t.Errorf("expected AgreementRate=0 (no multi-labelled tracks), got %f", result.AgreementRate)
	}
}

func TestComputeAgreement_PerfectAgreement(t *testing.T) {
	labels := []LabelRecord{
		// Track 1: two labellers agree
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track1", Label: "vehicle", LabellerID: "bob"},
		// Track 2: two labellers agree
		{TrackID: "track2", Label: "pedestrian", LabellerID: "alice"},
		{TrackID: "track2", Label: "pedestrian", LabellerID: "bob"},
		// Track 3: single labeller (should be ignored)
		{TrackID: "track3", Label: "vehicle", LabellerID: "alice"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 3 {
		t.Errorf("expected TotalTracks=3, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 2 {
		t.Errorf("expected MultiLabelledCount=2, got %d", result.MultiLabelledCount)
	}
	if result.DisagreementCount != 0 {
		t.Errorf("expected DisagreementCount=0, got %d", result.DisagreementCount)
	}
	if result.AgreementRate != 1.0 {
		t.Errorf("expected AgreementRate=1.0, got %f", result.AgreementRate)
	}
}

func TestComputeAgreement_PartialDisagreement(t *testing.T) {
	labels := []LabelRecord{
		// Track 1: agreement
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track1", Label: "vehicle", LabellerID: "bob"},
		// Track 2: disagreement
		{TrackID: "track2", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track2", Label: "pedestrian", LabellerID: "bob"},
		// Track 3: agreement
		{TrackID: "track3", Label: "pedestrian", LabellerID: "alice"},
		{TrackID: "track3", Label: "pedestrian", LabellerID: "bob"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 3 {
		t.Errorf("expected TotalTracks=3, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 3 {
		t.Errorf("expected MultiLabelledCount=3, got %d", result.MultiLabelledCount)
	}
	if result.DisagreementCount != 1 {
		t.Errorf("expected DisagreementCount=1, got %d", result.DisagreementCount)
	}
	expected := 2.0 / 3.0 // 2 out of 3 agree
	if result.AgreementRate < expected-0.001 || result.AgreementRate > expected+0.001 {
		t.Errorf("expected AgreementRate≈%f, got %f", expected, result.AgreementRate)
	}
}

func TestComputeAgreement_CompleteDisagreement(t *testing.T) {
	labels := []LabelRecord{
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track1", Label: "pedestrian", LabellerID: "bob"},
		{TrackID: "track2", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track2", Label: "pedestrian", LabellerID: "bob"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 2 {
		t.Errorf("expected TotalTracks=2, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 2 {
		t.Errorf("expected MultiLabelledCount=2, got %d", result.MultiLabelledCount)
	}
	if result.DisagreementCount != 2 {
		t.Errorf("expected DisagreementCount=2, got %d", result.DisagreementCount)
	}
	if result.AgreementRate != 0.0 {
		t.Errorf("expected AgreementRate=0.0, got %f", result.AgreementRate)
	}
}

func TestComputeAgreement_ThreeLabellers(t *testing.T) {
	labels := []LabelRecord{
		// Track with 3 labellers, all agree
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track1", Label: "vehicle", LabellerID: "bob"},
		{TrackID: "track1", Label: "vehicle", LabellerID: "charlie"},
		// Track with 3 labellers, one disagrees
		{TrackID: "track2", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track2", Label: "vehicle", LabellerID: "bob"},
		{TrackID: "track2", Label: "pedestrian", LabellerID: "charlie"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 2 {
		t.Errorf("expected TotalTracks=2, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 2 {
		t.Errorf("expected MultiLabelledCount=2, got %d", result.MultiLabelledCount)
	}
	if result.DisagreementCount != 1 {
		t.Errorf("expected DisagreementCount=1, got %d", result.DisagreementCount)
	}
	expected := 0.5 // 1 out of 2 tracks agree
	if result.AgreementRate != expected {
		t.Errorf("expected AgreementRate=%f, got %f", expected, result.AgreementRate)
	}
}

func TestComputeAgreement_MixedScenario(t *testing.T) {
	labels := []LabelRecord{
		// Track 1: single labeller (ignored)
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		// Track 2: two labellers agree
		{TrackID: "track2", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track2", Label: "vehicle", LabellerID: "bob"},
		// Track 3: three labellers disagree
		{TrackID: "track3", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track3", Label: "pedestrian", LabellerID: "bob"},
		{TrackID: "track3", Label: "bicycle", LabellerID: "charlie"},
		// Track 4: single labeller (ignored)
		{TrackID: "track4", Label: "pedestrian", LabellerID: "bob"},
		// Track 5: two labellers agree
		{TrackID: "track5", Label: "pedestrian", LabellerID: "alice"},
		{TrackID: "track5", Label: "pedestrian", LabellerID: "charlie"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 5 {
		t.Errorf("expected TotalTracks=5, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 3 {
		t.Errorf("expected MultiLabelledCount=3, got %d", result.MultiLabelledCount)
	}
	if result.DisagreementCount != 1 {
		t.Errorf("expected DisagreementCount=1 (track3), got %d", result.DisagreementCount)
	}
	expected := 2.0 / 3.0 // 2 out of 3 multi-labelled tracks agree
	if result.AgreementRate < expected-0.001 || result.AgreementRate > expected+0.001 {
		t.Errorf("expected AgreementRate≈%f, got %f", expected, result.AgreementRate)
	}
}

func TestComputeAgreement_SameLabelerMultipleTimes(t *testing.T) {
	// Edge case: same labeller provides multiple labels for the same track
	// This should still count as single-labeller
	labels := []LabelRecord{
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
		{TrackID: "track1", Label: "vehicle", LabellerID: "alice"},
	}

	result := ComputeAgreement(labels)

	if result.TotalTracks != 1 {
		t.Errorf("expected TotalTracks=1, got %d", result.TotalTracks)
	}
	if result.MultiLabelledCount != 0 {
		t.Errorf("expected MultiLabelledCount=0 (same labeller), got %d", result.MultiLabelledCount)
	}
}
