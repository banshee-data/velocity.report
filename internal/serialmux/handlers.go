package serialmux

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/banshee-data/velocity.report/internal/db"
)

// CurrentState holds the latest config values received from the device
// and is intentionally package-level so admin routes or tests can inspect it.
var CurrentState map[string]any

func HandleRadarObject(d *db.DB, payload string) error {
	log.Printf("Raw RadarObject Line: %+v", payload)
	// log to the database and return error if present
	return d.RecordRadarObject(payload)
}

func HandleRawData(d *db.DB, payload string) error {
	log.Printf("Raw Data Line: %+v", payload)
	// TODO: disable via flag/config
	return d.RecordRawData(payload)
}

func HandleConfigResponse(payload string) error {
	var configValues map[string]any

	if err := json.Unmarshal([]byte(payload), &configValues); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// update the current state with the new config values
	if CurrentState == nil {
		CurrentState = make(map[string]any)
	}
	for k, v := range configValues {
		CurrentState[k] = v
	}

	// log the current line
	log.Printf("Config Line: %+v", payload)

	return nil
}

func HandleEvent(d *db.DB, payload string) error {
	switch ClassifyPayload(payload) {
	case EventTypeRadarObject:
		if err := HandleRadarObject(d, payload); err != nil {
			return fmt.Errorf("failed to handle RadarObject event: %v", err)
		}
	case EventTypeRawData:
		if err := HandleRawData(d, payload); err != nil {
			return fmt.Errorf("failed to handle raw data event: %v", err)
		}
	case EventTypeConfig:
		if err := HandleConfigResponse(payload); err != nil {
			return fmt.Errorf("failed to handle config response: %v", err)
		}
	default:
		log.Printf("unknown event type: %s", payload)
	}
	return nil
}
