package analysis

import (
	"encoding/json"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/version"
	"os"
	"path/filepath"
	"testing"
)

func TestHeadingComparisonNullAndEligibleDeltas(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write := func(dir string, p50 float64, n int) {
		t.Helper()
		r := AnalysisReport{Version: version.Version, TrackSummary: TrackSummary{Alignment: &AlignmentSummary{CourseAlignmentTracks: n, CourseAlignmentP50Deg: &DistStats{P50: &p50}}, HeadingLock: &HeadingLockSummary{TerminalAssessed: n, TerminalUnrecovered: 1}}}
		data, _ := json.Marshal(r)
		if err := os.WriteFile(filepath.Join(dir, "analysis.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(a, 40, 5)
	write(b, 20, 10)
	r, err := CompareReports(a, b, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.QualityDelta.CourseAlignmentP50 == nil || r.QualityDelta.CourseAlignmentP50.Delta != -20 || r.QualityDelta.TerminalUnrecoveredRatio == nil || r.QualityDelta.CourseTracksA != 5 {
		t.Fatal("missing comparison evidence")
	}
	write(b, 0, 0)
	r, err = CompareReports(a, b, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.QualityDelta.CourseAlignmentP50 != nil || r.QualityDelta.TerminalUnrecoveredRatio != nil {
		t.Fatal("empty arm treated as perfect")
	}
}

func TestRecordedHeadingEpisodeParityAndCoasting(t *testing.T) {
	dir := t.TempDir()
	rec, err := recorder.NewRecorder(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	var live l5tracks.HeadingEpisodeState
	seq := "LLLLLUUUUULLLLL"
	for i, c := range seq {
		src := l5tracks.HeadingSourceAxis
		if c == 'L' {
			src = l5tracks.HeadingSourceAmbiguous
		}
		ts := int64(i+1) * 100000000
		live.Observe(src, ts)
		tr := l9endpoints.Track{TrackID: "car", ObservationCount: i + 1, State: l9endpoints.TrackStateConfirmed, FirstSeenNanos: 100000000, LastSeenNanos: ts, HeadingSource: int(src)}
		for j := 0; j < 2; j++ {
			if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(i*2 + j + 1), TimestampNanos: ts + int64(j), Tracks: &l9endpoints.TrackSet{Tracks: []l9endpoints.Track{tr}}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	r, _, err := GenerateReport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tracks) != 1 || r.Tracks[0].TerminalLockOutcome != live.Outcome() || r.Tracks[0].HeadingEpisodes.Episodes != live.Episodes || r.Tracks[0].HeadingEpisodes.LongestSeconds != live.LongestSeconds {
		t.Fatalf("live/offline differ: %+v", r.Tracks)
	}
	if r.TrackSummary.HeadingLock.TerminalUnrecovered != 1 {
		t.Fatal("terminal relock hidden")
	}
	if !r.Tracks[0].EverConfirmed || r.FrameSummary.CoLocation.ScoredFrames != 30 || r.FrameSummary.CoLocation.LiveFrames != 30 || r.FrameSummary.CoLocation.PairFrames != 0 {
		t.Fatal("missing eligibility or co-location denominators")
	}
}

func TestCoLocationAndCompletedTrackComparison(t *testing.T) {
	dirs := []string{t.TempDir(), t.TempDir()}
	for arm, dir := range dirs {
		rec, err := recorder.NewRecorder(dir, "test")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			state := l9endpoints.TrackStateConfirmed
			if i == 2 {
				state = l9endpoints.TrackStateDeleted
			}
			tracks := []l9endpoints.Track{
				{TrackID: "car", State: state, FirstSeenNanos: 1, LastSeenNanos: int64(i+1) * 1000000000, ObservationCount: i + 1},
				{TrackID: "nearby", State: state, X: float32(2 + arm*5), FirstSeenNanos: 1, LastSeenNanos: int64(i+1) * 1000000000, ObservationCount: i + 1},
			}
			if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(i + 1), TimestampNanos: int64(i+1) * 1000000000, Tracks: &l9endpoints.TrackSet{Tracks: tracks}}); err != nil {
				t.Fatal(err)
			}
		}
		if err := rec.Close(); err != nil {
			t.Fatal(err)
		}
		r, _, err := GenerateReport(dir)
		if err != nil {
			t.Fatal(err)
		}
		if r.FrameSummary.CoLocation.LiveFrames != 2 || r.FrameSummary.CoLocation.PairFrames != 2*(1-arm) {
			t.Fatalf("incorrect proximity: %+v", r.FrameSummary.CoLocation)
		}
	}
	r, err := CompareReports(dirs[0], dirs[1], "")
	if err != nil {
		t.Fatal(err)
	}
	if r.TrackMatching.MatchedPairs != 2 || r.TrackMatching.Method != "temporal_overlap_proxy_not_identity" {
		t.Fatal("completed tracks excluded or identity overclaimed")
	}
	if r.QualityDelta.CoLocatedFrameRatio == nil || r.QualityDelta.CoLocatedFrameRatio.Delta != -2.0/3 {
		t.Fatal("missing co-location delta")
	}
}
