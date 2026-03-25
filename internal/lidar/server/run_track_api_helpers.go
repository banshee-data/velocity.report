package server

import (
	"strings"
)

// parseRunPath extracts run_id and remaining path segments from /api/lidar/runs/{run_id}/...
func parseRunPath(path string) (runID string, subPath string) {
	trimmed := strings.TrimPrefix(path, "/api/lidar/runs/")
	if trimmed == path {
		// No prefix match, return empty
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	runID = parts[0]
	if len(parts) > 1 {
		subPath = parts[1]
	}
	return
}

// parseTrackPath extracts track_id and action from tracks/{track_id}/{action}
func parseTrackPath(path string) (trackID string, action string) {
	parts := strings.SplitN(path, "/", 2)
	trackID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return
}

func normaliseCommaSeparatedLabelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	normalised := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalised = append(normalised, part)
	}
	return strings.Join(normalised, ",")
}

func normaliseLinkedTrackIDsForRequest(linkedTrackIDs []string) []string {
	if len(linkedTrackIDs) == 0 {
		return nil
	}

	normalised := make([]string, 0, len(linkedTrackIDs))
	for _, linkedTrackID := range linkedTrackIDs {
		linkedTrackID = strings.TrimSpace(linkedTrackID)
		if linkedTrackID == "" {
			continue
		}
		normalised = append(normalised, linkedTrackID)
	}
	if len(normalised) == 0 {
		return nil
	}
	return normalised
}
