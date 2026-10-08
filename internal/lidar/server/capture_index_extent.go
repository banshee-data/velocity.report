package server

import (
	"os"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The capture index probes every capture for its first and last packet and its
// packet count. That is everything joining captures into a sequence needs, and
// obtaining it otherwise means reading the whole file. A motion pass, a clip
// replay and clip authoring each used to read every capture once to count it
// before doing their own work; these helpers let them take the probe instead,
// whenever it still describes the file.

// extentFromIndex is a capture's extent as the index probed it, or false when
// the probe cannot be trusted for this read: it failed or never ran, the file
// has a different size or modification time from the one indexed, or the
// caller reads a port other than the one probed (zero accepts any port).
func extentFromIndex(f sqlite.CaptureFile, path string, udpPort int,
	stat func(string) (os.FileInfo, error)) (capseq.Segment, bool) {

	if f.ProbeState != sqlite.ProbeStateOK || f.FirstPacketNs == nil || f.LastPacketNs == nil ||
		f.PacketCount == nil || *f.PacketCount <= 0 {
		return capseq.Segment{}, false
	}
	if udpPort > 0 && (f.UDPPort == nil || *f.UDPPort != udpPort) {
		return capseq.Segment{}, false
	}
	info, err := stat(path)
	if err != nil || info.Size() != f.SizeBytes || info.ModTime().UnixNano() != f.ModifiedAtNs {
		return capseq.Segment{}, false
	}
	return capseq.Segment{
		Path:        path,
		FirstPacket: time.Unix(0, *f.FirstPacketNs),
		LastPacket:  time.Unix(0, *f.LastPacketNs),
		PacketCount: uint64(*f.PacketCount),
	}, true
}

// indexedExtents returns a session's captures as the index probed them, for a
// motion pass to join without reading each one to count it. It returns nil, so
// the pass counts them itself, unless every capture's probe can be trusted. The
// pass reads on the port the index recorded, so any probed port is accepted.
func indexedExtents(files []sqlite.CaptureFile, paths []string,
	stat func(string) (os.FileInfo, error)) []capseq.Segment {

	if len(files) == 0 || len(files) != len(paths) {
		return nil
	}
	extents := make([]capseq.Segment, 0, len(files))
	for i, f := range files {
		e, ok := extentFromIndex(f, paths[i], 0, stat)
		if !ok {
			return nil
		}
		extents = append(extents, e)
	}
	return extents
}

// indexedCaptures is the capture index keyed by each capture's absolute path,
// for callers that name captures by path rather than by session.
type indexedCaptures struct {
	byPath map[string]sqlite.CaptureFile
	stat   func(string) (os.FileInfo, error)
}

// loadIndexedCaptures reads the index once for a batch of lookups. Without a
// database it is empty and every lookup misses, so callers count as before.
//
// Paths are joined to each root with its symlinks resolved, as resolvePCAPPath
// resolves the paths it returns, so the two agree on macOS's /var and
// /private/var. A capture reachable through two roots, one nested in the other,
// is kept from whichever root ListRoots gives first: the configured one.
func (ws *Server) loadIndexedCaptures() indexedCaptures {
	ic := indexedCaptures{byPath: map[string]sqlite.CaptureFile{}, stat: os.Stat}
	store, err := ws.captureStore()
	if err != nil {
		return ic
	}
	roots, err := store.ListRoots()
	if err != nil {
		return ic
	}
	for _, root := range roots {
		dir, err := filepath.EvalSymlinks(root.Path)
		if err != nil {
			continue
		}
		files, err := store.ListFiles(root.RootID)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.Present {
				continue
			}
			path := filepath.Join(dir, filepath.FromSlash(f.RelPath))
			if _, seen := ic.byPath[path]; !seen {
				ic.byPath[path] = f
			}
		}
	}
	return ic
}

// extent is the indexed extent of the capture at path, a symlink-resolved
// absolute path such as resolvePCAPPath returns, read on udpPort (zero for
// any). It misses when the index does not hold that exact file: matching on
// the file name alone would confuse copies of a capture kept in two folders.
func (ic indexedCaptures) extent(path string, udpPort int) (capseq.Segment, bool) {
	f, ok := ic.byPath[path]
	if !ok {
		return capseq.Segment{}, false
	}
	return extentFromIndex(f, path, udpPort, ic.stat)
}
