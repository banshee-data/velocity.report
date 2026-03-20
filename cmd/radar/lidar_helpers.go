package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
)

type logfFunc func(string, ...any)

type ringElevationsSetter interface {
	SetRingElevations([]float64) error
}

type vrlogReplayController interface {
	IsVRLogActive() bool
	StopVRLogReplay()
}

type replayModeController interface {
	SetVRLogMode(bool)
	SetReplayMode(bool)
}

type pcapProgressSetter interface {
	SetPCAPProgress(uint64, uint64)
}

type pcapTimestampsSetter interface {
	SetPCAPTimestamps(int64, int64)
}

var marshalTuningJSON = json.Marshal

func isNilHelperTarget(target any) bool {
	if target == nil {
		return true
	}
	value := reflect.ValueOf(target)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validateSupportedTuning(tuningCfg *config.TuningConfig) error {
	if tuningCfg.L3.Engine != "ema_baseline_v1" {
		return fmt.Errorf("unsupported l3.engine %q: this branch only implements ema_baseline_v1 at runtime", tuningCfg.L3.Engine)
	}
	if tuningCfg.L4.Engine != "dbscan_xy_v1" {
		return fmt.Errorf("unsupported l4.engine %q: this branch only implements dbscan_xy_v1 at runtime", tuningCfg.L4.Engine)
	}
	if tuningCfg.L5.Engine != "cv_kf_v1" {
		return fmt.Errorf("unsupported l5.engine %q: this branch only implements cv_kf_v1 at runtime", tuningCfg.L5.Engine)
	}
	return nil
}

func ensureSupportedTuning(tuningCfg *config.TuningConfig, fatalf logfFunc) {
	if err := validateSupportedTuning(tuningCfg); err != nil {
		fatalf("%v", err)
	}
}

func deprecatedLidarFlagWarnings(explicitFlags map[string]bool, tuningCfg *config.TuningConfig, configPath string) []string {
	var warnings []string
	if explicitFlags["lidar-sensor"] {
		warnings = append(warnings, fmt.Sprintf("Warning: --lidar-sensor is deprecated; using l1.sensor=%q from %s", tuningCfg.GetSensor(), configPath))
	}
	if explicitFlags["lidar-udp-port"] {
		warnings = append(warnings, fmt.Sprintf("Warning: --lidar-udp-port is deprecated; using l1.udp_port=%d from %s", tuningCfg.GetUDPPort(), configPath))
	}
	if explicitFlags["lidar-forward-port"] {
		warnings = append(warnings, fmt.Sprintf("Warning: --lidar-forward-port is deprecated; using l1.forward_port=%d from %s", tuningCfg.GetForwardPort(), configPath))
	}
	if explicitFlags["lidar-foreground-forward-port"] {
		warnings = append(warnings, fmt.Sprintf("Warning: --lidar-foreground-forward-port is deprecated; using l1.foreground_forward_port=%d from %s", tuningCfg.GetForegroundForwardPort(), configPath))
	}
	return warnings
}

func tuningHashOrWarn(tuningCfg *config.TuningConfig, warnf logfFunc) string {
	tuningJSON, err := marshalTuningJSON(tuningCfg)
	if err != nil {
		warnf("Warning: unable to compute tuning config provenance hash: %v", err)
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(tuningJSON))
}

func mustLoadValidatedPandarConfig(
	load func() (*parse.Pandar40PConfig, error),
	validate func(*parse.Pandar40PConfig) error,
	fatalf logfFunc,
) *parse.Pandar40PConfig {
	cfg, err := load()
	if err != nil {
		fatalf("Failed to load embedded lidar configuration: %v", err)
		return nil
	}
	if err := validate(cfg); err != nil {
		fatalf("Invalid embedded lidar configuration: %v", err)
		return nil
	}
	return cfg
}

func ringElevationLogMessage(backgroundManager ringElevationsSetter, sensorID string, cfg *parse.Pandar40PConfig) string {
	if err := backgroundManager.SetRingElevations(parse.ElevationsFromConfig(cfg)); err != nil {
		return fmt.Sprintf("Failed to set ring elevations for background manager %s: %v", sensorID, err)
	}
	return fmt.Sprintf("BackgroundManager ring elevations set for sensor %s", sensorID)
}

func ensureValidForwardMode(mode string, fatalf logfFunc) {
	validModes := map[string]bool{"lidarview": true, "grpc": true, "both": true}
	if !validModes[mode] {
		fatalf("Invalid --lidar-forward-mode: %s (must be: lidarview, grpc, or both)", mode)
	}
}

func handlePCAPStartedVisualiser(publisher vrlogReplayController, server replayModeController, logf logfFunc) {
	if !isNilHelperTarget(publisher) && publisher.IsVRLogActive() {
		publisher.StopVRLogReplay()
		logf("[Visualiser] Stopped VRLOG replay before PCAP start")
	}
	if !isNilHelperTarget(server) {
		server.SetVRLogMode(false)
		server.SetReplayMode(false)
		server.SetReplayMode(true)
		logf("[Visualiser] PCAP started - switched to replay mode")
	}
}

func publishPCAPProgress(server pcapProgressSetter, current, total uint64) {
	if !isNilHelperTarget(server) {
		server.SetPCAPProgress(current, total)
	}
}

func pcapProgressCallback(server pcapProgressSetter) func(uint64, uint64) {
	return func(current, total uint64) {
		publishPCAPProgress(server, current, total)
	}
}

func pcapStartedCallback(publisher vrlogReplayController, server replayModeController, logf logfFunc) func() {
	return func() {
		handlePCAPStartedVisualiser(publisher, server, logf)
	}
}

func pcapTimestampsCallback(server pcapTimestampsSetter) func(int64, int64) {
	return func(startNs, endNs int64) {
		if !isNilHelperTarget(server) {
			server.SetPCAPTimestamps(startNs, endNs)
		}
	}
}

func newVRLogRecorderOrLog(
	newRecorder func(string, string) (*recorder.Recorder, error),
	recordPath string,
	sensorID string,
	logf logfFunc,
) *recorder.Recorder {
	rec, err := newRecorder(recordPath, sensorID)
	if err != nil {
		logf("[Visualiser] VRLOG recording failed: %v", err)
		return nil
	}
	return rec
}
