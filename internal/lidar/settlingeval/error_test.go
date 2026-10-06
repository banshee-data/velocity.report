package settlingeval

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func restoreEvaluationSeams(t *testing.T) {
	t.Helper()
	originalBackgroundConfig := backgroundConfigForEvaluation
	originalLoadParser := loadEvaluationPandarConfig
	originalNewManager := newEvaluationBackgroundManager
	t.Cleanup(func() {
		backgroundConfigForEvaluation = originalBackgroundConfig
		loadEvaluationPandarConfig = originalLoadParser
		newEvaluationBackgroundManager = originalNewManager
	})
}

// TestRun_BadTuningConfig covers the tuning-load error branch (no pcap I/O, so
// it runs in the default tag-free suite).
func TestRun_BadTuningConfig(t *testing.T) {
	_, err := Run(Config{PCAPFile: "x.pcap", TuningFile: filepath.Join(t.TempDir(), "nonexistent.json"), SensorID: "s", UDPPort: 2369})
	if err == nil {
		t.Error("expected tuning-load error")
	}
}

func TestRun_SetupErrors(t *testing.T) {
	t.Run("invalid background config", func(t *testing.T) {
		restoreEvaluationSeams(t)
		backgroundConfigForEvaluation = func(*config.TuningConfig) *l3grid.BackgroundConfig {
			return &l3grid.BackgroundConfig{UpdateFraction: 0}
		}
		if _, err := Run(Config{PCAPFile: "capture.pcap", SensorID: "sensor", UDPPort: 2369}); err == nil {
			t.Fatal("expected background validation error")
		}
	})

	t.Run("parser config", func(t *testing.T) {
		restoreEvaluationSeams(t)
		loadEvaluationPandarConfig = func() (*parse.Pandar40PConfig, error) {
			return nil, errors.New("parser config failed")
		}
		if _, err := Run(Config{PCAPFile: "capture.pcap", SensorID: "sensor", UDPPort: 2369}); err == nil {
			t.Fatal("expected parser config error")
		}
	})

	t.Run("background manager", func(t *testing.T) {
		if _, err := Run(Config{PCAPFile: "capture.pcap", SensorID: "", UDPPort: 2369}); err == nil {
			t.Fatal("expected background manager construction error")
		}
	})

	t.Run("ring elevations", func(t *testing.T) {
		restoreEvaluationSeams(t)
		newEvaluationBackgroundManager = func(sensorID string, _, _ int, params l3grid.BackgroundParams) *l3grid.BackgroundManager {
			return l3grid.NewBackgroundManagerDI(sensorID, 1, 1, params, nil)
		}
		if _, err := Run(Config{PCAPFile: "capture.pcap", SensorID: "sensor", UDPPort: 2369}); err == nil {
			t.Fatal("expected ring elevations error")
		}
	})
}
