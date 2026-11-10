package main

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// spuriousTap is one stub tracker update at second t: n clusters, taken by
// the tracks named (empty for unassociated), with the active tracks given.
func spuriousTap(start time.Time, t float64, taken []string, tracks ...*l5tracks.TrackedObject) replayeval.TrackerTapFrame {
	clusters := make([]l5tracks.WorldCluster, len(taken))
	return replayeval.TrackerTapFrame{At: start.Add(time.Duration(t * float64(time.Second))),
		Clusters: clusters, Associations: taken, Tracks: tracks}
}

// A cluster is spurious unless a track that was confirmed at some frame took
// it: an unassociated cluster is, and so is one a track took that never got
// past tentative, even on frames before a lasting track was confirmed.
func TestSpuriousClustersCountClustersNoLastingTrackTook(t *testing.T) {
	start := time.Unix(1000, 0)
	tentative := &l5tracks.TrackedObject{TrackID: "a", TrackState: l5tracks.TrackTentative}
	confirmed := &l5tracks.TrackedObject{TrackID: "a", TrackState: l5tracks.TrackConfirmed}
	brief := &l5tracks.TrackedObject{TrackID: "b", TrackState: l5tracks.TrackTentative}
	c := newSpuriousCollector()
	// Settled at 10 s. Early window 10 to 40 s, steady 70 to 130 s.
	c.observe(spuriousTap(start, 5, nil))
	c.observe(spuriousTap(start, 10, []string{"a", "", "b"}, tentative, brief)) // a lasts: 2 spurious
	c.observe(spuriousTap(start, 20, []string{"a", ""}, confirmed))             // 1 spurious
	c.observe(spuriousTap(start, 70, []string{"a"}, confirmed))                 // 0
	c.observe(spuriousTap(start, 80, []string{""}))                             // 1
	c.observe(spuriousTap(start, 130, nil))                                     // after steady
	r := c.report(start.UnixNano(), true, 10, true)

	if r.LastingTracks != 1 {
		t.Errorf("lasting tracks %d, want 1", r.LastingTracks)
	}
	if got := []int{r.Frames[1].Spurious, r.Frames[2].Spurious, r.Frames[3].Spurious, r.Frames[4].Spurious}; got[0] != 2 || got[1] != 1 || got[2] != 0 || got[3] != 1 {
		t.Errorf("spurious per frame %v, want [2 1 0 1]", got)
	}
	if r.Frames[1].ElapsedSeconds != 10 {
		t.Errorf("elapsed %v, want 10", r.Frames[1].ElapsedSeconds)
	}
	if r.Early.Frames != 2 || r.Early.Spurious != 3 || r.Early.SpuriousPerFrame != 1.5 || !r.Early.Complete {
		t.Errorf("early window %+v", r.Early)
	}
	if r.Steady.Frames != 2 || r.Steady.Spurious != 1 || r.Steady.SpuriousPerFrame != 0.5 || !r.Steady.Complete {
		t.Errorf("steady window %+v", r.Steady)
	}
	if r.EarlyOverSteady == nil || math.Abs(*r.EarlyOverSteady-3) > 1e-12 || !r.QuietMoment {
		t.Errorf("early over steady %v, quiet %v; want 3, true", r.EarlyOverSteady, r.QuietMoment)
	}
}

// An unsettled replay has no windows, and a replay that ends inside a window
// says the window is incomplete.
func TestSpuriousClustersWindowsNeedASettleAndTheirWholeSpan(t *testing.T) {
	start := time.Unix(1000, 0)
	c := newSpuriousCollector()
	c.observe(spuriousTap(start, 31, []string{""}))
	c.observe(spuriousTap(start, 95, []string{""}))
	r := c.report(start.UnixNano(), false, 0, false)
	if r.Early.Frames != 0 || r.Steady.Frames != 0 || r.EarlyOverSteady != nil || r.QuietMoment {
		t.Errorf("unsettled replay has windows: %+v %+v", r.Early, r.Steady)
	}
	r = c.report(start.UnixNano(), true, 30, false)
	if !r.Early.Complete || r.Early.Frames != 1 {
		t.Errorf("early window %+v, want one frame and complete", r.Early)
	}
	if r.Steady.Complete || r.Steady.Frames != 1 {
		t.Errorf("steady window %+v, want one frame and incomplete", r.Steady)
	}
	if r.QuietMoment {
		t.Error("equal rates named a quiet moment")
	}
}

// With -spurious-clusters each case's first run writes the record from the
// tracker tap and the replay's settle, and a repeat that settled at another
// time fails the case.
func TestSpuriousClustersAreWrittenAndTheSettleRepeatChecked(t *testing.T) {
	start := time.Unix(1000, 0)
	stubbed := func(s *stubCorpus, settleOnRepeat float64) {
		stub := s.replay.replay
		runs := map[string]int{}
		s.replay.replay = func(cfg replayeval.Config) (*replayeval.Result, error) {
			result, err := stub(cfg)
			caseID := filepath.Base(filepath.Dir(cfg.OutDir))
			runs[caseID]++
			if cfg.TrackerTap != nil {
				cfg.TrackerTap(spuriousTap(start, 20, []string{""}))
			}
			if result != nil {
				result.Settled, result.SettledAfterSeconds, result.BackgroundStartUnixNanos = true, 16.9, start.UnixNano()
				if runs[caseID] == 2 {
					result.SettledAfterSeconds = settleOnRepeat
				}
			}
			return result, err
		}
	}
	s := newStubCorpus(t, false)
	s.replay.spuriousClusters = true
	stubbed(s, 16.9)
	if _, _, err := s.replay.run(s.cases); err != nil {
		t.Fatal(err)
	}
	var got spuriousReport
	readJSON(t, filepath.Join(s.caseOut("marina-webster-beach"), "first", spuriousClustersFile), &got)
	if !got.Settled || got.SettledAfterSeconds != 16.9 || got.Early.Frames != 1 || got.Early.Spurious != 1 {
		t.Errorf("record %+v", got)
	}

	s = newStubCorpus(t, false)
	s.replay.spuriousClusters = true
	stubbed(s, 30)
	if _, _, err := s.replay.run(s.cases); err == nil || !strings.Contains(err.Error(), "settle time differs on repeat") {
		t.Fatalf("run error %v, want the differing settle to fail", err)
	}

	s = newStubCorpus(t, false)
	if _, _, err := s.replay.run(s.cases); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(s.caseOut("marina-webster-beach"), "first", spuriousClustersFile)) {
		t.Error("a record was written without -spurious-clusters")
	}
}
