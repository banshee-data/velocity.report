package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/server"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

type logfFunc func(string, ...any)

// resolveLidarDir makes a configured LiDAR directory absolute.
//
// The safe-directory check compares cleaned absolute paths, so a relative
// configuration value has to be resolved before it can act as a boundary:
// left relative it would be re-resolved against whatever working directory
// the process happened to have, and a path that escapes the boundary would
// then compare as if it were inside it.
//
// A failure to resolve is logged and the configured value returned unchanged.
// That is deliberately not fatal: filepath.Abs only fails when the working
// directory cannot be read, and refusing to start the whole server over a
// plots directory would be the wrong trade. The caller still gets a usable,
// if relative, path. `kind` names the directory in that warning.
func resolveLidarDir(dir, kind string, logf logfFunc) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		logf("Warning: failed to resolve %s directory %q: %v", kind, dir, err)
		return dir
	}
	return abs
}

// resolveTuningConfigDir is the directory the replay form lists configs
// from: the flag when given, and otherwise the directory the --config file
// came from, so the shipped tree's config/ is the default.
func resolveTuningConfigDir(flagDir, configFile string) string {
	if flagDir != "" {
		return flagDir
	}
	return filepath.Dir(configFile)
}

// tuningSolidBodyFullMembers is the tuning config's solid_body.full_members:
// whether the pipeline hands the tracker every cluster member from the start.
func tuningSolidBodyFullMembers(cfg *config.TuningConfig) bool {
	if cfg == nil || cfg.L5.CvKfV1 == nil || cfg.L5.CvKfV1.SolidBody == nil {
		return false
	}
	return cfg.L5.CvKfV1.SolidBody.Enabled && cfg.L5.CvKfV1.SolidBody.FullMembers
}

// tuningBoxModelMembers is whether the tuning config's box model fits the
// rectangle to the cluster members, which the pipeline must then keep.
func tuningBoxModelMembers(cfg *config.TuningConfig) bool {
	return cfg != nil && cfg.L5.CvKfV1 != nil && cfg.L5.CvKfV1.BoxModel.NeedsMembers()
}

type ringElevationsSetter interface {
	SetRingElevations([]float64) error
}

type vrlogReplayController interface {
	IsVRLogActive() bool
	StopVRLogReplay()
	ClearBackground()
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

type callbackDrainer interface {
	WaitForCallbacks()
}

type liveObservationEnder interface {
	End(string)
}

type recordingMetadataSink interface {
	SetDeterministicConfig(runConfigID, paramSetID, configHash, paramsHash, schemaVersion, paramSetType, buildVersion, buildGitSHA string, executionConfig []byte)
	SetProvenance(sourceType, pcapPath, tuningHash string, playbackRate float64)
}

type recordingSourceInfo interface {
	CurrentSource() server.DataSource
	CurrentPCAPFile() string
	PCAPSpeedRatio() float64
}

type orphanedSweepRecoverer interface {
	RecoverOrphanedSweeps() (int64, error)
}

var marshalTuningJSON = json.Marshal

func isNilHelperTarget(target any) bool {
	if target == nil {
		return true
	}
	value := reflect.ValueOf(target)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func closeLiveObservationBoundary(capture liveObservationEnder, drainer callbackDrainer, reason string) {
	if isNilHelperTarget(capture) {
		return
	}
	if !isNilHelperTarget(drainer) {
		drainer.WaitForCallbacks()
	}
	capture.End(reason)
}

// validateSupportedTuning refuses an engine the pipeline does not run, by the
// check the offline tools share, and also a layer switched off. The shared
// check accepts "none" because lidar-bench and replay run the reduced
// profiles; the server refuses it, as it did before the check moved to config.
func validateSupportedTuning(tuningCfg *config.TuningConfig) error {
	if err := tuningCfg.ValidateImplementedEngines(); err != nil {
		return err
	}
	if tuningCfg.L4.Engine == config.EngineNone {
		return config.UnsupportedEngineError("l4", tuningCfg.L4.Engine, config.ImplementedL4Engine)
	}
	if tuningCfg.L5.Engine == config.EngineNone {
		return config.UnsupportedEngineError("l5", tuningCfg.L5.Engine, config.ImplementedL5Engine)
	}
	return nil
}

func ensureSupportedTuning(tuningCfg *config.TuningConfig, fatalf logfFunc) {
	if err := validateSupportedTuning(tuningCfg); err != nil {
		fatalf("%v", err)
	}
}

func validateLidarNetworkingFlags(udpPort, udpRcvBuf, forwardPort, foregroundForwardPort int) error {
	if udpPort <= 0 || udpPort > 65535 {
		return fmt.Errorf("invalid --lidar-udp-port=%d (must be in [1, 65535])", udpPort)
	}
	if udpRcvBuf <= 0 {
		return fmt.Errorf("invalid --lidar-udp-rcv-buf=%d (must be positive)", udpRcvBuf)
	}
	if err := validateOptionalLidarPortFlag("--lidar-forward-port", forwardPort); err != nil {
		return err
	}
	if err := validateOptionalLidarPortFlag("--lidar-foreground-forward-port", foregroundForwardPort); err != nil {
		return err
	}
	return nil
}

func validateOptionalLidarPortFlag(name string, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid %s=%d (must be in [0, 65535])", name, port)
	}
	return nil
}

func ensureValidLidarNetworkingFlags(udpPort, udpRcvBuf, forwardPort, foregroundForwardPort int, fatalf logfFunc) {
	if err := validateLidarNetworkingFlags(udpPort, udpRcvBuf, forwardPort, foregroundForwardPort); err != nil {
		fatalf("%v", err)
	}
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
	if !isNilHelperTarget(publisher) {
		// Drop the live scene the client is holding. A PCAP replay rebuilds the
		// grid from the capture, so until it settles there is no background to
		// show — and a settle-before-recording run publishes nothing new until
		// its settled snapshot is restored. Without this the live grid stays on
		// screen underneath replayed foreground for that whole stretch.
		publisher.ClearBackground()
		logf("[Visualiser] Cleared client background for PCAP start")
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

func handleReplayStoppedVisualiser(publisher vrlogReplayController, server replayModeController, logf logfFunc) {
	if !isNilHelperTarget(publisher) {
		// Returning to live resets the grid, so the replay's cached background
		// no longer describes anything. Clear it before a reconnecting client
		// can be handed the recording's scene under live foreground.
		publisher.ClearBackground()
	}
	if !isNilHelperTarget(server) {
		server.SetReplayMode(false)
		logf("[Visualiser] Replay stopped: switched to live mode")
	}
}

func replayStoppedCallback(publisher vrlogReplayController, server replayModeController, logf logfFunc) func() {
	return func() {
		handleReplayStoppedVisualiser(publisher, server, logf)
	}
}

// sensorIsSilent applies the live-input timeout without reading the wall clock
// internally, so the never-seen, boundary and clock-skew cases stay explicit.
func sensorIsSilent(lastPacketAt, now time.Time, silentAfter time.Duration) bool {
	if lastPacketAt.IsZero() {
		return true
	}
	return now.Sub(lastPacketAt) > silentAfter
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

func applyRecordingMetadata(rec recordingMetadataSink, lidarDB sqlite.DBClient, lidarServer recordingSourceInfo, runID, tuningHash string, logger *log.Logger) {
	if logger == nil {
		logger = log.Default()
	}

	sourceType := "live"
	pcapPath := ""
	playbackRate := 0.0
	if lidarServer != nil {
		src := lidarServer.CurrentSource()
		if src == server.DataSourceCapture || src == server.DataSourceCaptureAnalysis {
			sourceType = "pcap"
			pcapPath = filepath.Base(lidarServer.CurrentPCAPFile())
			playbackRate = lidarServer.PCAPSpeedRatio()
		}
	}

	runStore := sqlite.NewRunStore(lidarDB)
	run, err := runStore.GetRun(runID)
	if err != nil {
		logger.Printf("[Visualiser] Warning: failed to load run metadata for %s: %v", runID, err)
	}

	effectiveTuningHash := tuningHash
	if run != nil && strings.TrimSpace(run.RunConfigID) != "" {
		configStore := configasset.NewStore(lidarDB)
		runConfig, err := configStore.GetRunConfig(run.RunConfigID)
		if err != nil {
			logger.Printf("[Visualiser] Warning: failed to load immutable config for run %s: %v", runID, err)
		} else {
			rec.SetDeterministicConfig(
				run.RunConfigID,
				runConfig.ParamSetID,
				runConfig.ConfigHash,
				runConfig.ParamsHash,
				runConfig.ParamSchemaVersion,
				runConfig.ParamSetType,
				runConfig.BuildVersion,
				runConfig.BuildGitSHA,
				runConfig.ComposedJSON,
			)
			if runConfig.ParamsHash != "" {
				effectiveTuningHash = runConfig.ParamsHash
			}
		}
	}

	rec.SetProvenance(sourceType, pcapPath, effectiveTuningHash, playbackRate)
}

func recoverOrphanedSweepsOnStart(sweepStore orphanedSweepRecoverer, logger *log.Logger) {
	if logger == nil {
		logger = log.Default()
	}

	if n, err := sweepStore.RecoverOrphanedSweeps(); err != nil {
		logger.Printf("WARNING: failed to recover orphaned sweeps: %v", err)
	} else if n > 0 {
		logger.Printf("Recovered %d orphaned sweep(s) from previous run", n)
	}
}

// visualiserPlaybackProbe adapts the visualiser gRPC server to the monitor
// server's PlaybackProbe interface.
//
// The two packages describe the same replay position with structurally
// identical types rather than a shared one: internal/lidar/server already
// imports l9endpoints, so a shared type would need l9endpoints to import back.
// Converting here — in the composition root that already knows both — keeps
// that dependency edge one-way.
type visualiserPlaybackProbe struct {
	server *l9endpoints.Server
}

func (p visualiserPlaybackProbe) PlaybackPosition() server.PlaybackPosition {
	if p.server == nil {
		return server.PlaybackPosition{Rate: 1.0}
	}
	info := p.server.PlaybackPosition()
	return server.PlaybackPosition{
		Paused:       info.Paused,
		Rate:         info.Rate,
		Seekable:     info.Seekable,
		CurrentFrame: info.CurrentFrame,
		TotalFrames:  info.TotalFrames,
		TimestampNs:  info.TimestampNs,
		LogStartNs:   info.LogStartNs,
		LogEndNs:     info.LogEndNs,
		ReplayEpoch:  info.ReplayEpoch,
	}
}

// filterRecordHost is the tracker's hook for its filter record
// (*l5tracks.Tracker.SetFilterStepObserver).
type filterRecordHost interface {
	SetFilterStepObserver(l5tracks.FilterStepObserver)
}

// attachFilterRecord opens the filter-record sidecar beside a recording and
// attaches it to the tracker, when the recording is of a PCAP replay. The
// live pipeline attaches no observer (filter_steps.go), so a live recording
// gets no sidecar and the device path is unchanged. Returns nil when nothing
// was attached; a sidecar that cannot be opened is logged and the recording
// goes on without it, since the VRLOG is the one thing a replay must leave.
func attachFilterRecord(tracker filterRecordHost, source recordingSourceInfo, recordPath, sensorID string, logf logfFunc) *recorder.FilterRecordWriter {
	if isNilHelperTarget(tracker) || isNilHelperTarget(source) {
		return nil
	}
	if src := source.CurrentSource(); src != server.DataSourceCapture && src != server.DataSourceCaptureAnalysis {
		return nil
	}
	w, err := recorder.NewFilterRecordWriter(recordPath, recorder.FilterRecordHeader{
		SensorID: sensorID, EstimatorID: config.ImplementedL5Engine,
	})
	if err != nil {
		logf("[Visualiser] filter record not started, the recording will have no smoothed trail: %v", err)
		return nil
	}
	tracker.SetFilterStepObserver(w)
	return w
}

// detachFilterRecord stops the tracker recording its filter record and closes
// the sidecar, logging a write error. Safe on a nil writer.
func detachFilterRecord(tracker filterRecordHost, w *recorder.FilterRecordWriter, logf logfFunc) {
	if w == nil {
		return
	}
	if !isNilHelperTarget(tracker) {
		tracker.SetFilterStepObserver(nil)
	}
	if err := w.Close(); err != nil {
		logf("[Visualiser] filter record incomplete: %v", err)
	}
}
