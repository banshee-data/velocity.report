package network

import (
	"errors"
	"io"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/google/gopacket"
)

// scriptedSource is a PacketDataSource that returns a fixed sequence of
// packet data and errors, then io.EOF.
type scriptedSource struct {
	steps []scriptStep
	next  int
}

type scriptStep struct {
	data []byte
	err  error
}

func (s *scriptedSource) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	if s.next >= len(s.steps) {
		return nil, gopacket.CaptureInfo{}, io.EOF
	}
	step := s.steps[s.next]
	s.next++
	if step.err != nil {
		return nil, gopacket.CaptureInfo{}, step.err
	}
	return step.data, gopacket.CaptureInfo{CaptureLength: len(step.data), Length: len(step.data)}, nil
}

// temporaryError is a net.Error that reports itself temporary.
type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

func newScriptedPacketSource(steps ...scriptStep) *gopacket.PacketSource {
	return gopacket.NewPacketSource(&scriptedSource{steps: steps}, gopacket.DecodePayload)
}

func drainNext(src *gopacket.PacketSource) [][]byte {
	var out [][]byte
	for packet := nextPacket(src); packet != nil; packet = nextPacket(src) {
		out = append(out, packet.Data())
	}
	return out
}

func withRetryPause(t *testing.T, d time.Duration) {
	t.Helper()
	saved := pcapRetryPause
	pcapRetryPause = d
	t.Cleanup(func() { pcapRetryPause = saved })
}

// Temporary errors and EAGAIN are retried at once and a stray unknown error
// after a pause, so every packet arrives, in order, and the source then ends.
func TestNextPacketRetriesAsGopacketDoes(t *testing.T) {
	withRetryPause(t, 0)
	src := newScriptedPacketSource(
		scriptStep{data: []byte{1}},
		scriptStep{err: syscall.EAGAIN},
		scriptStep{err: temporaryError{}},
		scriptStep{data: []byte{2}},
		scriptStep{err: errors.New("unrecognised read error")},
		scriptStep{data: []byte{3}},
	)
	got := drainNext(src)
	if want := [][]byte{{1}, {2}, {3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if nextPacket(src) != nil {
		t.Fatal("an exhausted source returned another packet")
	}
}

// Each error gopacket treats as unrecoverable ends the source.
func TestNextPacketEndsOnUnrecoverableErrors(t *testing.T) {
	for _, end := range []error{
		io.EOF, io.ErrUnexpectedEOF, io.ErrNoProgress, io.ErrClosedPipe, io.ErrShortBuffer,
		syscall.EBADF, errors.New("read: use of closed file"),
	} {
		src := newScriptedPacketSource(scriptStep{data: []byte{7}}, scriptStep{err: end}, scriptStep{data: []byte{8}})
		if got := drainNext(src); !reflect.DeepEqual(got, [][]byte{{7}}) {
			t.Errorf("%v: got %v, want only the packet before it", end, got)
		}
	}
}

// Reading in place yields exactly the packets the Packets() channel does.
func TestNextPacketMatchesPacketsChannel(t *testing.T) {
	steps := make([]scriptStep, 0, 2600)
	for i := 0; i < 2500; i++ {
		steps = append(steps, scriptStep{data: []byte{byte(i), byte(i >> 8)}})
		if i%97 == 0 {
			steps = append(steps, scriptStep{err: syscall.EAGAIN}, scriptStep{err: temporaryError{}})
		}
	}
	viaNext := drainNext(newScriptedPacketSource(steps...))
	var viaChannel [][]byte
	for packet := range newScriptedPacketSource(steps...).Packets() {
		viaChannel = append(viaChannel, packet.Data())
	}
	if len(viaNext) != 2500 || !reflect.DeepEqual(viaNext, viaChannel) {
		t.Fatalf("in place: %d packets, channel: %d, equal %v", len(viaNext), len(viaChannel), reflect.DeepEqual(viaNext, viaChannel))
	}
}
