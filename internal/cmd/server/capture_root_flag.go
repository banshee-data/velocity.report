package server

import "strings"

// captureRootList collects a repeatable --lidar-capture-root flag.
//
// Capture roots are the volumes the capture index scans. They are process
// configuration, deliberately not something the web API can add to: the
// safe-directory boundary that stops replay reading arbitrary server paths is
// only a boundary while the set of readable roots is fixed outside the API. The
// UI selects among these; an operator adds one by restarting with another flag.
type captureRootList []string

// String renders the roots for flag package diagnostics.
func (c *captureRootList) String() string {
	if c == nil {
		return ""
	}
	return strings.Join(*c, ",")
}

// Set appends a root. Blank values are ignored so a stray empty flag does not
// become an empty root path.
func (c *captureRootList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*c = append(*c, value)
	return nil
}
