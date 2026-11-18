package scene

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func followingProvenanceFixture() *FollowingProvenance {
	return &FollowingProvenance{EvidenceSHA256: "sha256:fixture", CalibrationID: "calibration/v1/fixture", AnalysisBuild: "fixture-build", MethodID: "fixture-method", Status: "provisional_estimates"}
}

func TestFollowingExportExactSamplesAndLocalLeaderKeys(t *testing.T) {
	vr := writeVRLOG(t, evenTimestamps(3))
	out := filepath.Join(t.TempDir(), "scene")
	gap, timeGap := 12.3, 1.2
	_, err := Export(Options{VRLOGPath: vr, OutDir: out, Following: followingProvenanceFixture(), FollowingAt: func(at int64) (map[string]FollowingJSON, error) {
		if at != baseNs {
			return nil, nil
		}
		return map[string]FollowingJSON{"source-track-0": {Leader: "source-track-1", Bucket: "following_eligible", GapM: &gap, TimeGapS: &timeGap}, "source-track-2": {Leader: "missing", Bucket: "following_eligible", GapM: &gap}, "absent-follower": {Bucket: "no_leader"}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	frames := readFrames(t, out)
	for i, f := range frames {
		if len(f.Tracks) != 3 {
			t.Fatal("following evidence invented a recorded actor")
		}
		for j, tr := range f.Tracks {
			if strings.Contains(tr.ID, "source-track") {
				t.Fatal("raw identifier leaked")
			}
			if i > 0 && tr.Following != nil {
				t.Fatal("interpolated following")
			}
			if i == 0 && j == 0 {
				if tr.Following == nil || tr.Following.Leader != f.Tracks[1].ID || *tr.Following.GapM != gap || *tr.Following.TimeGapS != timeGap {
					t.Fatal(tr)
				}
			}
			if i == 0 && j == 2 {
				if tr.Following == nil || tr.Following.Bucket != "leader_not_in_frame" || tr.Following.GapM != nil || tr.Following.Leader != "" {
					t.Fatal(tr)
				}
			}
		}
	}
}

func TestFollowingExportFailsClosedOnEvidenceError(t *testing.T) {
	failure := errors.New("source mismatch")
	_, err := Export(Options{VRLOGPath: writeVRLOG(t, evenTimestamps(2)), OutDir: filepath.Join(t.TempDir(), "scene"), Following: followingProvenanceFixture(), FollowingAt: func(int64) (map[string]FollowingJSON, error) { return nil, failure }})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestFollowingExportRefusesUnboundAndMixedTrailInputs(t *testing.T) {
	provider := func(int64) (map[string]FollowingJSON, error) { return nil, nil }
	for _, opts := range []Options{{FollowingAt: provider}, {Following: followingProvenanceFixture()}, {Following: followingProvenanceFixture(), FollowingAt: provider, SmoothedTrail: true}, {Following: &FollowingProvenance{}, FollowingAt: provider}} {
		if _, err := Export(opts); err == nil || !strings.Contains(err.Error(), "following") {
			t.Fatal("unbound following or mixed trail accepted")
		}
	}
}
