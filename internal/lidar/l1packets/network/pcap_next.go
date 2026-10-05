package network

import (
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/google/gopacket"
)

// pcapRetryPause is how long nextPacket waits before retrying a read error it
// does not recognise, as gopacket's own reader goroutine does.
var pcapRetryPause = 5 * time.Millisecond

// nextPacket reads the next packet from src on the caller's goroutine, and
// returns nil when the source is exhausted.
//
// gopacket's PacketSource.Packets() reads on a goroutine of its own and hands
// each packet over a channel. An offline replay consumes packets more slowly
// than that goroutine reads them, so the channel stays full and every receive
// wakes the blocked reader: one cross-thread wake-up per packet. In a corpus
// replay's CPU profile that handoff was most of the Go scheduler's share,
// about a quarter of all samples. CountPCAPPackets reads with this directly;
// a replay reads through packetReader, which calls it on its own goroutine.
//
// The error handling is packetsToChannel's, so the sequence of packets is the
// same: temporary network errors and EAGAIN retry at once; EOF and the other
// unrecoverable errors end the source; anything else waits pcapRetryPause and
// retries.
func nextPacket(src *gopacket.PacketSource) gopacket.Packet {
	return nextPacketOrStop(src, nil)
}

// nextPacketOrStop is nextPacket that also returns nil, at its next retry,
// once stop is closed. A nil stop never fires.
func nextPacketOrStop(src *gopacket.PacketSource, stop <-chan struct{}) gopacket.Packet {
	for {
		packet, err := src.NextPacket()
		if err == nil {
			return packet
		}
		if nerr, ok := err.(net.Error); ok && nerr.Temporary() { //nolint:staticcheck // mirrors gopacket
			continue
		}
		if err == syscall.EAGAIN {
			continue
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF ||
			err == io.ErrNoProgress || err == io.ErrClosedPipe || err == io.ErrShortBuffer ||
			err == syscall.EBADF ||
			strings.Contains(err.Error(), "use of closed file") {
			return nil
		}
		select {
		case <-stop:
			return nil
		case <-time.After(pcapRetryPause):
		}
	}
}

// pcapBatchSize is how many packets packetReader hands over at a time.
const pcapBatchSize = 256

// packetReader reads a PacketSource on a goroutine of its own and hands the
// packets over in batches, in order.
//
// Reading on the replay's goroutine (nextPacket) removed gopacket's
// per-packet wake-up but put the read itself, libpcap through cgo, on the
// replay's critical path. A reader goroutine keeps the read beside the
// replay, and batching keeps the handoff to one wake-up per batch.
type packetReader struct {
	batches chan []gopacket.Packet
	stop    chan struct{}
	done    chan struct{}
	current []gopacket.Packet
	next    int
}

// newPacketReader starts reading src. Close must be called before the source
// is closed: it stops the goroutine and waits for it.
func newPacketReader(src *gopacket.PacketSource, batchSize int) *packetReader {
	r := &packetReader{
		batches: make(chan []gopacket.Packet, 2),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go func() {
		defer close(r.done)
		defer close(r.batches)
		for {
			batch := make([]gopacket.Packet, 0, batchSize)
			for len(batch) < batchSize {
				packet := nextPacketOrStop(src, r.stop)
				if packet == nil {
					break
				}
				batch = append(batch, packet)
			}
			if len(batch) > 0 {
				select {
				case r.batches <- batch:
				case <-r.stop:
					return
				}
			}
			if len(batch) < batchSize {
				return // the source is exhausted, or the reader was stopped
			}
		}
	}()
	return r
}

// Next returns the next packet in source order, or nil once the source is
// exhausted.
func (r *packetReader) Next() gopacket.Packet {
	for r.next >= len(r.current) {
		batch, ok := <-r.batches
		if !ok {
			return nil
		}
		r.current, r.next = batch, 0
	}
	packet := r.current[r.next]
	r.current[r.next] = nil // let the batch's packets go as they are used
	r.next++
	return packet
}

// Close stops the reader and waits for its goroutine to return, so the
// source can be closed safely afterwards. Call it once.
func (r *packetReader) Close() {
	close(r.stop)
	<-r.done
}
