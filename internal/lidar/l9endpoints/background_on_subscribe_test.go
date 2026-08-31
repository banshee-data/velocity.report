package l9endpoints

import (
	"bytes"
	"os"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

// A background frame is published once and not repeated until the next
// refresh — 30s on live input, and on a replay only if the recording happens
// to contain another one. A client whose stream starts just after one was
// published therefore renders over nothing.
//
// That is not a rare race. Loading a replay does it every time: the server
// publishes the recording's background while handling the load, and the client
// restarts its stream immediately afterwards to pick the replay up. On
// 2026-08-27 the gap was 42ms and the grid never appeared.

func TestNewClientReceivesTheCurrentBackground(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	bg := &FrameBundle{
		FrameID:    7,
		FrameType:  FrameTypeBackground,
		Background: &BackgroundSnapshot{SequenceNumber: 3, X: []float32{1, 2, 3}},
	}
	p.rememberBackground(bg)

	client := p.addClient("late-joiner", &pb.StreamRequest{SensorId: "all"})

	select {
	case got := <-client.frameCh:
		if got.FrameType != FrameTypeBackground {
			t.Errorf("first frame type = %v, want background", got.FrameType)
		}
		if got.FrameID != 7 {
			t.Errorf("FrameID = %d, want the cached background's 7", got.FrameID)
		}
	default:
		t.Fatal("a client subscribing after the background was published received nothing; it would render over an empty scene")
	}
}

func TestNewClientGetsNothingWhenNoBackgroundHasBeenPublished(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	client := p.addClient("first-joiner", &pb.StreamRequest{SensorId: "all"})

	select {
	case got := <-client.frameCh:
		t.Fatalf("queued %+v with no background published yet", got)
	default:
	}
}

// Only backgrounds are remembered. Caching a foreground frame would replay one
// arbitrary moment of perception data to every future client.
func TestOnlyBackgroundFramesAreRemembered(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	p.rememberBackground(&FrameBundle{FrameID: 1, FrameType: FrameTypeForeground})
	p.rememberBackground(&FrameBundle{FrameID: 2, FrameType: FrameTypeEmpty})

	if got := p.latestBackground(); got != nil {
		t.Errorf("cached a non-background frame: %+v", got)
	}
}

// A later background supersedes the earlier one, so a new client gets the
// current scene rather than the first one ever sent.
func TestLatestBackgroundSupersedesTheEarlierOne(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	p.rememberBackground(&FrameBundle{FrameID: 1, FrameType: FrameTypeBackground})
	p.rememberBackground(&FrameBundle{FrameID: 2, FrameType: FrameTypeBackground})

	got := p.latestBackground()
	if got == nil || got.FrameID != 2 {
		t.Errorf("latestBackground() = %+v, want the most recent (FrameID 2)", got)
	}
}

func TestRememberBackgroundIgnoresNil(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})
	p.rememberBackground(nil)
	if got := p.latestBackground(); got != nil {
		t.Errorf("latestBackground() = %+v, want nil", got)
	}
}

// The gRPC handler must subscribe through addClient rather than registering a
// client itself. It used to do the latter, which duplicated the registration
// and skipped everything else addClient does — chiefly handing the new client
// the current background.
//
// The bug hid behind the recordings it was tested with: a VRLOG carrying its
// background at frame 2 looked correct, because the background arrived with the
// first frames anyway. One carrying it at frame 116 drew foreground over an
// empty grid until the next refresh.
func TestGRPCHandlerSubscribesThroughAddClient(t *testing.T) {
	src, err := os.ReadFile("grpc_server.go")
	if err != nil {
		t.Fatalf("read grpc_server.go: %v", err)
	}

	if bytes.Contains(src, []byte("s.publisher.clients[clientID] = &clientStream{")) {
		t.Error("streamFromPublisher registers its own client; that skips the background " +
			"handover addClient performs. Call s.publisher.addClient instead.")
	}
	if !bytes.Contains(src, []byte("s.publisher.addClient(")) {
		t.Error("streamFromPublisher should subscribe through addClient")
	}
}
