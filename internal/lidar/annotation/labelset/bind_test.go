package labelset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

func udpFrame(t *testing.T, port int, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: []byte{2, 0, 0, 0, 0, 1}, DstMAC: []byte{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: []byte{192, 168, 1, 201}, DstIP: []byte{192, 168, 1, 10}}
	udp := &layers.UDP{SrcPort: 10000, DstPort: layers.UDPPort(port)}
	udp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func payloads(n int, tag string) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = []byte(strings.Repeat(tag, 3) + string(rune('A'+i)))
	}
	return out
}

func writePCAP(t *testing.T, path string, port int, ps [][]byte, ng bool) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ts := time.Unix(1764973024, 0)
	if ng {
		w, err := pcapgo.NewNgWriter(f, layers.LinkTypeEthernet)
		if err != nil {
			t.Fatal(err)
		}
		for i, p := range ps {
			d := udpFrame(t, port, p)
			if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: ts.Add(time.Duration(i) * time.Millisecond), CaptureLength: len(d), Length: len(d)}, d); err != nil {
				t.Fatal(err)
			}
		}
		w.Flush()
		return
	}
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(65536, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	for i, p := range ps {
		d := udpFrame(t, port, p)
		if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: ts.Add(time.Duration(i) * time.Millisecond), CaptureLength: len(d), Length: len(d)}, d); err != nil {
			t.Fatal(err)
		}
	}
}

func setOverPayloads(t *testing.T, ps [][]byte) string {
	t.Helper()
	s := State{Domains: []Domain{{DomainID: "fg", Kind: DomainForeground}}}
	for _, p := range ps {
		s.Packets = append(s.Packets, KeyOf(p))
	}
	per := Pandar40P.PerPacket()
	s.Windows = []Window{{WindowID: 0, Start: 0, End: 2 * per}, {WindowID: 1, Start: 2 * per, End: uint64(len(ps)) * per}}
	dir := filepath.Join(t.TempDir(), "set")
	if _, err := Create(dir, Manifest{Layout: Pandar40P}, s, Change{Author: "test"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBindAcrossContainersAndClips(t *testing.T) {
	ps := payloads(5, "x")
	dir := setOverPayloads(t, ps)
	tmp := t.TempDir()
	full, clip := filepath.Join(tmp, "full.pcap"), filepath.Join(tmp, "clip.pcapng")
	// The full capture interleaves another port's traffic.
	var mixed [][]byte
	for _, p := range ps {
		mixed = append(mixed, p)
	}
	writePCAP(t, full, 2369, mixed, false)
	writePCAP(t, clip, 2369, ps[:2], true) // a clipped, re-containered copy
	res, err := Bind(dir, full, 2369, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.Binding.PacketsFound != 5 || res.Binding.WindowsComplete != 2 {
		t.Fatalf("full: %+v", res.Binding)
	}
	res, err = Bind(dir, clip, 2369, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.Binding.PacketsFound != 2 || res.Binding.PacketsMissing != 3 || res.Binding.WindowsComplete != 1 || res.Binding.WindowsMissing != 1 {
		t.Fatalf("clip: %+v", res.Binding)
	}
	ls, _ := Open(dir)
	if len(ls.Manifest.AppliesTo) != 2 || ls.Revision() != 1 {
		t.Fatalf("bindings %d at revision %d", len(ls.Manifest.AppliesTo), ls.Revision())
	}
	if _, err := Bind(dir, full, 2370, "test"); err == nil {
		t.Fatal("bound a file with none of the packets on that port")
	}
}

func TestBindRefusesDuplicates(t *testing.T) {
	ps := payloads(4, "y")
	dir := setOverPayloads(t, ps)
	path := filepath.Join(t.TempDir(), "dup.pcap")
	writePCAP(t, path, 2369, append(ps, ps[1]), false)
	if _, err := Bind(dir, path, 2369, "test"); err == nil {
		t.Fatal("bound a file holding a packet twice")
	}
	c, err := CensusOf(path, 2369)
	if err != nil {
		t.Fatal(err)
	}
	if c.Packets != 5 || c.Unique != 3 || c.Duplicated != 1 || c.SamePayload != 1 || c.Extra != 1 {
		t.Fatalf("census %+v", c)
	}
	if !strings.HasPrefix(c.SHA256, "sha256:") || c.Bytes == 0 {
		t.Fatalf("file digest %q %d", c.SHA256, c.Bytes)
	}
}
