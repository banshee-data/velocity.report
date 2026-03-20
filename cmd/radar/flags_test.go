package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
)

// TestBgFlushEnableCondition verifies the logic that determines whether
// background flushing should be enabled. This mirrors the condition in radar.go:
//
//	backgroundManager != nil && flushInterval > 0 && flushEnable
func TestBgFlushEnableCondition(t *testing.T) {
	tests := []struct {
		name          string
		hasManager    bool
		flushInterval time.Duration
		flushEnable   bool
		wantEnabled   bool
	}{
		{
			name:          "default settings - flushing disabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushEnable:   false,
			wantEnabled:   false,
		},
		{
			name:          "enable flag set - flushing enabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushEnable:   true,
			wantEnabled:   true,
		},
		{
			name:          "zero interval - flushing disabled",
			hasManager:    true,
			flushInterval: 0,
			flushEnable:   true,
			wantEnabled:   false,
		},
		{
			name:          "no manager - flushing disabled",
			hasManager:    false,
			flushInterval: 60 * time.Second,
			flushEnable:   true,
			wantEnabled:   false,
		},
		{
			name:          "all disabled conditions",
			hasManager:    false,
			flushInterval: 0,
			flushEnable:   false,
			wantEnabled:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the condition from radar.go
			enabled := tc.hasManager && tc.flushInterval > 0 && tc.flushEnable

			if enabled != tc.wantEnabled {
				t.Errorf("bgFlushEnabled = %v, want %v", enabled, tc.wantEnabled)
			}
		})
	}
}

func createTestTeXRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("failed to create bin directory: %v", err)
	}

	compilerPath := filepath.Join(binDir, "xelatex")
	if err := os.WriteFile(compilerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to create xelatex stub: %v", err)
	}

	return root
}

func TestResolvePrecompiledTeXRoot(t *testing.T) {
	texRoot := createTestTeXRoot(t)

	got, err := resolvePrecompiledTeXRoot(texRoot)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want, err := filepath.Abs(texRoot)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if got != want {
		t.Fatalf("resolved root mismatch: got %q want %q", got, want)
	}
}

func TestResolvePrecompiledTeXRootMissingCompiler(t *testing.T) {
	texRoot := t.TempDir()

	_, err := resolvePrecompiledTeXRoot(texRoot)
	if err == nil {
		t.Fatal("expected error for TeX root without bin/xelatex")
	}
}

func TestConfigurePDFLaTeXFlowPrecompiled(t *testing.T) {
	t.Setenv("VELOCITY_TEX_ROOT", "")
	texRoot := createTestTeXRoot(t)

	if err := configurePDFLaTeXFlow("precompiled", texRoot); err != nil {
		t.Fatalf("configurePDFLaTeXFlow returned error: %v", err)
	}

	want, err := filepath.Abs(texRoot)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if got := os.Getenv("VELOCITY_TEX_ROOT"); got != want {
		t.Fatalf("VELOCITY_TEX_ROOT mismatch: got %q want %q", got, want)
	}
}

func TestConfigurePDFLaTeXFlowFull(t *testing.T) {
	t.Setenv("VELOCITY_TEX_ROOT", "/tmp/should-be-cleared")

	if err := configurePDFLaTeXFlow("full", ""); err != nil {
		t.Fatalf("configurePDFLaTeXFlow returned error: %v", err)
	}

	if got := os.Getenv("VELOCITY_TEX_ROOT"); got != "" {
		t.Fatalf("expected VELOCITY_TEX_ROOT to be unset, got %q", got)
	}
}

func TestConfigurePDFLaTeXFlowInvalid(t *testing.T) {
	if err := configurePDFLaTeXFlow("invalid-flow", ""); err == nil {
		t.Fatal("expected error for invalid --pdf-latex-flow")
	}
}

func TestVisitedFlags(t *testing.T) {
	oldCommandLine := flag.CommandLine
	defer func() {
		flag.CommandLine = oldCommandLine
	}()

	fs := flag.NewFlagSet("visited-flags", flag.ContinueOnError)
	fs.Bool("alpha", false, "")
	fs.Bool("beta", false, "")
	if err := fs.Parse([]string{"-alpha"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	flag.CommandLine = fs

	got := visitedFlags()
	if !got["alpha"] {
		t.Fatalf("visitedFlags did not mark alpha as visited: %#v", got)
	}
	if got["beta"] {
		t.Fatalf("visitedFlags unexpectedly marked beta as visited: %#v", got)
	}
}

func TestValidateSupportedTuning(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	if err := validateSupportedTuning(cfg); err != nil {
		t.Fatalf("validateSupportedTuning(default) returned error: %v", err)
	}

	cfg = config.MustLoadDefaultConfig()
	cfg.L3.Engine = "other"
	if err := validateSupportedTuning(cfg); err == nil || !strings.Contains(err.Error(), "unsupported l3.engine") {
		t.Fatalf("expected l3 error, got %v", err)
	}

	cfg = config.MustLoadDefaultConfig()
	cfg.L4.Engine = "other"
	if err := validateSupportedTuning(cfg); err == nil || !strings.Contains(err.Error(), "unsupported l4.engine") {
		t.Fatalf("expected l4 error, got %v", err)
	}

	cfg = config.MustLoadDefaultConfig()
	cfg.L5.Engine = "other"
	if err := validateSupportedTuning(cfg); err == nil || !strings.Contains(err.Error(), "unsupported l5.engine") {
		t.Fatalf("expected l5 error, got %v", err)
	}
}

func TestEnsureSupportedTuning(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	cfg.L3.Engine = "other"
	var got string
	ensureSupportedTuning(cfg, func(format string, args ...any) {
		got = fmt.Sprintf(format, args...)
	})
	if !strings.Contains(got, "unsupported l3.engine") {
		t.Fatalf("unexpected fatal message: %q", got)
	}
}

func TestDeprecatedLidarFlagWarnings(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	warnings := deprecatedLidarFlagWarnings(map[string]bool{
		"lidar-sensor":                  true,
		"lidar-udp-port":                true,
		"lidar-forward-port":            true,
		"lidar-foreground-forward-port": true,
	}, cfg, "config/tuning.defaults.json")

	if len(warnings) != 4 {
		t.Fatalf("expected 4 warnings, got %d: %#v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "--lidar-sensor") || !strings.Contains(warnings[3], "--lidar-foreground-forward-port") {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	if got := deprecatedLidarFlagWarnings(map[string]bool{}, cfg, "config/tuning.defaults.json"); len(got) != 0 {
		t.Fatalf("expected no warnings, got %#v", got)
	}
}

type stubRingElevationsSetter struct {
	err      error
	lastElev []float64
}

func (s *stubRingElevationsSetter) SetRingElevations(elev []float64) error {
	s.lastElev = append([]float64(nil), elev...)
	return s.err
}

func TestRingElevationLogMessage(t *testing.T) {
	cfg := &parse.Pandar40PConfig{}
	cfg.AngleCorrections[0].Elevation = 1.25

	setter := &stubRingElevationsSetter{}
	if msg := ringElevationLogMessage(setter, "sensor-a", cfg); msg != "BackgroundManager ring elevations set for sensor sensor-a" {
		t.Fatalf("unexpected success message: %q", msg)
	}
	if len(setter.lastElev) != len(cfg.AngleCorrections) || setter.lastElev[0] != 1.25 {
		t.Fatalf("unexpected elevations: %#v", setter.lastElev)
	}

	setter.err = errors.New("boom")
	if msg := ringElevationLogMessage(setter, "sensor-b", cfg); !strings.Contains(msg, "Failed to set ring elevations for background manager sensor-b: boom") {
		t.Fatalf("unexpected error message: %q", msg)
	}
}

func TestTuningHashOrWarn(t *testing.T) {
	cfg := config.MustLoadDefaultConfig()
	originalMarshal := marshalTuningJSON
	t.Cleanup(func() {
		marshalTuningJSON = originalMarshal
	})

	hash := tuningHashOrWarn(cfg, func(string, ...any) {})
	if hash == "" {
		t.Fatal("expected non-empty tuning hash")
	}

	var warned string
	marshalTuningJSON = func(any) ([]byte, error) {
		return nil, errors.New("nope")
	}
	hash = tuningHashOrWarn(cfg, func(format string, args ...any) {
		warned = fmt.Sprintf(format, args...)
	})
	if hash != "" || !strings.Contains(warned, "unable to compute tuning config provenance hash") {
		t.Fatalf("unexpected warning result: hash=%q warned=%q", hash, warned)
	}
}

func TestMustLoadValidatedPandarConfig(t *testing.T) {
	var fatal string
	fatalf := func(format string, args ...any) {
		fatal = fmt.Sprintf(format, args...)
	}

	cfg := &parse.Pandar40PConfig{}
	got := mustLoadValidatedPandarConfig(
		func() (*parse.Pandar40PConfig, error) { return cfg, nil },
		func(*parse.Pandar40PConfig) error { return nil },
		fatalf,
	)
	if got != cfg || fatal != "" {
		t.Fatalf("unexpected success result: got=%v fatal=%q", got, fatal)
	}

	fatal = ""
	got = mustLoadValidatedPandarConfig(
		func() (*parse.Pandar40PConfig, error) { return nil, errors.New("load boom") },
		func(*parse.Pandar40PConfig) error { return nil },
		fatalf,
	)
	if got != nil || !strings.Contains(fatal, "Failed to load embedded lidar configuration: load boom") {
		t.Fatalf("unexpected load failure result: got=%v fatal=%q", got, fatal)
	}

	fatal = ""
	got = mustLoadValidatedPandarConfig(
		func() (*parse.Pandar40PConfig, error) { return cfg, nil },
		func(*parse.Pandar40PConfig) error { return errors.New("invalid") },
		fatalf,
	)
	if got != nil || !strings.Contains(fatal, "Invalid embedded lidar configuration: invalid") {
		t.Fatalf("unexpected validate failure result: got=%v fatal=%q", got, fatal)
	}
}

func TestEnsureValidForwardMode(t *testing.T) {
	ensureValidForwardMode("grpc", func(string, ...any) {})

	var fatal string
	ensureValidForwardMode("bad", func(format string, args ...any) {
		fatal = fmt.Sprintf(format, args...)
	})
	if !strings.Contains(fatal, "Invalid --lidar-forward-mode: bad") {
		t.Fatalf("unexpected fatal message: %q", fatal)
	}
}

type stubReplayPublisher struct {
	active  bool
	stopped bool
}

func (s *stubReplayPublisher) IsVRLogActive() bool {
	return s.active
}

func (s *stubReplayPublisher) StopVRLogReplay() {
	s.stopped = true
}

type stubReplayServer struct {
	vrlogModes  []bool
	replayModes []bool
	progress    [][2]uint64
	timestamps  [][2]int64
}

func (s *stubReplayServer) SetVRLogMode(v bool) {
	s.vrlogModes = append(s.vrlogModes, v)
}

func (s *stubReplayServer) SetReplayMode(v bool) {
	s.replayModes = append(s.replayModes, v)
}

func (s *stubReplayServer) SetPCAPProgress(current, total uint64) {
	s.progress = append(s.progress, [2]uint64{current, total})
}

func (s *stubReplayServer) SetPCAPTimestamps(startNs, endNs int64) {
	s.timestamps = append(s.timestamps, [2]int64{startNs, endNs})
}

func TestHandlePCAPStartedVisualiserAndPublishProgress(t *testing.T) {
	var logs []string
	logf := func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	publisher := &stubReplayPublisher{active: true}
	server := &stubReplayServer{}
	handlePCAPStartedVisualiser(publisher, server, logf)
	if !publisher.stopped {
		t.Fatal("expected active VRLOG replay to be stopped")
	}
	if len(server.vrlogModes) != 1 || len(server.replayModes) != 2 || server.replayModes[0] || !server.replayModes[1] {
		t.Fatalf("unexpected replay mode transitions: %+v %+v", server.vrlogModes, server.replayModes)
	}
	if len(logs) != 2 {
		t.Fatalf("unexpected log count: %#v", logs)
	}

	publishPCAPProgress(server, 10, 20)
	if len(server.progress) != 1 || server.progress[0] != [2]uint64{10, 20} {
		t.Fatalf("unexpected progress updates: %#v", server.progress)
	}
	pcapProgressCallback(server)(30, 40)
	if len(server.progress) != 2 || server.progress[1] != [2]uint64{30, 40} {
		t.Fatalf("unexpected callback progress updates: %#v", server.progress)
	}
	pcapStartedCallback(publisher, server, logf)()
	pcapTimestampsCallback(server)(50, 60)
	if len(server.timestamps) != 1 || server.timestamps[0] != [2]int64{50, 60} {
		t.Fatalf("unexpected timestamps: %#v", server.timestamps)
	}

	handlePCAPStartedVisualiser(nil, nil, logf)
	publishPCAPProgress(nil, 1, 2)
}

func TestHandlePCAPStartedVisualiserAndCallbacks_TypedNil(t *testing.T) {
	var publisher *stubReplayPublisher
	var server *stubReplayServer

	handlePCAPStartedVisualiser(publisher, server, func(string, ...any) {})
	publishPCAPProgress(server, 1, 2)
	pcapTimestampsCallback(server)(3, 4)
}

func TestNewVRLogRecorderOrLog(t *testing.T) {
	recordPath := t.TempDir()
	var logs []string
	logf := func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}

	rec := newVRLogRecorderOrLog(recorder.NewRecorder, recordPath, "sensor-a", logf)
	if rec == nil {
		t.Fatal("expected recorder to be created")
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rec = newVRLogRecorderOrLog(
		func(string, string) (*recorder.Recorder, error) { return nil, errors.New("recorder boom") },
		recordPath,
		"sensor-a",
		logf,
	)
	if rec != nil || len(logs) == 0 || !strings.Contains(logs[len(logs)-1], "recorder boom") {
		t.Fatalf("unexpected recorder failure result: rec=%v logs=%#v", rec, logs)
	}
}
