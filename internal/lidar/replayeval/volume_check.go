package replayeval

import (
	"log"
	"os"
	"path/filepath"
	"syscall"
)

// deviceID returns the device a path's filesystem entry lives on, so two
// paths can be compared for "same physical volume" without knowing anything
// about mount points or drive letters. It resolves path if that exact entry
// does not exist yet (an evidence file replay is about to create), since the
// device is a property of the containing filesystem, not of the file.
func deviceID(path string) (dev uint64, ok bool) {
	info, err := os.Stat(path)
	if err != nil {
		parent := filepath.Dir(path)
		if parent == path {
			return 0, false
		}
		return deviceID(parent)
	}
	stat, isStatT := info.Sys().(*syscall.Stat_t)
	if !isStatT {
		return 0, false
	}
	return uint64(stat.Dev), true
}

// warnIfSharedVolume logs a warning when a pcap source and an evidence output
// resolve to the same device as a mounted, non-root volume.
//
// Reading pcap packets and writing observations, estimates, and recorded VRLOG
// frames to the same physical disk queue them against each other: replay
// throughput on a shared drive falls well below what an isolated read or an
// isolated write achieves on it, silently, with no error and no counter that
// points at the cause. This was found the hard way running the 24-site S2
// evidence corpus: pcap sources and evidence output both lived on the same
// external volume, and replay only reached its expected throughput once
// evidence output moved to a separate device.
//
// The check is against the *root* device, not literally "/Volumes": a mounted
// external drive is what this guards against regardless of the operating
// system's mount-path convention, and comparing device IDs directly also
// catches a second external volume the evidence happens to share, not only
// the one pcap reads from.
func warnIfSharedVolume(pcapPaths []string, evidencePaths []string) {
	warnIfSharedVolumeWith(pcapPaths, evidencePaths, deviceID, log.Printf)
}

// warnIfSharedVolumeWith takes the device resolver and log sink as
// parameters so a test can supply a deterministic fake filesystem instead of
// depending on this machine's actual mounted volumes.
func warnIfSharedVolumeWith(
	pcapPaths []string, evidencePaths []string,
	resolve func(string) (uint64, bool), logf func(string, ...any),
) {
	rootDev, haveRootDev := resolve("/")

	pcapDeviceSource := make(map[uint64]string)
	for _, p := range pcapPaths {
		if p == "" {
			continue
		}
		dev, ok := resolve(p)
		if !ok || (haveRootDev && dev == rootDev) {
			continue
		}
		if _, seen := pcapDeviceSource[dev]; !seen {
			pcapDeviceSource[dev] = p
		}
	}
	if len(pcapDeviceSource) == 0 {
		return
	}

	warned := make(map[string]bool)
	for _, e := range evidencePaths {
		if e == "" {
			continue
		}
		dev, ok := resolve(e)
		if !ok {
			continue
		}
		src, shared := pcapDeviceSource[dev]
		if !shared || warned[e] {
			continue
		}
		warned[e] = true
		logf("WARNING: evidence output %s and pcap source %s are on the same mounted volume. "+
			"Reads and writes will contend for that disk's I/O, and replay will run slower than its "+
			"throughput history predicts with no error to explain why. Point evidence output "+
			"(-out, -observations-db, -evidence-dir) at a separate device from the pcap source.", e, src)
	}
}
