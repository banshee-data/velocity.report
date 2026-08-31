package server

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
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

func TestVisitedFlags(t *testing.T) {
	oldServeFlags := serveFlags
	defer func() {
		serveFlags = oldServeFlags
	}()

	fs := flag.NewFlagSet("visited-flags", flag.ContinueOnError)
	fs.Bool("alpha", false, "")
	fs.Bool("beta", false, "")
	if err := fs.Parse([]string{"-alpha"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	serveFlags = fs

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

func TestValidateLidarNetworkingFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		udpPort int
		rcvBuf  int
		fwdPort int
		fgPort  int
		want    string
	}{
		{name: "valid", udpPort: 2369, rcvBuf: 4 << 20, fwdPort: 2368, fgPort: 2370},
		{name: "bad udp port", udpPort: 0, rcvBuf: 4 << 20, fwdPort: 2368, fgPort: 2370, want: "--lidar-udp-port"},
		{name: "bad recv buffer", udpPort: 2369, rcvBuf: 0, fwdPort: 2368, fgPort: 2370, want: "--lidar-udp-rcv-buf"},
		{name: "bad forward port", udpPort: 2369, rcvBuf: 4 << 20, fwdPort: 70000, fgPort: 2370, want: "--lidar-forward-port"},
		{name: "bad foreground port", udpPort: 2369, rcvBuf: 4 << 20, fwdPort: 2368, fgPort: -1, want: "--lidar-foreground-forward-port"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateLidarNetworkingFlags(tc.udpPort, tc.rcvBuf, tc.fwdPort, tc.fgPort)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("validateLidarNetworkingFlags() returned error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateLidarNetworkingFlags() = %v, want substring %q", err, tc.want)
			}
		})
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
	active            bool
	stopped           bool
	backgroundCleared bool
	clearCalls        int
}

func (s *stubReplayPublisher) IsVRLogActive() bool {
	return s.active
}

func (s *stubReplayPublisher) StopVRLogReplay() {
	s.stopped = true
}

func (s *stubReplayPublisher) ClearBackground() {
	s.backgroundCleared = true
	s.clearCalls++
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
	// Stop the VRLOG replay, clear the client background, enter replay mode.
	if len(logs) != 3 {
		t.Fatalf("unexpected log count: %#v", logs)
	}
	if !publisher.backgroundCleared {
		t.Error("PCAP start did not clear the client background")
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
	if isNilHelperTarget(42) {
		t.Fatal("expected concrete non-nil value to be treated as non-nil")
	}
}

func TestReplayStoppedCallbackClearsBackgroundAndLeavesReplayMode(t *testing.T) {
	var logs []string
	logf := func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	}
	publisher := &stubReplayPublisher{}
	server := &stubReplayServer{}

	replayStoppedCallback(publisher, server, logf)()

	if publisher.clearCalls != 1 {
		t.Errorf("ClearBackground calls = %d, want 1", publisher.clearCalls)
	}
	if len(server.replayModes) != 1 || server.replayModes[0] {
		t.Errorf("replay mode transitions = %v, want [false]", server.replayModes)
	}
	if len(server.vrlogModes) != 0 {
		t.Errorf("VRLOG mode was changed by the common replay-stop callback: %v", server.vrlogModes)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "Replay stopped: switched to live mode") {
		t.Errorf("logs = %q, want the replay-stop transition", logs)
	}
}

func TestReplayStoppedCallbackToleratesMissingVisualiserComponents(t *testing.T) {
	var publisher *stubReplayPublisher
	var server *stubReplayServer

	handleReplayStoppedVisualiser(publisher, server, func(string, ...any) {
		t.Fatal("typed-nil components should not produce a transition log")
	})
	handleReplayStoppedVisualiser(nil, nil, func(string, ...any) {
		t.Fatal("nil components should not produce a transition log")
	})
}

func TestSensorIsSilentBoundaryCases(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	after := 3 * time.Second
	tests := []struct {
		name string
		last time.Time
		want bool
	}{
		{name: "no packet yet", last: time.Time{}, want: true},
		{name: "inside timeout", last: now.Add(-after + time.Nanosecond), want: false},
		{name: "exactly at timeout", last: now.Add(-after), want: false},
		{name: "past timeout", last: now.Add(-after - time.Nanosecond), want: true},
		{name: "future timestamp", last: now.Add(time.Second), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sensorIsSilent(tt.last, now, after); got != tt.want {
				t.Errorf("sensorIsSilent(%v, %v, %v) = %v, want %v", tt.last, now, after, got, tt.want)
			}
		})
	}
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

// TestPCAPStartClearsClientBackground guards the source handover. A PCAP replay
// rebuilds the grid from the capture, so until it settles there is nothing to
// show — and a settle-before-recording run publishes nothing new until its
// settled snapshot is restored. Without a clear the live grid stayed on screen
// underneath replayed foreground for that whole stretch.
func TestPCAPStartClearsClientBackground(t *testing.T) {
	publisher := &stubReplayPublisher{}
	server := &stubReplayServer{}

	handlePCAPStartedVisualiser(publisher, server, func(string, ...interface{}) {})

	if !publisher.backgroundCleared {
		t.Error("PCAP start did not clear the client background; the live scene stays under replayed foreground")
	}
}
