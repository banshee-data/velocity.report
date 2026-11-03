package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// With -settling-trajectory each case's first run writes the background's
// convergence checks beside its baseline, and a repeat that checked
// differently fails the case as a differing baseline would.
func TestSettlingTrajectoryIsWrittenAndRepeatChecked(t *testing.T) {
	s := newStubCorpus(t, false)
	s.replay.settlingTrajectory = true
	checks := []l3grid.SettlingCheck{
		{ElapsedSeconds: 6, Measured: false, CoverageRate: 0.9, MeanConfidence: 40},
		{ElapsedSeconds: 7, Measured: true, SpreadDeltaRate: 0.0042, CoverageRate: 0.9, RegionStability: 1, MeanConfidence: 44},
	}
	stub := s.replay.replay
	s.replay.replay = func(cfg replayeval.Config) (*replayeval.Result, error) {
		result, err := stub(cfg)
		if result != nil {
			result.SettlingTrajectory = checks
		}
		return result, err
	}
	if _, _, err := s.replay.run(s.cases); err != nil {
		t.Fatal(err)
	}
	var got settlingTrajectory
	readJSON(t, filepath.Join(s.caseOut("marina-webster-beach"), "first", settlingTrajectoryFile), &got)
	if !reflect.DeepEqual(got.Checks, checks) {
		t.Errorf("trajectory %+v, want %+v", got.Checks, checks)
	}

	s = newStubCorpus(t, false)
	s.replay.settlingTrajectory = true
	stub = s.replay.replay
	runs := 0
	s.replay.replay = func(cfg replayeval.Config) (*replayeval.Result, error) {
		result, err := stub(cfg)
		runs++
		if result != nil && runs == 1 {
			result.SettlingTrajectory = checks
		}
		return result, err
	}
	if _, _, err := s.replay.run(s.cases); err == nil || !strings.Contains(err.Error(), "settling trajectory differs on repeat") {
		t.Fatalf("run error %v, want the differing repeat to fail", err)
	}
}

// Without the flag no trajectory file is written.
func TestSettlingTrajectoryIsOffByDefault(t *testing.T) {
	s := newStubCorpus(t, false)
	if _, _, err := s.replay.run(s.cases); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(s.caseOut("marina-webster-beach"), "first", settlingTrajectoryFile)) {
		t.Error("a trajectory was written without -settling-trajectory")
	}
}
