package replayeval

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayRejectsInvalidWindowsBeforeCreatingOutput(t *testing.T) {
	for _, tc := range []Config{{StartSeconds: -1}, {DurationSeconds: -.5}, {WarmupSeconds: -1}, {StartSeconds: 1, WarmupSeconds: 2}, {StartSeconds: math.NaN()}, {DurationSeconds: math.Inf(1)}, {StartSeconds: 1e30}, {StartSeconds: 5e9, WarmupSeconds: 5e9, DurationSeconds: 5e9}} {
		tc.PCAPFile = "unused"
		tc.OutDir = filepath.Join(t.TempDir(), "out")
		if _, err := Run(tc); err == nil {
			t.Fatal("invalid window accepted")
		}
		if _, err := os.Stat(tc.OutDir); !os.IsNotExist(err) {
			t.Fatal("invalid window created output")
		}
	}
}

func TestReplayNeverOverwritesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "keep")
	if err := os.WriteFile(p, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Config{PCAPFile: "unused", OutDir: dir}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "keep" {
		t.Fatal("existing output changed")
	}
	if _, err := Run(Config{PCAPFile: "unused", OutDir: p}); err == nil {
		t.Fatal("file output accepted")
	}
}

func TestPublisherWarmupDoesNotRecordOrCountEmptyScoredFrames(t *testing.T) {
	p := recordingPublisher{recordAfterNanos: 1000}
	p.Publish(&l9endpoints.FrameBundle{TimestampNanos: 999})
	if p.warmupFrames != 1 || p.recorded != 0 || p.emptyFrames != 0 {
		t.Fatal("warm-up leaked into scored metrics")
	}
}

func TestFileSHA256FailureAndIdentity(t *testing.T) {
	if _, err := fileSHA256(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing hash succeeded")
	}
	if _, err := fileSHA256(t.TempDir()); err == nil {
		t.Fatal("directory hash succeeded")
	}
	p := filepath.Join(t.TempDir(), "capture")
	if err := os.WriteFile(p, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := fileSHA256(p)
	if err != nil || h != "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal(h, err)
	}
}
