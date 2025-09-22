package serialmux

import (
    "encoding/json"
    "fmt"
    "log"
    "strings"

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
    if strings.Contains(payload, "end_time") || strings.Contains(payload, "classifier") {
        // This is a rollup event
        if err := HandleRadarObject(d, payload); err != nil {
            return fmt.Errorf("failed to handle RadarObject event: %v", err)
        }
    } else if strings.Contains(payload, `magnitude`) || strings.Contains(payload, `speed`) {
        // This is a raw data event
        if err := HandleRawData(d, payload); err != nil {
            return fmt.Errorf("failed to handle raw data event: %v", err)
        }
    } else if strings.HasPrefix(payload, `{`) {
        // This is a config response
        if err := HandleConfigResponse(payload); err != nil {
            return fmt.Errorf("failed to handle config response: %v", err)
        }
    } else {
        // Unknown event type
        log.Printf("unknown event type: %s", payload)
    }
    return nil
}
