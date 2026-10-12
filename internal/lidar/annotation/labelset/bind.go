package labelset

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
)

// Packet is one UDP packet read from a capture file.
type Packet struct {
	Key       PacketKey
	CaptureNs int64
	Ordinal   int // position among the file's matching packets, from 0
	Payload   []byte
}

// ScanResult describes a capture file read end to end.
type ScanResult struct {
	SHA256  string
	Bytes   int64
	Packets int
}

// ScanCapture reads every UDP packet to or from port in a PCAP or PCAPNG file
// and calls fn with its key. It also digests the whole file, so a binding
// names exactly the bytes it checked. The payload slice is only valid during
// the call.
func ScanCapture(path string, port int, fn func(Packet) error) (ScanResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return ScanResult{}, err
	}
	defer f.Close()
	h := sha256.New()
	counted := &countingReader{r: io.TeeReader(bufio.NewReaderSize(f, 1<<20), h)}
	br := bufio.NewReaderSize(counted, 1<<16)
	magic, err := br.Peek(4)
	if err != nil {
		return ScanResult{}, fmt.Errorf("read capture header: %w", err)
	}
	var next func() ([]byte, gopacket.CaptureInfo, error)
	var link layers.LinkType
	if binary.BigEndian.Uint32(magic) == 0x0A0D0D0A {
		r, err := pcapgo.NewNgReader(br, pcapgo.DefaultNgReaderOptions)
		if err != nil {
			return ScanResult{}, fmt.Errorf("open pcapng: %w", err)
		}
		next, link = r.ReadPacketData, r.LinkType()
	} else {
		r, err := pcapgo.NewReader(br)
		if err != nil {
			return ScanResult{}, fmt.Errorf("open pcap: %w", err)
		}
		next, link = r.ReadPacketData, r.LinkType()
	}
	ordinal := 0
	for {
		data, ci, err := next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ScanResult{}, fmt.Errorf("read packet after %d: %w", ordinal, err)
		}
		payload, ok := udpPayload(data, link, port)
		if !ok {
			continue
		}
		if err := fn(Packet{Key: KeyOf(payload), CaptureNs: ci.Timestamp.UnixNano(), Ordinal: ordinal, Payload: payload}); err != nil {
			return ScanResult{}, err
		}
		ordinal++
	}
	// Digest any trailing bytes the reader did not need.
	if _, err := io.Copy(io.Discard, br); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{SHA256: "sha256:" + hex.EncodeToString(h.Sum(nil)), Bytes: counted.n, Packets: ordinal}, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// udpPayload returns the UDP payload of a packet to or from port, as the
// replay's "udp port N" filter selects it. Ethernet carrying IPv4 is decoded
// directly; anything else goes through gopacket.
func udpPayload(data []byte, link layers.LinkType, port int) ([]byte, bool) {
	if link == layers.LinkTypeEthernet && len(data) >= 14+20+8 && data[12] == 0x08 && data[13] == 0x00 {
		ip := data[14:]
		ihl := int(ip[0]&0x0f) * 4
		if ip[0]>>4 == 4 && ihl >= 20 && ip[9] == 17 && len(ip) >= ihl+8 {
			// A fragment other than the first carries no UDP header.
			if binary.BigEndian.Uint16(ip[6:8])&0x1fff != 0 {
				return nil, false
			}
			udp := ip[ihl:]
			src, dst := int(binary.BigEndian.Uint16(udp[0:2])), int(binary.BigEndian.Uint16(udp[2:4]))
			if src != port && dst != port {
				return nil, false
			}
			length := int(binary.BigEndian.Uint16(udp[4:6]))
			if length < 8 || length > len(udp) {
				return nil, false
			}
			return udp[8:length], true
		}
	}
	pkt := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
	l := pkt.Layer(layers.LayerTypeUDP)
	if l == nil {
		return nil, false
	}
	udp := l.(*layers.UDP)
	if int(udp.SrcPort) != port && int(udp.DstPort) != port {
		return nil, false
	}
	return udp.Payload, true
}

// BindResult is what binding found, before it is recorded.
type BindResult struct {
	Binding    Binding
	Duplicates int // table keys the file holds more than once
}

// Bind checks which of the label set's packets a capture file holds and, when
// every key it holds appears once, records the file in labels.json. A key the
// file holds twice makes the file ambiguous for this label set and refuses
// the binding. The state is unchanged, so binding is not a revision.
func Bind(dir, capturePath string, port int, build string) (*BindResult, error) {
	ls, err := Open(dir)
	if err != nil {
		return nil, err
	}
	index := make(map[PacketKey]int, len(ls.State.Packets))
	for i, k := range ls.State.Packets {
		index[k] = i
	}
	found := make([]bool, len(ls.State.Packets))
	dups := 0
	scan, err := ScanCapture(capturePath, port, func(p Packet) error {
		i, ok := index[p.Key]
		if !ok {
			return nil
		}
		if found[i] {
			dups++
		}
		found[i] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	b := Binding{
		Name: filepath.Base(capturePath), SHA256: scan.SHA256, Bytes: scan.Bytes, UDPPort: port,
		VerifiedUTC: time.Now().UTC().Format(time.RFC3339), VerifiedBuild: build,
	}
	for _, f := range found {
		if f {
			b.PacketsFound++
		} else {
			b.PacketsMissing++
		}
	}
	per := ls.Manifest.Layout.PerPacket()
	for _, w := range ls.State.Windows {
		first, last := int(w.Start/per), int((w.End-1)/per)
		have := 0
		for i := first; i <= last; i++ {
			if found[i] {
				have++
			}
		}
		switch {
		case have == last-first+1:
			b.WindowsComplete++
		case have > 0:
			b.WindowsPartial++
		default:
			b.WindowsMissing++
		}
	}
	res := &BindResult{Binding: b, Duplicates: dups}
	if dups > 0 {
		return res, fmt.Errorf("%s holds %d of this label set's packets more than once; refusing to bind", b.Name, dups)
	}
	if b.PacketsFound == 0 {
		return res, fmt.Errorf("%s holds none of this label set's %d packets", b.Name, len(found))
	}
	if err := recordBinding(dir, b); err != nil {
		return res, err
	}
	return res, nil
}

func recordBinding(dir string, b Binding) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockSet(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	m, err := readManifest(root)
	if err != nil {
		return err
	}
	kept := m.AppliesTo[:0:0]
	for _, x := range m.AppliesTo {
		if x.SHA256 != b.SHA256 {
			kept = append(kept, x)
		}
	}
	m.AppliesTo = append(kept, b)
	return writeManifest(root, m)
}

// KeyCensus counts a capture's packets by key, for checking that keys are
// unique within a file.
type KeyCensus struct {
	ScanResult
	Unique     int
	Duplicated int // keys seen more than once
	Extra      int // packets beyond the first for duplicated keys
	// SamePayload counts duplicated keys whose payloads are byte-identical:
	// the same packet captured twice, rather than a hash collision.
	SamePayload int
	Examples    []string
}

// CensusOf reads a capture and counts duplicate packet keys.
func CensusOf(path string, port int) (*KeyCensus, error) {
	type seen struct {
		count  int
		digest [32]byte
	}
	keys := make(map[PacketKey]*seen)
	c := &KeyCensus{}
	identical := make(map[PacketKey]bool)
	scan, err := ScanCapture(path, port, func(p Packet) error {
		full := sha256.Sum256(p.Payload)
		s, ok := keys[p.Key]
		if !ok {
			keys[p.Key] = &seen{count: 1, digest: full}
			return nil
		}
		s.count++
		if s.count == 2 {
			identical[p.Key] = s.digest == full
			if len(c.Examples) < 5 {
				c.Examples = append(c.Examples, fmt.Sprintf("%s at packet %d", p.Key, p.Ordinal))
			}
		} else if s.digest != full {
			identical[p.Key] = false
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.ScanResult = scan
	for k, s := range keys {
		if s.count == 1 {
			c.Unique++
			continue
		}
		c.Duplicated++
		c.Extra += s.count - 1
		if identical[k] {
			c.SamePayload++
		}
	}
	return c, nil
}
