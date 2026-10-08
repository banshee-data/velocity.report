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
	index := ws.loadIndexedCaptures()
	extents := make([]sqlite.CaseSequenceExtent, 0, len(paths))
	for _, path := range paths {
		resolved, err := ws.resolvePCAPPath(path)
		if err != nil {
			return nil, err
		}
		extent, err := ws.captureExtent(index, path, resolved)
		if err != nil {
			return nil, err
		}
		extents = append(extents, extent)
	}
	return sqlite.ValidateCaseSequence(extents)
}

// captureExtent obtains one capture's packet-time bounds, from the index if it
// holds that exact file and by reading the file if it does not.
func (ws *Server) captureExtent(index indexedCaptures, requested, resolved string) (sqlite.CaseSequenceExtent, error) {
	// Any probed port will do: authoring checks that the captures abut, and
	// the probe found the port each one was recorded on.
	if indexed, ok := index.extent(resolved, 0); ok {
		return sqlite.CaseSequenceExtent{
			PCAPFile:    requested,
			FirstPacket: indexed.FirstPacket,
			LastPacket:  indexed.LastPacket,
			PacketCount: indexed.PacketCount,
		}, nil
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
