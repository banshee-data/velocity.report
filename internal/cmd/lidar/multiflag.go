//go:build pcap
// +build pcap

package lidar

import "strings"

// multiFlag collects a repeatable string flag in the order it was given.
type multiFlag []string

// String renders the values for flag package diagnostics.
func (m *multiFlag) String() string {
	if m == nil {
		return ""
	}
	return strings.Join(*m, ",")
}

// Set appends a value, ignoring blanks so a stray empty flag does not become an
// empty path.
func (m *multiFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*m = append(*m, value)
	return nil
}
