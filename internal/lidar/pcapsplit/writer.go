//go:build pcap
// +build pcap

package pcapsplit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
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
// Only packets inside cfg's window (StartSeconds, DurationSeconds) are copied,
// by the rule the analysis applied when it read them — network.ReplayWindow,
// per file — so the segments together hold exactly the packets the analysis
// (and a --dry-run) counted, and none from outside the window.
//
// Segments must be the time-ordered, contiguous output of BuildSegments.
func WriteSegments(cfg SplitConfig, segs []Segment) error {
	if len(segs) == 0 {
		return nil
	}
	steps, err := planReplay(cfg)
	if err != nil {
		return err
	}
	return writeSegmentSteps(cfg, steps, segs)
}

// writeSegmentSteps is WriteSegments over read steps already planned, normally
// the analysis's own (Analysis.Steps), which spares a multi-file run from
// counting its captures a second time.
func writeSegmentSteps(cfg SplitConfig, steps []capseq.ReadStep, segs []Segment) error {
	if len(segs) == 0 {
		return nil
	}

	// segs must be the run's complete, contiguous segment list, because
	// segmentIndexForTime deliberately clamps: a packet before the first
	// segment routes to the first and one after the last routes to the last, so
	// that no packet in the window is silently dropped. (The window itself is
	// applied before routing; the clamp only places the packets between the
	// window's edges and the first or last whole rotation.) Handing it a
	// filtered subset would make every out-of-range packet land in whichever
	// segment survived the filter. Selection is therefore applied here, on the
	// write side, leaving routing to see the whole timeline.
	wanted := selectionMask(segs, cfg.SelectSegments)

	writers := make([]*segWriter, len(segs))
	defer func() {
		for _, w := range writers {
			if w != nil {
				_ = w.close()
			}
		}
	}()

	prog := newWriteProgress(cfg)

	// Segments are routed by packet timestamp, so a segment spanning a
	// capture-file boundary needs no special handling here: the writers stay
	// open across the sources and the packets simply keep arriving.
	for _, step := range steps {
		if err := writeSegmentsFrom(cfg, step, segs, wanted, writers, prog); err != nil {
			return err
		}
	}
	return nil
}

// writeSegmentsFrom copies one read step's packets into the segment writers,
// opening each output lazily so a segment that draws nothing from this source
// does not produce an empty file.
func writeSegmentsFrom(cfg SplitConfig, step capseq.ReadStep, segs []Segment, wanted []bool,
	writers []*segWriter, prog *network.ProgressStats) error {

	handle, err := pcap.OpenOffline(step.Path)
	if err != nil {
		return fmt.Errorf("open pcap %s: %w", step.Path, err)
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

	// The same window, anchored on the same first packet, as the analysis's
	// ReadPCAPFile for this step.
	window := network.NewReplayWindow(step.StartSecs, step.DurationSecs)

	// Read in place, as ReadPCAPFile does, rather than through Packets(): the
	// error handling is then the replay's, and stopping at the window's end
	// leaves no reader goroutine blocked on a channel send.
	packets := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := network.NextPacket(packets); packet != nil; packet = network.NextPacket(packets) {
		ci := packet.Metadata().CaptureInfo

		action := window.Admit(ci.Timestamp)
		if action == network.WindowSkip {
			continue
		}
		if action == network.WindowStop {
			break
		}
		// The analysis counts a packet only when it carries a UDP payload;
		// copy the same set so the counts agree.
		if !hasUDPPayload(packet) {
			continue
		}
		data := packet.Data()

		// Re-reading can leave CaptureLength/Length unset on some inputs;
		// pcapgo rejects a mismatch, so align them with the raw bytes.
		ci.CaptureLength = len(data)
		if ci.Length == 0 {
			ci.Length = len(data)
		}

		// Pass 2 progress, paced the same way as the scan pass. The writer copies
		// packets without decoding them, so only packet-oriented columns are shown.
		if prog != nil {
			prog.AddPacket(len(data))
			prog.ObserveProgress(ci.Timestamp, 0)
		}

		idx := segmentIndexForTime(segs, ci.Timestamp)
		if idx < 0 || !wanted[idx] {
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

// hasUDPPayload reports whether packet decodes to a UDP layer with a non-empty
// payload: the packets network.ReadPCAPFile counts.
func hasUDPPayload(packet gopacket.Packet) bool {
	udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
	return ok && len(udp.Payload) > 0
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

// newWriteProgress builds the pass-2 progress reporter, or nil when progress is
// disabled (cfg.ProgressSecs <= 0). It mirrors wrapProgress (used by the scan
// pass) but drives a bare *network.ProgressStats directly, because the write
// loop reads packets through gopacket rather than network.ReadPCAPFile. There is
// no inner sink: WriteSegments tracks its own per-segment packet counts, and the
// write phase omits points/RPM columns because it never decodes those values.
func newWriteProgress(cfg SplitConfig) *network.ProgressStats {
	if cfg.ProgressSecs <= 0 {
		return nil
	}
	var size int64
	if fi, err := os.Stat(cfg.PCAPFile); err == nil {
		size = fi.Size()
	}
	interval := time.Duration(cfg.ProgressSecs * float64(time.Second))
	p, _ := network.NewPacketOnlyProgressStats(nil, size, interval, "write", os.Stderr).(*network.ProgressStats)
	return p
}
