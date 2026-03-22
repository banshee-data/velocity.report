package serialmux

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/banshee-data/velocity.report/internal/db"
)

var (
	currentStateMu sync.RWMutex
	currentState   map[string]any
)

// CurrentStateSnapshot returns a shallow copy of the latest config values
// received from the device. Returns nil if no config has been received yet.
// Callers must treat the returned map and all nested values (maps, slices, etc.)
// as read-only. Code that needs to modify the data should first deep-copy it.
func CurrentStateSnapshot() map[string]any {
	currentStateMu.RLock()
	defer currentStateMu.RUnlock()
	if currentState == nil {
		return nil
	}
	out := make(map[string]any, len(currentState))
	for k, v := range currentState {
		out[k] = v
	}
	return out
}

// resetCurrentState clears the current config state. Used by tests.
func resetCurrentState() {
	currentStateMu.Lock()
	defer currentStateMu.Unlock()
	currentState = nil
}

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

	currentStateMu.Lock()
	if currentState == nil {
		currentState = make(map[string]any)
	}
	for k, v := range configValues {
		currentState[k] = v
	}
	currentStateMu.Unlock()

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
