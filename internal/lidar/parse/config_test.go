package parse

import (
	"testing"
)

func TestLoadEmbeddedPandar40PConfig(t *testing.T) {
	config, err := LoadEmbeddedPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load embedded config: %v", err)
	}

	// Validate configuration
	err = config.Validate()
	if err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	// Test that we have all channels
	if len(config.AngleCorrections) != CHANNELS_PER_BLOCK {
		t.Errorf("Expected %d angle corrections, got %d", CHANNELS_PER_BLOCK, len(config.AngleCorrections))
	}

	if len(config.FiretimeCorrections) != CHANNELS_PER_BLOCK {
		t.Errorf("Expected %d firetime corrections, got %d", CHANNELS_PER_BLOCK, len(config.FiretimeCorrections))
	}

	t.Logf("Successfully loaded embedded configuration for %d channels", CHANNELS_PER_BLOCK)
}

func TestPandar40PConfig_Validate(t *testing.T) {
	// Test valid configuration
	config := &Pandar40PConfig{}

	// Fill with valid data
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		config.AngleCorrections[i] = AngleCorrection{
			Channel:   i + 1,
			Elevation: float64(i-20) * 0.5, // Range from -10 to +10 degrees
			Azimuth:   -1.0,                // Reasonable azimuth offset
		}

		config.FiretimeCorrections[i] = FiretimeCorrection{
			Channel:  i + 1,
			FireTime: float64(i) * -0.1, // Small time offsets
		}
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Valid configuration should pass validation: %v", err)
	}
}

func TestPandar40PConfig_ValidateInvalidChannel(t *testing.T) {
	config := &Pandar40PConfig{}

	// Fill with invalid channel numbers
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		config.AngleCorrections[i] = AngleCorrection{
			Channel:   i, // Invalid: should be 1-40, not 0-39
			Elevation: 0,
			Azimuth:   0,
		}

		config.FiretimeCorrections[i] = FiretimeCorrection{
			Channel:  i, // Invalid: should be 1-40, not 0-39
			FireTime: 0,
		}
	}

	err := config.Validate()
	if err == nil {
		t.Error("Invalid configuration should fail validation")
	}

	if err != nil {
		t.Logf("Expected validation error: %v", err)
	}
}

func TestPandar40PConfig_ValidateExtremeValues(t *testing.T) {
	config := &Pandar40PConfig{}

	// Fill with extreme but potentially valid values
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		config.AngleCorrections[i] = AngleCorrection{
			Channel:   i + 1,
			Elevation: 89.0,  // Very high elevation
			Azimuth:   180.0, // Large azimuth offset
		}

		config.FiretimeCorrections[i] = FiretimeCorrection{
			Channel:  i + 1,
			FireTime: -1000.0, // Large negative time offset
		}
	}

	err := config.Validate()
	if err != nil {
		t.Logf("Extreme values validation result: %v", err)
		// This might be expected depending on validation rules
	}
}

func TestAngleCorrection_Struct(t *testing.T) {
	correction := AngleCorrection{
		Channel:   1,
		Elevation: 15.21,
		Azimuth:   -1.042,
	}

	if correction.Channel != 1 {
		t.Errorf("Expected channel 1, got %d", correction.Channel)
	}

	if correction.Elevation != 15.21 {
		t.Errorf("Expected elevation 15.21, got %f", correction.Elevation)
	}

	if correction.Azimuth != -1.042 {
		t.Errorf("Expected azimuth -1.042, got %f", correction.Azimuth)
	}
}

func TestFiretimeCorrection_Struct(t *testing.T) {
	correction := FiretimeCorrection{
		Channel:  1,
		FireTime: -3.62,
	}

	if correction.Channel != 1 {
		t.Errorf("Expected channel 1, got %d", correction.Channel)
	}

	if correction.FireTime != -3.62 {
		t.Errorf("Expected fire time -3.62, got %f", correction.FireTime)
	}
}

func TestLoadPandar40PConfig_Wrapper(t *testing.T) {
	// Test that the wrapper function works the same as the direct function
	config1, err1 := LoadPandar40PConfig()
	if err1 != nil {
		t.Fatalf("LoadPandar40PConfig failed: %v", err1)
	}

	config2, err2 := LoadEmbeddedPandar40PConfig()
	if err2 != nil {
		t.Fatalf("LoadEmbeddedPandar40PConfig failed: %v", err2)
	}

	// Both should have the same number of channels
	if len(config1.AngleCorrections) != len(config2.AngleCorrections) {
		t.Errorf("Configs have different number of angle corrections: %d vs %d",
			len(config1.AngleCorrections), len(config2.AngleCorrections))
	}

	if len(config1.FiretimeCorrections) != len(config2.FiretimeCorrections) {
		t.Errorf("Configs have different number of firetime corrections: %d vs %d",
			len(config1.FiretimeCorrections), len(config2.FiretimeCorrections))
	}

	// Check that first channel data matches
	if config1.AngleCorrections[0].Channel != config2.AngleCorrections[0].Channel {
		t.Error("First channel number doesn't match between wrapper and direct function")
	}
}

func BenchmarkLoadEmbeddedPandar40PConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := LoadEmbeddedPandar40PConfig()
		if err != nil {
			b.Fatalf("Config loading failed: %v", err)
		}
	}
}

func BenchmarkPandar40PConfig_Validate(b *testing.B) {
	config, err := LoadEmbeddedPandar40PConfig()
	if err != nil {
		b.Fatalf("Failed to load config: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := config.Validate()
		if err != nil {
			b.Fatalf("Validation failed: %v", err)
		}
	}
}
