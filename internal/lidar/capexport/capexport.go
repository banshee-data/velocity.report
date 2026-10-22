// Package capexport cuts a window of an ordered capture sequence into one
// standalone PCAPNG: the extract a clip publishes in place of its sources.
//
// The cut follows rubric 1 of the survey capture export plan for a single
// part. It begins on the first LiDAR packet wholly past an azimuth wrap at or
// after the window start, and ends before the first such packet at or after
// the window end, so the file holds whole rotations and every rotation it
// holds completes inside its own packets. Packets are copied as captured, on
// every port, in file order: nothing is re-timed, re-ordered or dropped
// between the two cut points. The output carries no build or host stamp, so
// cutting the same sources under the same rubric yields the same bytes.
//
// The package reads captures with pcapgo rather than libpcap, so it builds
// and tests without the pcap build tag.
package capexport

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// RubricVersion names the cutting rule. It is recorded in the output's section
// comment and in every sidecar, and changes whenever the bytes a cut produces
// would change.
const RubricVersion = 1

// Pandar40P payload layout, as l1packets/parse reads it. Blocks start at
// offset 0; each is a 0xFFEE preamble, a little-endian azimuth in hundredths
// of a degree, and 40 channels of three bytes.
const (
	payloadStandard = 1262
	payloadSequence = 1266
	blocksPerPacket = 10
	blockSize       = 124
	// A backwards step of more than half a turn between consecutive blocks is
	// a wrap through zero. Dual-return packets repeat each azimuth and never
	// step backwards inside a rotation.
	wrapThreshold = 18000
)

// decodeLead is how far before the window start packets are decoded, so the
// azimuth the first candidate packet is compared with is a real one.
const decodeLead = time.Second

// ErrOutputExists refuses to overwrite an extract: a published file keeps
// its digest, so a re-cut goes to a new name.
var ErrOutputExists = errors.New("capexport: output already exists")

// ErrNoBoundary means the window holds no frame boundary to anchor on.
var ErrNoBoundary = errors.New("capexport: no frame boundary inside the window")

// Request names the sources, the window and the output.
type Request struct {
	// Sources are absolute paths, in capture order. Each is read to its end
	// so its digest covers the whole file.
	Sources []string
	// Start and End bound the window in capture time. A zero End runs to the
	// end of the sources.
	Start, End time.Time
	// UDPPort is the LiDAR data port whose packets mark frame boundaries.
	UDPPort int
	// Output is the absolute path of the PCAPNG to write. It must not exist.
	Output string
	// Comment is written to the section header after the rubric line.
	Comment string
}

// SourceDigest is one source as read.
type SourceDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Result describes what was written.
type Result struct {
	Sources []SourceDigest `json:"sources"`
	SHA256  string         `json:"sha256"`
	Bytes   int64          `json:"bytes"`
	// PacketCount is every packet written; LidarPacketCount those on UDPPort.
	PacketCount      int64 `json:"packet_count"`
	LidarPacketCount int64 `json:"lidar_packet_count"`
	// FrameCount is the complete rotations written.
	FrameCount    int64 `json:"frame_count"`
	FirstPacketNs int64 `json:"first_packet_ns"`
	LastPacketNs  int64 `json:"last_packet_ns"`
	// ClosedBy is "window" when the cut ended at a boundary at or after End,
	// or "end" when the sources ran out first.
	ClosedBy      string `json:"closed_by"`
	RubricVersion int    `json:"rubric_version"`
}

// Progress reports bytes read across all sources.
type Progress func(read, total int64)

// SectionComment is the deterministic section comment for an extract.
func SectionComment(extra string) string {
	line := fmt.Sprintf("velocity.report capture export, rubric %d", RubricVersion)
	if extra == "" {
		return line
	}
	return line + "\n" + extra
}

// Cut writes the extract and returns its description. On any error the
// partial output is removed.
func Cut(ctx context.Context, req Request, progress Progress) (Result, error) {
	if len(req.Sources) == 0 {
		return Result{}, errors.New("capexport: no sources")
	}
	if req.Output == "" || !filepath.IsAbs(req.Output) {
		return Result{}, errors.New("capexport: output must be an absolute path")
	}
	if req.UDPPort <= 0 || req.UDPPort > 65535 {
		return Result{}, fmt.Errorf("capexport: invalid UDP port %d", req.UDPPort)
	}
	if !req.End.IsZero() && !req.End.After(req.Start) {
		return Result{}, errors.New("capexport: window ends before it starts")
	}
	if _, err := os.Lstat(req.Output); err == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrOutputExists, req.Output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	var total int64
	for _, src := range req.Sources {
		info, err := os.Stat(src)
		if err != nil {
			return Result{}, fmt.Errorf("capexport: %w", err)
		}
		total += info.Size()
	}

	partial := filepath.Join(filepath.Dir(req.Output), "."+filepath.Base(req.Output)+".partial")
	out, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("capexport: create output: %w", err)
	}
	c := &cutter{
		req:      req,
		total:    total,
		progress: progress,
		out:      out,
		outHash:  sha256.New(),
		prevAz:   -1,
	}
	res, err := c.run(ctx)
	if cerr := out.Close(); err == nil && cerr != nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(partial)
		return Result{}, err
	}
	if err := os.Rename(partial, req.Output); err != nil {
		_ = os.Remove(partial)
		return Result{}, fmt.Errorf("capexport: publish output: %w", err)
	}
	return res, nil
}

type packetReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
}

type cutter struct {
	req      Request
	total    int64
	read     int64
	progress Progress

	out     *os.File
	outHash hash.Hash
	outN    int64
	writer  *pcapgo.NgWriter
	link    layers.LinkType
	snaplen uint32

	// prevAz is the last block azimuth of the previous LiDAR packet, or -1.
	prevAz int
	// wrapPending says the previous LiDAR packet held a wrap, so this one is
	// the first wholly past it.
	wrapPending bool

	writing bool
	done    bool
	res     Result
}

func (c *cutter) run(ctx context.Context) (Result, error) {
	for _, src := range c.req.Sources {
		digest, err := c.source(ctx, src)
		if err != nil {
			return Result{}, err
		}
		c.res.Sources = append(c.res.Sources, digest)
	}
	if c.writer == nil {
		return Result{}, ErrNoBoundary
	}
	if !c.done {
		c.res.ClosedBy = "end"
	}
	if err := c.writer.Flush(); err != nil {
		return Result{}, err
	}
	if err := c.out.Sync(); err != nil {
		return Result{}, err
	}
	c.res.SHA256 = "sha256:" + hex.EncodeToString(c.outHash.Sum(nil))
	c.res.Bytes = c.outN
	c.res.RubricVersion = RubricVersion
	return c.res, nil
}

// countingWriter hashes and counts what the writer sends to the file.
type countingWriter struct {
	w io.Writer
	h hash.Hash
	n *int64
}

func (cw countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.h.Write(p[:n])
	*cw.n += int64(n)
	return n, err
}

// progressReader counts bytes for the progress callback.
type progressReader struct {
	r io.Reader
	c *cutter
}

func (pr progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.c.read += int64(n)
	return n, err
}

func (c *cutter) source(ctx context.Context, path string) (SourceDigest, error) {
	f, err := os.Open(path)
	if err != nil {
		return SourceDigest{}, fmt.Errorf("capexport: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	var n int64
	tee := io.TeeReader(progressReader{r: f, c: c}, countingWriter{w: io.Discard, h: h, n: &n})
	digest := func() SourceDigest {
		return SourceDigest{Path: path, SHA256: "sha256:" + hex.EncodeToString(h.Sum(nil)), Bytes: n}
	}

	if c.done {
		// The window has closed: only the digest is still wanted.
		if _, err := io.Copy(io.Discard, tee); err != nil {
			return SourceDigest{}, fmt.Errorf("capexport: read %s: %w", path, err)
		}
		return digest(), nil
	}

	buffered := bufio.NewReaderSize(tee, 1<<20)
	reader, snaplen, err := openReader(buffered)
	if err != nil {
		return SourceDigest{}, fmt.Errorf("capexport: %s: %w", path, err)
	}
	if c.writer != nil && reader.LinkType() != c.link {
		return SourceDigest{}, fmt.Errorf("capexport: %s: link type %v differs from %v", path, reader.LinkType(), c.link)
	}
	if c.writer == nil {
		c.link = reader.LinkType()
		c.snaplen = snaplen
	}

	for count := 0; ; count++ {
		if count&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return SourceDigest{}, err
			}
			if c.progress != nil {
				c.progress(c.read, c.total)
			}
		}
		data, ci, err := reader.ReadPacketData()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// A capture cut off mid-packet by the recorder ends here, as it
			// does for libpcap.
			break
		}
		if err != nil {
			return SourceDigest{}, fmt.Errorf("capexport: read %s: %w", path, err)
		}
		if err := c.packet(data, ci); err != nil {
			return SourceDigest{}, err
		}
		if c.done {
			break
		}
	}
	if _, err := io.Copy(io.Discard, buffered); err != nil {
		return SourceDigest{}, fmt.Errorf("capexport: read %s: %w", path, err)
	}
	if c.progress != nil {
		c.progress(c.read, c.total)
	}
	return digest(), nil
}

// openReader sniffs the container and returns a packet reader over it.
func openReader(r *bufio.Reader) (packetReader, uint32, error) {
	magic, err := r.Peek(4)
	if err != nil {
		return nil, 0, fmt.Errorf("read header: %w", err)
	}
	if binary.LittleEndian.Uint32(magic) == 0x0A0D0D0A {
		ng, err := pcapgo.NewNgReader(r, pcapgo.DefaultNgReaderOptions)
		if err != nil {
			return nil, 0, err
		}
		var snaplen uint32
		if intf, err := ng.Interface(0); err == nil {
			snaplen = intf.SnapLength
		}
		return ng, snaplen, nil
	}
	classic, err := pcapgo.NewReader(r)
	if err != nil {
		return nil, 0, err
	}
	return classic, classic.Snaplen(), nil
}

// packet advances the cut by one packet.
func (c *cutter) packet(data []byte, ci gopacket.CaptureInfo) error {
	ts := ci.Timestamp
	boundary, lidar := false, false
	if c.writing || !ts.Before(c.req.Start.Add(-decodeLead)) {
		if payload, ok := udpPayload(data, c.link, c.req.UDPPort); ok {
			if azimuths, ok := blockAzimuths(payload); ok {
				lidar = true
				boundary = c.observe(azimuths)
			}
		}
	}

	if !c.writing {
		if !boundary || ts.Before(c.req.Start) {
			return nil
		}
		if err := c.open(); err != nil {
			return err
		}
		c.writing = true
		c.res.FirstPacketNs = ts.UnixNano()
	} else if boundary {
		c.res.FrameCount++
		if !c.req.End.IsZero() && !ts.Before(c.req.End) {
			c.done = true
			c.res.ClosedBy = "window"
			return nil
		}
	}

	ci.InterfaceIndex = 0
	ci.CaptureLength = len(data)
	if ci.Length < len(data) {
		ci.Length = len(data)
	}
	if err := c.writer.WritePacket(ci, data); err != nil {
		return fmt.Errorf("capexport: write packet: %w", err)
	}
	c.res.PacketCount++
	if lidar {
		c.res.LidarPacketCount++
	}
	c.res.LastPacketNs = ts.UnixNano()
	return nil
}

// observe records one LiDAR packet's block azimuths and reports whether it is
// the first packet wholly past a wrap.
func (c *cutter) observe(azimuths [blocksPerPacket]int) bool {
	first := c.wrapPending || (c.prevAz >= 0 && azimuths[0]+wrapThreshold < c.prevAz)
	c.wrapPending = false
	for i := 1; i < blocksPerPacket; i++ {
		if azimuths[i]+wrapThreshold < azimuths[i-1] {
			c.wrapPending = true
		}
	}
	c.prevAz = azimuths[blocksPerPacket-1]
	return first
}

func (c *cutter) open() error {
	intf := pcapgo.NgInterface{
		Name:                "lidar",
		LinkType:            c.link,
		SnapLength:          c.snaplen,
		TimestampResolution: 9,
	}
	opts := pcapgo.NgWriterOptions{SectionInfo: pcapgo.NgSectionInfo{
		Comment: SectionComment(c.req.Comment),
	}}
	w, err := pcapgo.NewNgWriterInterface(countingWriter{w: c.out, h: c.outHash, n: &c.outN}, intf, opts)
	if err != nil {
		return fmt.Errorf("capexport: start output: %w", err)
	}
	c.writer = w
	return nil
}

// udpPayload returns the payload of a UDP datagram to port, for the link types
// a Hesai capture uses.
func udpPayload(data []byte, link layers.LinkType, port int) ([]byte, bool) {
	packet := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
	udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if !ok {
		return nil, false
	}
	if int(udp.DstPort) != port && int(udp.SrcPort) != port {
		return nil, false
	}
	return udp.Payload, true
}

// blockAzimuths reads the ten block azimuths of a Pandar40P payload.
func blockAzimuths(payload []byte) ([blocksPerPacket]int, bool) {
	var az [blocksPerPacket]int
	if len(payload) != payloadStandard && len(payload) != payloadSequence {
		return az, false
	}
	for i := range blocksPerPacket {
		off := i * blockSize
		if payload[off] != 0xFF || payload[off+1] != 0xEE {
			return az, false
		}
		az[i] = int(binary.LittleEndian.Uint16(payload[off+2 : off+4]))
	}
	return az, true
}

// FirstPacketTime returns the capture timestamp of a file's first packet: the
// origin a clip's pcap_start_secs is measured from.
func FirstPacketTime(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("capexport: %w", err)
	}
	defer f.Close()
	reader, _, err := openReader(bufio.NewReader(f))
	if err != nil {
		return time.Time{}, fmt.Errorf("capexport: %s: %w", path, err)
	}
	_, ci, err := reader.ReadPacketData()
	if err != nil {
		return time.Time{}, fmt.Errorf("capexport: %s: first packet: %w", path, err)
	}
	return ci.Timestamp, nil
}
