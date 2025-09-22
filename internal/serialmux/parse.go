package serialmux

import "strings"

const (
	EventTypeRadarObject = "radar_object"
	EventTypeRawData     = "raw_data"
	EventTypeConfig      = "config"
	EventTypeUnknown     = "unknown"
)

// ClassifyPayload inspects a payload string and returns a simple event type
// token. The classification is intentionally conservative and mirrors the
// previous logic used in handlers.
func ClassifyPayload(payload string) string {
	if strings.Contains(payload, "end_time") || strings.Contains(payload, "classifier") {
		return EventTypeRadarObject
	}
	if strings.Contains(payload, "magnitude") || strings.Contains(payload, "speed") {
		return EventTypeRawData
	}
	if strings.HasPrefix(payload, "{") {
		return EventTypeConfig
	}
	return EventTypeUnknown
}
