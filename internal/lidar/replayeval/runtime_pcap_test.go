//go:build pcap

package replayeval

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

func TestReplayInfrastructureFailuresAreReported(t *testing.T) {
	pcap := requireKirk0(t)
	failure := errors.New("injected infrastructure failure")
	for _, tc := range []struct {
		name   string
		change func(*replayRuntime)
	}{
		{"hash capture", func(d *replayRuntime) { d.hashFile = func(string) (string, error) { return "", failure } }},
		{"load parser config", func(d *replayRuntime) { d.loadParser = func() (*parse.Pandar40PConfig, error) { return nil, failure } }},
		{"background", func(d *replayRuntime) {
			d.background = func(string, *config.TuningConfig, []float64) (*l3grid.BackgroundManager, error) { return nil, failure }
		}},
		{"create recorder", func(d *replayRuntime) {
			d.newRecorder = func(string, string) (*recorder.Recorder, error) { return nil, failure }
		}},
		{"marshal tuning config", func(d *replayRuntime) { d.marshal = func(any) ([]byte, error) { return nil, failure } }},
		{"close recording", func(d *replayRuntime) {
			d.closeRecorder = func(r *recorder.Recorder) error { return errors.Join(r.Close(), failure) }
		}},
		{"marshal calibration", func(d *replayRuntime) {
			marshal := d.marshal
			d.marshal = func(v any) ([]byte, error) {
				if _, ok := v.(*parse.Pandar40PConfig); ok {
					return nil, failure
				}
				return marshal(v)
			}
		}},
		{"marshal replay manifest", func(d *replayRuntime) {
			d.marshalIndent = func(any, string, string) ([]byte, error) { return nil, failure }
		}},
		{"write replay manifest", func(d *replayRuntime) { d.writeFile = func(string, []byte, os.FileMode) error { return failure } }},
		{"write baseline", func(d *replayRuntime) {
			d.writeBaseline = func(string, l5tracks.TrackingMetrics) error { return failure }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := defaultRuntime()
			tc.change(&d)
			out := filepath.Join(t.TempDir(), "run")
			result, err := run(Config{PCAPFile: pcap, OutDir: out, UDPPort: 2369, DurationSeconds: 0.2}, d)
			if result != nil || !errors.Is(err, failure) {
				t.Fatalf("failure not propagated: result=%+v err=%v", result, err)
			}
			if tc.name == "marshal tuning config" {
				if _, err := os.Stat(filepath.Join(out, "header.json")); err != nil {
					t.Fatal("recorder not closed after provenance failure:", err)
				}
			}
		})
	}
}

func TestReplayRecordFailureIsLatched(t *testing.T) {
	d := defaultRuntime()
	d.newRecorder = func(path, sensor string) (*recorder.Recorder, error) {
		r, err := recorder.NewRecorder(path, sensor)
		if err != nil {
			return nil, err
		}
		if err := r.Close(); err != nil {
			return nil, err
		}
		return r, nil
	}
	result, err := run(Config{PCAPFile: requireKirk0(t), OutDir: filepath.Join(t.TempDir(), "run"), UDPPort: 2369, DurationSeconds: 0.2}, d)
	if result != nil || err == nil || !strings.Contains(err.Error(), "record frame") {
		t.Fatalf("closed recorder accepted: %+v %v", result, err)
	}
}

func TestPrepareBackgroundValidation(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	if _, err := prepareBackground("", cfg, nil); err == nil {
		t.Fatal("empty sensor accepted")
	}
	if _, err := prepareBackground("s", cfg, []float64{1}); err == nil {
		t.Fatal("wrong elevation count accepted")
	}
	cfg.L3.EmaBaselineV1.BackgroundUpdateFraction = 2
	if _, err := prepareBackground("s", cfg, nil); err == nil {
		t.Fatal("invalid background config accepted")
	}
}
