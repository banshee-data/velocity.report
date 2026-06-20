//go:build pcap
// +build pcap

package pcapsplit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

// defaultSnaplen is used when the source handle reports no snapshot length.
const defaultSnaplen uint32 = 262144

// WriteSegments performs pass 2: it re-reads the input PCAP and copies each
// packet, unmodified, into the output file for the segment whose time interval
// contains the packet's capture timestamp. Output writers are opened lazily so
// empty segments produce no file, and packets stream straight to disk (no
// buffering). It updates each segment's PacketCount in place.
//
// Segments must be the time-ordered, contiguous output of BuildSegments.
func WriteSegments(cfg SplitConfig, segs []Segment) error {
	if len(segs) == 0 {
		return nil
	}

	handle, err := pcap.OpenOffline(cfg.PCAPFile)
	if err != nil {
		return fmt.Errorf("open pcap: %w", err)
	}
	defer handle.Close()

	if err := handle.SetBPFFilter(fmt.Sprintf("udp port %d", cfg.UDPPort)); err != nil {
		return fmt.Errorf("set bpf filter: %w", err)
	}

	linkType := handle.LinkType()
	snaplen := uint32(handle.SnapLen())
	if snaplen == 0 {
		snaplen = defaultSnaplen
	}

	writers := make([]*segWriter, len(segs))
	defer func() {
		for _, w := range writers {
			if w != nil {
				_ = w.close()
			}
		}
	}()

	source := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range source.Packets() {
		data := packet.Data()
		if len(data) == 0 {
			continue
		}
		ci := packet.Metadata().CaptureInfo
		// Re-reading can leave CaptureLength/Length unset on some inputs;
		// pcapgo rejects a mismatch, so align them with the raw bytes.
		ci.CaptureLength = len(data)
		if ci.Length == 0 {
			ci.Length = len(data)
		}

		idx := segmentIndexForTime(segs, ci.Timestamp)
		if idx < 0 {
			continue
		}
		if writers[idx] == nil {
			w, err := newSegWriter(cfg.OutputDir, segs[idx].Filename, snaplen, linkType)
			if err != nil {
				return err
			}
			writers[idx] = w
		}
		if err := writers[idx].w.WritePacket(ci, data); err != nil {
			return fmt.Errorf("write packet to %s: %w", segs[idx].Filename, err)
		}
		segs[idx].PacketCount++
	}
	return nil
}

// segWriter pairs an output file with its legacy-pcap writer.
type segWriter struct {
	f *os.File
	w *pcapgo.Writer
}

func newSegWriter(dir, name string, snaplen uint32, lt layers.LinkType) (*segWriter, error) {
	// Reject anything that isn't a bare filename so a crafted --prefix (e.g.
	// "../") cannot write outside the output directory.
	if name != filepath.Base(name) {
		return nil, fmt.Errorf("unsafe segment filename %q", name)
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("create segment file %s: %w", name, err)
	}
	// Nanosecond writer preserves PCAPNG-sourced sub-microsecond timestamps.
	w := pcapgo.NewWriterNanos(f)
	if err := w.WriteFileHeader(snaplen, lt); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write file header for %s: %w", name, err)
	}
	return &segWriter{f: f, w: w}, nil
}

func (s *segWriter) close() error {
	return s.f.Close()
}
