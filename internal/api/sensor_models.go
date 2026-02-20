package api

// SensorModel defines capabilities and initialisation commands for a radar sensor
type SensorModel struct {
	Slug            string   `json:"slug"`
	DisplayName     string   `json:"display_name"`
	HasDoppler      bool     `json:"has_doppler"`
	HasFMCW         bool     `json:"has_fmcw"`
	HasDistance      bool     `json:"has_distance"`
	DefaultBaudRate int      `json:"default_baud_rate"`
	InitCommands    []string `json:"init_commands"`
	Description     string   `json:"description"`
}

// SupportedSensorModels is the application-level registry of sensor models
var SupportedSensorModels = map[string]SensorModel{
	"ops243-a": {
		Slug:            "ops243-a",
		DisplayName:     "OmniPreSense OPS243-A",
		HasDoppler:      true,
		HasFMCW:         false,
		HasDistance:      false,
		DefaultBaudRate: 19200,
		InitCommands:    []string{"AX", "OJ", "OS", "OM", "OH", "OC"},
		Description:     "Doppler radar with speed measurement only",
	},
	"ops243-c": {
		Slug:            "ops243-c",
		DisplayName:     "OmniPreSense OPS243-C",
		HasDoppler:      true,
		HasFMCW:         true,
		HasDistance:      true,
		DefaultBaudRate: 19200,
		InitCommands:    []string{"AX", "OJ", "OS", "oD", "OM", "oM", "OH", "OC"},
		Description:     "FMCW radar with both speed and distance measurement",
	},
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
