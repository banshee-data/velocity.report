package server

import (
	"path/filepath"
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
