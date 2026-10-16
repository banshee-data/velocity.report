package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// This file is a deliberately small pcapng reader. It exists so the tool can
// seek a five-minute, 725 MB capture to a time and decode ten seconds of it
// without reading the minutes before: the archive's captures are pcapng from
// dumpcap, which has no index, and libpcap can only read forward. It reads one
// section, Enhanced Packet Blocks, and each interface's timestamp resolution;
// anything else is skipped by its length.

const (
	blockSectionHeader  = 0x0A0D0D0A
	blockInterfaceDesc  = 0x00000001
	blockEnhancedPacket = 0x00000006
	byteOrderMagic      = 0x1A2B3C4D

	// maxBlockLength bounds what a resynchronising reader accepts as a block
	// length; real blocks here are 1,340 bytes.
	maxBlockLength = 1 << 18
	// syncWindow is how far ahead of an arbitrary offset the reader looks for
	// the next block boundary.
	syncWindow = 1 << 16
	// seekSlack is where the binary search stops and the reader scans forward:
	// about 0.1 s of packets at the Pandar40P's 1,800 packets a second.
	seekSlack = 1 << 18
)

// ngFile is one pcapng capture opened for timed reads.
type ngFile struct {
	path  string
	f     *os.File
	size  int64
	order binary.ByteOrder
	// ifaces holds each interface's link type and timestamp resolution.
	ifaces []ngIface
	// firstBlock is the offset of the first block after the section and
	// interface headers.
	firstBlock int64
}

type ngIface struct {
	linkType uint16
	// ticksPerSecond is 10^n or 2^n from if_tsresol; 10^6 by default.
	ticksPerSecond float64
	// decimalNanosPerTick is set when ticks are a whole number of nanoseconds,
	// so conversion stays exact in integers.
	decimalNanosPerTick int64
}

func (i ngIface) nanos(ticks uint64) int64 {
	if i.decimalNanosPerTick > 0 {
		return int64(ticks) * i.decimalNanosPerTick
	}
	return int64(math.Round(float64(ticks) * 1e9 / i.ticksPerSecond))
}

// openPcapng opens path and reads its section and interface headers.
func openPcapng(path string) (*ngFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	n := &ngFile{path: path, f: f, size: st.Size()}
	if err := n.readHeaders(); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return n, nil
}

func (n *ngFile) Close() error { return n.f.Close() }

func (n *ngFile) readHeaders() error {
	head := make([]byte, 12)
	if _, err := n.f.ReadAt(head, 0); err != nil {
		return fmt.Errorf("read section header: %w", err)
	}
	if binary.BigEndian.Uint32(head[0:4]) != blockSectionHeader {
		return errors.New("not a pcapng file (classic pcap is not supported by this tool)")
	}
	switch {
	case binary.LittleEndian.Uint32(head[8:12]) == byteOrderMagic:
		n.order = binary.LittleEndian
	case binary.BigEndian.Uint32(head[8:12]) == byteOrderMagic:
		n.order = binary.BigEndian
	default:
		return errors.New("pcapng section header has no byte-order magic")
	}
	off := int64(0)
	for off < n.size {
		typ, length, err := n.blockHeaderAt(off)
		if err != nil {
			return err
		}
		switch typ {
		case blockSectionHeader:
			if off != 0 {
				return errors.New("more than one pcapng section is not supported")
			}
		case blockInterfaceDesc:
			body := make([]byte, length-12)
			if _, err := n.f.ReadAt(body, off+8); err != nil {
				return fmt.Errorf("read interface description: %w", err)
			}
			iface, err := parseInterface(body, n.order)
			if err != nil {
				return err
			}
			n.ifaces = append(n.ifaces, iface)
		case blockEnhancedPacket:
			n.firstBlock = off
			if len(n.ifaces) == 0 {
				return errors.New("packet block before any interface description")
			}
			return nil
		}
		off += int64(length)
	}
	n.firstBlock = n.size
	return nil
}

func (n *ngFile) blockHeaderAt(off int64) (uint32, uint32, error) {
	b := make([]byte, 8)
	if _, err := n.f.ReadAt(b, off); err != nil {
		return 0, 0, fmt.Errorf("read block header at %d: %w", off, err)
	}
	// The section header's type word is a palindrome, so it reads the same
	// in either byte order.
	typ := n.order.Uint32(b[0:4])
	length := n.order.Uint32(b[4:8])
	if length < 12 || length%4 != 0 || length > maxBlockLength {
		return 0, 0, fmt.Errorf("implausible block length %d at offset %d", length, off)
	}
	return typ, length, nil
}

// parseInterface reads an Interface Description Block body (after the type and
// length words, without the trailing length).
func parseInterface(body []byte, order binary.ByteOrder) (ngIface, error) {
	if len(body) < 8 {
		return ngIface{}, errors.New("short interface description")
	}
	iface := ngIface{linkType: order.Uint16(body[0:2]), ticksPerSecond: 1e6, decimalNanosPerTick: 1000}
	opts := body[8:]
	for len(opts) >= 4 {
		code := order.Uint16(opts[0:2])
		l := int(order.Uint16(opts[2:4]))
		if code == 0 {
			break
		}
		padded := (l + 3) &^ 3
		if 4+padded > len(opts) {
			return ngIface{}, errors.New("interface option overruns its block")
		}
		if code == 9 && l >= 1 { // if_tsresol
			v := opts[4]
			if v&0x80 == 0 {
				exp := int(v)
				iface.ticksPerSecond = math.Pow10(exp)
				iface.decimalNanosPerTick = 0
				if exp <= 9 {
					iface.decimalNanosPerTick = int64(math.Pow10(9 - exp))
				}
			} else {
				iface.ticksPerSecond = math.Exp2(float64(v & 0x7f))
				iface.decimalNanosPerTick = 0
			}
		}
		opts = opts[4+padded:]
	}
	return iface, nil
}

// ngPacket is one Enhanced Packet Block's capture time and frame bytes. Data
// is only valid until the next read.
type ngPacket struct {
	UnixNanos int64
	Iface     int
	Data      []byte
}

// syncAt finds the first Enhanced Packet Block at or after off, validating its
// trailing length and the next block's, and returns its offset and time.
// ok is false when no block starts in the window after off.
func (n *ngFile) syncAt(off int64) (int64, int64, bool, error) {
	if off < n.firstBlock {
		off = n.firstBlock
	}
	off &^= 3
	buf := make([]byte, syncWindow+2*maxBlockLength)
	read, err := n.f.ReadAt(buf, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, 0, false, err
	}
	buf = buf[:read]
	for p := 0; p+28 <= len(buf) && p < syncWindow; p += 4 {
		if n.order.Uint32(buf[p:p+4]) != blockEnhancedPacket {
			continue
		}
		length := int(n.order.Uint32(buf[p+4 : p+8]))
		if length < 32 || length%4 != 0 || length > maxBlockLength || p+length > len(buf) {
			continue
		}
		if int(n.order.Uint32(buf[p+length-4:p+length])) != length {
			continue
		}
		// The block after must also be well formed, unless this one ends the file.
		next := p + length
		if off+int64(next) < n.size {
			if next+8 > len(buf) {
				continue
			}
			nl := int(n.order.Uint32(buf[next+4 : next+8]))
			if nl < 12 || nl%4 != 0 || nl > maxBlockLength ||
				(next+nl <= len(buf) && int(n.order.Uint32(buf[next+nl-4:next+nl])) != nl) {
				continue
			}
		}
		ifaceID := int(n.order.Uint32(buf[p+8 : p+12]))
		if ifaceID >= len(n.ifaces) {
			continue
		}
		ticks := uint64(n.order.Uint32(buf[p+12:p+16]))<<32 | uint64(n.order.Uint32(buf[p+16:p+20]))
		return off + int64(p), n.ifaces[ifaceID].nanos(ticks), true, nil
	}
	return 0, 0, false, nil
}

// seekOffset returns a block offset at or before the first packet with time at
// or after unixNanos, within about seekSlack bytes of it. Capture time is
// monotonic to well within that slack in field captures; the forward scan that
// follows skips earlier packets by their own times.
func (n *ngFile) seekOffset(unixNanos int64) (int64, error) {
	lo, hi := n.firstBlock, n.size
	for hi-lo > seekSlack {
		mid := lo + (hi-lo)/2
		off, ts, ok, err := n.syncAt(mid)
		if err != nil {
			return 0, err
		}
		if !ok || off >= hi {
			hi = mid
			continue
		}
		if ts < unixNanos {
			lo = off
		} else {
			hi = mid
		}
	}
	off, _, ok, err := n.syncAt(lo)
	if err != nil {
		return 0, err
	}
	if !ok {
		return n.size, nil
	}
	return off, nil
}

// readFrom calls fn for each Enhanced Packet Block from offset off to the end
// of the file, until fn returns false.
func (n *ngFile) readFrom(off int64, fn func(ngPacket) bool) error {
	r := bufio.NewReaderSize(io.NewSectionReader(n.f, off, n.size-off), 1<<20)
	head := make([]byte, 8)
	body := make([]byte, 0, 4096)
	for {
		if _, err := io.ReadFull(r, head); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("%s: truncated block header", n.path)
			}
			return err
		}
		typ := n.order.Uint32(head[0:4])
		length := int(n.order.Uint32(head[4:8]))
		if length < 12 || length%4 != 0 || length > maxBlockLength {
			return fmt.Errorf("%s: implausible block length %d", n.path, length)
		}
		if cap(body) < length-8 {
			body = make([]byte, length-8)
		}
		body = body[:length-8]
		if _, err := io.ReadFull(r, body); err != nil {
			// A capture cut off mid-block ends where its last whole block does.
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		if typ != blockEnhancedPacket {
			continue
		}
		if len(body) < 24 {
			return fmt.Errorf("%s: short packet block", n.path)
		}
		ifaceID := int(n.order.Uint32(body[0:4]))
		if ifaceID >= len(n.ifaces) {
			return fmt.Errorf("%s: packet names interface %d of %d", n.path, ifaceID, len(n.ifaces))
		}
		ticks := uint64(n.order.Uint32(body[4:8]))<<32 | uint64(n.order.Uint32(body[8:12]))
		capLen := int(n.order.Uint32(body[12:16]))
		if 20+capLen > len(body) {
			return fmt.Errorf("%s: packet overruns its block", n.path)
		}
		if !fn(ngPacket{UnixNanos: n.ifaces[ifaceID].nanos(ticks), Iface: ifaceID, Data: body[20 : 20+capLen]}) {
			return nil
		}
	}
}

// extent returns the capture times of the first and last packets.
func (n *ngFile) extent() (int64, int64, error) {
	var first, last int64
	found := false
	if err := n.readFrom(n.firstBlock, func(p ngPacket) bool {
		first, found = p.UnixNanos, true
		return false
	}); err != nil {
		return 0, 0, err
	}
	if !found {
		return 0, 0, fmt.Errorf("%s: no packets", n.path)
	}
	tail := n.size - seekSlack
	if tail < n.firstBlock {
		tail = n.firstBlock
	}
	off, _, ok, err := n.syncAt(tail)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		off = n.firstBlock
	}
	if err := n.readFrom(off, func(p ngPacket) bool {
		if p.UnixNanos > last {
			last = p.UnixNanos
		}
		return true
	}); err != nil {
		return 0, 0, err
	}
	return first, last, nil
}

// udpPayload returns the UDP payload of an Ethernet or raw-IPv4 frame sent to
// dstPort, or nil.
func udpPayload(linkType uint16, frame []byte, dstPort uint16) []byte {
	var ip []byte
	switch linkType {
	case 1: // Ethernet
		if len(frame) < 14 {
			return nil
		}
		etherType := binary.BigEndian.Uint16(frame[12:14])
		ip = frame[14:]
		if etherType == 0x8100 && len(frame) >= 18 { // one VLAN tag
			etherType = binary.BigEndian.Uint16(frame[16:18])
			ip = frame[18:]
		}
		if etherType != 0x0800 {
			return nil
		}
	case 101, 228: // raw IPv4
		ip = frame
	default:
		return nil
	}
	if len(ip) < 20 || ip[0]>>4 != 4 || ip[9] != 17 {
		return nil
	}
	if binary.BigEndian.Uint16(ip[6:8])&0x3fff != 0 { // fragment
		return nil
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl+8 {
		return nil
	}
	udp := ip[ihl:]
	if binary.BigEndian.Uint16(udp[2:4]) != dstPort {
		return nil
	}
	udpLen := int(binary.BigEndian.Uint16(udp[4:6]))
	if udpLen < 8 || udpLen > len(udp) {
		return nil
	}
	return udp[8:udpLen]
}
