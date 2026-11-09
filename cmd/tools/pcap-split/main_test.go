//go:build pcap
// +build pcap

package main

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/cmd/lidar"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

// The synthetic capture: a Pandar40P-shaped stream at 600 RPM, 36 packets per
// rotation (1° per block), so 360 packets a second, plus a slower trickle on
// another UDP port that the BPF filter must keep out of the segments.
const (
	lidarPort      = 2369
	otherPort      = 10110
	packetsPerRev  = 36
	captureSeconds = 20
	packetInterval = 2778 * time.Microsecond // ≈ 1/360 s, whole microseconds
	pandarBlocks   = 10
	pandarChannels = 40
	pandarSize     = 1262
	pandarTail     = 1240
)

var captureStart = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// pandarPayload builds one 1262-byte Pandar40P data packet: ten blocks of forty
// 10 m returns at successive whole-degree azimuths, then a tail reporting
// 600 RPM and single (strongest) return.
func pandarPayload(seq int) []byte {
	p := make([]byte, pandarSize)
	off := 0
	for b := 0; b < pandarBlocks; b++ {
		binary.LittleEndian.PutUint16(p[off:], 0xEEFF)
		azimuth := ((seq%packetsPerRev)*pandarBlocks + b) * 100 // 0.01° units
		binary.LittleEndian.PutUint16(p[off+2:], uint16(azimuth))
		off += 4
		for c := 0; c < pandarChannels; c++ {
			binary.LittleEndian.PutUint16(p[off:], 10000/4) // 10 m in 4 mm units
			p[off+2] = 100
			off += 3
		}
	}
	tail := p[pandarTail:]
	binary.LittleEndian.PutUint16(tail[8:], 600) // motor speed, RPM
	binary.LittleEndian.PutUint32(tail[10:], uint32(seq%1_000_000))
	tail[14] = 0x37 // strongest return
	tail[15] = 0x42
	copy(tail[16:22], []byte{26, 11, 9, 12, 0, 0})
	return p
}

func udpFrame(t *testing.T, port int, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 1},
		DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 2},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP,
		SrcIP: net.IPv4(192, 168, 1, 201), DstIP: net.IPv4(192, 168, 1, 10)}
	udp := &layers.UDP{SrcPort: 10000, DstPort: layers.UDPPort(port)}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeCapture writes the synthetic pcapng and returns how many LiDAR packets
// it holds.
func writeCapture(t *testing.T, path string) int {
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
	n := int(time.Duration(captureSeconds) * time.Second / packetInterval)
	for i := 0; i < n; i++ {
		ts := captureStart.Add(time.Duration(i) * packetInterval)
		if i%50 == 0 {
			// Interleave a packet the BPF filter rejects.
			data := udpFrame(t, otherPort, []byte("$GPRMC"))
			if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: ts, CaptureLength: len(data), Length: len(data)}, data); err != nil {
				t.Fatal(err)
			}
		}
		data := udpFrame(t, lidarPort, pandarPayload(i))
		if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: ts, CaptureLength: len(data), Length: len(data)}, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	return n
}

type splitReport struct {
	TotalPackets int       `json:"total_packets"`
	FirstPacket  time.Time `json:"first_packet_time"`
	LastPacket   time.Time `json:"last_packet_time"`
	Segments     []struct {
		Filename    string `json:"filename"`
		PacketCount int    `json:"packet_count"`
	} `json:"segments"`
}

func runSplit(t *testing.T, input, out string, start, duration float64, dryRun bool) splitReport {
	t.Helper()
	args := []string{
		"--pcap", input, "--port", strconv.Itoa(lidarPort), "--output", out,
		"--prefix", "win", "--export-json", "--progress", "0",
		"--start-seconds", strconv.FormatFloat(start, 'f', -1, 64),
		"--duration-seconds", strconv.FormatFloat(duration, 'f', -1, 64),
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if code := lidar.SplitMain(args); code != 0 {
		t.Fatalf("pcap-split %v exited %d", args, code)
	}
	raw, err := os.ReadFile(filepath.Join(out, "segments.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r splitReport
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// readSegments opens every segment capture in dir with libpcap — so each must
// carry a valid file header — and returns the packet count, the first and last
// capture times across them, and how many files there were.
func readSegments(t *testing.T, dir string) (count int, first, last time.Time, files int) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.pcap"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		h, err := pcap.OpenOffline(path)
		if err != nil {
			t.Fatalf("segment %s is not a readable capture: %v", filepath.Base(path), err)
		}
		if lt := h.LinkType(); lt != layers.LinkTypeEthernet {
			t.Errorf("segment %s link type = %v, want Ethernet", filepath.Base(path), lt)
		}
		src := gopacket.NewPacketSource(h, h.LinkType())
		for p := range src.Packets() {
			udp, ok := p.Layer(layers.LayerTypeUDP).(*layers.UDP)
			if !ok || udp.DstPort != lidarPort {
				t.Errorf("segment %s holds a packet that is not LiDAR data", filepath.Base(path))
			}
			ts := p.Metadata().Timestamp
			if first.IsZero() || ts.Before(first) {
				first = ts
			}
			if ts.After(last) {
				last = ts
			}
			count++
		}
		h.Close()
		files++
	}
	return count, first, last, files
}

// TestWindowWritesOnlyTheDryRunPackets pins the fix for windows that limited
// the analysis but not the writer: a five-second window from the middle of the
// capture must write exactly the packets a dry run of that window counts, from
// the same first to the same last packet — not the whole capture.
func TestWindowWritesOnlyTheDryRunPackets(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "synthetic.pcapng")
	total := writeCapture(t, input)

	const start, duration = 7.0, 5.0
	dry := runSplit(t, input, filepath.Join(dir, "dry"), start, duration, true)
	if dry.TotalPackets == 0 {
		t.Fatal("dry run counted no packets in the window")
	}
	// 360 packets a second, closed at both ends: about 1,800 for five seconds.
	if dry.TotalPackets >= total/2 {
		t.Fatalf("dry run counted %d of %d packets for a %.0f s window of a %d s capture",
			dry.TotalPackets, total, duration, captureSeconds)
	}
	wantFirst := captureStart.Add(time.Duration(start * float64(time.Second)))
	if dry.FirstPacket.Before(wantFirst) || dry.FirstPacket.Sub(wantFirst) >= packetInterval {
		t.Errorf("dry-run first packet %v, want the first at or after %v", dry.FirstPacket, wantFirst)
	}

	outDir := filepath.Join(dir, "out")
	wrote := runSplit(t, input, outDir, start, duration, false)
	if wrote.TotalPackets != dry.TotalPackets {
		t.Errorf("write run analysed %d packets, dry run %d", wrote.TotalPackets, dry.TotalPackets)
	}

	n, first, last, files := readSegments(t, outDir)
	if files == 0 {
		t.Fatal("no segment captures written")
	}
	if n != dry.TotalPackets {
		t.Errorf("segments hold %d packets, dry run counted %d (capture has %d)", n, dry.TotalPackets, total)
	}
	if !first.Equal(dry.FirstPacket) {
		t.Errorf("first written packet %v, dry run's first %v", first, dry.FirstPacket)
	}
	if !last.Equal(dry.LastPacket) {
		t.Errorf("last written packet %v, dry run's last %v", last, dry.LastPacket)
	}
	var reported int
	for _, s := range wrote.Segments {
		reported += s.PacketCount
	}
	if reported != n {
		t.Errorf("segments.json reports %d packets written, the files hold %d", reported, n)
	}
}

// TestWindowPastTheEndWritesNoPackets: a window that starts after the last
// packet holds nothing, so nothing is written.
func TestWindowPastTheEndWritesNoPackets(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "synthetic.pcapng")
	writeCapture(t, input)

	outDir := filepath.Join(dir, "out")
	r := runSplit(t, input, outDir, captureSeconds+10, 5, false)
	if r.TotalPackets != 0 {
		t.Errorf("a window past the end analysed %d packets, want 0", r.TotalPackets)
	}
	if n, _, _, _ := readSegments(t, outDir); n != 0 {
		t.Errorf("a window past the end wrote %d data packets, want 0", n)
	}
}
