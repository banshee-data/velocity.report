package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSegmentSelectorsAreReadOnceFromTheFileNamed(t *testing.T) {
	// This package's directory holds no config/, as a deployed server's
	// working directory does not: the binary's own copy is used.
	if c, err := loadSegmentSelectors(""); err != nil || c.Source != "embedded" {
		t.Fatalf("no file named: %+v %v", c, err)
	}
	repository := filepath.Join("..", "..", "..", "config", "segment-selectors.defaults.json")
	if c, err := loadSegmentSelectors(repository); err != nil || c.Source != filepath.Clean(repository) {
		t.Fatalf("the repository's file: %+v %v", c, err)
	}
	if _, err := loadSegmentSelectors(filepath.Join(t.TempDir(), "selectors.json")); err == nil {
		t.Fatal("a file that is not there was replaced by the defaults")
	}
	if got := flagDefault(t, "lidar-segment-selectors"); got != "" {
		t.Fatalf("lidar-segment-selectors default = %q, want empty", got)
	}
}

// At startup the server says which catalogue it ranks with, or refuses to
// start with none.
func TestTheServerStartsOnlyWithTheCatalogueItWasGiven(t *testing.T) {
	var fatal, logged []string
	record := func(into *[]string) logfFunc {
		return func(format string, args ...any) { *into = append(*into, fmt.Sprintf(format, args...)) }
	}
	c := mustLoadSegmentSelectors("", record(&fatal), record(&logged))
	if c == nil || len(fatal) != 0 || len(logged) != 1 || !strings.HasPrefix(logged[0], "Loaded 7 segment selectors from embedded (sha256:") {
		t.Fatalf("the binary's own copy: %v, fatal %q, logged %q", c, fatal, logged)
	}
	fatal, logged = nil, nil
	missing := filepath.Join(t.TempDir(), "selectors.json")
	if c := mustLoadSegmentSelectors(missing, record(&fatal), record(&logged)); c != nil || len(logged) != 0 ||
		len(fatal) != 1 || !strings.Contains(fatal[0], "Failed to load segment selectors") {
		t.Fatalf("a missing file: %v, fatal %q, logged %q", c, fatal, logged)
	}
}
