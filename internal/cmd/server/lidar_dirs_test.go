package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The point of these tests is the independence, not the paths.
//
// Recordings and plots used to be derived from --lidar-pcap-dir, so pointing
// the capture path at an external volume moved the writes there too and a
// replay contended with itself for one device. A future tidy-up that folds
// them back into one path would reintroduce exactly that, and every other test
// in this package would still pass.

// TestVRLogAndPlotsDirsAreIndependentFlags fails if either write path is
// derived from the capture path again.
func TestVRLogAndPlotsDirsAreIndependentFlags(t *testing.T) {
	for _, name := range []string{"lidar-vrlog-dir", "lidar-plots-dir"} {
		if serveFlags.Lookup(name) == nil {
			t.Errorf("flag %q is not registered: write paths must be settable without --lidar-pcap-dir", name)
		}
	}
}

// TestWritePathDefaultsDoNotFollowTheCaptureDir pins the decoupling: setting
// only the capture directory must leave the recording and plot directories
// where they were.
func TestWritePathDefaultsDoNotFollowTheCaptureDir(t *testing.T) {
	vrlogDefault := flagDefault(t, "lidar-vrlog-dir")
	plotsDefault := flagDefault(t, "lidar-plots-dir")
	pcapDefault := flagDefault(t, "lidar-pcap-dir")

	// Both defaults are the path the old derived form produced under the
	// default capture directory, so a deployment setting neither flag is
	// unchanged. Asserting equality here is what keeps that promise.
	if want := filepath.Join(pcapDefault, "vrlog"); vrlogDefault != want {
		t.Errorf("vrlog default = %q, want %q: existing deployments would move", vrlogDefault, want)
	}
	if want := filepath.Join(pcapDefault, "plots"); plotsDefault != want {
		t.Errorf("plots default = %q, want %q: existing deployments would move", plotsDefault, want)
	}

	// And the decoupling itself: an external capture volume must not drag the
	// writes onto it. This is the contention the split exists to prevent.
	const external = "/Volumes/lidar/lidar"
	for _, dir := range []string{vrlogDefault, plotsDefault} {
		if strings.HasPrefix(dir, external) {
			t.Errorf("write default %q sits under the capture volume", dir)
		}
	}
}

// TestWritePathDefaultsAreNotAbsoluteMachinePaths guards against someone
// "fixing" a default by hardcoding their own disk.
func TestWritePathDefaultsAreNotAbsoluteMachinePaths(t *testing.T) {
	for _, name := range []string{"lidar-vrlog-dir", "lidar-plots-dir", "lidar-pcap-dir"} {
		if got := flagDefault(t, name); filepath.IsAbs(got) {
			t.Errorf("%s default = %q: defaults are repo-relative so they work on any machine", name, got)
		}
	}
}

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
