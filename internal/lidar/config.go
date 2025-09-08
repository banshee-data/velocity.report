package lidar

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadPandar40PConfig loads configuration from CSV files
func LoadPandar40PConfig(angleCorrectionFile, firetimeCorrectionFile string) (*Pandar40PConfig, error) {
	config := &Pandar40PConfig{}

	// Load angle corrections
	err := loadAngleCorrections(angleCorrectionFile, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load angle corrections: %v", err)
	}

	// Load firetime corrections
	err = loadFiretimeCorrections(firetimeCorrectionFile, config)
	if err != nil {
		return nil, fmt.Errorf("failed to load firetime corrections: %v", err)
	}

	return config, nil
}

// loadAngleCorrections loads angle correction data from CSV
func loadAngleCorrections(filename string, config *Pandar40PConfig) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open angle correction file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %v", err)
	}

	// Skip header row
	if len(records) < 2 {
		return fmt.Errorf("insufficient data in angle correction file")
	}

	// Validate header
	header := records[0]
	if len(header) != 3 ||
		strings.ToLower(header[0]) != "channel" ||
		strings.ToLower(header[1]) != "elevation" ||
		strings.ToLower(header[2]) != "azimuth" {
		return fmt.Errorf("invalid header in angle correction file, expected: Channel,Elevation,Azimuth")
	}

	// Parse data rows
	for i, record := range records[1:] {
		if len(record) != 3 {
			return fmt.Errorf("invalid record at line %d: expected 3 fields", i+2)
		}

		channel, err := strconv.Atoi(record[0])
		if err != nil {
			return fmt.Errorf("invalid channel number at line %d: %v", i+2, err)
		}

		elevation, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return fmt.Errorf("invalid elevation at line %d: %v", i+2, err)
		}

		azimuth, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			return fmt.Errorf("invalid azimuth at line %d: %v", i+2, err)
		}

		// Validate channel range (1-40)
		if channel < 1 || channel > CHANNELS_PER_BLOCK {
			return fmt.Errorf("channel number %d out of range (1-%d) at line %d", channel, CHANNELS_PER_BLOCK, i+2)
		}

		// Store in zero-based array
		config.AngleCorrections[channel-1] = AngleCorrection{
			Channel:   channel,
			Elevation: elevation,
			Azimuth:   azimuth,
		}
	}

	return nil
}

// loadFiretimeCorrections loads firetime correction data from CSV
func loadFiretimeCorrections(filename string, config *Pandar40PConfig) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open firetime correction file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %v", err)
	}

	// Skip header row
	if len(records) < 2 {
		return fmt.Errorf("insufficient data in firetime correction file")
	}

	// Validate header
	header := records[0]
	if len(header) != 2 ||
		strings.ToLower(header[0]) != "channel" ||
		!strings.Contains(strings.ToLower(header[1]), "fire time") {
		return fmt.Errorf("invalid header in firetime correction file, expected: Channel,fire time(μs)")
	}

	// Parse data rows
	for i, record := range records[1:] {
		if len(record) != 2 {
			return fmt.Errorf("invalid record at line %d: expected 2 fields", i+2)
		}

		channel, err := strconv.Atoi(record[0])
		if err != nil {
			return fmt.Errorf("invalid channel number at line %d: %v", i+2, err)
		}

		fireTime, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return fmt.Errorf("invalid fire time at line %d: %v", i+2, err)
		}

		// Validate channel range (1-40)
		if channel < 1 || channel > CHANNELS_PER_BLOCK {
			return fmt.Errorf("channel number %d out of range (1-%d) at line %d", channel, CHANNELS_PER_BLOCK, i+2)
		}

		// Store in zero-based array
		config.FiretimeCorrections[channel-1] = FiretimeCorrection{
			Channel:  channel,
			FireTime: fireTime,
		}
	}

	return nil
}

// DefaultPandar40PConfig returns a default configuration (embedded fallback)
func DefaultPandar40PConfig() *Pandar40PConfig {
	// This could contain hardcoded values from the CSV files as a fallback
	// For now, return an empty config that would need to be loaded from files
	return &Pandar40PConfig{}
}

// ValidateConfig validates that the configuration is complete
func (config *Pandar40PConfig) Validate() error {
	// Check that all channels have angle corrections
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		if config.AngleCorrections[i].Channel == 0 {
			return fmt.Errorf("missing angle correction for channel %d", i+1)
		}
		if config.FiretimeCorrections[i].Channel == 0 {
			return fmt.Errorf("missing firetime correction for channel %d", i+1)
		}
	}
	return nil
}
