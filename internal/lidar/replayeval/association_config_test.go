package replayeval

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestAssociationFilesMatchReplayAliasesAndLiveMapping(t *testing.T) {
	for _, tc := range []struct {
		name              string
		file              string
		likelihood, claim bool
	}{{"control", "association-containment-control.json", false, false}, {"likelihood", "association-likelihood.json", true, false}, {"claim", "association-containment-claim.json", false, true}, {"both", "association-likelihood-claim.json", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			l5 := config.MustLoadDefaultConfig().L5.CvKfV1
			aliases := []string{ExperimentSolidBody, ExperimentSolidBodyContainment, ExperimentSolidBodyFullMembers}
			if tc.likelihood {
				aliases = append(aliases, ExperimentLikelihoodCost)
			}
			if tc.claim {
				aliases = append(aliases, ExperimentContainmentClaim)
			}
			want, err := trackerConfigFor(l5, "", aliases, nil)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.LoadTuningConfig(filepath.Join("..", "..", "..", "config", "experiments", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			l5 = cfg.L5.CvKfV1
			got, err := trackerConfigFor(l5, "", nil, nil)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("file/alias mismatch: %v\n%+v\n%+v", err, got, want)
			}
			live := l5tracks.TrackerConfigFromTuning(l5)
			if live.LikelihoodAssociationCost != tc.likelihood || live.ContainmentClaim != tc.claim || !live.SolidBody.Containment || !solidBodyFullMembers(l5, nil) {
				t.Fatal("live mapping or replay lost association input")
			}
			both, err := trackerConfigFor(l5, "", aliases, nil)
			if err != nil || !reflect.DeepEqual(both, got) {
				t.Fatal("matching aliases changed the file's settings", err)
			}
		})
	}
}

func TestAssociationClaimReplayRefusesMissingInputs(t *testing.T) {
	for _, body := range []*config.L5SolidBody{nil, {Enabled: true, FullMembers: true}, {Enabled: true, Containment: true}} {
		l5 := config.MustLoadDefaultConfig().L5.CvKfV1
		l5.Association = &config.L5Association{ContainmentClaim: true}
		l5.SolidBody = body
		if _, err := trackerConfigFor(l5, "", nil, nil); err == nil {
			t.Fatal("claim accepted missing containment or full membership")
		}
	}
}

func TestAssociationTuningCannotBypassNearEdgeRefusal(t *testing.T) {
	for _, fromFile := range []bool{false, true} {
		l5 := config.MustLoadDefaultConfig().L5.CvKfV1
		var aliases []string
		if fromFile {
			l5.Association = &config.L5Association{LikelihoodCost: true}
			aliases = []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack}
		} else {
			l5.SolidBody = &config.L5SolidBody{Enabled: true, FullMembers: true, NearEdgeTracking: true}
			aliases = []string{ExperimentLikelihoodCost}
		}
		if _, err := trackerConfigFor(l5, "", aliases, nil); err == nil {
			t.Fatal("mixed tuning/alias options bypassed the existing incompatibility")
		}
	}
}
