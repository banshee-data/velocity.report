//go:build pcap

package replayeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// kirk0MovingWindow scores capture seconds 70-76 after a six-second warm-up.
// It carries vehicles above 5 m/s, so the prediction interval matters to what
// the tracker concludes, and it is short enough to replay three times.
func kirk0MovingWindow(t *testing.T, outDir string) Config {
	return Config{
		PCAPFile: requireKirk0(t), OutDir: outDir, SensorID: "test-replay", UDPPort: 2369,
		StartSeconds: 70, WarmupSeconds: 6, DurationSeconds: 6,
	}
}

// kirk0GapWindow is the established kirk0 window, 20 s of warm-up from the
// start of the capture so the background is settled, scored over 20-28 s.
// With that background, kirk0 has several runs of frames with no clusters
// between 21 and 27 s, the longest about 2 s. The pipeline does not call the
// tracker on such a frame, so the tracker sees each run as a single
// capture-time gap beyond max_predict_dt: the case the clamp, the gap option
// and capture-time coast age exist for. Warmed from a later start the same
// seconds carry noise clusters and no gap, which is why this window starts at
// zero.
func kirk0GapWindow(t *testing.T, outDir string) Config {
	return Config{
		PCAPFile: requireKirk0(t), OutDir: outDir, SensorID: "test-replay", UDPPort: 2369,
		StartSeconds: 20, WarmupSeconds: 20, DurationSeconds: 8,
	}
}

// trackFingerprint reduces a recording to what the tracker decided, frame by
// frame, with track identity canonicalised. Track IDs are random UUIDs, so
// they are dropped; each track's lifecycle, timestamps, state, covariance,
// geometry, class and trail remain, which is enough to expose any change in
// what the estimator concluded. Entries within a frame are sorted, because
// the adapter publishes tracks in map order.
func trackFingerprint(t *testing.T, dir string) (frames []string, trackFrames int) {
	t.Helper()
	reader, err := recorder.NewReplayer(dir)
	if err != nil {
		t.Fatalf("open recording %s: %v", dir, err)
	}
	defer reader.Close()
	for {
		frame, err := reader.ReadFrame()
		if err == io.EOF {
			return frames, trackFrames
		}
		if err != nil {
			t.Fatalf("read recording %s: %v", dir, err)
		}
		var entries []string
		if frame.Tracks != nil {
			for _, track := range frame.Tracks.Tracks {
				track.TrackID = ""
				entries = append(entries, fmt.Sprintf("%+v", track))
				trackFrames++
			}
			for _, trail := range frame.Tracks.Trails {
				entries = append(entries, fmt.Sprintf("trail%+v", trail.Points))
			}
		}
		sort.Strings(entries)
		frames = append(frames, fmt.Sprintf("%d@%d[%d] %s", frame.FrameID, frame.TimestampNanos, frame.FrameType,
			strings.Join(entries, " ; ")))
	}
}

func readBaseline(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// requireSameDecisions fails with the first frame at which two recordings
// disagree, which is far more useful than a digest mismatch.
func requireSameDecisions(t *testing.T, name string, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: %d recorded frames, want %d", name, len(got), len(want))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("%s: frame %d differs:\nwant %s\n got %s", name, i, want[i], got[i])
		}
	}
}

// Q3 and the capture-gap option on kirk0. This is the in-repo smoke run of
// the harness a local agent points at the S2 corpus: each arm is the default
// replay with one experiment switched on. It asserts the wiring end to end
// (the option changes the estimate, is named in the manifest and moves the
// parameter hash) and logs the residual bands, so the size of the effect on
// this capture is on record. It deliberately asserts no direction: one
// capture is not evidence, and Q3's answer comes from the corpus.
func TestTimeDomainExperimentsOnKirk0(t *testing.T) {
	dir := t.TempDir()
	type arm struct {
		name        string
		experiments []string
	}
	arms := []arm{
		{"default", nil},
		{"measurement_time", []string{ExperimentMeasurementTime}},
		{"capture_gap_predict", []string{ExperimentCaptureGapPredict}},
	}
	baselines := map[string][]byte{}
	fingerprints := map[string]string{}
	hashes := map[string]string{}
	for _, a := range arms {
		cfg := kirk0GapWindow(t, filepath.Join(dir, a.name))
		cfg.Experiments = a.experiments
		res, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		baselines[a.name] = readBaseline(t, cfg.OutDir)
		frames, _ := trackFingerprint(t, cfg.OutDir)
		fingerprints[a.name] = strings.Join(frames, "\n")

		var manifest struct {
			Params      string          `json:"params_sha256"`
			Experiments []string        `json:"experiments"`
			TimeDomain  json.RawMessage `json:"time_domain"`
		}
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, "replay_manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &manifest); err != nil {
			t.Fatal(err)
		}
		if strings.Join(manifest.Experiments, ",") != strings.Join(a.experiments, ",") {
			t.Fatalf("%s: manifest records experiments %v", a.name, manifest.Experiments)
		}
		if len(manifest.TimeDomain) == 0 {
			t.Fatalf("%s: manifest has no time_domain block", a.name)
		}
		hashes[a.name] = manifest.Params
		if a.name == "default" && res.TimeDomain.ClampedGaps == 0 {
			t.Fatalf("the gap window produced no capture-time gap beyond max_predict_dt: %+v", res.TimeDomain)
		}
		t.Logf("%s: time domain %+v", a.name, res.TimeDomain)
		t.Logf("%s: tracking baseline\n%s", a.name, baselines[a.name])
	}
	for _, a := range arms[1:] {
		if hashes[a.name] == hashes["default"] {
			t.Errorf("%s: parameter hash equals the default's; the arm is not identifiable", a.name)
		}
		if bytes.Equal(baselines[a.name], baselines["default"]) && fingerprints[a.name] == fingerprints["default"] {
			t.Errorf("%s: the recording is identical to the default; the option did not reach the tracker", a.name)
		}
	}
}
