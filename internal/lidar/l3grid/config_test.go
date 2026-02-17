package l3grid

import (
	"testing"
	"time"
)

func TestDefaultBackgroundConfig(t *testing.T) {
	cfg := DefaultBackgroundConfig()

	// Structural: all numeric fields are within valid ranges.
	if cfg.UpdateFraction <= 0 || cfg.UpdateFraction > 1 {
		t.Errorf("UpdateFraction must be in (0, 1], got %f", cfg.UpdateFraction)
	}
	if cfg.ClosenessSensitivity <= 0 {
		t.Errorf("ClosenessSensitivity must be positive, got %f", cfg.ClosenessSensitivity)
	}
	if cfg.SafetyMargin < 0 {
		t.Errorf("SafetyMargin must be non-negative, got %f", cfg.SafetyMargin)
	}
	if cfg.FreezeDuration < 0 {
		t.Errorf("FreezeDuration must be non-negative, got %v", cfg.FreezeDuration)
	}
	if cfg.NeighborConfirmation < 0 || cfg.NeighborConfirmation > 8 {
		t.Errorf("NeighborConfirmation must be in [0, 8], got %d", cfg.NeighborConfirmation)
	}
	if cfg.NoiseRelativeFraction < 0 || cfg.NoiseRelativeFraction > 1 {
		t.Errorf("NoiseRelativeFraction must be in [0, 1], got %f", cfg.NoiseRelativeFraction)
	}
	if cfg.MinConfidenceFloor < 0 {
		t.Errorf("MinConfidenceFloor must be non-negative, got %d", cfg.MinConfidenceFloor)
	}
	if cfg.SettlingPeriod < 0 {
		t.Errorf("SettlingPeriod must be non-negative, got %v", cfg.SettlingPeriod)
	}
	if cfg.WarmupDuration < 0 {
		t.Errorf("WarmupDuration must be non-negative, got %v", cfg.WarmupDuration)
	}
	if cfg.WarmupMinFrames < 0 {
		t.Errorf("WarmupMinFrames must be non-negative, got %d", cfg.WarmupMinFrames)
	}
	if cfg.SnapshotInterval <= 0 {
		t.Errorf("SnapshotInterval must be positive, got %v", cfg.SnapshotInterval)
	}

	// The config produced from defaults must pass its own validation.
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config must pass Validate(): %v", err)
	}
}

func TestBackgroundConfig_Validate_Valid(t *testing.T) {
	cfg := DefaultBackgroundConfig()

	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid, got error: %v", err)
	}
}

func TestBackgroundConfig_Validate_InvalidUpdateFraction(t *testing.T) {
	tests := []struct {
		name  string
		value float32
	}{
		{"zero", 0},
		{"negative", -0.1},
		{"greater than 1", 1.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultBackgroundConfig()
			cfg.UpdateFraction = tc.value
			if err := cfg.Validate(); err == nil {
				t.Error("expected error for invalid UpdateFraction")
			}
		})
	}
}

func TestBackgroundConfig_Validate_InvalidClosenessSensitivity(t *testing.T) {
	cfg := DefaultBackgroundConfig()
	cfg.ClosenessSensitivity = -1.0

	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative ClosenessSensitivity")
	}
}

func TestBackgroundConfig_Validate_InvalidSafetyMargin(t *testing.T) {
	cfg := DefaultBackgroundConfig()
	cfg.SafetyMargin = -0.5

	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative SafetyMargin")
	}
}

func TestBackgroundConfig_Validate_InvalidNeighborConfirmation(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"negative", -1},
		{"greater than 8", 9},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultBackgroundConfig()
			cfg.NeighborConfirmation = tc.value
			if err := cfg.Validate(); err == nil {
				t.Error("expected error for invalid NeighborConfirmation")
			}
		})
	}
}

func TestBackgroundConfig_Validate_InvalidNoiseRelativeFraction(t *testing.T) {
	tests := []struct {
		name  string
		value float32
	}{
		{"negative", -0.1},
		{"greater than 1", 1.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultBackgroundConfig()
			cfg.NoiseRelativeFraction = tc.value
			if err := cfg.Validate(); err == nil {
				t.Error("expected error for invalid NoiseRelativeFraction")
			}
		})
	}
}

func TestBackgroundConfig_Validate_NegativeDurations(t *testing.T) {
	tests := []struct {
		name     string
		modifier func(*BackgroundConfig)
	}{
		{"FreezeDuration", func(c *BackgroundConfig) { c.FreezeDuration = -1 * time.Second }},
		{"SettlingPeriod", func(c *BackgroundConfig) { c.SettlingPeriod = -1 * time.Minute }},
		{"WarmupDuration", func(c *BackgroundConfig) { c.WarmupDuration = -1 * time.Second }},
		{"SnapshotInterval", func(c *BackgroundConfig) { c.SnapshotInterval = -1 * time.Hour }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultBackgroundConfig()
			tc.modifier(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for negative %s", tc.name)
			}
		})
	}
}

func TestBackgroundConfig_Validate_NegativeInts(t *testing.T) {
	tests := []struct {
		name     string
		modifier func(*BackgroundConfig)
	}{
		{"WarmupMinFrames", func(c *BackgroundConfig) { c.WarmupMinFrames = -1 }},
		{"ChangeThresholdSnapshot", func(c *BackgroundConfig) { c.ChangeThresholdSnapshot = -1 }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultBackgroundConfig()
			tc.modifier(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for negative %s", tc.name)
			}
		})
	}
}

func TestBackgroundConfig_ToBackgroundParams(t *testing.T) {
	cfg := DefaultBackgroundConfig()
	cfg.UpdateFraction = 0.05
	cfg.ClosenessSensitivity = 6.0
	cfg.SafetyMargin = 0.3
	cfg.FreezeDuration = 10 * time.Second
	cfg.NeighborConfirmation = 5
	cfg.NoiseRelativeFraction = 0.02
	cfg.SeedFromFirstObservation = false
	cfg.SettlingPeriod = 10 * time.Minute
	cfg.WarmupDuration = 1 * time.Minute
	cfg.WarmupMinFrames = 200
	cfg.SnapshotInterval = 1 * time.Hour
	cfg.ChangeThresholdSnapshot = 50

	params := cfg.ToBackgroundParams()

	if params.BackgroundUpdateFraction != 0.05 {
		t.Errorf("expected UpdateFraction 0.05, got %f", params.BackgroundUpdateFraction)
	}
	if params.ClosenessSensitivityMultiplier != 6.0 {
		t.Errorf("expected ClosenessSensitivity 6.0, got %f", params.ClosenessSensitivityMultiplier)
	}
	if params.SafetyMarginMeters != 0.3 {
		t.Errorf("expected SafetyMargin 0.3, got %f", params.SafetyMarginMeters)
	}
	if params.FreezeDurationNanos != (10 * time.Second).Nanoseconds() {
		t.Errorf("expected FreezeDurationNanos %d, got %d", (10 * time.Second).Nanoseconds(), params.FreezeDurationNanos)
	}
	if params.NeighborConfirmationCount != 5 {
		t.Errorf("expected NeighborConfirmation 5, got %d", params.NeighborConfirmationCount)
	}
	if params.NoiseRelativeFraction != 0.02 {
		t.Errorf("expected NoiseRelativeFraction 0.02, got %f", params.NoiseRelativeFraction)
	}
	if params.SeedFromFirstObservation != false {
		t.Error("expected SeedFromFirstObservation false")
	}
	if params.SettlingPeriodNanos != (10 * time.Minute).Nanoseconds() {
		t.Errorf("expected SettlingPeriodNanos %d, got %d", (10 * time.Minute).Nanoseconds(), params.SettlingPeriodNanos)
	}
	if params.WarmupDurationNanos != (1 * time.Minute).Nanoseconds() {
		t.Errorf("expected WarmupDurationNanos %d, got %d", (1 * time.Minute).Nanoseconds(), params.WarmupDurationNanos)
	}
	if params.WarmupMinFrames != 200 {
		t.Errorf("expected WarmupMinFrames 200, got %d", params.WarmupMinFrames)
	}
	if params.SnapshotIntervalNanos != (1 * time.Hour).Nanoseconds() {
		t.Errorf("expected SnapshotIntervalNanos %d, got %d", (1 * time.Hour).Nanoseconds(), params.SnapshotIntervalNanos)
	}
	if params.ChangeThresholdForSnapshot != 50 {
		t.Errorf("expected ChangeThreshold 50, got %d", params.ChangeThresholdForSnapshot)
	}
}

func TestBackgroundConfig_FluentAPI(t *testing.T) {
	cfg := DefaultBackgroundConfig().
		WithUpdateFraction(0.1).
		WithClosenessSensitivity(5.0).
		WithSafetyMargin(0.2).
		WithFreezeDuration(3 * time.Second).
		WithNeighborConfirmation(4).
		WithNoiseRelativeFraction(0.03).
		WithSeedFromFirstObservation(false).
		WithSettlingPeriod(3 * time.Minute).
		WithWarmupDuration(20 * time.Second).
		WithWarmupMinFrames(50).
		WithSnapshotInterval(30 * time.Minute).
		WithChangeThresholdSnapshot(25)

	if cfg.UpdateFraction != 0.1 {
		t.Errorf("expected UpdateFraction 0.1, got %f", cfg.UpdateFraction)
	}
	if cfg.ClosenessSensitivity != 5.0 {
		t.Errorf("expected ClosenessSensitivity 5.0, got %f", cfg.ClosenessSensitivity)
	}
	if cfg.SafetyMargin != 0.2 {
		t.Errorf("expected SafetyMargin 0.2, got %f", cfg.SafetyMargin)
	}
	if cfg.FreezeDuration != 3*time.Second {
		t.Errorf("expected FreezeDuration 3s, got %v", cfg.FreezeDuration)
	}
	if cfg.NeighborConfirmation != 4 {
		t.Errorf("expected NeighborConfirmation 4, got %d", cfg.NeighborConfirmation)
	}
	if cfg.NoiseRelativeFraction != 0.03 {
		t.Errorf("expected NoiseRelativeFraction 0.03, got %f", cfg.NoiseRelativeFraction)
	}
	if cfg.SeedFromFirstObservation != false {
		t.Error("expected SeedFromFirstObservation false")
	}
	if cfg.SettlingPeriod != 3*time.Minute {
		t.Errorf("expected SettlingPeriod 3m, got %v", cfg.SettlingPeriod)
	}
	if cfg.WarmupDuration != 20*time.Second {
		t.Errorf("expected WarmupDuration 20s, got %v", cfg.WarmupDuration)
	}
	if cfg.WarmupMinFrames != 50 {
		t.Errorf("expected WarmupMinFrames 50, got %d", cfg.WarmupMinFrames)
	}
	if cfg.SnapshotInterval != 30*time.Minute {
		t.Errorf("expected SnapshotInterval 30m, got %v", cfg.SnapshotInterval)
	}
	if cfg.ChangeThresholdSnapshot != 25 {
		t.Errorf("expected ChangeThresholdSnapshot 25, got %d", cfg.ChangeThresholdSnapshot)
	}
}

func TestBackgroundConfig_Validate_EdgeCases(t *testing.T) {
	// Edge case: UpdateFraction exactly 1 should be valid
	cfg := DefaultBackgroundConfig()
	cfg.UpdateFraction = 1.0
	if err := cfg.Validate(); err != nil {
		t.Errorf("UpdateFraction=1.0 should be valid, got: %v", err)
	}

	// Edge case: NeighborConfirmation exactly 0 and 8 should be valid
	cfg = DefaultBackgroundConfig()
	cfg.NeighborConfirmation = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("NeighborConfirmation=0 should be valid, got: %v", err)
	}

	cfg.NeighborConfirmation = 8
	if err := cfg.Validate(); err != nil {
		t.Errorf("NeighborConfirmation=8 should be valid, got: %v", err)
	}

	// Edge case: NoiseRelativeFraction exactly 0 and 1 should be valid
	cfg = DefaultBackgroundConfig()
	cfg.NoiseRelativeFraction = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("NoiseRelativeFraction=0 should be valid, got: %v", err)
	}

	cfg.NoiseRelativeFraction = 1.0
	if err := cfg.Validate(); err != nil {
		t.Errorf("NoiseRelativeFraction=1.0 should be valid, got: %v", err)
	}

	// Edge case: Zero durations should be valid (disables the feature)
	cfg = DefaultBackgroundConfig()
	cfg.FreezeDuration = 0
	cfg.SettlingPeriod = 0
	cfg.WarmupDuration = 0
	cfg.SnapshotInterval = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Zero durations should be valid, got: %v", err)
	}
}
