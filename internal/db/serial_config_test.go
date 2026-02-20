package db

import (
	"os"
	"testing"
)

func TestSerialConfig(t *testing.T) {
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

	// Test GetSerialConfigs - fresh DB may have no configs
	configs, err := db.GetSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get serial configs: %v", err)
	}
	initialCount := len(configs)

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
	if len(enabledConfigs) != initialCount+1 {
		t.Fatalf("Expected %d enabled configs, got %d", initialCount+1, len(enabledConfigs))
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

	// Verify enabled count decreased
	enabledConfigs, err = db.GetEnabledSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get enabled serial configs after update: %v", err)
	}
	if len(enabledConfigs) != initialCount {
		t.Fatalf("Expected %d enabled config after update, got %d", initialCount, len(enabledConfigs))
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

	// Verify we're back to initial count
	configs, err = db.GetSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get serial configs after delete: %v", err)
	}
	if len(configs) != initialCount {
		t.Fatalf("Expected %d configs after delete, got %d", initialCount, len(configs))
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

	// Create a config first
	firstConfig := &SerialConfig{
		Name:        "Test Config",
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    19200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     true,
		Description: "First config",
		SensorModel: "ops243-a",
	}
	_, err = db.CreateSerialConfig(firstConfig)
	if err != nil {
		t.Fatalf("Failed to create first config: %v", err)
	}

	// Try to create a config with the same name
	duplicateConfig := &SerialConfig{
		Name:        "Test Config",
		PortPath:    "/dev/ttyUSB1",
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

func TestSerialConfigUpdateNotFound(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_update_nf_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer database.Close()

	err = database.UpdateSerialConfig(&SerialConfig{
		ID:          99999,
		Name:        "Does Not Exist",
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    19200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     true,
		SensorModel: "ops243-a",
	})
	if err == nil {
		t.Error("Expected error updating non-existent config, got nil")
	}
}

func TestSerialConfigDeleteNotFound(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_delete_nf_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer database.Close()

	err = database.DeleteSerialConfig(99999)
	if err == nil {
		t.Error("Expected error deleting non-existent config, got nil")
	}
}

func TestSerialConfigGetNonExistent(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_get_ne_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer database.Close()

	cfg, err := database.GetSerialConfig(99999)
	if err != nil {
		t.Fatalf("Expected nil error for non-existent config, got %v", err)
	}
	if cfg != nil {
		t.Errorf("Expected nil config for non-existent ID, got %+v", cfg)
	}
}

func TestSerialConfigCreateDisabledFlag(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_disabled_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer database.Close()

	// Create with Enabled=false
	cfg := &SerialConfig{
		Name:        "Disabled Config",
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    19200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     false,
		SensorModel: "ops243-a",
	}
	id, err := database.CreateSerialConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to create disabled config: %v", err)
	}

	// GetEnabledSerialConfigs should not include it
	enabled, err := database.GetEnabledSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get enabled configs: %v", err)
	}
	for _, c := range enabled {
		if c.ID == int(id) {
			t.Error("Disabled config should not appear in enabled list")
		}
	}

	// But GetSerialConfigs should include it
	all, err := database.GetSerialConfigs()
	if err != nil {
		t.Fatalf("Failed to get all configs: %v", err)
	}
	found := false
	for _, c := range all {
		if c.ID == int(id) {
			found = true
			if c.Enabled {
				t.Error("Config should have Enabled=false")
			}
		}
	}
	if !found {
		t.Error("Expected to find disabled config in all configs list")
	}
}

func TestSerialConfigGetSerialConfigs_DBError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_db_err_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	database.Close()

	_, err = database.GetSerialConfigs()
	if err == nil {
		t.Error("Expected error from closed DB, got nil")
	}
}

func TestSerialConfigGetSerialConfig_DBError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_get_db_err_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	database.Close()

	_, err = database.GetSerialConfig(1)
	if err == nil {
		t.Error("Expected error from closed DB, got nil")
	}
}

func TestSerialConfigGetEnabledSerialConfigs_DBError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_enabled_db_err_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	database.Close()

	_, err = database.GetEnabledSerialConfigs()
	if err == nil {
		t.Error("Expected error from closed DB, got nil")
	}
}

func TestSerialConfigUpdateSerialConfig_DBError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_update_db_err_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	database.Close()

	err = database.UpdateSerialConfig(&SerialConfig{
		ID: 1, Name: "X", PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a",
		BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	if err == nil {
		t.Error("Expected error from closed DB, got nil")
	}
}

func TestSerialConfigDeleteSerialConfig_DBError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_delete_db_err_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	database.Close()

	err = database.DeleteSerialConfig(1)
	if err == nil {
		t.Error("Expected error from closed DB, got nil")
	}
}

func TestSerialConfigCreateSerialConfig_DBError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_serial_config_create_db_err_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	database.Close()

	_, err = database.CreateSerialConfig(&SerialConfig{
		Name: "X", PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a",
		BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	if err == nil {
		t.Error("Expected error from closed DB, got nil")
	}
}
