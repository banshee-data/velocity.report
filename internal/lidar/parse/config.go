package parse

import (
	"embed"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

//go:embed sensor_configs/*.csv
var embeddedConfigs embed.FS

// LoadPandar40PConfig loads configuration from embedded CSV files
func LoadPandar40PConfig() (*Pandar40PConfig, error) {
	return LoadEmbeddedPandar40PConfig()
}

// LoadEmbeddedPandar40PConfig loads configuration from embedded CSV files only
func LoadEmbeddedPandar40PConfig() (*Pandar40PConfig, error) {
	config := &Pandar40PConfig{}

	// Load embedded angle corrections
	err := loadEmbeddedAngleCorrections(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded angle corrections: %v", err)
	}

	// Load embedded firetime corrections
	err = loadEmbeddedFiretimeCorrections(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded firetime corrections: %v", err)
	}

	return config, nil
}

// loadEmbeddedAngleCorrections loads angle correction data from embedded CSV
func loadEmbeddedAngleCorrections(config *Pandar40PConfig) error {
	file, err := embeddedConfigs.Open("sensor_configs/Pandar40P_Angle Correction File.csv")
	if err != nil {
		return fmt.Errorf("failed to open embedded angle correction file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read embedded CSV: %v", err)
	}

	return parseAngleCorrections(records, config)
}

// parseAngleCorrections parses angle correction records (shared by file and embedded loading)
func parseAngleCorrections(records [][]string, config *Pandar40PConfig) error {
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

// loadEmbeddedFiretimeCorrections loads firetime correction data from embedded CSV
func loadEmbeddedFiretimeCorrections(config *Pandar40PConfig) error {
	file, err := embeddedConfigs.Open("sensor_configs/Pandar40P_Firetime Correction File.csv")
	if err != nil {
		return fmt.Errorf("failed to open embedded firetime correction file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read embedded CSV: %v", err)
	}

	return parseFiretimeCorrections(records, config)
}

// parseFiretimeCorrections parses firetime correction records (shared by file and embedded loading)
func parseFiretimeCorrections(records [][]string, config *Pandar40PConfig) error {
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

// DefaultPandar40PConfig returns a default configuration using embedded CSV files
func DefaultPandar40PConfig() *Pandar40PConfig {
	config, err := LoadEmbeddedPandar40PConfig()
	if err != nil {
		// Return empty config if embedded loading fails (shouldn't happen in normal operation)
		return &Pandar40PConfig{}
	}
	return config
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

// ConfigureTimestampMode configures the parser's timestamp mode based on environment variable
// LIDAR_TIMESTAMP_MODE. Valid values are: "system", "gps", "internal".
// If not set or invalid, defaults to "system" mode.
func ConfigureTimestampMode(parser *Pandar40PParser) {
	timestampMode := os.Getenv("LIDAR_TIMESTAMP_MODE")
	switch timestampMode {
	case "system":
		parser.SetTimestampMode(TimestampModeSystemTime)
		log.Println("LiDAR timestamp mode: System time")
	case "gps":
		parser.SetTimestampMode(TimestampModeGPS)
		log.Println("LiDAR timestamp mode: GPS (requires GPS-synchronized LiDAR)")
	case "internal":
		parser.SetTimestampMode(TimestampModeInternal)
		log.Println("LiDAR timestamp mode: Internal (device boot time)")
	case "lidar":
		parser.SetTimestampMode(TimestampModeLiDAR)
		log.Println("LiDAR timestamp mode: LiDAR native (DateTime + Timestamp fields)")
	default:
		// Default to SystemTime for stability until PTP hardware is available
		parser.SetTimestampMode(TimestampModeSystemTime)
		log.Println("LiDAR timestamp mode: System time (default - stable until PTP hardware available)")
	}
}
