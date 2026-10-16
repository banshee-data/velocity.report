package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

const (
	packetsPerRotation = 180 // 1,800 blocks of 0.2 degrees, ten to a packet
	packetInterval     = 100 * time.Millisecond / packetsPerRotation
)

// pandarPayload is a 1,266-byte Pandar40P packet whose ten blocks start at
// azimuth firstAz (0.01-degree units) in 0.2-degree steps, every channel at
// distance units of 4 mm.
func pandarPayload(firstAz int, distance uint16, seq uint32) []byte {
	b := make([]byte, parse.PACKET_SIZE_SEQUENCE)
	for blk := 0; blk < parse.BLOCKS_PER_PACKET; blk++ {
		o := blk * parse.BLOCK_SIZE
		b[o], b[o+1] = 0xFF, 0xEE
		binary.LittleEndian.PutUint16(b[o+2:], uint16((firstAz+20*blk)%parse.ROTATION_MAX_UNITS))
		for ch := 0; ch < parse.CHANNELS_PER_BLOCK; ch++ {
			c := o + 4 + 3*ch
			binary.LittleEndian.PutUint16(b[c:], distance)
			b[c+2] = 100
		}
	}
	tail := b[parse.TAIL_START:]
	binary.LittleEndian.PutUint16(tail[8:], 600)
	tail[14], tail[15] = 0x37, 0x42
	copy(tail[16:22], []byte{26, 9, 2, 18, 32, 30})
	binary.LittleEndian.PutUint32(b[parse.PACKET_SIZE_STANDARD:], seq)
	return b
}

// udpFrame wraps payload in Ethernet, IPv4 and UDP headers to dstPort.
func udpFrame(payload []byte, dstPort uint16) []byte {
	f := make([]byte, 14+20+8+len(payload))
	binary.BigEndian.PutUint16(f[12:], 0x0800)
	ip := f[14:]
	ip[0], ip[9] = 0x45, 17
	binary.BigEndian.PutUint16(ip[2:], uint16(20+8+len(payload)))
	udp := ip[20:]
	binary.BigEndian.PutUint16(udp[0:], 10000)
	binary.BigEndian.PutUint16(udp[2:], dstPort)
	binary.BigEndian.PutUint16(udp[4:], uint16(8+len(payload)))
	copy(udp[8:], payload)
	return f
}

// writeCapture writes seconds of a constant-range scene from start, with a
// stray packet to another port every hundred, at the given if_tsresol.
func writeCapture(t *testing.T, path string, resolution uint8, start time.Time, seconds float64, distance uint16) int {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	intf := pcapgo.DefaultNgInterface
	intf.LinkType = layers.LinkTypeEthernet
	intf.TimestampResolution = pcapgo.NgResolution(resolution)
	w, err := pcapgo.NewNgWriterInterface(f, intf, pcapgo.DefaultNgWriterOptions)
	if err != nil {
		t.Fatal(err)
	}
	n := int(seconds * float64(time.Second) / float64(packetInterval))
	for i := 0; i < n; i++ {
		ts := start.Add(time.Duration(i) * packetInterval)
		frame := udpFrame(pandarPayload((i%packetsPerRotation)*200, distance, uint32(i)), 2369)
		if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: ts, CaptureLength: len(frame), Length: len(frame)}, frame); err != nil {
			t.Fatal(err)
		}
		if i%100 == 50 {
			stray := udpFrame([]byte("not lidar"), 10110)
			if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: ts, CaptureLength: len(stray), Length: len(stray)}, stray); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPcapngExtentAndSeek(t *testing.T) {
	for _, res := range []uint8{6, 9} {
		path := filepath.Join(t.TempDir(), "c.pcapng")
		start := time.Unix(1788373940, 123456000)
		n := writeCapture(t, path, res, start, 3, 2500)
		f, err := openPcapng(path)
		if err != nil {
			t.Fatal(err)
		}
		first, last, err := f.extent()
		if err != nil {
			t.Fatal(err)
		}
		wantLast := start.Add(time.Duration(n-1) * packetInterval)
		quantum := int64(1000) // microsecond resolution truncates
		if res == 9 {
			quantum = 1
		}
		if first != start.UnixNano() || math.Abs(float64(last-wantLast.UnixNano())) >= float64(quantum) {
			t.Fatalf("res %d: extent %d..%d, want %d..%d", res, first, last, start.UnixNano(), wantLast.UnixNano())
		}
		target := start.Add(1700 * time.Millisecond).UnixNano()
		off, err := f.seekOffset(target)
		if err != nil {
			t.Fatal(err)
		}
		var firstSeen int64 = -1
		var atOrAfter int64 = -1
		if err := f.readFrom(off, func(p ngPacket) bool {
			if firstSeen < 0 {
				firstSeen = p.UnixNanos
			}
			if p.UnixNanos >= target {
				atOrAfter = p.UnixNanos
				return false
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if firstSeen > target || target-firstSeen > int64(300*time.Millisecond) {
			t.Fatalf("res %d: seek landed at %d, %.3f s before the target", res, firstSeen, float64(target-firstSeen)/1e9)
		}
		if atOrAfter-target > int64(packetInterval) {
			t.Fatalf("res %d: first packet at or after the target is %d ns late", res, atOrAfter-target)
		}
		f.Close()
	}
}

func TestClassicPcapIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.pcap")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(65536, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := openPcapng(path); err == nil || !strings.Contains(err.Error(), "not a pcapng") {
		t.Fatalf("classic pcap: %v", err)
	}
}

func TestDecodeWindowAcrossTwoCaptures(t *testing.T) {
	dir := t.TempDir()
	start := time.Unix(1788373940, 0)
	n1 := writeCapture(t, filepath.Join(dir, "a.pcapng"), 6, start, 2, 2500)
	writeCapture(t, filepath.Join(dir, "b.pcapng"), 6, start.Add(time.Duration(n1)*packetInterval), 2, 2500)
	var caps []captureInfo
	for _, name := range []string{"a.pcapng", "b.pcapng"} {
		f, err := openPcapng(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		first, last, err := f.extent()
		if err != nil {
			t.Fatal(err)
		}
		caps = append(caps, captureInfo{Path: name, FirstNanos: first, LastNanos: last, file: f})
	}
	cfg, err := parse.LoadEmbeddedPandar40PConfig()
	if err != nil {
		t.Fatal(err)
	}
	sp := staticParams{MinCoverage: 0.8, MaxIQR: 0.08, MaxIQRFraction: 0.004, MinRange: 2.5, MaxRange: 120}
	w, err := decodeWindow(caps, start.Add(1500*time.Millisecond).UnixNano(), start.Add(2500*time.Millisecond).UnixNano(), 2369, cfg, sp)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Captures) != 2 {
		t.Fatalf("window read %v, want both captures", w.Captures)
	}
	if w.Packets < 1795 || w.Packets > 1805 || w.Rotations < 10 || w.Rotations > 12 {
		t.Fatalf("%d packets, %d rotations; want about 1,800 and 11", w.Packets, w.Rotations)
	}
	if w.ReturnMode["0x37"] != w.Packets || w.MotorRPM != 600 || w.Truncated {
		t.Fatalf("modes %v, rpm %v, truncated %v", w.ReturnMode, w.MotorRPM, w.Truncated)
	}
	if w.Static < numCells*95/100 {
		t.Fatalf("%d of %d cells static on a constant scene", w.Static, numCells)
	}
	for c, s := range w.cells {
		if s.Static && math.Abs(s.Median-10) > 1e-6 {
			t.Fatalf("cell %d median %.4f m, want 10", c, s.Median)
		}
	}
}

func TestStaticCellsNeedCoverageAndALowSpread(t *testing.T) {
	w := newWindowAccum()
	const rotations = 100
	add := func(rot, channel int, az, dist float64) {
		raw := uint16(az * 100)
		w.addPoint(l2frames.PointPolar{Channel: channel, Azimuth: az, Distance: dist, RawBlockAzimuth: raw})
	}
	for r := 0; r < rotations; r++ {
		add(r, 10, 1.1, 20) // constant
		if r%5 != 0 {       // blocked by a passing object one rotation in five
			add(r, 11, 2.1, 20)
		} else {
			add(r, 11, 2.1, 5)
		}
		if r < 60 { // an object parked across 40% of the window
			add(r, 12, 3.1, 20)
		} else {
			add(r, 12, 3.1, 5)
		}
		if r%2 == 0 { // seen in half the rotations
			add(r, 13, 4.1, 20)
		}
		add(r, 14, 5.1, 1.0) // the mount: too near
		add(r, 1, 359.9, 30) // wraps the rotation
	}
	if w.rotations() != rotations {
		t.Fatalf("counted %d rotations, want %d", w.rotations(), rotations)
	}
	cells := w.finish(staticParams{MinCoverage: 0.8, MaxIQR: 0.08, MaxIQRFraction: 0.004, MinRange: 2.5, MaxRange: 120})
	at := func(channel int, az float64) cellStat {
		c, _ := cellOf(channel, az)
		return cells[c]
	}
	if s := at(10, 1.1); !s.Static || s.Median != 20 {
		t.Errorf("constant cell: %+v", s)
	}
	if s := at(11, 2.1); !s.Static || s.Median != 20 {
		t.Errorf("a 20%% passing object should leave the cell static at its background: %+v", s)
	}
	if s := at(12, 3.1); s.Static {
		t.Errorf("a cell occupied 40%% of the window is not static: %+v", s)
	}
	if s := at(13, 4.1); s.Static || math.Abs(s.Coverage-0.5) > 0.01 {
		t.Errorf("half coverage: %+v", s)
	}
	if s := at(14, 5.1); s.Static {
		t.Errorf("a return nearer than the minimum range is not static: %+v", s)
	}
}

// ngBuilder writes a little-endian pcapng file block by block, so a test can
// lay out exactly the malformed file it needs.
type ngBuilder struct{ b []byte }

// block appends one block of the given type around body, padded to four
// bytes, and returns its offset.
func (g *ngBuilder) block(typ uint32, body []byte) int {
	off := len(g.b)
	padded := (len(body) + 3) &^ 3
	length := uint32(12 + padded)
	g.b = binary.LittleEndian.AppendUint32(g.b, typ)
	g.b = binary.LittleEndian.AppendUint32(g.b, length)
	g.b = append(g.b, body...)
	g.b = append(g.b, make([]byte, padded-len(body))...)
	g.b = binary.LittleEndian.AppendUint32(g.b, length)
	return off
}

func (g *ngBuilder) section() int {
	body := binary.LittleEndian.AppendUint32(nil, byteOrderMagic)
	body = binary.LittleEndian.AppendUint16(body, 1)
	body = binary.LittleEndian.AppendUint16(body, 0)
	body = binary.LittleEndian.AppendUint64(body, math.MaxUint64) // section length unknown
	return g.block(blockSectionHeader, body)
}

// iface describes an Ethernet interface at the default microsecond resolution.
func (g *ngBuilder) iface() int {
	body := binary.LittleEndian.AppendUint16(nil, 1)
	body = binary.LittleEndian.AppendUint16(body, 0)
	body = binary.LittleEndian.AppendUint32(body, 65535)
	return g.block(blockInterfaceDesc, body)
}

// packet writes an Enhanced Packet Block on interface id at us microseconds
// whose captured-length field says capLen, whatever data holds.
func (g *ngBuilder) packet(id uint32, us uint64, capLen uint32, data []byte) int {
	body := binary.LittleEndian.AppendUint32(nil, id)
	body = binary.LittleEndian.AppendUint32(body, uint32(us>>32))
	body = binary.LittleEndian.AppendUint32(body, uint32(us))
	body = binary.LittleEndian.AppendUint32(body, capLen)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(data)))
	body = append(body, data...)
	return g.block(blockEnhancedPacket, body)
}

// open writes the first size bytes of the file and opens them.
func (g *ngBuilder) open(t *testing.T, size int) *ngFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.pcapng")
	if err := os.WriteFile(path, g.b[:size], 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := openPcapng(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func readAll(f *ngFile, off int64) ([]ngPacket, error) {
	var got []ngPacket
	err := f.readFrom(off, func(p ngPacket) bool {
		got = append(got, ngPacket{UnixNanos: p.UnixNanos, Iface: p.Iface, Data: append([]byte(nil), p.Data...)})
		return true
	})
	return got, err
}

// A captured length or interface number from a packet block is checked as the
// unsigned 32-bit value it is, so a value past int32 or past the block is an
// error, never a panic or a slice into the block's trailing length word.
func TestPacketBlockLengthsAreChecked(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	for _, tc := range []struct {
		name   string
		id     uint32
		capLen uint32
		want   string
	}{
		{"whole data", 0, 4, ""},
		{"into the trailing length", 0, 8, "claims 8 captured bytes; the block holds at most 4"},
		{"past int32", 0, 1 << 31, "claims 2147483648 captured bytes"},
		{"largest", 0, math.MaxUint32, "claims 4294967295 captured bytes"},
		{"interface past int32", math.MaxUint32, 4, "names interface 4294967295, but the head of the file describes 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g ngBuilder
			g.section()
			g.iface()
			g.packet(tc.id, 1_000_000, tc.capLen, data)
			f := g.open(t, len(g.b))
			got, err := readAll(f, f.firstBlock)
			if tc.want == "" {
				if err != nil || len(got) != 1 || string(got[0].Data) != string(data) || got[0].UnixNanos != 1e9 {
					t.Fatalf("packets %+v, err %v", got, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
			// The seek's resynchronising scan must not trust such a block either.
			if _, _, _, err := f.syncAt(f.firstBlock); err != nil {
				t.Fatalf("sync: %v", err)
			}
		})
	}
}

// An interface described after the first packet block cannot be known to a
// reader that seeks, so a read that meets it, or a packet naming it, is
// refused with an error that says so; a read that stops before it is not. A
// second section is refused the same way.
func TestInterfaceAfterTheFirstPacketIsRefused(t *testing.T) {
	var g ngBuilder
	g.section()
	g.iface()
	g.packet(0, 1_000_000, 4, []byte{1, 2, 3, 4})
	late := g.iface()
	second := g.packet(1, 2_000_000, 4, []byte{5, 6, 7, 8})
	f := g.open(t, len(g.b))
	if len(f.ifaces) != 1 {
		t.Fatalf("head holds %d interfaces, want 1", len(f.ifaces))
	}
	got, err := readAll(f, f.firstBlock)
	want := fmt.Sprintf("interface description block at offset %d follows the first packet block", late)
	if len(got) != 1 || err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("read over the late interface: %d packets, err %v", len(got), err)
	}
	if _, err := readAll(f, int64(second)); err == nil || !strings.Contains(err.Error(), "names interface 1, but the head of the file describes 1") {
		t.Fatalf("read from its packet: %v", err)
	}
	n := 0
	if err := f.readFrom(f.firstBlock, func(ngPacket) bool { n++; return false }); err != nil || n != 1 {
		t.Fatalf("a read stopping before the late interface: %d packets, err %v", n, err)
	}

	var two ngBuilder
	two.section()
	two.iface()
	two.packet(0, 1_000_000, 4, []byte{1, 2, 3, 4})
	at := two.section()
	tf := two.open(t, len(two.b))
	if _, err := readAll(tf, tf.firstBlock); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("a second section begins at offset %d", at)) {
		t.Fatalf("second section: %v", err)
	}
}

// A file that ends inside a block ends at its last whole block, whether the
// cut falls in the block's header, just after it, or in its body, and the cut
// is recorded; an uncut file records none.
func TestCutCaptureEndsAtItsLastWholeBlock(t *testing.T) {
	var g ngBuilder
	g.section()
	g.iface()
	g.packet(0, 1_000_000, 4, []byte{1, 2, 3, 4})
	g.packet(0, 2_000_000, 4, []byte{5, 6, 7, 8})
	third := g.packet(0, 3_000_000, 4, []byte{9, 10, 11, 12})
	for _, tc := range []struct {
		name string
		size int
		want int
		cut  int64
	}{
		{"whole", len(g.b), 3, -1},
		{"in the header", third + 3, 2, int64(third)},
		{"after the header", third + 8, 2, int64(third)},
		{"in the body", third + 20, 2, int64(third)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := g.open(t, tc.size)
			got, err := readAll(f, f.firstBlock)
			if err != nil || len(got) != tc.want || f.cutAt != tc.cut {
				t.Fatalf("%d packets, cut at %d, err %v; want %d packets, cut at %d", len(got), f.cutAt, err, tc.want, tc.cut)
			}
			first, last, err := f.extent()
			if err != nil || first != 1e9 || last != int64(tc.want)*1e9 {
				t.Fatalf("extent %d..%d, err %v", first, last, err)
			}
		})
	}
}
