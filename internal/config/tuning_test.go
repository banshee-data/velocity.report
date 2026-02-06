package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultTuningConfig(t *testing.T) {
	cfg := DefaultTuningConfig()

	if cfg.Lidar == nil {
		t.Fatal("Expected Lidar config to be non-nil")
	}

	// Test background defaults
	if cfg.Lidar.Background == nil {
		t.Fatal("Expected Background config to be non-nil")
	}
	bg := cfg.Lidar.Background
	if bg.NoiseRelativeFraction != 0.04 {
		t.Errorf("Expected NoiseRelativeFraction 0.04, got %f", bg.NoiseRelativeFraction)
	}
	if bg.FlushInterval != "60s" {
		t.Errorf("Expected FlushInterval '60s', got '%s'", bg.FlushInterval)
	}
	if bg.FlushDisable != false {
		t.Errorf("Expected FlushDisable false, got %v", bg.FlushDisable)
	}
	if bg.SeedFromFirst != true {
		t.Errorf("Expected SeedFromFirst true, got %v", bg.SeedFromFirst)
	}

	// Test frame builder defaults
	if cfg.Lidar.FrameBuilder == nil {
		t.Fatal("Expected FrameBuilder config to be non-nil")
	}
	fb := cfg.Lidar.FrameBuilder
	if fb.BufferTimeout != "500ms" {
		t.Errorf("Expected BufferTimeout '500ms', got '%s'", fb.BufferTimeout)
	}
	if fb.MinFramePoints != 1000 {
		t.Errorf("Expected MinFramePoints 1000, got %d", fb.MinFramePoints)
	}
}

func TestLoadTuningConfig(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.json")

	// Write test config
	testJSON := `{
  "lidar": {
    "background": {
      "noise_relative_fraction": 0.05,
      "flush_interval": "120s",
      "flush_disable": true,
      "seed_from_first": false
    },
    "frame_builder": {
      "buffer_timeout": "250ms",
      "min_frame_points": 500
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(testJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load the config
	cfg, err := LoadTuningConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values
	if cfg.Lidar == nil {
		t.Fatal("Expected Lidar config to be non-nil")
	}

	bg := cfg.Lidar.Background
	if bg.NoiseRelativeFraction != 0.05 {
		t.Errorf("Expected NoiseRelativeFraction 0.05, got %f", bg.NoiseRelativeFraction)
	}
	if bg.FlushInterval != "120s" {
		t.Errorf("Expected FlushInterval '120s', got '%s'", bg.FlushInterval)
	}
	if bg.FlushDisable != true {
		t.Errorf("Expected FlushDisable true, got %v", bg.FlushDisable)
	}
	if bg.SeedFromFirst != false {
		t.Errorf("Expected SeedFromFirst false, got %v", bg.SeedFromFirst)
	}

	fb := cfg.Lidar.FrameBuilder
	if fb.BufferTimeout != "250ms" {
		t.Errorf("Expected BufferTimeout '250ms', got '%s'", fb.BufferTimeout)
	}
	if fb.MinFramePoints != 500 {
		t.Errorf("Expected MinFramePoints 500, got %d", fb.MinFramePoints)
	}
}

func TestLoadTuningConfigMissing(t *testing.T) {
	_, err := LoadTuningConfig("/nonexistent/path/to/config.json")
	if err == nil {
		t.Error("Expected error when loading missing file, got nil")
	}
}

func TestLoadTuningConfigInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_config.json")

	// Write invalid JSON
	invalidJSON := `{
  "lidar": {
    "background": {
      "noise_relative_fraction": "invalid"
    }
  }
`
	if err := os.WriteFile(configPath, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadTuningConfig(configPath)
	if err == nil {
		t.Error("Expected error when loading invalid JSON, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *TuningConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     DefaultTuningConfig(),
			wantErr: false,
		},
		{
			name: "missing lidar config",
			cfg: &TuningConfig{
				Lidar: nil,
			},
			wantErr: true,
		},
		{
			name: "missing background config",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: nil,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid noise fraction (too low)",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: &BackgroundTuning{
						NoiseRelativeFraction: -0.1,
						FlushInterval:         "60s",
					},
					FrameBuilder: &FrameBuilderTuning{
						BufferTimeout:  "500ms",
						MinFramePoints: 1000,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid noise fraction (too high)",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: &BackgroundTuning{
						NoiseRelativeFraction: 1.5,
						FlushInterval:         "60s",
					},
					FrameBuilder: &FrameBuilderTuning{
						BufferTimeout:  "500ms",
						MinFramePoints: 1000,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid flush interval",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: &BackgroundTuning{
						NoiseRelativeFraction: 0.04,
						FlushInterval:         "invalid",
					},
					FrameBuilder: &FrameBuilderTuning{
						BufferTimeout:  "500ms",
						MinFramePoints: 1000,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing frame builder config",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: &BackgroundTuning{
						NoiseRelativeFraction: 0.04,
						FlushInterval:         "60s",
					},
					FrameBuilder: nil,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid buffer timeout",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: &BackgroundTuning{
						NoiseRelativeFraction: 0.04,
						FlushInterval:         "60s",
					},
					FrameBuilder: &FrameBuilderTuning{
						BufferTimeout:  "invalid",
						MinFramePoints: 1000,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "negative min frame points",
			cfg: &TuningConfig{
				Lidar: &LidarTuning{
					Background: &BackgroundTuning{
						NoiseRelativeFraction: 0.04,
						FlushInterval:         "60s",
					},
					FrameBuilder: &FrameBuilderTuning{
						BufferTimeout:  "500ms",
						MinFramePoints: -1,
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetFlushInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     time.Duration
	}{
		{
			name:     "60 seconds",
			interval: "60s",
			want:     60 * time.Second,
		},
		{
			name:     "2 minutes",
			interval: "2m",
			want:     2 * time.Minute,
		},
		{
			name:     "1 hour",
			interval: "1h",
			want:     1 * time.Hour,
		},
		{
			name:     "empty string",
			interval: "",
			want:     0,
		},
		{
			name:     "invalid duration returns 0",
			interval: "invalid",
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg := &BackgroundTuning{
				FlushInterval: tt.interval,
			}
			got := bg.GetFlushInterval()
			if got != tt.want {
				t.Errorf("GetFlushInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBufferTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{
			name:    "500 milliseconds",
			timeout: "500ms",
			want:    500 * time.Millisecond,
		},
		{
			name:    "1 second",
			timeout: "1s",
			want:    1 * time.Second,
		},
		{
			name:    "250 milliseconds",
			timeout: "250ms",
			want:    250 * time.Millisecond,
		},
		{
			name:    "empty string",
			timeout: "",
			want:    0,
		},
		{
			name:    "invalid duration returns 0",
			timeout: "invalid",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &FrameBuilderTuning{
				BufferTimeout: tt.timeout,
			}
			got := fb.GetBufferTimeout()
			if got != tt.want {
				t.Errorf("GetBufferTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadDefaultConfigFile(t *testing.T) {
	cfg, err := LoadTuningConfig("../../config/tuning.defaults.json")
	if err != nil {
		t.Fatalf("Failed to load defaults: %v", err)
	}
	if cfg.Lidar.Background.NoiseRelativeFraction != 0.04 {
		t.Errorf("Expected 0.04, got %f", cfg.Lidar.Background.NoiseRelativeFraction)
	}
}

func TestLoadExampleConfigFile(t *testing.T) {
	cfg, err := LoadTuningConfig("../../config/tuning.example.json")
	if err != nil {
		t.Fatalf("Failed to load example: %v", err)
	}
	if cfg.Lidar.Background.NoiseRelativeFraction != 0.06 {
		t.Errorf("Expected 0.06, got %f", cfg.Lidar.Background.NoiseRelativeFraction)
	}
	if cfg.Lidar.FrameBuilder.MinFramePoints != 500 {
		t.Errorf("Expected 500, got %d", cfg.Lidar.FrameBuilder.MinFramePoints)
	}
}

func TestLoadTuningConfigPartial(t *testing.T) {
	// Partial config: only override noise; everything else should keep defaults.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.json")

	partialJSON := `{
  "lidar": {
    "background": {
      "noise_relative_fraction": 0.08
    },
    "frame_builder": {}
  }
}`
	if err := os.WriteFile(configPath, []byte(partialJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadTuningConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load partial config: %v", err)
	}

	// Overridden value
	if cfg.Lidar.Background.NoiseRelativeFraction != 0.08 {
		t.Errorf("Expected overridden NoiseRelativeFraction 0.08, got %f", cfg.Lidar.Background.NoiseRelativeFraction)
	}
	// Default values should be preserved
	if cfg.Lidar.Background.FlushInterval != "60s" {
		t.Errorf("Expected default FlushInterval '60s', got '%s'", cfg.Lidar.Background.FlushInterval)
	}
	if cfg.Lidar.Background.SeedFromFirst != true {
		t.Errorf("Expected default SeedFromFirst true, got %v", cfg.Lidar.Background.SeedFromFirst)
	}
	if cfg.Lidar.FrameBuilder.BufferTimeout != "500ms" {
		t.Errorf("Expected default BufferTimeout '500ms', got '%s'", cfg.Lidar.FrameBuilder.BufferTimeout)
	}
	if cfg.Lidar.FrameBuilder.MinFramePoints != 1000 {
		t.Errorf("Expected default MinFramePoints 1000, got %d", cfg.Lidar.FrameBuilder.MinFramePoints)
	}
}

func TestLoadTuningConfigRejectsPathTraversal(t *testing.T) {
	// Path traversal with ".." is allowed since this is a CLI-only flag,
	// but the file must still have a .json extension.
	_, err := LoadTuningConfig("../../etc/passwd")
	if err == nil {
		t.Error("Expected error for non-.json path, got nil")
	}
}

func TestLoadTuningConfigRejectsNonJSON(t *testing.T) {
	_, err := LoadTuningConfig("/some/path/config.yaml")
	if err == nil {
		t.Error("Expected error for non-.json extension, got nil")
	}
}
