package api

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// SensorModel defines capabilities and initialisation commands for a radar sensor
type SensorModel struct {
	Slug               string   `json:"slug"`
	DisplayName        string   `json:"display_name"`
	HasDoppler         bool     `json:"has_doppler"`
	HasFMCW            bool     `json:"has_fmcw"`
	HasDistance        bool     `json:"has_distance"`
	DefaultBaudRate    int      `json:"default_baud_rate"`
	SupportedBaudRates []int    `json:"supported_baud_rates"`
	InitCommands       []string `json:"init_commands"`
	Description        string   `json:"description"`
}

//go:embed sensor_models.json
var sensorModelsJSON []byte

// SupportedSensorModels is the application-level registry of sensor models,
// loaded from the embedded sensor_models.json asset at package init.
var SupportedSensorModels = loadSensorModels()

func loadSensorModels() map[string]SensorModel {
	var file struct {
		Models []SensorModel `json:"models"`
	}
	if err := json.Unmarshal(sensorModelsJSON, &file); err != nil {
		panic(fmt.Sprintf("sensor_models.json: %v", err))
	}
	out := make(map[string]SensorModel, len(file.Models))
	for _, m := range file.Models {
		out[m.Slug] = m
	}
	return out
}

// GetSensorModel looks up a sensor model by slug
func GetSensorModel(slug string) (SensorModel, bool) {
	model, ok := SupportedSensorModels[slug]
	return model, ok
}

// GetAllSensorModels returns a slice of all supported sensor models
func GetAllSensorModels() []SensorModel {
	models := make([]SensorModel, 0, len(SupportedSensorModels))
	for _, model := range SupportedSensorModels {
		models = append(models, model)
	}
	return models
}
