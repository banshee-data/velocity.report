package l9endpoints

import (
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
