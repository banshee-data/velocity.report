package monitor

import (
	"reflect"
	"strings"
	"testing"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

func TestSnapshotStoreAndCloneTuningConfig(t *testing.T) {
	if cloneTuningConfig(nil) != nil {
		t.Fatal("cloneTuningConfig(nil) should return nil")
	}

	ws := &WebServer{}
	if ws.hasStoredTuningConfig() {
		t.Fatal("expected no stored config")
	}

	defaultCfg := ws.snapshotTuningConfig()
	if defaultCfg == nil {
		t.Fatal("snapshotTuningConfig returned nil")
	}

	ws.storeTuningConfig(cfgpkg.MustLoadDefaultConfig())
	if !ws.hasStoredTuningConfig() {
		t.Fatal("expected stored config after storeTuningConfig")
	}

	snap := ws.snapshotTuningConfig()
	snap.L1.Sensor = "mutated"
	if ws.snapshotTuningConfig().L1.Sensor == "mutated" {
		t.Fatal("snapshotTuningConfig should return an isolated clone")
	}
}

func TestRuntimeTuningConfigSyncsRuntimeState(t *testing.T) {
	cfg := cloneTuningConfig(cfgpkg.MustLoadDefaultConfig())
	params := l3grid.DefaultBackgroundConfig().ToBackgroundParams()
	params.BackgroundUpdateFraction = 0.07
	params.ClosenessSensitivityMultiplier = 9
	params.SafetyMarginMeters = 0.9
	params.NoiseRelativeFraction = 0.12
	params.NeighborConfirmationCount = 4
	params.SeedFromFirstObservation = false
	params.WarmupDurationNanos = 9
	params.WarmupMinFrames = 8
	params.PostSettleUpdateFraction = 0.2
	params.FreezeThresholdMultiplier = 6
	params.ChangeThresholdForSnapshot = 17
	params.ReacquisitionBoostMultiplier = 2
	params.MinConfidenceFloor = 5
	params.LockedBaselineThreshold = 6
	params.LockedBaselineMultiplier = 7
	params.SensorMovementForegroundThreshold = 0.4
	params.BackgroundDriftThresholdMeters = 0.5
	params.BackgroundDriftRatioThreshold = 0.6
	params.SettlingMinCoverage = 0.7
	params.SettlingMaxSpreadDelta = 0.8
	params.SettlingMinRegionStability = 0.9
	params.SettlingMinConfidence = 1.1
	params.ForegroundDBSCANEps = 1.2
	params.ForegroundMinClusterPoints = 10
	params.ForegroundMaxInputPoints = 9000
	bm := l3grid.NewBackgroundManager("stored-sensor", 16, 360, params, nil)
	bm.SetEnableDiagnostics(true)

	tracker := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tracker.UpdateConfig(func(cfg *l5tracks.TrackerConfig) {
		cfg.GatingDistanceSquared = 44
		cfg.ProcessNoisePos = 0.2
		cfg.ProcessNoiseVel = 0.3
		cfg.MeasurementNoise = 0.4
		cfg.OcclusionCovInflation = 0.5
		cfg.HitsToConfirm = 6
		cfg.MaxMisses = 7
		cfg.MaxMissesConfirmed = 8
		cfg.MaxTracks = 9
		cfg.MaxReasonableSpeedMps = 10
		cfg.MaxPositionJumpMeters = 11
		cfg.MaxPredictDt = 12
		cfg.MaxCovarianceDiag = 13
		cfg.MinPointsForPCA = 14
		cfg.OBBHeadingSmoothingAlpha = 0.15
		cfg.OBBAspectRatioLockThreshold = 0.16
		cfg.MaxTrackHistoryLength = 17
		cfg.MaxSpeedHistoryLength = 18
		cfg.MergeSizeRatio = 1.7
		cfg.SplitSizeRatio = 1.8
		cfg.DeletedTrackGracePeriod = 2 * time.Second
		cfg.MinObservationsForClassification = 19
	})

	ws := &WebServer{
		sensorID:          "runtime-sensor",
		udpPort:           4242,
		forwardPort:       5252,
		udpListenerConfig: network.UDPListenerConfig{RcvBuf: 64 << 10},
		currentSource:     DataSourcePCAP,
		tracker:           tracker,
	}
	ws.storeTuningConfig(cfg)

	runtimeCfg := ws.runtimeTuningConfig(bm)
	if runtimeCfg.L1.Sensor != "runtime-sensor" ||
		runtimeCfg.L1.UDPPort != 4242 ||
		runtimeCfg.L1.UDPRcvBuf != 64<<10 ||
		runtimeCfg.L1.ForwardPort != 5252 ||
		runtimeCfg.L1.DataSource != string(DataSourcePCAP) {
		t.Fatalf("unexpected L1 runtime sync: %+v", runtimeCfg.L1)
	}
	if !approxEqualFloat64(runtimeCfg.L3.EmaBaselineV1.NoiseRelative, 0.12) ||
		runtimeCfg.L3.EmaBaselineV1.EnableDiagnostics != true ||
		runtimeCfg.L4.DbscanXyV1.ForegroundMaxInputPoints != 9000 {
		t.Fatalf("unexpected L3/L4 runtime sync: %+v %+v", runtimeCfg.L3.EmaBaselineV1, runtimeCfg.L4.DbscanXyV1)
	}
	if runtimeCfg.L5.CvKfV1.GatingDistanceSquared != 44 ||
		runtimeCfg.L5.CvKfV1.MinObservationsForClassification != 19 ||
		runtimeCfg.L5.CvKfV1.DeletedTrackGracePeriod != "2s" {
		t.Fatalf("unexpected L5 runtime sync: %+v", runtimeCfg.L5.CvKfV1)
	}

	wsNoStored := &WebServer{sensorID: "no-store-sensor", currentSource: DataSourcePCAPAnalysis}
	fallback := wsNoStored.runtimeTuningConfig(bm)
	if fallback.L1.Sensor != "no-store-sensor" || fallback.L1.DataSource != string(DataSourcePCAPAnalysis) {
		t.Fatalf("unexpected fallback L1 sync: %+v", fallback.L1)
	}
	if fallback.L3.EmaBaselineV1.NoiseRelative == 0.12 {
		t.Fatal("runtime state should not be synced without a stored config snapshot")
	}
}

func TestNormaliseTuningPatchAndValidateRuntimePath(t *testing.T) {
	patch, err := normaliseTuningPatch(map[string]interface{}{
		"l3": map[string]interface{}{
			"ema_baseline_v1": map[string]interface{}{
				"noise_relative": 0.2,
			},
		},
		"l4.dbscan_xy_v1.foreground_dbscan_eps": 0.8,
	})
	if err != nil {
		t.Fatalf("normaliseTuningPatch returned error: %v", err)
	}
	if len(patch) != 2 ||
		patch["l3.ema_baseline_v1.noise_relative"] != 0.2 ||
		patch["l4.dbscan_xy_v1.foreground_dbscan_eps"] != 0.8 {
		t.Fatalf("unexpected flattened patch: %#v", patch)
	}

	if _, err := normaliseTuningPatch(map[string]interface{}{"": 1}); err == nil || !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("expected empty-key error, got %v", err)
	}
	if err := flattenTuningPatch("", 1, map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("expected root object error, got %v", err)
	}

	allowed := []string{
		"l3.ema_baseline_v1.noise_relative",
		"l4.dbscan_xy_v1.foreground_dbscan_eps",
		"l4.dbscan_xy_v1.foreground_min_cluster_points",
		"l4.dbscan_xy_v1.foreground_max_input_points",
		"l5.cv_kf_v1.max_tracks",
	}
	for _, path := range allowed {
		if err := validateRuntimeTuningPath(path); err != nil {
			t.Fatalf("validateRuntimeTuningPath(%q) returned error: %v", path, err)
		}
	}

	disallowed := map[string]string{
		"pipeline.buffer_timeout":           "not runtime-updatable",
		"l4.dbscan_xy_v1.height_band_floor": "not runtime-updatable",
		"unknown.path":                      "unknown tuning path",
	}
	for path, want := range disallowed {
		if err := validateRuntimeTuningPath(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("validateRuntimeTuningPath(%q) = %v, want substring %q", path, err, want)
		}
	}
}

func TestSetConfigValueByPathAndReflectionHelpers(t *testing.T) {
	cfg := cloneTuningConfig(cfgpkg.MustLoadDefaultConfig())
	cfg.L3.EmaTrackAssistV2 = nil
	if err := setConfigValueByPath(cfg, "l3.ema_track_assist_v2.promotion_threshold", 0.4); err != nil {
		t.Fatalf("setConfigValueByPath allocated pointer returned error: %v", err)
	}
	if cfg.L3.EmaTrackAssistV2 == nil || cfg.L3.EmaTrackAssistV2.PromotionThreshold != 0.4 {
		t.Fatalf("unexpected track assist config: %+v", cfg.L3.EmaTrackAssistV2)
	}

	if err := setConfigValueByPath(cfg, "l1.udp_port", "bad"); err == nil || !strings.Contains(err.Error(), "cannot unmarshal string") {
		t.Fatalf("expected type error, got %v", err)
	}
	if err := setConfigValueByPath(cfg, "unknown.path", 1); err == nil || !strings.Contains(err.Error(), "unknown tuning path segment") {
		t.Fatalf("expected unknown path error, got %v", err)
	}
	if err := setConfigValueByPath(cfg, "", 1); err == nil || !strings.Contains(err.Error(), `unknown tuning path segment ""`) {
		t.Fatalf("expected empty-path error, got %v", err)
	}

	type embedded struct {
		Value int `json:"value"`
	}
	type container struct {
		*embedded
		Name string `json:"name"`
		skip string `json:"skip"`
	}
	field, err := fieldByJSONName(reflect.ValueOf(container{}), "value")
	if err != nil || field.Kind() != reflect.Int {
		t.Fatalf("fieldByJSONName(value) = (%v, %v)", field, err)
	}
	if _, err := fieldByJSONName(reflect.ValueOf(container{}), "missing"); err == nil || !strings.Contains(err.Error(), "unknown tuning path segment") {
		t.Fatalf("expected missing field error, got %v", err)
	}

	var target int
	if err := assignJSONValue(reflect.ValueOf(&target).Elem(), make(chan int), "path"); err == nil || !strings.Contains(err.Error(), "marshal value") {
		t.Fatalf("expected marshal error, got %v", err)
	}
	if err := assignJSONValue(reflect.ValueOf(&target).Elem(), "bad", "path"); err == nil || !strings.Contains(err.Error(), "cannot unmarshal string") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}

	typ := reflect.TypeOf(container{})
	if name, tagged := jsonName(typ.Field(1)); name != "name" || !tagged {
		t.Fatalf("jsonName(tagged) = (%q, %v)", name, tagged)
	}
	if name, tagged := jsonName(typ.Field(0)); name != "embedded" || tagged {
		t.Fatalf("jsonName(empty tag default) = (%q, %v)", name, tagged)
	}
}

func approxEqualFloat64(a, b float64) bool {
	const eps = 1e-6
	if a > b {
		return a-b < eps
	}
	return b-a < eps
}

func TestApplyRuntimeTuningPatchAndPathErrors(t *testing.T) {
	cfg := cloneTuningConfig(cfgpkg.MustLoadDefaultConfig())
	params := l3grid.DefaultBackgroundConfig().ToBackgroundParams()
	bm := l3grid.NewBackgroundManager("patch-sensor", 16, 360, params, nil)
	tracker := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	classifier := l6objects.NewTrackClassifierWithMinObservations(5)
	ws := &WebServer{
		sensorID:      "patch-sensor",
		tracker:       tracker,
		classifier:    classifier,
		currentSource: DataSourceLive,
	}
	ws.storeTuningConfig(cfg)

	patch := map[string]interface{}{
		"l3.ema_baseline_v1.noise_relative":               0.2,
		"l4.dbscan_xy_v1.foreground_max_input_points":     5000,
		"l5.cv_kf_v1.min_observations_for_classification": 10,
		"l5.cv_kf_v1.deleted_track_grace_period":          "3s",
		"l5.cv_kf_v1.max_tracks":                          55,
	}
	if err := applyRuntimeTuningPatch(ws, bm, patch); err != nil {
		t.Fatalf("applyRuntimeTuningPatch returned error: %v", err)
	}
	if got := bm.GetParams().NoiseRelativeFraction; got != 0.2 {
		t.Fatalf("background manager noise_relative = %v, want 0.2", got)
	}
	if got := bm.GetParams().ForegroundMaxInputPoints; got != 5000 {
		t.Fatalf("background manager foreground_max_input_points = %d, want 5000", got)
	}
	if tracker.Config.MinObservationsForClassification != 10 || classifier.MinObservations != 10 {
		t.Fatalf("expected min observations 10, got tracker=%d classifier=%d", tracker.Config.MinObservationsForClassification, classifier.MinObservations)
	}
	if tracker.Config.DeletedTrackGracePeriod != 3*time.Second || tracker.Config.MaxTracks != 55 {
		t.Fatalf("unexpected tracker runtime update: %+v", tracker.Config)
	}
	if ws.snapshotTuningConfig().L3.EmaBaselineV1.NoiseRelative != 0.2 {
		t.Fatal("stored tuning config was not updated")
	}

	if err := applyRuntimeTuningPatch(ws, bm, map[string]interface{}{"unknown.path": 1}); err == nil || !strings.Contains(err.Error(), "unknown tuning path") {
		t.Fatalf("expected unknown path error, got %v", err)
	}
	if err := applyRuntimeTuningPatch(ws, bm, map[string]interface{}{"l3.ema_baseline_v1.noise_relative": 2.0}); err == nil || !strings.Contains(err.Error(), "noise_relative must be between 0 and 1") {
		t.Fatalf("expected validation error, got %v", err)
	}
	if err := applyRuntimeTuningPath(ws, nil, ws.snapshotTuningConfig(), "l3.ema_baseline_v1.noise_relative"); err == nil || !strings.Contains(err.Error(), "requires an active background manager") {
		t.Fatalf("expected missing background manager error, got %v", err)
	}
	if err := applyRuntimeTuningPath(&WebServer{}, bm, ws.snapshotTuningConfig(), "l5.cv_kf_v1.max_tracks"); err == nil || !strings.Contains(err.Error(), "requires an active tracker") {
		t.Fatalf("expected missing tracker error, got %v", err)
	}
	if err := applyRuntimeTuningPath(ws, bm, ws.snapshotTuningConfig(), "l4.dbscan_xy_v1.height_band_floor"); err == nil || !strings.Contains(err.Error(), "unsupported runtime path") {
		t.Fatalf("expected unsupported background path error, got %v", err)
	}
	if err := applyRuntimeTuningPath(ws, bm, ws.snapshotTuningConfig(), "l5.cv_kf_v1.unknown"); err == nil || !strings.Contains(err.Error(), "unsupported runtime path") {
		t.Fatalf("expected unsupported tracker path error, got %v", err)
	}
}
