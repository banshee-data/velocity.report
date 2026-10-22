package capexport

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

const (
	lidarPort = 2369
	gpsPort   = 10110
	// packetStep is the spacing of synthetic LiDAR packets; 18 packets make a
	// rotation, so a rotation lasts 90 ms.
	packetStep = 5 * time.Millisecond
)

var t0 = time.Date(2025, 12, 5, 22, 17, 4, 0, time.UTC)

type synthPacket struct {
	ts   time.Time
	data []byte
	// boundary is true for the first LiDAR packet wholly past a wrap.
	boundary bool
	lidar    bool
}

func udpFrame(t *testing.T, port int, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 1, 2, 3, 4, 5},
		DstMAC:       net.HardwareAddr{6, 7, 8, 9, 10, 11},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP,
		SrcIP: net.IP{192, 168, 1, 201}, DstIP: net.IP{192, 168, 1, 100},
	}
	udp := &layers.UDP{SrcPort: layers.UDPPort(port), DstPort: layers.UDPPort(port)}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), buf.Bytes()...)
}

func hesaiPayload(base int) []byte {
	payload := make([]byte, payloadStandard)
	for i := range blocksPerPacket {
		off := i * blockSize
		payload[off], payload[off+1] = 0xFF, 0xEE
		binary.LittleEndian.PutUint16(payload[off+2:], uint16((base+i*200)%36000))
	}
	return payload
}

// stream builds n LiDAR packets whose first block azimuth advances 2000 units
// a packet from offset, with a GPS sentence after every seventh.
func stream(t *testing.T, n, offset int) []synthPacket {
	t.Helper()
	var out []synthPacket
	prevLast := -1
	wrapInside := false
	for k := range n {
		base := (k*2000 + offset) % 36000
		first := wrapInside || (prevLast >= 0 && base+wrapThreshold < prevLast)
		wrapInside = base+9*200 >= 36000
		prevLast = (base + 9*200) % 36000
		ts := t0.Add(time.Duration(k) * packetStep)
		out = append(out, synthPacket{ts: ts, data: udpFrame(t, lidarPort, hesaiPayload(base)), boundary: first, lidar: true})
		if k%7 == 3 {
			out = append(out, synthPacket{ts: ts.Add(time.Millisecond), data: udpFrame(t, gpsPort, []byte("$GPRMC,stub*00"))})
		}
	}
	return out
}

func writeClassic(t *testing.T, path string, packets []synthPacket) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := pcapgo.NewWriterNanos(f)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	for _, p := range packets {
		ci := gopacket.CaptureInfo{Timestamp: p.ts, CaptureLength: len(p.data), Length: len(p.data)}
		if err := w.WritePacket(ci, p.data); err != nil {
			t.Fatal(err)
		}
	}
}

func writeNg(t *testing.T, path string, packets []synthPacket) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w, err := pcapgo.NewNgWriter(f, layers.LinkTypeEthernet)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packets {
		ci := gopacket.CaptureInfo{Timestamp: p.ts, CaptureLength: len(p.data), Length: len(p.data)}
		if err := w.WritePacket(ci, p.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
}

type readBack struct {
	times   []time.Time
	comment string
}

func readOutput(t *testing.T, path string) readBack {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}
	var rb readBack
	rb.comment = r.SectionInfo().Comment
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		rb.times = append(rb.times, ci.Timestamp)
	}
	return rb
}

// expected applies the rubric to the synthetic stream by hand.
func expected(packets []synthPacket, start, end time.Time) (first, last int, frames int64) {
	first, last = -1, -1
	for i, p := range packets {
		if first < 0 {
			if p.boundary && !p.ts.Before(start) {
				first = i
			}
			continue
		}
		if p.boundary {
			frames++
			if !end.IsZero() && !p.ts.Before(end) {
				return first, i - 1, frames
			}
		}
		last = i
	}
	return first, last, frames
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestCutWholeRotationsBetweenBoundaries(t *testing.T) {
	for _, offset := range []int{0, 1000} {
		dir := t.TempDir()
		packets := stream(t, 200, offset)
		src := filepath.Join(dir, "src.pcap")
		writeClassic(t, src, packets)

		start, end := t0.Add(203*time.Millisecond), t0.Add(611*time.Millisecond)
		out := filepath.Join(dir, "clip.pcapng")
		res, err := Cut(context.Background(), Request{
			Sources: []string{src}, Start: start, End: end, UDPPort: lidarPort,
			Output: out, Comment: "clip test",
		}, nil)
		if err != nil {
			t.Fatalf("offset %d: %v", offset, err)
		}

		first, last, frames := expected(packets, start, end)
		rb := readOutput(t, out)
		if got, want := len(rb.times), last-first+1; got != want || int64(want) != res.PacketCount {
			t.Fatalf("offset %d: wrote %d packets (result %d), want %d", offset, got, res.PacketCount, want)
		}
		if !rb.times[0].Equal(packets[first].ts) || !rb.times[len(rb.times)-1].Equal(packets[last].ts) {
			t.Fatalf("offset %d: output spans %v..%v, want %v..%v", offset,
				rb.times[0], rb.times[len(rb.times)-1], packets[first].ts, packets[last].ts)
		}
		if res.FrameCount != frames || frames != 4 {
			t.Fatalf("offset %d: frame count %d, want %d (expected 4)", offset, res.FrameCount, frames)
		}
		if res.ClosedBy != "window" || res.RubricVersion != RubricVersion {
			t.Fatalf("offset %d: closed by %q rubric %d", offset, res.ClosedBy, res.RubricVersion)
		}
		if res.LidarPacketCount >= res.PacketCount {
			t.Fatalf("offset %d: GPS packets inside the window were not copied", offset)
		}
		if !strings.HasPrefix(rb.comment, "velocity.report capture export, rubric 1\nclip test") {
			t.Fatalf("offset %d: section comment %q", offset, rb.comment)
		}
		if res.SHA256 != fileDigest(t, out) {
			t.Fatalf("offset %d: result digest does not match the file", offset)
		}
		if res.Sources[0].SHA256 != fileDigest(t, src) {
			t.Fatalf("offset %d: source digest does not cover the whole source", offset)
		}
	}
}

func TestCutIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.pcap")
	writeClassic(t, src, stream(t, 120, 0))
	req := Request{Sources: []string{src}, Start: t0.Add(100 * time.Millisecond), UDPPort: lidarPort}
	var digests []string
	for _, name := range []string{"a.pcapng", "b.pcapng"} {
		req.Output = filepath.Join(dir, name)
		res, err := Cut(context.Background(), req, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.ClosedBy != "end" {
			t.Fatalf("closed by %q, want end", res.ClosedBy)
		}
		digests = append(digests, res.SHA256)
	}
	if digests[0] != digests[1] {
		t.Fatalf("two cuts differ: %v", digests)
	}
}

func TestCutAcrossSourcesOfBothContainers(t *testing.T) {
	dir := t.TempDir()
	packets := stream(t, 240, 1000)
	a, b := filepath.Join(dir, "a.pcapng"), filepath.Join(dir, "b.pcap")
	writeNg(t, a, packets[:130])
	writeClassic(t, b, packets[130:])

	start, end := t0.Add(400*time.Millisecond), t0.Add(900*time.Millisecond)
	out := filepath.Join(dir, "clip.pcapng")
	var lastRead, lastTotal int64
	res, err := Cut(context.Background(), Request{
		Sources: []string{a, b}, Start: start, End: end, UDPPort: lidarPort, Output: out,
	}, func(read, total int64) { lastRead, lastTotal = read, total })
	if err != nil {
		t.Fatal(err)
	}
	first, last, frames := expected(packets, start, end)
	if res.PacketCount != int64(last-first+1) || res.FrameCount != frames {
		t.Fatalf("got %d packets %d frames, want %d and %d", res.PacketCount, res.FrameCount, last-first+1, frames)
	}
	if len(res.Sources) != 2 || res.Sources[0].SHA256 != fileDigest(t, a) || res.Sources[1].SHA256 != fileDigest(t, b) {
		t.Fatalf("source digests %+v", res.Sources)
	}
	if lastRead != lastTotal || lastTotal == 0 {
		t.Fatalf("progress ended at %d of %d", lastRead, lastTotal)
	}
}

func TestCutRefusals(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.pcap")
	writeClassic(t, src, stream(t, 60, 0))

	existing := filepath.Join(dir, "taken.pcapng")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Cut(context.Background(), Request{Sources: []string{src}, Start: t0, UDPPort: lidarPort, Output: existing}, nil)
	if !errors.Is(err, ErrOutputExists) {
		t.Fatalf("existing output: %v", err)
	}

	late := filepath.Join(dir, "late.pcapng")
	_, err = Cut(context.Background(), Request{Sources: []string{src}, Start: t0.Add(time.Hour), UDPPort: lidarPort, Output: late}, nil)
	if !errors.Is(err, ErrNoBoundary) {
		t.Fatalf("window past the data: %v", err)
	}
	if _, statErr := os.Stat(late); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a refused cut left %s behind", late)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".*.partial")); len(matches) != 0 {
		t.Fatalf("partial files left behind: %v", matches)
	}

	wrongPort := filepath.Join(dir, "port.pcapng")
	_, err = Cut(context.Background(), Request{Sources: []string{src}, Start: t0, UDPPort: 9999, Output: wrongPort}, nil)
	if !errors.Is(err, ErrNoBoundary) {
		t.Fatalf("no LiDAR packets on the port: %v", err)
	}

	for name, req := range map[string]Request{
		"no sources":    {Start: t0, UDPPort: lidarPort, Output: filepath.Join(dir, "x.pcapng")},
		"relative path": {Sources: []string{src}, UDPPort: lidarPort, Output: "x.pcapng"},
		"bad port":      {Sources: []string{src}, Output: filepath.Join(dir, "y.pcapng")},
		"inverted":      {Sources: []string{src}, Start: t0.Add(time.Second), End: t0, UDPPort: lidarPort, Output: filepath.Join(dir, "z.pcapng")},
	} {
		if _, err := Cut(context.Background(), req, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCutHonoursCancellation(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.pcap")
	writeClassic(t, src, stream(t, 60, 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := filepath.Join(dir, "clip.pcapng")
	if _, err := Cut(ctx, Request{Sources: []string{src}, Start: t0, UDPPort: lidarPort, Output: out}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cut: %v", err)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a cancelled cut published its output")
	}
}

func TestFirstPacketTime(t *testing.T) {
	dir := t.TempDir()
	packets := stream(t, 10, 0)
	for _, name := range []string{"a.pcap", "b.pcapng"} {
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, "ng") {
			writeNg(t, path, packets)
		} else {
			writeClassic(t, path, packets)
		}
		got, err := FirstPacketTime(path)
		if err != nil || !got.Equal(packets[0].ts) {
			t.Fatalf("%s: %v %v, want %v", name, got, err, packets[0].ts)
		}
	}
	empty := filepath.Join(dir, "empty.pcap")
	writeClassic(t, empty, nil)
	if _, err := FirstPacketTime(empty); err == nil {
		t.Fatal("an empty capture has a first packet")
	}
}
