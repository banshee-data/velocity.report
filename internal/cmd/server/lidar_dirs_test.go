package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestAnnotationDirIsItsOwnFlag pins the two properties the annotation pack
// directory needs. It is registered as a flag of its own, so it can be set
// without moving --lidar-pcap-dir: captures usually live on an external
// volume, and deriving the pack path from the capture path would drag pack
// writes onto that volume too. And its default is repo-relative, so it works
// on any machine rather than naming one developer's disk.
func TestAnnotationDirIsItsOwnFlag(t *testing.T) {
	got := flagDefault(t, "lidar-annotation-dir")
	if filepath.IsAbs(got) {
		t.Errorf("lidar-annotation-dir default = %q: defaults are repo-relative so they work on any machine", got)
	}
	if !strings.HasSuffix(got, "annotation-packs") {
		t.Errorf("lidar-annotation-dir default = %q, want it to name the annotation-packs directory", got)
	}
}

func TestResolveLidarDirMakesRelativePathsAbsolute(t *testing.T) {
	// The safe-directory check compares cleaned absolute paths, so a relative
	// value has to be resolved before it can act as a boundary.
	got := resolveLidarDir("../sensor_data/lidar/vrlog", "VRLOG", func(string, ...any) {})
	if !filepath.IsAbs(got) {
		t.Fatalf("resolveLidarDir returned %q, want an absolute path", got)
	}
	if strings.Contains(got, "..") {
		t.Errorf("resolveLidarDir returned %q, want the traversal resolved away", got)
	}
	if !strings.HasSuffix(got, filepath.Join("sensor_data", "lidar", "vrlog")) {
		t.Errorf("resolveLidarDir returned %q, want it to still name the configured directory", got)
	}
}

func TestResolveLidarDirLeavesAnAbsolutePathAlone(t *testing.T) {
	const dir = "/var/lib/velocity/vrlog"
	if got := resolveLidarDir(dir, "VRLOG", func(string, ...any) {}); got != dir {
		t.Errorf("resolveLidarDir(%q) = %q, want it unchanged", dir, got)
	}
}

// TestResolveLidarDirIsSilentOnSuccess names what it actually checks.
//
// The warning path cannot be reached portably: filepath.Abs only fails when
// the working directory cannot be read. So this pins the other half — that a
// resolvable path logs nothing — which is what would break if the warning
// were moved out of the error branch and fired on every call.
func TestResolveLidarDirIsSilentOnSuccess(t *testing.T) {
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }

	for _, dir := range []string{"", ".", "../sensor_data/lidar/vrlog", "/var/lib/velocity"} {
		resolveLidarDir(dir, "VRLOG", logf)
	}
	if len(logged) != 0 {
		t.Errorf("resolveLidarDir logged %v for resolvable paths, want silence", logged)
	}
}

func flagDefault(t *testing.T, name string) string {
	t.Helper()
	f := serveFlags.Lookup(name)
	if f == nil {
		t.Fatalf("flag %q is not registered", name)
	}
	return f.DefValue
}
