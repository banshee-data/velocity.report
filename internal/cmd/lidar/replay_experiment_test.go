//go:build pcap
// +build pcap

package lidar

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
	"github.com/google/gopacket/pcap"
)

// --experiment takes the harness's names and combination rules, and refuses
// what pcap-replay cannot supply.
func TestReplayExperimentsFlag(t *testing.T) {
	cases := []struct {
		value string
		want  []string
		err   string
	}{
		{"", []string{}, ""},
		{" , ", []string{}, ""},
		{"dwell_policy", []string{"dwell_policy"}, ""},
		{"region_overrides_cell_tercile, dwell_policy,dwell_policy", []string{"dwell_policy", "region_overrides_cell_tercile"}, ""},
		{"solid_body,solid_body_full_members,near_edge_track", []string{"near_edge_track", "solid_body", "solid_body_full_members"}, ""},
		{"dwel_policy", nil, "unknown experiment"},
		{"dwell_policy,dwell_policy_t40", nil, "settings of one option"},
		{"no_region_overrides,region_overrides_cell_tercile", nil, "would change nothing"},
		{"overlap_end_1,overlap_end_3", nil, "name one"},
		{"overlap_end_body", nil, "without it"},
		{"coast_support", nil, "coverage declaration"},
		{"solid_body", nil, "evidence database"},
	}
	for _, c := range cases {
		got, err := replayExperiments(c.value, config.DefaultConfigPath)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%q: error %v, want one containing %q", c.value, err, c.err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %v, %v; want %v", c.value, got, err, c.want)
		}
	}
}

// A refused selection is a usage error, reported before the capture is
// opened: the capture here does not exist, and the exit is still 2, not 1.
func TestReplayEvalRefusesExperimentsBeforeReading(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.pcap")
	for _, value := range []string{"nope", "dwell_policy,dwell_policy_t60", "occlusion_continuity", "solid_body"} {
		out := filepath.Join(t.TempDir(), "out")
		code := silence(t, func() int {
			return ReplayEvalMain([]string{"--pcap", absent, "--output", out, "--experiment", value})
		})
		if code != 2 {
			t.Errorf("--experiment %q exited %d, want 2", value, code)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Errorf("--experiment %q created the output directory before refusing", value)
		}
	}
}

// Without --experiment, or with an empty one, the replay is handed exactly
// the configuration this command built before the flag existed.
func TestReplayEvalWithoutExperimentIsUnchanged(t *testing.T) {
	src := truncatedCapture(t, 400)
	out := filepath.Join(t.TempDir(), "out")
	obs := filepath.Join(t.TempDir(), "obs")
	stop := errors.New("captured")
	var got replayeval.Config
	runReplay = func(cfg replayeval.Config) (*replayeval.Result, error) { got = cfg; return nil, stop }
	t.Cleanup(func() { runReplay = replayeval.Run })

	before := replayeval.Config{
		PCAPFile: src, OutDir: out, TuningFile: config.DefaultConfigPath, SensorID: "pcap-replay",
		UDPPort: 2369, DurationSeconds: 3, ProgressEvery: 200,
		ObservationLogDir: obs, ReplayCaseID: "kirk0", ObservationCalibration: replayCalibration("pcap-replay"),
	}
	common := []string{"--pcap", src, "--output", out, "--duration-seconds", "3", "--observations", obs, "--replay-case-id", "kirk0"}
	for _, extra := range [][]string{nil, {"--experiment", ""}, {"--experiment", " , "}} {
		got = replayeval.Config{}
		if code := silence(t, func() int { return ReplayEvalMain(append(append([]string{}, common...), extra...)) }); code != 1 {
			t.Fatalf("%v: exited %d, want the stub's 1", extra, code)
		}
		if !reflect.DeepEqual(got, before) {
			t.Fatalf("%v: configuration changed:\n got %+v\nwant %+v", extra, got, before)
		}
	}
}

// kirk0Capture is the whole in-repo reference capture, skipped when it is
// absent or only a Git LFS pointer.
func kirk0Capture(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "lidar", "perf", "pcap", "kirk0.pcapng"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := pcap.OpenOffline(src)
	if err != nil {
		t.Skipf("reference capture unavailable: %v", err)
	}
	h.Close()
	return src
}

// captureStdout runs fn with os.Stdout read into a buffer, and stderr
// silenced.
func captureStdout(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(done) }()
	oldOut, oldErr := os.Stdout, os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = w, devnull
	code := fn()
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = w.Close()
	<-done
	_ = devnull.Close()
	return code, buf.String()
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

// End to end on kirk0: without the flag the command's recording is the one
// replayeval.Run makes from the configuration the command always built, byte
// for byte wherever a replay is reproducible; under an experiment the
// recording changes and says which experiment made it, in the replay
// manifest, the observation container's manifest and the printed summary.
//
// Track IDs are random UUIDs, so the frame chunks and the filter record
// differ between any two runs, flag or none; the files compared byte for
// byte are the ones that carry no track ID, and header.json is compared
// without its creation time.
func TestReplayEvalExperimentOnKirk0(t *testing.T) {
	src := kirk0Capture(t)
	root := t.TempDir()
	// Every arm writes to the same path, then moves aside, so the manifests
	// name the same output.
	run := filepath.Join(root, "run")
	keep := func(name string) string {
		dst := filepath.Join(root, name)
		if err := os.Rename(run, dst); err != nil {
			t.Fatal(err)
		}
		return dst
	}

	if _, err := replayeval.Run(replayeval.Config{
		PCAPFile: src, OutDir: run, TuningFile: config.DefaultConfigPath, SensorID: "pcap-replay",
		UDPPort: 2369, DurationSeconds: 20,
	}); err != nil {
		t.Fatalf("direct replay: %v", err)
	}
	direct := keep("direct")

	common := []string{"--pcap", src, "--output", run, "--duration-seconds", "20", "--progress-frames", "0", "--analyse=false"}
	if code := silence(t, func() int { return ReplayEvalMain(common) }); code != 0 {
		t.Fatalf("flagless replay exited %d", code)
	}
	flagless := keep("flagless")

	for _, name := range []string{"replay_manifest.json", "tracking_baseline.json", "execution_config.json", "index.bin"} {
		a, errA := os.ReadFile(filepath.Join(direct, name))
		b, errB := os.ReadFile(filepath.Join(flagless, name))
		if errA != nil || errB != nil {
			t.Fatalf("%s: %v, %v", name, errA, errB)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs between the direct replay and the flagless command", name)
		}
	}
	headerA, headerB := readJSON(t, filepath.Join(direct, "header.json")), readJSON(t, filepath.Join(flagless, "header.json"))
	delete(headerA, "created_ns")
	delete(headerB, "created_ns")
	if !reflect.DeepEqual(headerA, headerB) {
		t.Errorf("header.json differs beyond its creation time:\n%v\n%v", headerA, headerB)
	}

	const experiment = replayeval.ExperimentRegionOverridesCellTercile
	obs := filepath.Join(root, "cell.vrlog")
	code, stdout := captureStdout(t, func() int {
		return ReplayEvalMain(append(append([]string{}, common...),
			"--experiment", experiment, "--observations", obs, "--replay-case-id", "kirk0"))
	})
	if code != 0 {
		t.Fatalf("experiment replay exited %d:\n%s", code, stdout)
	}
	cell := keep("cell")
	if !strings.Contains(stdout, "experiments: "+experiment+"\n") {
		t.Errorf("the summary does not name the experiment:\n%s", stdout)
	}

	manifest := readJSON(t, filepath.Join(cell, "replay_manifest.json"))
	if got, _ := json.Marshal(manifest["experiments"]); string(got) != `["`+experiment+`"]` {
		t.Errorf("replay_manifest.json experiments = %s", got)
	}
	if got, _ := json.Marshal(readJSON(t, filepath.Join(flagless, "replay_manifest.json"))["experiments"]); string(got) != `[]` {
		t.Errorf("the flagless manifest's experiments = %s, want []", got)
	}
	if readJSON(t, filepath.Join(cell, "header.json"))["params_hash"] == headerB["params_hash"] {
		t.Error("the experiment did not move the params hash")
	}
	// The experiment changed the recording, not only its name: the tracker
	// saw different foreground.
	baseline, _ := os.ReadFile(filepath.Join(flagless, "tracking_baseline.json"))
	changed, _ := os.ReadFile(filepath.Join(cell, "tracking_baseline.json"))
	if bytes.Equal(baseline, changed) {
		t.Error("tracking_baseline.json is the default replay's: the experiment changed nothing")
	}

	r, err := vrlog.Open(obs, vrlog.Options{})
	if err != nil {
		t.Fatalf("open observation container: %v", err)
	}
	defer r.Close()
	if got := r.Manifest().Provenance.Experiments; !reflect.DeepEqual(got, []string{experiment}) {
		t.Errorf("observation provenance experiments = %v", got)
	}
	if _, err := r.Verify(); err != nil {
		t.Errorf("observation container does not verify: %v", err)
	}
	if _, inspect := captureStdout(t, func() int { return ObservationsMain([]string{"inspect", "--records", "0", obs}) }); !strings.Contains(inspect, "experiments  "+experiment) {
		t.Errorf("observations inspect does not name the experiment:\n%s", inspect)
	}
}
