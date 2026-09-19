package replayeval

import "testing"

// fakeDevices is a resolver for tests: a path not present is "unresolvable"
// (mirrors a stat failure), matching how deviceID behaves for a path this
// session cannot see.
type fakeDevices map[string]uint64

func (f fakeDevices) resolve(path string) (uint64, bool) {
	dev, ok := f[path]
	return dev, ok
}

func captureLogs(t *testing.T) (*[]string, func(string, ...any)) {
	t.Helper()
	lines := &[]string{}
	return lines, func(format string, args ...any) {
		*lines = append(*lines, format)
	}
}

func TestWarnIfSharedVolume_SamePcapAndEvidenceDeviceWarns(t *testing.T) {
	devices := fakeDevices{
		"/":                   1, // root device
		"/mnt/lidar/cap.pcap": 2, // mounted volume, distinct from root
		"/mnt/lidar/evidence": 2, // same mounted volume as the pcap source
	}
	lines, logf := captureLogs(t)

	warnIfSharedVolumeWith(
		[]string{"/mnt/lidar/cap.pcap"},
		[]string{"/mnt/lidar/evidence"},
		devices.resolve, logf,
	)

	if len(*lines) != 1 {
		t.Fatalf("expected exactly one warning, got %d: %v", len(*lines), *lines)
	}
}

func TestWarnIfSharedVolume_SeparateDevicesStaysQuiet(t *testing.T) {
	devices := fakeDevices{
		"/":                   1,
		"/mnt/lidar/cap.pcap": 2, // pcap source: mounted volume
		"/data/evidence":      1, // evidence output: root device, not shared with the pcap volume
	}
	lines, logf := captureLogs(t)

	warnIfSharedVolumeWith(
		[]string{"/mnt/lidar/cap.pcap"},
		[]string{"/data/evidence"},
		devices.resolve, logf,
	)

	if len(*lines) != 0 {
		t.Fatalf("expected no warning when evidence is on a different device, got: %v", *lines)
	}
}

func TestWarnIfSharedVolume_PcapOnRootDeviceStaysQuiet(t *testing.T) {
	// The pcap source living on the primary disk is not the failure mode this
	// warns about, even if evidence output lands right beside it.
	devices := fakeDevices{
		"/":              1,
		"/home/cap.pcap": 1,
		"/home/evidence": 1,
	}
	lines, logf := captureLogs(t)

	warnIfSharedVolumeWith(
		[]string{"/home/cap.pcap"},
		[]string{"/home/evidence"},
		devices.resolve, logf,
	)

	if len(*lines) != 0 {
		t.Fatalf("expected no warning when the pcap source is on the root device, got: %v", *lines)
	}
}

func TestWarnIfSharedVolume_MultipleEvidencePathsEachChecked(t *testing.T) {
	// Mirrors the real call site: OutDir and ObservationDBPath are checked
	// independently, since a caller may point only one of them at the pcap
	// volume.
	devices := fakeDevices{
		"/":                     1,
		"/mnt/lidar/cap.pcap":   2,
		"/mnt/lidar/vrlog":      2, // shares the pcap volume
		"/data/observations.db": 1, // does not
	}
	lines, logf := captureLogs(t)

	warnIfSharedVolumeWith(
		[]string{"/mnt/lidar/cap.pcap"},
		[]string{"/mnt/lidar/vrlog", "/data/observations.db"},
		devices.resolve, logf,
	)

	if len(*lines) != 1 {
		t.Fatalf("expected exactly one warning (for the shared path only), got %d: %v", len(*lines), *lines)
	}
}

func TestWarnIfSharedVolume_EmptyEvidencePathIgnored(t *testing.T) {
	// ObservationDBPath is frequently empty (observation persistence is
	// opt-in); an empty path must never be resolved or warned about.
	devices := fakeDevices{
		"/":                   1,
		"/mnt/lidar/cap.pcap": 2,
	}
	lines, logf := captureLogs(t)

	warnIfSharedVolumeWith(
		[]string{"/mnt/lidar/cap.pcap"},
		[]string{"/mnt/lidar/out", ""},
		devices.resolve, logf,
	)

	if len(*lines) != 0 {
		t.Fatalf("expected no warning: /mnt/lidar/out is unresolvable and \"\" must be skipped, got: %v", *lines)
	}
}

func TestDeviceID_ResolvesThroughMissingLeaf(t *testing.T) {
	// A pcap replay's evidence file (e.g. observations.db) usually does not
	// exist yet; deviceID must resolve its containing directory instead of
	// failing outright.
	dir := t.TempDir()
	dirDev, ok := deviceID(dir)
	if !ok {
		t.Fatalf("expected to resolve the device of an existing directory")
	}
	fileDev, ok := deviceID(dir + "/does-not-exist-yet.db")
	if !ok {
		t.Fatalf("expected to resolve the device of a not-yet-created file via its parent")
	}
	if dirDev != fileDev {
		t.Fatalf("expected the not-yet-created file to resolve to its parent directory's device")
	}
}
