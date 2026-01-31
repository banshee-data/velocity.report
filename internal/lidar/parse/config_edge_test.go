package parse

import (
	"testing"
)

// TestLoadEmbeddedPandar40PConfig_Success tests successful loading of embedded config
func TestLoadEmbeddedPandar40PConfig_Success(t *testing.T) {
	config, err := LoadEmbeddedPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load embedded config: %v", err)
	}

	if config == nil {
		t.Fatal("Config is nil")
	}

	// Verify all 40 channels have corrections
	for i := 0; i < 40; i++ {
		if config.AngleCorrections[i].Channel == 0 {
			t.Errorf("Missing angle correction for channel index %d", i)
		}
		if config.FiretimeCorrections[i].Channel == 0 {
			t.Errorf("Missing firetime correction for channel index %d", i)
		}
	}
}

// TestLoadPandar40PConfig_Success tests loading config through main function
func TestLoadPandar40PConfig_Success(t *testing.T) {
	config, err := LoadPandar40PConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config == nil {
		t.Fatal("Config is nil")
	}
}

// TestDefaultPandar40PConfig_ReturnsValidConfig tests default config creation
func TestDefaultPandar40PConfig_ReturnsValidConfig(t *testing.T) {
	config := DefaultPandar40PConfig()

	if config == nil {
		t.Fatal("DefaultPandar40PConfig returned nil")
	}

	// Validate the config
	err := config.Validate()
	if err != nil {
		t.Errorf("Default config validation failed: %v", err)
	}
}

// TestPandar40PConfig_Validate_AllChannels tests that validation checks all channels
func TestPandar40PConfig_Validate_AllChannels(t *testing.T) {
	// Create config with missing channel in the middle
	config := &Pandar40PConfig{}

	// Fill all channels except channel 20 (index 19)
	for i := 0; i < 40; i++ {
		if i != 19 {
			config.AngleCorrections[i] = AngleCorrection{
				Channel:   i + 1,
				Elevation: float64(i),
				Azimuth:   float64(i),
			}
			config.FiretimeCorrections[i] = FiretimeCorrection{
				Channel:  i + 1,
				FireTime: float64(i),
			}
		}
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for missing channel, got nil")
	}
}

// TestPandar40PConfig_Validate_MissingAngleCorrection tests angle correction validation
func TestPandar40PConfig_Validate_MissingAngleCorrection(t *testing.T) {
	config := &Pandar40PConfig{}

	// Fill firetime corrections but not angle corrections
	for i := 0; i < 40; i++ {
		config.FiretimeCorrections[i] = FiretimeCorrection{
			Channel:  i + 1,
			FireTime: float64(i),
		}
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for missing angle corrections, got nil")
	}
}

// TestPandar40PConfig_Validate_MissingFiretimeCorrection tests firetime correction validation
func TestPandar40PConfig_Validate_MissingFiretimeCorrection(t *testing.T) {
	config := &Pandar40PConfig{}

	// Fill angle corrections but not firetime corrections
	for i := 0; i < 40; i++ {
		config.AngleCorrections[i] = AngleCorrection{
			Channel:   i + 1,
			Elevation: float64(i),
			Azimuth:   float64(i),
		}
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for missing firetime corrections, got nil")
	}
}

// TestPandar40PConfig_EmptyConfig tests validation of completely empty config
func TestPandar40PConfig_EmptyConfig(t *testing.T) {
	config := &Pandar40PConfig{}

	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error for empty config, got nil")
	}
}

// TestElevationsFromConfig_EdgeCase tests extraction of elevations from config
func TestElevationsFromConfig_EdgeCase(t *testing.T) {
	config := DefaultPandar40PConfig()

	elevations := ElevationsFromConfig(config)

	if len(elevations) != 40 {
		t.Errorf("Expected 40 elevations, got %d", len(elevations))
	}

	// Check that elevations are reasonable values (typically -16 to +15 degrees)
	for i, elev := range elevations {
		if elev < -90 || elev > 90 {
			t.Errorf("Elevation %d out of range: %f", i, elev)
		}
	}
}

// TestElevationsFromConfig_NilConfig tests handling of nil config
func TestElevationsFromConfig_NilConfig(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("ElevationsFromConfig panicked with nil config as expected: %v", r)
		}
	}()

	elevations := ElevationsFromConfig(nil)
	if len(elevations) != 0 {
		t.Logf("ElevationsFromConfig returned %d elevations for nil config", len(elevations))
	}
}

// TestElevationsFromConfig_PartialConfig tests handling of partial config
func TestElevationsFromConfig_PartialConfig(t *testing.T) {
	config := &Pandar40PConfig{}

	// Only fill first 10 channels
	for i := 0; i < 10; i++ {
		config.AngleCorrections[i] = AngleCorrection{
			Channel:   i + 1,
			Elevation: float64(i * 2),
			Azimuth:   0,
		}
	}

	elevations := ElevationsFromConfig(config)

	// Should still return 40 elevations (zeros for unfilled)
	if len(elevations) != 40 {
		t.Errorf("Expected 40 elevations, got %d", len(elevations))
	}

	// First 10 should have values
	for i := 0; i < 10; i++ {
		expected := float64(i * 2)
		if elevations[i] != expected {
			t.Errorf("Elevation %d mismatch: expected %f, got %f", i, expected, elevations[i])
		}
	}

	// Rest should be zero
	for i := 10; i < 40; i++ {
		if elevations[i] != 0 {
			t.Errorf("Elevation %d should be 0, got %f", i, elevations[i])
		}
	}
}

// TestParseAngleCorrections_InvalidHeader tests parsing with invalid header
func TestParseAngleCorrections_InvalidHeader(t *testing.T) {
	config := &Pandar40PConfig{}

	testCases := []struct {
		name    string
		records [][]string
	}{
		{
			name:    "empty_records",
			records: [][]string{},
		},
		{
			name:    "header_only",
			records: [][]string{{"Channel", "Elevation", "Azimuth"}},
		},
		{
			name:    "wrong_header",
			records: [][]string{{"Wrong", "Headers", "Here"}, {"1", "2.0", "3.0"}},
		},
		{
			name: "missing_column",
			records: [][]string{
				{"Channel", "Elevation"},
				{"1", "2.0"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseAngleCorrections(tc.records, config)
			if err == nil {
				t.Error("Expected error for invalid records, got nil")
			}
		})
	}
}

// TestParseAngleCorrections_InvalidData tests parsing with invalid data values
func TestParseAngleCorrections_InvalidData(t *testing.T) {
	config := &Pandar40PConfig{}

	testCases := []struct {
		name    string
		records [][]string
	}{
		{
			name: "invalid_channel",
			records: [][]string{
				{"Channel", "Elevation", "Azimuth"},
				{"abc", "2.0", "3.0"},
			},
		},
		{
			name: "invalid_elevation",
			records: [][]string{
				{"Channel", "Elevation", "Azimuth"},
				{"1", "not_a_number", "3.0"},
			},
		},
		{
			name: "invalid_azimuth",
			records: [][]string{
				{"Channel", "Elevation", "Azimuth"},
				{"1", "2.0", "not_a_number"},
			},
		},
		{
			name: "channel_out_of_range_high",
			records: [][]string{
				{"Channel", "Elevation", "Azimuth"},
				{"41", "2.0", "3.0"},
			},
		},
		{
			name: "channel_out_of_range_low",
			records: [][]string{
				{"Channel", "Elevation", "Azimuth"},
				{"0", "2.0", "3.0"},
			},
		},
		{
			name: "negative_channel",
			records: [][]string{
				{"Channel", "Elevation", "Azimuth"},
				{"-1", "2.0", "3.0"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseAngleCorrections(tc.records, config)
			if err == nil {
				t.Error("Expected error for invalid data, got nil")
			}
		})
	}
}

// TestParseFiretimeCorrections_InvalidHeader tests firetime parsing with invalid header
func TestParseFiretimeCorrections_InvalidHeader(t *testing.T) {
	config := &Pandar40PConfig{}

	testCases := []struct {
		name    string
		records [][]string
	}{
		{
			name:    "empty_records",
			records: [][]string{},
		},
		{
			name:    "header_only",
			records: [][]string{{"Channel", "fire time(μs)"}},
		},
		{
			name:    "wrong_header",
			records: [][]string{{"Wrong", "Headers"}, {"1", "2.0"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseFiretimeCorrections(tc.records, config)
			if err == nil {
				t.Error("Expected error for invalid records, got nil")
			}
		})
	}
}

// TestParseFiretimeCorrections_InvalidData tests firetime parsing with invalid data
func TestParseFiretimeCorrections_InvalidData(t *testing.T) {
	config := &Pandar40PConfig{}

	testCases := []struct {
		name    string
		records [][]string
	}{
		{
			name: "invalid_channel",
			records: [][]string{
				{"Channel", "fire time(μs)"},
				{"abc", "2.0"},
			},
		},
		{
			name: "invalid_firetime",
			records: [][]string{
				{"Channel", "fire time(μs)"},
				{"1", "not_a_number"},
			},
		},
		{
			name: "channel_out_of_range",
			records: [][]string{
				{"Channel", "fire time(μs)"},
				{"50", "2.0"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseFiretimeCorrections(tc.records, config)
			if err == nil {
				t.Error("Expected error for invalid data, got nil")
			}
		})
	}
}

// TestParseAngleCorrections_ValidData tests successful parsing
func TestParseAngleCorrections_ValidData(t *testing.T) {
	config := &Pandar40PConfig{}

	records := [][]string{
		{"Channel", "Elevation", "Azimuth"},
	}

	// Add 40 valid channel records
	for i := 1; i <= 40; i++ {
		records = append(records, []string{
			string(rune('0'+i/10)) + string(rune('0'+i%10)),
			"1.5",
			"0.5",
		})
	}

	// Fix: use proper number formatting
	records = [][]string{{"Channel", "Elevation", "Azimuth"}}
	for i := 1; i <= 40; i++ {
		records = append(records, []string{
			intToString(i),
			"1.5",
			"0.5",
		})
	}

	err := parseAngleCorrections(records, config)
	if err != nil {
		t.Fatalf("Failed to parse valid angle corrections: %v", err)
	}

	// Verify all channels were parsed
	for i := 0; i < 40; i++ {
		if config.AngleCorrections[i].Channel != i+1 {
			t.Errorf("Channel %d not correctly parsed", i+1)
		}
	}
}

// TestParseFiretimeCorrections_ValidData tests successful firetime parsing
func TestParseFiretimeCorrections_ValidData(t *testing.T) {
	config := &Pandar40PConfig{}

	records := [][]string{{"Channel", "fire time(μs)"}}
	for i := 1; i <= 40; i++ {
		records = append(records, []string{intToString(i), "1.234"})
	}

	err := parseFiretimeCorrections(records, config)
	if err != nil {
		t.Fatalf("Failed to parse valid firetime corrections: %v", err)
	}

	// Verify all channels were parsed
	for i := 0; i < 40; i++ {
		if config.FiretimeCorrections[i].Channel != i+1 {
			t.Errorf("Channel %d not correctly parsed", i+1)
		}
		if config.FiretimeCorrections[i].FireTime != 1.234 {
			t.Errorf("FireTime for channel %d incorrect: %f", i+1, config.FiretimeCorrections[i].FireTime)
		}
	}
}

// intToString is a simple helper to convert int to string
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
