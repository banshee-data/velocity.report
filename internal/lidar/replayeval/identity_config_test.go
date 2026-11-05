package replayeval

import (
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestIdentityTuningAndReplayAliasesAreEquivalent(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	aliases := []string{ExperimentOverlapEnd1, ExperimentOverlapEndBody, ExperimentOverlapEndOlder, ExperimentTentativePriority}
	want, err := trackerConfigFor(l5, "", aliases, nil)
	if err != nil {
		t.Fatal(err)
	}
	l5.Identity = &config.L5Identity{TentativePriority: true, OverlapEndFrames: 1, OverlapEndOnBody: true, OverlapEndKeepOlder: true}
	got, err := trackerConfigFor(l5, "", nil, nil)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("config/alias mismatch: %v\n%+v\n%+v", err, got, want)
	}
	live := l5tracks.TrackerConfigFromTuning(l5)
	if !live.TentativePriority || live.OverlapEndFrames != 1 || !live.OverlapEndOnBody || !live.OverlapEndKeepOlder {
		t.Fatal("live mapping lost identity options")
	}
	if _, err := trackerConfigFor(l5, "", []string{ExperimentOverlapEnd3}, nil); err == nil {
		t.Fatal("conflicting overlap alias accepted")
	}
	if _, err := trackerConfigFor(l5, "", aliases, nil); err != nil {
		t.Fatal("matching aliases refused", err)
	}
}
