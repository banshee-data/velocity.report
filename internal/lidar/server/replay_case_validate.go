package server

import (
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// validateCaseFiles checks that a proposed case's captures form one continuous
// stream, and returns the sequence so a caller can report its joins.
//
// The extents come from the capture index when the files are known to it, which
// costs nothing, and from probing the file otherwise, which costs a full read.
// Preferring the index is what makes authoring a case from the Captures page
// immediate rather than a minute-long wait per file.
func (ws *Server) validateCaseFiles(paths []string) (*capseq.Sequence, error) {
	extents := make([]sqlite.CaseSequenceExtent, 0, len(paths))
	for _, path := range paths {
		resolved, err := ws.resolvePCAPPath(path)
		if err != nil {
			return nil, err
		}
		extent, err := ws.captureExtent(path, resolved)
		if err != nil {
			return nil, err
		}
		extents = append(extents, extent)
	}
	return sqlite.ValidateCaseSequence(extents)
}

// captureExtent obtains one capture's packet-time bounds, from the index if it
// is there and by reading the file if it is not.
func (ws *Server) captureExtent(requested, resolved string) (sqlite.CaseSequenceExtent, error) {
	if indexed, ok := ws.indexedExtent(requested); ok {
		indexed.PCAPFile = requested
		return indexed, nil
	}

	// The same prober the index uses, so a capture recorded on another port is
	// read the same way here as it is there. Three callers reaching for the
	// packet extent must not each decide the port question differently.
	extent, err := probeExtent(resolved, ws.udpPort)
	if err != nil {
		return sqlite.CaseSequenceExtent{}, fmt.Errorf("probing %s: %w", requested, err)
	}
	if extent.PacketCount == 0 {
		return sqlite.CaseSequenceExtent{}, fmt.Errorf(
			"%s contains no LiDAR packets", requested)
	}
	return sqlite.CaseSequenceExtent{
		PCAPFile:    requested,
		FirstPacket: time.Unix(0, extent.FirstPacketNs),
		LastPacket:  time.Unix(0, extent.LastPacketNs),
		PacketCount: extent.PacketCount,
	}, nil
}

// indexedExtent looks a capture up in the index by the tail of its path, and
// reports whether it was found with a usable extent.
func (ws *Server) indexedExtent(requested string) (sqlite.CaseSequenceExtent, bool) {
	store, err := ws.captureStore()
	if err != nil {
		return sqlite.CaseSequenceExtent{}, false
	}
	files, err := store.ListFiles("")
	if err != nil {
		return sqlite.CaseSequenceExtent{}, false
	}
	for _, f := range files {
		if !matchesCapturePath(f.RelPath, requested) {
			continue
		}
		if f.FirstPacketNs == nil || f.LastPacketNs == nil || f.ProbeState != sqlite.ProbeStateOK {
			continue
		}
		var count uint64
		if f.PacketCount != nil {
			count = uint64(*f.PacketCount)
		}
		return sqlite.CaseSequenceExtent{
			FirstPacket: time.Unix(0, *f.FirstPacketNs),
			LastPacket:  time.Unix(0, *f.LastPacketNs),
			PacketCount: count,
		}, true
	}
	return sqlite.CaseSequenceExtent{}, false
}

// matchesCapturePath reports whether an indexed relative path refers to the
// same capture a caller named. Callers pass a path relative to the replay safe
// directory and the index stores one relative to its root, so the two agree on
// the tail rather than the whole string.
func matchesCapturePath(indexedRel, requested string) bool {
	if indexedRel == requested {
		return true
	}
	return pathTail(indexedRel) == pathTail(requested)
}

// pathTail is a path's final element, slash- or separator-delimited.
func pathTail(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
