package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
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

// writeCapture writes a stand-in capture file and returns its path and
// SHA-256. It is not a PCAP, so a replay that gets past the split ends in
// the replay's own failure.
func writeCapture(t *testing.T, dir, name, content string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return path, "sha256:" + hex.EncodeToString(sum[:])
}

// A frozen split holds the replayed case to its role before anything replays:
// the use is refused with exit 2, or announced and recorded. The role binds
// to the case's captures, so -case cannot relabel another case's capture.
func TestFrozenSplitHoldsTheCaseToItsRole(t *testing.T) {
	dir := t.TempDir()
	kirk0, kirk0SHA := writeCapture(t, dir, "kirk0.pcapng", "kirk0 capture")
	held, heldSHA := writeCapture(t, dir, "held.pcapng", "held-out capture")
	f, err := annotation.FreezeSplit(annotation.FreezeOptions{
		Draft: &annotation.SplitDraft{Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
			Cases: []annotation.SplitCase{
				{CaseID: "kirk0", Role: annotation.SplitRoleTuning, Captures: []annotation.CaseCapture{{Basename: "kirk0.pcapng", SHA256: kirk0SHA}}},
				{CaseID: "held", Role: annotation.SplitRoleHeldOut, Captures: []annotation.CaseCapture{{Basename: "held.pcapng", SHA256: heldSHA}}},
			}},
		Author: "operator", Now: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC), BuildVersion: "test", BuildGitSHA: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	splitPath := filepath.Join(dir, "split.json")
	if err := annotation.WriteFrozenSplit(splitPath, f); err != nil {
		t.Fatal(err)
	}
	base := []string{"-pcap", kirk0, "-out", filepath.Join(dir, "out"), "-evidence-db", filepath.Join(dir, "e.db")}
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"split without a case": {[]string{"-pcap", "x", "-out", "y", "-split-manifest", splitPath}, "go together"},
		"held out, no split":   {append(base, "-case", "kirk0", "-held-out"), "go together"},
		"missing split":        {append(base, "-case", "kirk0", "-split-manifest", filepath.Join(dir, "none.json")), "open split manifest"},
		"held-out case":        {append(base, "-case", "held", "-split-manifest", splitPath), "replay it only as a held-out score"},
		"tuning case held out": {append(base, "-case", "kirk0", "-split-manifest", splitPath, "-held-out"), "held-out score was requested"},
		// The review's scenario: the held-out capture replayed under the
		// tuning case's name.
		"a held-out capture as a tuning case": {[]string{"-pcap", held, "-out", filepath.Join(dir, "out"), "-evidence-db", filepath.Join(dir, "e.db"),
			"-case", "kirk0", "-split-manifest", splitPath}, `is not one of case "kirk0"'s captures`},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(tc.args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%s: exit %d, want 2 mentioning %q (%s)", name, code, tc.want, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	args := append(append([]string(nil), base...), "-case", "kirk0", "-split-manifest", splitPath)
	if code := run(args, &stdout, &stderr); code != 1 {
		t.Fatalf("accepted use: exit %d, want the replay's failure (%s)", code, stderr.String())
	}
	if want := "case kirk0 is tuning in frozen split " + f.SplitDigest + " (revision 1)"; !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout %q lacks %q", stdout.String(), want)
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
