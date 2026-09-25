package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

func TestFlagValidation(t *testing.T) {
	for name, args := range map[string][]string{
		"no capture":            {"-out", "x"},
		"no output":             {"-pcap", "x"},
		"database without case": {"-pcap", "x", "-out", "y", "-evidence-db", "z"},
		"unknown experiment":    {"-pcap", "x", "-out", "y", "-experiment", "nonsense"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%s: exit %d, want 2 (%s)", name, code, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-h"}, &stdout, &stderr); code != 0 {
		t.Fatalf("-h exited %d", code)
	}
}

// Every refined arm gets one evaluator command, paired with the online arm on
// the same database, naming all five version fields and declaring both arms
// baselines.
func TestPerFrameCommandsNameEveryVersionField(t *testing.T) {
	report := &replayeval.RefinementReport{
		SourceID: "source/v1/abc", ObservationModelID: "medoid_v0",
		Arms: []l8analytics.RefinementArmMetrics{
			{Label: "online", Stage: "online", EstimatorID: "cv_kf_v1", ParamHash: "sha256:online"},
			{Label: "fixed_lag 0.5s", Stage: "fixed_lag", Lag: "0.5s", EstimatorID: "cv_kf_v1+rts", ParamHash: "sha256:half"},
			{Label: "final track", Stage: "final", Lag: "track", EstimatorID: "cv_kf_v1+rts", ParamHash: "sha256:track"},
		},
	}
	commands := perFrameCommands("/data/evidence.db", report)
	if len(commands) != 2 {
		t.Fatalf("%d commands, want one per refined arm", len(commands))
	}
	half := commands[0]
	for _, want := range []string{
		"-a-db /data/evidence.db", "-a-source source/v1/abc", "-a-estimator cv_kf_v1 ", "-a-param-hash sha256:online",
		"-a-stage online", "-a-declared-baseline", `-b-label "fixed_lag 0.5s"`, "-b-param-hash sha256:half",
		"-b-stage fixed_lag", "-b-observation-model medoid_v0", "-b-declared-baseline", "refinement-perframe-0p5s.json",
	} {
		if !strings.Contains(half, want) {
			t.Errorf("command lacks %q:\n%s", want, half)
		}
	}
	if !strings.Contains(commands[1], "-b-stage final") {
		t.Errorf("final arm command:\n%s", commands[1])
	}
}
