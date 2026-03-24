package server

import (
	"testing"
)

// --- normaliseCommaSeparatedLabelValue ---

func TestNormaliseCSV_Empty(t *testing.T) {
	if got := normaliseCommaSeparatedLabelValue(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestNormaliseCSV_WhitespaceOnly(t *testing.T) {
	if got := normaliseCommaSeparatedLabelValue("   "); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestNormaliseCSV_SingleValue(t *testing.T) {
	if got := normaliseCommaSeparatedLabelValue("car"); got != "car" {
		t.Errorf("got %q, want car", got)
	}
}

func TestNormaliseCSV_TrimsParts(t *testing.T) {
	if got := normaliseCommaSeparatedLabelValue(" car , van , "); got != "car,van" {
		t.Errorf("got %q, want 'car,van'", got)
	}
}

func TestNormaliseCSV_EmptyPartsRemoved(t *testing.T) {
	if got := normaliseCommaSeparatedLabelValue(",,,a,,b,,"); got != "a,b" {
		t.Errorf("got %q, want 'a,b'", got)
	}
}

func TestNormaliseCSV_AllEmptyParts(t *testing.T) {
	if got := normaliseCommaSeparatedLabelValue(" , , , "); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// --- normaliseLinkedTrackIDsForRequest ---

func TestNormaliseLTIDs_Nil(t *testing.T) {
	if got := normaliseLinkedTrackIDsForRequest(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestNormaliseLTIDs_EmptySlice(t *testing.T) {
	if got := normaliseLinkedTrackIDsForRequest([]string{}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestNormaliseLTIDs_AllWhitespace(t *testing.T) {
	if got := normaliseLinkedTrackIDsForRequest([]string{"  ", "\t", ""}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestNormaliseLTIDs_MixedContent(t *testing.T) {
	got := normaliseLinkedTrackIDsForRequest([]string{" t1 ", "", " t2 ", "  "})
	if len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Errorf("got %v, want [t1 t2]", got)
	}
}

func TestNormaliseLTIDs_SingleValid(t *testing.T) {
	got := normaliseLinkedTrackIDsForRequest([]string{"track-42"})
	if len(got) != 1 || got[0] != "track-42" {
		t.Errorf("got %v, want [track-42]", got)
	}
}

// --- parseRunPath additional edge cases ---

func TestParseRunPath_RunIDOnly(t *testing.T) {
	runID, sub := parseRunPath("/api/lidar/runs/my-run")
	if runID != "my-run" {
		t.Errorf("runID = %q, want my-run", runID)
	}
	if sub != "" {
		t.Errorf("sub = %q, want empty", sub)
	}
}

// --- parseTrackPath additional edge cases ---

func TestParseTrackPath_MultiSegmentAction(t *testing.T) {
	trackID, action := parseTrackPath("track-1/label/extra")
	if trackID != "track-1" {
		t.Errorf("trackID = %q, want track-1", trackID)
	}
	if action != "label/extra" {
		t.Errorf("action = %q, want label/extra", action)
	}
}
