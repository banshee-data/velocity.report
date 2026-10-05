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
// about a quarter of all samples. Reading in place removes it.
//
// The error handling is packetsToChannel's, so the sequence of packets is the
// same: temporary network errors and EAGAIN retry at once; EOF and the other
// unrecoverable errors end the source; anything else waits pcapRetryPause and
// retries.
func nextPacket(src *gopacket.PacketSource) gopacket.Packet {
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
		time.Sleep(pcapRetryPause)
	}
}
