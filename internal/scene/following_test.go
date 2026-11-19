package scene

import (
	"errors"
	"math"
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

func TestFollowingTimelineKeepsMissingDistinctFromZero(t *testing.T) {
	a := newTimelineAccumulator(5)
	zero, gap, peak := 0.0, 1.2, 2.5
	a.observe(Frame{TimeUs: 0, Tracks: []TrackJSON{{ID: "f", Following: &FollowingJSON{Bucket: "following_eligible", TimeGapS: &gap, DRAC: &zero, DRACStatus: "supported"}}}})
	a.observe(Frame{TimeUs: 1_000_000, Tracks: []TrackJSON{{ID: "f", Following: &FollowingJSON{Bucket: "following_eligible", TimeGapS: &gap, DRAC: &peak, DRACStatus: "supported"}}}})
	a.observe(Frame{TimeUs: 5_000_000, Tracks: []TrackJSON{{ID: "f", Following: &FollowingJSON{Bucket: "record_gap", TimeGapS: &zero, DRAC: &peak}}}})
	s := a.summary()
	if len(s.Buckets) != 2 || s.Buckets[0].MaxDRAC == nil || *s.Buckets[0].MaxDRAC != peak || s.Buckets[0].MinimumTimeGapS == nil || *s.Buckets[0].MinimumTimeGapS != gap || s.Buckets[1].MaxDRAC != nil || s.Buckets[1].MinimumTimeGapS != nil {
		t.Fatal(s)
	}
	a.observe(Frame{TimeUs: 10_000_000, Tracks: []TrackJSON{{ID: "f", Following: &FollowingJSON{Bucket: "following_excluded", DRAC: &peak, DRACStatus: "supported"}}}})
	last := a.summary().Buckets[2]
	if last.MaxDRAC == nil || *last.MaxDRAC != peak || last.MinimumTimeGapS != nil {
		t.Fatal("DRAC acquired a headway speed floor", last)
	}
	a.observe(Frame{TimeUs: 15_000_000, Tracks: []TrackJSON{{ID: "f", Following: &FollowingJSON{Bucket: "no_exact_following_sample", MinimumTimeGapS: &gap}}}})
	if bucket := a.summary().Buckets[3]; bucket.MinimumTimeGapS != nil || bucket.MaxDRAC != nil {
		t.Fatal("whole-recording minimum became an instantaneous measurement", bucket)
	}
}

func TestFollowingMinimumPersistsWithoutAnExactPair(t *testing.T) {
	minimum, gap, timeGap, drac, speed := 0.6, 12.0, 1.2, 0.3, 8.0
	out := filepath.Join(t.TempDir(), "scene")
	_, err := Export(Options{VRLOGPath: writeVRLOG(t, evenTimestamps(3)), OutDir: out,
		Following: followingProvenanceFixture(), MinimumTimeGapByTrack: map[string]float64{"source-track-0": minimum, "absent": minimum},
		FollowingAt: func(at int64) (map[string]FollowingJSON, error) {
			if at != baseNs {
				return nil, nil
			}
			return map[string]FollowingJSON{"source-track-0": {Leader: "missing", Bucket: "following_eligible", GapM: &gap, TimeGapS: &timeGap, DRAC: &drac, DRACStatus: "supported", LeaderSpeed: &speed, GapPath: [][2]float64{{0, 0}, {12, 0}}}}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	for i, frame := range readFrames(t, out) {
		if len(frame.Tracks) != 3 {
			t.Fatal("minimum invented an actor", frame)
		}
		value := frame.Tracks[0].Following
		if value == nil || value.MinimumTimeGapS == nil || *value.MinimumTimeGapS != minimum {
			t.Fatal("whole-recording minimum disappeared", frame)
		}
		if value.Leader != "" || value.GapM != nil || value.TimeGapS != nil || value.DRAC != nil || value.LeaderSpeed != nil || len(value.GapPath) != 0 {
			t.Fatal("minimum invented an instantaneous relationship", value)
		}
		if i == 0 && value.Bucket != "leader_not_in_frame" || i > 0 && value.Bucket != "no_exact_following_sample" {
			t.Fatal("instantaneous availability was lost", value)
		}
	}
}

func TestFollowingMinimumRefusesUnboundAndInvalidValues(t *testing.T) {
	if _, err := Export(Options{MinimumTimeGapByTrack: map[string]float64{"f": 1}}); err == nil {
		t.Fatal("unbound whole-recording metadata accepted")
	}
	for _, row := range []struct {
		id    string
		value float64
	}{{"", 1}, {"f", -1}, {"f", math.NaN()}, {"f", math.Inf(1)}} {
		if _, err := Export(Options{Following: followingProvenanceFixture(), FollowingAt: func(int64) (map[string]FollowingJSON, error) { return nil, nil }, MinimumTimeGapByTrack: map[string]float64{row.id: row.value}}); err == nil {
			t.Fatal("invalid whole-recording minimum accepted", row)
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
