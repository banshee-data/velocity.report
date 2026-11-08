package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// syntheticReplay drives a tracker with the shadow solid body under the
// face-state options over a synthetic pass, through the hooks the replay
// harness offers: the filter-step observer attached before the first frame,
// the tap after every update, the second half of the pass scored.
func syntheticReplay(t *testing.T, cfg replayeval.Config) {
	t.Helper()
	tc := l5tracks.DefaultTrackerConfig()
	tc.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: "test: synthetic pass in the sensor frame",
		CourseAlignedFaces: true, ExtentState: true, CensoredExtent: true, SpanBoundsBelief: true,
		HeadingFaceNoise: true, AnisotropicReferenceBias: true}
	tracker := l5tracks.NewTracker(tc)
	if cfg.FilterStepObserver != nil {
		tracker.SetFilterStepObserver(cfg.FilterStepObserver)
	}
	params := l4perception.DefaultDBSCANParams()
	params.Eps, params.MinPts, params.MaxSamplePoints = 0.6, 5, 512
	params.MeanCentre, params.MaxClusterAspectRatio = false, 15
	base := time.Unix(1_700_000_000, 0).Sub(time.Unix(0, 0))
	pass := l4perception.DefaultSyntheticPass()
	pass.Frames = 60
	for i, f := range l4perception.GenerateSyntheticPass(pass) {
		points := make([]l4perception.WorldPoint, len(f.Points))
		for j, p := range f.Points {
			p.Timestamp = p.Timestamp.Add(base)
			points[j] = p
		}
		at := f.Timestamp.Add(base)
		clusters := l4perception.DBSCAN(points, params)
		tracker.Update(clusters, at)
		if cfg.TrackerTap != nil {
			cfg.TrackerTap(replayeval.TrackerTapFrame{At: at, Scored: i >= 10, Clusters: clusters,
				Associations: tracker.GetLastAssociations(), Tracks: tracker.GetActiveTracks()})
		}
	}
}

// -gest1-report attaches the collector to the first run only, writes the
// report beside the first run's recording with both sources, and names it in
// the case summary; the repeat runs without it.
func TestGEST1ReportIsWrittenFromTheFirstRun(t *testing.T) {
	s := newStubCorpus(t, false)
	s.replay.gest1 = &gest1Options{measurementVariance: 0.05, maxCoastSecs: 2.5}
	stub := s.replay.replay
	hooked := map[string]bool{}
	s.replay.replay = func(cfg replayeval.Config) (*replayeval.Result, error) {
		hooked[filepath.Base(cfg.OutDir)] = cfg.FilterStepObserver != nil || cfg.TrackerTap != nil
		syntheticReplay(t, cfg)
		return stub(cfg)
	}
	cases, _, err := s.replay.run(s.cases[:1])
	if err != nil {
		t.Fatal(err)
	}
	if !hooked["first"] || hooked["repeat"] {
		t.Fatalf("hooks first %v repeat %v: the collector belongs to the first run only", hooked["first"], hooked["repeat"])
	}
	id := s.cases[0].corpusCase.ID
	want := filepath.Join(id, "first", gest1ReportFile)
	if len(cases) != 1 || cases[0].GEST1Report != want {
		t.Fatalf("summary names %q, want %q", cases[0].GEST1Report, want)
	}
	var report l8analytics.GEST1Report
	readJSON(t, filepath.Join(s.replay.outDir, want), &report)
	if len(report.Sources) != 2 || report.Sources[0].Source != l8analytics.GEST1SourceTracked ||
		report.Sources[1].Source != l8analytics.GEST1SourceSolidBody {
		t.Fatalf("report sources %+v", report.Sources)
	}
	for _, src := range report.Sources {
		if src.MovingTracks == 0 || src.Samples == 0 || src.Criterion5Cases == 0 {
			t.Errorf("%s: moving %d samples %d criterion 5 cases %d", src.Source, src.MovingTracks, src.Samples, src.Criterion5Cases)
		}
	}
	if report.Config.Occlusion.HorizonsSecs[2] != 2.5 || report.CrossRead == nil {
		t.Errorf("config horizons %v, cross-read %v", report.Config.Occlusion.HorizonsSecs, report.CrossRead)
	}
	if record := readCaseRecord(t, s.caseOut(id)); record.Case.GEST1Report != want {
		t.Errorf("case record names %q", record.Case.GEST1Report)
	}
}

// The longest permitted coast is the tuning's seconds bound where it sets
// one, and its confirmed miss count in 10 Hz frames otherwise; the shipped
// tuning bounds it by 25 misses.
func TestGEST1SettingsReadTheShippedTuning(t *testing.T) {
	variance, coast, err := gest1Settings("")
	if err != nil {
		t.Fatal(err)
	}
	if variance != float64(float32(0.05)) || coast != 25*gest1FramePeriodSecs {
		t.Errorf("measurement variance %v, longest coast %v s; want 0.05 and 2.5", variance, coast)
	}
}

func TestChainTaps(t *testing.T) {
	if chainTaps(nil, nil) != nil {
		t.Error("no taps should chain to nil")
	}
	var order []int
	tap := chainTaps(func(replayeval.TrackerTapFrame) { order = append(order, 1) }, nil,
		func(replayeval.TrackerTapFrame) { order = append(order, 2) })
	tap(replayeval.TrackerTapFrame{})
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("order %v", order)
	}
}
