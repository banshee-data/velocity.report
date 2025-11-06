package db

import (
	"os"
	"testing"
)

func TestSerialConfig(t *testing.T) {
	// Create a temporary database
	tmpDB, err := os.CreateTemp("", "test_serial_config_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	db, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	// Test GetSerialConfigs - should return default config
	configs, err := db.GetSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get serial configs: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("Expected 1 default config, got %d", len(configs))
	}

	defaultConfig := configs[0]
	if defaultConfig.Name != "Default HAT" {
		t.Errorf("Expected default config name 'Default HAT', got '%s'", defaultConfig.Name)
	}
	if defaultConfig.PortPath != "/dev/ttySC1" {
		t.Errorf("Expected default port '/dev/ttySC1', got '%s'", defaultConfig.PortPath)
	}
	if defaultConfig.BaudRate != 19200 {
		t.Errorf("Expected default baud rate 19200, got %d", defaultConfig.BaudRate)
	}
	if !defaultConfig.Enabled {
		t.Error("Expected default config to be enabled")
	}

	// Test CreateSerialConfig
	newConfig := &SerialConfig{
		Name:        "USB Radar #1",
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    19200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     true,
		Description: "USB-connected radar sensor",
		SensorModel: "ops243-a",
	}

	id, err := db.CreateSerialConfig(newConfig)
	if err != nil {
		t.Fatalf("Failed to create serial config: %v", err)
	}

	if id <= 0 {
		t.Errorf("Expected positive ID, got %d", id)
	}

	// Test GetSerialConfig
	retrieved, err := db.GetSerialConfig(int(id))
	if err != nil {
		t.Fatalf("Failed to get serial config by ID: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected to retrieve config, got nil")
	}

	if retrieved.Name != newConfig.Name {
		t.Errorf("Expected name '%s', got '%s'", newConfig.Name, retrieved.Name)
	}
	if retrieved.PortPath != newConfig.PortPath {
		t.Errorf("Expected port '%s', got '%s'", newConfig.PortPath, retrieved.PortPath)
	}

	// Test GetEnabledSerialConfigs
	enabledConfigs, err := db.GetEnabledSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get enabled serial configs: %v", err)
	}

	if len(enabledConfigs) != 2 {
		t.Fatalf("Expected 2 enabled configs, got %d", len(enabledConfigs))
	}

	// Test UpdateSerialConfig
	retrieved.Description = "Updated description"
	retrieved.Enabled = false
	err = db.UpdateSerialConfig(retrieved)
	if err != nil {
		t.Fatalf("Failed to update serial config: %v", err)
	}

	updated, err := db.GetSerialConfig(int(id))
	if err != nil {
		t.Fatalf("Failed to get updated config: %v", err)
	}

	if updated.Description != "Updated description" {
		t.Errorf("Expected updated description, got '%s'", updated.Description)
	}
	if updated.Enabled {
		t.Error("Expected config to be disabled")
	}

	// Verify only 1 enabled config now
	enabledConfigs, err = db.GetEnabledSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get enabled serial configs after update: %v", err)
	}

	if len(enabledConfigs) != 1 {
		t.Fatalf("Expected 1 enabled config after update, got %d", len(enabledConfigs))
	}

	// Test DeleteSerialConfig
	err = db.DeleteSerialConfig(int(id))
	if err != nil {
		t.Fatalf("Failed to delete serial config: %v", err)
	}

	deleted, err := db.GetSerialConfig(int(id))
	if err != nil {
		t.Fatalf("Failed to check deleted config: %v", err)
	}

	if deleted != nil {
		t.Error("Expected config to be deleted, but it still exists")
	}

	// Verify we're back to just the default config
	configs, err = db.GetSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get serial configs after delete: %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("Expected 1 config after delete, got %d", len(configs))
	}
}

func TestSerialConfigUniqueConstraint(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_unique_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	db, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	// Try to create a config with the same name as the default
	duplicateConfig := &SerialConfig{
		Name:        "Default HAT",
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    19200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     true,
		Description: "Duplicate name",
		SensorModel: "ops243-a",
	}

	_, err = db.CreateSerialConfig(duplicateConfig)
	if err == nil {
		t.Error("Expected error when creating config with duplicate name, got nil")
	}
}

func TestSerialConfigInvalidSensorModel(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_invalid_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	db, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	// Try to create a config with invalid sensor model
	invalidConfig := &SerialConfig{
		Name:        "Invalid Sensor",
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    19200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     true,
		Description: "Invalid sensor model",
		SensorModel: "invalid-model",
	}

	_, err = db.CreateSerialConfig(invalidConfig)
	if err == nil {
		t.Error("Expected error when creating config with invalid sensor model, got nil")
	}
}
