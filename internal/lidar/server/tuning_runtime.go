package server

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// roundTo6 rounds a float64 to 6 decimal places, trimming noise from
// float32→float64 conversions (e.g. 0.10000000149011612 → 0.1).
func roundTo6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// compactDuration formats a time.Duration using only the most significant
// non-zero units with lowercase single-letter suffixes separated by spaces.
// Examples: "5m", "2h", "3h 15m 30s", "500ms".
func compactDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	var parts []string
	if h := int(d.Hours()); h != 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
		d -= time.Duration(h) * time.Hour
	}
	if m := int(d.Minutes()); m != 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
		d -= time.Duration(m) * time.Minute
	}
	if s := int(d.Seconds()); s != 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
		d -= time.Duration(s) * time.Second
	}
	if ms := int(d.Milliseconds()); ms != 0 {
		parts = append(parts, fmt.Sprintf("%dms", ms))
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

var (
	runtimeTuningAliasOnce sync.Once
	runtimeTuningAliases   map[string]string
)

func (ws *Server) snapshotTuningConfig() *cfgpkg.TuningConfig {
	ws.tuningConfigMu.RLock()
	cfg := cloneTuningConfig(ws.tuningConfig)
	ws.tuningConfigMu.RUnlock()
	if cfg != nil {
		return cfg
	}
	return cloneTuningConfig(cfgpkg.MustLoadDefaultConfig())
}

func (ws *Server) hasStoredTuningConfig() bool {
	ws.tuningConfigMu.RLock()
	defer ws.tuningConfigMu.RUnlock()
	return ws.tuningConfig != nil
}

func (ws *Server) storeTuningConfig(cfg *cfgpkg.TuningConfig) {
	ws.tuningConfigMu.Lock()
	ws.tuningConfig = cloneTuningConfig(cfg)
	ws.tuningConfigMu.Unlock()
}

func (ws *Server) runtimeTuningConfig(bm *l3grid.BackgroundManager) *cfgpkg.TuningConfig {
	return ws.runtimeTuningConfigForSource(bm, "")
}

func (ws *Server) runtimeTuningConfigForSource(bm *l3grid.BackgroundManager, sourceOverride DataSource) *cfgpkg.TuningConfig {
	cfg := ws.snapshotTuningConfig()
	syncRuntimeState := ws.hasStoredTuningConfig()

	if ws.sensorID != "" {
		cfg.L1.Sensor = ws.sensorID
	}
	source := sourceOverride
	if source == "" {
		source = ws.CurrentSource()
	}
	if source != "" {
		cfg.L1.DataSource = string(source)
	}

	if bm != nil && syncRuntimeState {
		params := bm.GetParams()
		l3 := cfg.L3.ActiveCommon()
		l4 := cfg.L4.ActiveCommon()
		if l3 != nil {
			l3.BackgroundUpdateFraction = roundTo6(float64(params.BackgroundUpdateFraction))
			l3.ClosenessMultiplier = roundTo6(float64(params.ClosenessSensitivityMultiplier))
			l3.SafetyMarginMetres = roundTo6(float64(params.SafetyMarginMetres))
			l3.NoiseRelative = roundTo6(float64(params.NoiseRelativeFraction))
			l3.NeighbourConfirmationCount = params.NeighbourConfirmationCount
			l3.SeedFromFirst = params.SeedFromFirstObservation
			l3.WarmupDurationNanos = params.WarmupDurationNanos
			l3.WarmupMinFrames = params.WarmupMinFrames
			l3.PostSettleUpdateFraction = roundTo6(float64(params.PostSettleUpdateFraction))
			l3.EnableDiagnostics = bm.GetEnableDiagnostics()
			l3.FreezeThresholdMultiplier = roundTo6(float64(params.FreezeThresholdMultiplier))
			l3.ChangeThresholdSnapshot = params.ChangeThresholdForSnapshot
			l3.ReacquisitionBoostMultiplier = roundTo6(float64(params.ReacquisitionBoostMultiplier))
			l3.MinConfidenceFloor = int(params.MinConfidenceFloor)
			l3.LockedBaselineThreshold = int(params.LockedBaselineThreshold)
			l3.LockedBaselineMultiplier = roundTo6(float64(params.LockedBaselineMultiplier))
			l3.SensorMovementForegroundThreshold = roundTo6(float64(params.SensorMovementForegroundThreshold))
			l3.BackgroundDriftThresholdMetres = roundTo6(float64(params.BackgroundDriftThresholdMetres))
			l3.BackgroundDriftRatioThreshold = roundTo6(float64(params.BackgroundDriftRatioThreshold))
			l3.SettlingMinCoverage = roundTo6(float64(params.SettlingMinCoverage))
			l3.SettlingMaxSpreadDelta = roundTo6(float64(params.SettlingMaxSpreadDelta))
			l3.SettlingMinRegionStability = roundTo6(float64(params.SettlingMinRegionStability))
			l3.SettlingMinConfidence = roundTo6(float64(params.SettlingMinConfidence))
		}
		if l4 != nil {
			l4.ForegroundDBSCANEps = roundTo6(float64(params.ForegroundDBSCANEps))
			l4.ForegroundMinClusterPoints = params.ForegroundMinClusterPoints
			l4.ForegroundMaxInputPoints = params.ForegroundMaxInputPoints
		}
		if l3 != nil {
			// These remain driven from config snapshots because runtime keeps them in Params.
			l3.FreezeDuration = compactDuration(cfg.GetFreezeDuration())
			l3.SettlingPeriod = compactDuration(cfg.GetSettlingPeriod())
			l3.SnapshotInterval = compactDuration(cfg.GetSnapshotInterval())
		}
	}

	if ws.tracker != nil && syncRuntimeState {
		l5 := cfg.L5.ActiveCommon()
		if l5 != nil {
			trackerCfg := ws.tracker.GetConfig()
			l5.GatingDistanceSquared = roundTo6(float64(trackerCfg.GatingDistanceSquared))
			l5.ProcessNoisePos = roundTo6(float64(trackerCfg.ProcessNoisePos))
			l5.ProcessNoiseVel = roundTo6(float64(trackerCfg.ProcessNoiseVel))
			l5.MeasurementNoise = roundTo6(float64(trackerCfg.MeasurementNoise))
			l5.OcclusionCovInflation = roundTo6(float64(trackerCfg.OcclusionCovInflation))
			l5.HitsToConfirm = trackerCfg.HitsToConfirm
			l5.MaxMisses = trackerCfg.MaxMisses
			l5.MaxMissesConfirmed = trackerCfg.MaxMissesConfirmed
			l5.MaxTracks = trackerCfg.MaxTracks
			l5.MaxReasonableSpeedMps = roundTo6(float64(trackerCfg.MaxReasonableSpeedMps))
			l5.MaxPositionJumpMetres = roundTo6(float64(trackerCfg.MaxPositionJumpMetres))
			l5.MaxPredictDt = roundTo6(float64(trackerCfg.MaxPredictDt))
			l5.MaxCovarianceDiag = roundTo6(float64(trackerCfg.MaxCovarianceDiag))
			l5.MinPointsForPCA = trackerCfg.MinPointsForPCA
			l5.OBBHeadingSmoothingAlpha = roundTo6(float64(trackerCfg.OBBHeadingSmoothingAlpha))
			l5.OBBAspectRatioLockThreshold = roundTo6(float64(trackerCfg.OBBAspectRatioLockThreshold))
			l5.MaxTrackHistoryLength = trackerCfg.MaxTrackHistoryLength
			l5.MaxSpeedHistoryLength = trackerCfg.MaxSpeedHistoryLength
			l5.MergeSizeRatio = roundTo6(float64(trackerCfg.MergeSizeRatio))
			l5.SplitSizeRatio = roundTo6(float64(trackerCfg.SplitSizeRatio))
			l5.DeletedTrackGracePeriod = compactDuration(trackerCfg.DeletedTrackGracePeriod)
			l5.MinObservationsForClassification = trackerCfg.MinObservationsForClassification
		}
	}

	return cfg
}

func normaliseTuningPatch(raw map[string]interface{}) (map[string]interface{}, error) {
	flat := make(map[string]interface{})
	if err := flattenTuningPatch("", raw, flat); err != nil {
		return nil, err
	}
	normalised := make(map[string]interface{}, len(flat))
	for path, value := range flat {
		normalised[canonicalRuntimeTuningPath(path)] = value
	}
	return normalised, nil
}

func flattenTuningPatch(prefix string, value interface{}, out map[string]interface{}) error {
	obj, ok := value.(map[string]interface{})
	if !ok {
		if prefix == "" {
			return fmt.Errorf("tuning patch must be a JSON object")
		}
		out[prefix] = value
		return nil
	}

	for key, child := range obj {
		if key == "" {
			return fmt.Errorf("tuning patch contains an empty key")
		}
		if prefix == "" && strings.Contains(key, ".") {
			out[key] = child
			continue
		}

		next := key
		if prefix != "" {
			next = prefix + "." + key
		}
		if err := flattenTuningPatch(next, child, out); err != nil {
			return err
		}
	}
	return nil
}

func canonicalRuntimeTuningPath(path string) string {
	if mapped, ok := runtimeTuningPathAliases()[path]; ok {
		return mapped
	}
	return path
}

func runtimeTuningPathAliases() map[string]string {
	runtimeTuningAliasOnce.Do(func() {
		runtimeTuningAliases = buildRuntimeTuningPathAliases()
	})
	return runtimeTuningAliases
}

func buildRuntimeTuningPathAliases() map[string]string {
	aliases := make(map[string]string)

	raw, err := json.Marshal(cfgpkg.MustLoadDefaultConfig())
	if err != nil {
		return aliases
	}
	var nested map[string]interface{}
	if err := json.Unmarshal(raw, &nested); err != nil {
		return aliases
	}
	flat := make(map[string]interface{})
	if err := flattenTuningPatch("", nested, flat); err != nil {
		return aliases
	}

	suffixCounts := make(map[string]int)
	validPaths := make([]string, 0, len(flat))
	for path := range flat {
		if validateRuntimeTuningPath(path) != nil {
			continue
		}
		validPaths = append(validPaths, path)
		suffixCounts[lastTuningPathSegment(path)]++
	}

	replacer := strings.NewReplacer("neighbour", "neighbor", "metres", "meters")
	for _, path := range validPaths {
		suffix := lastTuningPathSegment(path)
		if suffixCounts[suffix] == 1 {
			aliases[suffix] = path
		}
		aliasPath := replacer.Replace(path)
		if aliasPath != path {
			aliases[aliasPath] = path
			if suffixCounts[suffix] == 1 {
				aliases[lastTuningPathSegment(aliasPath)] = path
			}
		}
	}

	return aliases
}

func lastTuningPathSegment(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx == -1 {
		return path
	}
	return path[idx+1:]
}

func applyRuntimeTuningPatch(ws *Server, bm *l3grid.BackgroundManager, paths map[string]interface{}) error {
	cfg := ws.runtimeTuningConfig(bm)

	orderedPaths := make([]string, 0, len(paths))
	for path := range paths {
		orderedPaths = append(orderedPaths, path)
	}
	sort.Strings(orderedPaths)

	// Filter to runtime-editable paths; silently skip non-editable fields
	// so callers can POST the full nested config without errors.
	editablePaths := make([]string, 0, len(orderedPaths))
	for _, path := range orderedPaths {
		if validateRuntimeTuningPath(path) == nil {
			editablePaths = append(editablePaths, path)
		}
	}
	if len(editablePaths) == 0 {
		return fmt.Errorf("no runtime-editable parameters in patch")
	}

	for _, path := range editablePaths {
		if err := setConfigValueByPath(cfg, path, paths[path]); err != nil {
			return err
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	for _, path := range editablePaths {
		if err := applyRuntimeTuningPath(ws, bm, cfg, path); err != nil {
			return err
		}
	}

	ws.storeTuningConfig(cfg)
	return nil
}

func validateRuntimeTuningPath(path string) error {
	switch {
	case strings.HasPrefix(path, "l3.ema_baseline_v1."):
		return nil
	case strings.HasPrefix(path, "l4.dbscan_xy_v1."):
		field := strings.TrimPrefix(path, "l4.dbscan_xy_v1.")
		switch field {
		case "foreground_dbscan_eps", "foreground_min_cluster_points", "foreground_max_input_points":
			return nil
		default:
			return fmt.Errorf("%s is not runtime-updatable on this branch", path)
		}
	case strings.HasPrefix(path, "l5.cv_kf_v1."):
		return nil
	case path == "l1.sensor", path == "l1.data_source",
		path == "pipeline.buffer_timeout", path == "pipeline.min_frame_points",
		path == "pipeline.flush_interval", path == "pipeline.background_flush",
		path == "version", path == "l3.engine", path == "l4.engine", path == "l5.engine":
		return fmt.Errorf("%s is not runtime-updatable on this branch", path)
	default:
		return fmt.Errorf("unknown tuning path %q", path)
	}
}

func applyRuntimeTuningPath(ws *Server, bm *l3grid.BackgroundManager, cfg *cfgpkg.TuningConfig, path string) error {
	switch {
	case strings.HasPrefix(path, "l3.ema_baseline_v1."), strings.HasPrefix(path, "l4.dbscan_xy_v1."):
		if bm == nil {
			return fmt.Errorf("%s requires an active background manager", path)
		}
		params := bm.GetParams()
		l3 := cfg.L3.ActiveCommon()
		l4 := cfg.L4.ActiveCommon()
		switch path {
		case "l3.ema_baseline_v1.background_update_fraction":
			params.BackgroundUpdateFraction = float32(l3.BackgroundUpdateFraction)
		case "l3.ema_baseline_v1.closeness_multiplier":
			params.ClosenessSensitivityMultiplier = float32(l3.ClosenessMultiplier)
		case "l3.ema_baseline_v1.safety_margin_metres":
			params.SafetyMarginMetres = float32(l3.SafetyMarginMetres)
		case "l3.ema_baseline_v1.noise_relative":
			params.NoiseRelativeFraction = float32(l3.NoiseRelative)
		case "l3.ema_baseline_v1.neighbour_confirmation_count":
			params.NeighbourConfirmationCount = l3.NeighbourConfirmationCount
		case "l3.ema_baseline_v1.seed_from_first":
			params.SeedFromFirstObservation = l3.SeedFromFirst
		case "l3.ema_baseline_v1.warmup_duration_nanos":
			params.WarmupDurationNanos = l3.WarmupDurationNanos
		case "l3.ema_baseline_v1.warmup_min_frames":
			params.WarmupMinFrames = l3.WarmupMinFrames
		case "l3.ema_baseline_v1.post_settle_update_fraction":
			params.PostSettleUpdateFraction = float32(l3.PostSettleUpdateFraction)
		case "l3.ema_baseline_v1.enable_diagnostics":
			bm.SetEnableDiagnostics(l3.EnableDiagnostics)
		case "l3.ema_baseline_v1.freeze_duration":
			params.FreezeDurationNanos = cfg.GetFreezeDuration().Nanoseconds()
		case "l3.ema_baseline_v1.freeze_threshold_multiplier":
			params.FreezeThresholdMultiplier = float32(l3.FreezeThresholdMultiplier)
		case "l3.ema_baseline_v1.settling_period":
			params.SettlingPeriodNanos = cfg.GetSettlingPeriod().Nanoseconds()
		case "l3.ema_baseline_v1.snapshot_interval":
			params.SnapshotIntervalNanos = cfg.GetSnapshotInterval().Nanoseconds()
		case "l3.ema_baseline_v1.change_threshold_snapshot":
			params.ChangeThresholdForSnapshot = l3.ChangeThresholdSnapshot
		case "l3.ema_baseline_v1.reacquisition_boost_multiplier":
			params.ReacquisitionBoostMultiplier = float32(l3.ReacquisitionBoostMultiplier)
		case "l3.ema_baseline_v1.min_confidence_floor":
			params.MinConfidenceFloor = uint32(l3.MinConfidenceFloor)
		case "l3.ema_baseline_v1.locked_baseline_threshold":
			params.LockedBaselineThreshold = uint32(l3.LockedBaselineThreshold)
		case "l3.ema_baseline_v1.locked_baseline_multiplier":
			params.LockedBaselineMultiplier = float32(l3.LockedBaselineMultiplier)
		case "l3.ema_baseline_v1.sensor_movement_foreground_threshold":
			params.SensorMovementForegroundThreshold = float32(l3.SensorMovementForegroundThreshold)
		case "l3.ema_baseline_v1.background_drift_threshold_metres":
			params.BackgroundDriftThresholdMetres = float32(l3.BackgroundDriftThresholdMetres)
		case "l3.ema_baseline_v1.background_drift_ratio_threshold":
			params.BackgroundDriftRatioThreshold = float32(l3.BackgroundDriftRatioThreshold)
		case "l3.ema_baseline_v1.settling_min_coverage":
			params.SettlingMinCoverage = float32(l3.SettlingMinCoverage)
		case "l3.ema_baseline_v1.settling_max_spread_delta":
			params.SettlingMaxSpreadDelta = float32(l3.SettlingMaxSpreadDelta)
		case "l3.ema_baseline_v1.settling_min_region_stability":
			params.SettlingMinRegionStability = float32(l3.SettlingMinRegionStability)
		case "l3.ema_baseline_v1.settling_min_confidence":
			params.SettlingMinConfidence = float32(l3.SettlingMinConfidence)
		case "l4.dbscan_xy_v1.foreground_dbscan_eps":
			params.ForegroundDBSCANEps = float32(l4.ForegroundDBSCANEps)
		case "l4.dbscan_xy_v1.foreground_min_cluster_points":
			params.ForegroundMinClusterPoints = l4.ForegroundMinClusterPoints
		case "l4.dbscan_xy_v1.foreground_max_input_points":
			params.ForegroundMaxInputPoints = l4.ForegroundMaxInputPoints
		default:
			return fmt.Errorf("unsupported runtime path %q", path)
		}
		return bm.SetParams(params)

	case strings.HasPrefix(path, "l5.cv_kf_v1."):
		if ws.tracker == nil {
			return fmt.Errorf("%s requires an active tracker", path)
		}
		l5 := cfg.L5.ActiveCommon()
		var gracePeriodErr error
		gracePeriod := cfg.GetDeletedTrackGracePeriod()
		ws.tracker.UpdateConfig(func(trackerCfg *l5tracks.TrackerConfig) {
			switch path {
			case "l5.cv_kf_v1.gating_distance_squared":
				trackerCfg.GatingDistanceSquared = float32(l5.GatingDistanceSquared)
			case "l5.cv_kf_v1.process_noise_pos":
				trackerCfg.ProcessNoisePos = float32(l5.ProcessNoisePos)
			case "l5.cv_kf_v1.process_noise_vel":
				trackerCfg.ProcessNoiseVel = float32(l5.ProcessNoiseVel)
			case "l5.cv_kf_v1.measurement_noise":
				trackerCfg.MeasurementNoise = float32(l5.MeasurementNoise)
			case "l5.cv_kf_v1.occlusion_cov_inflation":
				trackerCfg.OcclusionCovInflation = float32(l5.OcclusionCovInflation)
			case "l5.cv_kf_v1.hits_to_confirm":
				trackerCfg.HitsToConfirm = l5.HitsToConfirm
			case "l5.cv_kf_v1.max_misses":
				trackerCfg.MaxMisses = l5.MaxMisses
			case "l5.cv_kf_v1.max_misses_confirmed":
				trackerCfg.MaxMissesConfirmed = l5.MaxMissesConfirmed
			case "l5.cv_kf_v1.max_tracks":
				trackerCfg.MaxTracks = l5.MaxTracks
			case "l5.cv_kf_v1.max_reasonable_speed_mps":
				trackerCfg.MaxReasonableSpeedMps = float32(l5.MaxReasonableSpeedMps)
			case "l5.cv_kf_v1.max_position_jump_metres":
				trackerCfg.MaxPositionJumpMetres = float32(l5.MaxPositionJumpMetres)
			case "l5.cv_kf_v1.max_predict_dt":
				trackerCfg.MaxPredictDt = float32(l5.MaxPredictDt)
			case "l5.cv_kf_v1.max_covariance_diag":
				trackerCfg.MaxCovarianceDiag = float32(l5.MaxCovarianceDiag)
			case "l5.cv_kf_v1.min_points_for_pca":
				trackerCfg.MinPointsForPCA = l5.MinPointsForPCA
			case "l5.cv_kf_v1.obb_heading_smoothing_alpha":
				trackerCfg.OBBHeadingSmoothingAlpha = float32(l5.OBBHeadingSmoothingAlpha)
			case "l5.cv_kf_v1.obb_aspect_ratio_lock_threshold":
				trackerCfg.OBBAspectRatioLockThreshold = float32(l5.OBBAspectRatioLockThreshold)
			case "l5.cv_kf_v1.max_track_history_length":
				trackerCfg.MaxTrackHistoryLength = l5.MaxTrackHistoryLength
			case "l5.cv_kf_v1.max_speed_history_length":
				trackerCfg.MaxSpeedHistoryLength = l5.MaxSpeedHistoryLength
			case "l5.cv_kf_v1.merge_size_ratio":
				trackerCfg.MergeSizeRatio = float32(l5.MergeSizeRatio)
			case "l5.cv_kf_v1.split_size_ratio":
				trackerCfg.SplitSizeRatio = float32(l5.SplitSizeRatio)
			case "l5.cv_kf_v1.deleted_track_grace_period":
				trackerCfg.DeletedTrackGracePeriod = gracePeriod
			case "l5.cv_kf_v1.min_observations_for_classification":
				trackerCfg.MinObservationsForClassification = l5.MinObservationsForClassification
			default:
				gracePeriodErr = fmt.Errorf("unsupported runtime path %q", path)
			}
		})
		if gracePeriodErr != nil {
			return gracePeriodErr
		}
		if path == "l5.cv_kf_v1.min_observations_for_classification" && ws.classifier != nil {
			ws.classifier.MinObservations = l5.MinObservationsForClassification
		}
		return nil
	}

	return fmt.Errorf("unsupported runtime path %q", path)
}

func setConfigValueByPath(cfg *cfgpkg.TuningConfig, path string, value interface{}) error {
	current := reflect.ValueOf(cfg)
	segments := strings.Split(path, ".")
	for _, segment := range segments[:len(segments)-1] {
		if current.Kind() == reflect.Ptr {
			if current.IsNil() {
				current.Set(reflect.New(current.Type().Elem()))
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return fmt.Errorf("%s does not resolve to a config field", path)
		}

		field, err := fieldByJSONName(current, segment)
		if err != nil {
			return err
		}
		current = field
	}
	if current.Kind() == reflect.Ptr {
		if current.IsNil() {
			current.Set(reflect.New(current.Type().Elem()))
		}
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct {
		return fmt.Errorf("%s does not resolve to a config field", path)
	}
	field, err := fieldByJSONName(current, segments[len(segments)-1])
	if err != nil {
		return err
	}
	return assignJSONValue(field, value, path)
}

func fieldByJSONName(v reflect.Value, name string) (reflect.Value, error) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)
		if fieldType.PkgPath != "" && !fieldType.Anonymous {
			continue
		}
		tagName, tagged := jsonName(fieldType)
		if tagName == "-" {
			continue
		}
		fieldValue := v.Field(i)
		if fieldType.Anonymous && !tagged {
			nested := fieldValue
			if nested.Kind() == reflect.Ptr {
				if nested.IsNil() {
					nested = reflect.New(nested.Type().Elem()).Elem()
				} else {
					nested = nested.Elem()
				}
			}
			if nested.IsValid() && nested.Kind() == reflect.Struct {
				candidate, err := fieldByJSONName(nested, name)
				if err == nil {
					return candidate, nil
				}
			}
		}
		if tagName == name {
			return fieldValue, nil
		}
	}
	return reflect.Value{}, fmt.Errorf("unknown tuning path segment %q", name)
}

func assignJSONValue(field reflect.Value, value interface{}, path string) error {
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%s: marshal value: %w", path, err)
	}
	target := reflect.New(field.Type())
	if err := json.Unmarshal(data, target.Interface()); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	field.Set(target.Elem())
	return nil
}

func jsonName(field reflect.StructField) (string, bool) {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name, false
	}
	if tag == "-" {
		return "-", true
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		return field.Name, true
	}
	return name, true
}
