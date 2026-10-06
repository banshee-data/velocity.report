package l9endpoints

import (
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

func TestSourceModeToProto(t *testing.T) {
	tests := []struct {
		token string
		want  pb.SourceMode
	}{
		{"live", pb.SourceMode_SOURCE_MODE_LIVE},
		{"pcap", pb.SourceMode_SOURCE_MODE_PCAP},
		{"pcap_analysis", pb.SourceMode_SOURCE_MODE_PCAP_ANALYSIS},
		{"vrlog", pb.SourceMode_SOURCE_MODE_VRLOG},
		// An empty or unknown token must not assert a mode.
		{"", pb.SourceMode_SOURCE_MODE_UNSPECIFIED},
		{"nonsense", pb.SourceMode_SOURCE_MODE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			if got := sourceModeToProto(tt.token); got != tt.want {
				t.Errorf("sourceModeToProto(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestFrameBundleCarriesSourceMode(t *testing.T) {
	frame := &FrameBundle{
		FrameID: 1,
		PlaybackInfo: &PlaybackInfo{
			Seekable:            true,
			SourceMode:          "vrlog",
			Recording:           true,
			Settling:            true,
			SettlingElapsedSecs: 4.25,
			SensorSilent:        true,
			ReplayEpoch:         9,
		},
	}

	pbFrame := frameBundleToProto(frame, &pb.StreamRequest{})
	if pbFrame.PlaybackInfo == nil {
		t.Fatal("PlaybackInfo missing from converted frame")
	}
	if got := pbFrame.PlaybackInfo.SourceMode; got != pb.SourceMode_SOURCE_MODE_VRLOG {
		t.Errorf("SourceMode = %v, want %v", got, pb.SourceMode_SOURCE_MODE_VRLOG)
	}
	if !pbFrame.PlaybackInfo.Recording {
		t.Error("Recording not carried through to the wire")
	}
	if !pbFrame.PlaybackInfo.Settling {
		t.Error("Settling not carried through to the wire")
	}
	if got := pbFrame.PlaybackInfo.SettlingElapsedSeconds; got != 4.25 {
		t.Errorf("SettlingElapsedSeconds = %v, want 4.25", got)
	}
	if !pbFrame.PlaybackInfo.SensorSilent {
		t.Error("SensorSilent not carried through to the wire")
	}
	if got := pbFrame.PlaybackInfo.ReplayEpoch; got != 9 {
		t.Errorf("ReplayEpoch = %d, want 9", got)
	}
	// Seekability is an independent axis and must survive unchanged.
	if !pbFrame.PlaybackInfo.Seekable {
		t.Error("Seekable was not preserved")
	}
}

func TestServerSourceModeProvider(t *testing.T) {
	s := NewServer(nil)

	if mode, recording := s.currentSourceMode(); mode != "" || recording {
		t.Errorf("unwired provider returned (%q, %v), want (\"\", false)", mode, recording)
	}

	s.SetSourceModeProvider(func() (string, bool) { return "pcap_analysis", true })
	mode, recording := s.currentSourceMode()
	if mode != "pcap_analysis" || !recording {
		t.Errorf("currentSourceMode() = (%q, %v), want (\"pcap_analysis\", true)", mode, recording)
	}
}

func TestServerSensorSilentProvider(t *testing.T) {
	s := NewServer(nil)

	if s.currentSensorSilent() {
		t.Fatal("unwired sensor-silent provider reported silence")
	}

	s.SetSensorSilentProvider(func() bool { return true })
	if !s.currentSensorSilent() {
		t.Fatal("wired sensor-silent provider result was not returned")
	}

	s.SetSensorSilentProvider(func() bool { return false })
	if s.currentSensorSilent() {
		t.Fatal("false sensor-silent provider result was not returned")
	}
}

func TestDecoratePlaybackInfoComposesLiveState(t *testing.T) {
	s := NewServer(nil)
	s.SetSourceModeProvider(func() (string, bool) { return "live", false })
	s.SetSettlingProvider(func() (bool, float32) { return true, 4.25 })
	s.SetSensorSilentProvider(func() bool { return true })

	frame := &FrameBundle{}
	s.decoratePlaybackInfo(frame)

	if frame.PlaybackInfo == nil {
		t.Fatal("live frame received no PlaybackInfo")
	}
	got := frame.PlaybackInfo
	if got.SourceMode != "live" {
		t.Errorf("SourceMode = %q, want live", got.SourceMode)
	}
	if got.PlaybackRate != 1 {
		t.Errorf("PlaybackRate = %v, want live default 1", got.PlaybackRate)
	}
	if !got.Settling || got.SettlingElapsedSecs != 4.25 {
		t.Errorf("settling = (%v, %v), want (true, 4.25)", got.Settling, got.SettlingElapsedSecs)
	}
	if !got.SensorSilent {
		t.Error("SensorSilent = false, want true")
	}
}

func TestDecoratePlaybackInfoKeepsLiveOnlyStateOffReplays(t *testing.T) {
	for _, mode := range []string{"pcap", "pcap_analysis", "vrlog"} {
		t.Run(mode, func(t *testing.T) {
			s := NewServer(nil)
			s.SetSourceModeProvider(func() (string, bool) { return mode, true })
			s.SetSettlingProvider(func() (bool, float32) { return true, 9 })
			s.SetSensorSilentProvider(func() bool { return true })

			frame := &FrameBundle{PlaybackInfo: &PlaybackInfo{
				PlaybackRate:        2,
				Settling:            true,
				SettlingElapsedSecs: 7,
				SensorSilent:        true,
			}}
			s.decoratePlaybackInfo(frame)

			got := frame.PlaybackInfo
			if got.SourceMode != mode || !got.Recording {
				t.Errorf("source state = (%q, %v), want (%q, true)", got.SourceMode, got.Recording, mode)
			}
			if got.Settling || got.SettlingElapsedSecs != 0 {
				t.Errorf("replay inherited live settling state: %+v", got)
			}
			if got.SensorSilent {
				t.Error("replay inherited live sensor silence")
			}
			if got.PlaybackRate != 2 {
				t.Errorf("existing PlaybackRate = %v, want 2", got.PlaybackRate)
			}
		})
	}
}

func TestDecoratePlaybackInfoDoesNotInventAnUnwiredSource(t *testing.T) {
	s := NewServer(nil)
	s.SetSettlingProvider(func() (bool, float32) { return true, 3 })
	s.SetSensorSilentProvider(func() bool { return true })

	frame := &FrameBundle{}
	s.decoratePlaybackInfo(frame)

	if frame.PlaybackInfo != nil {
		t.Errorf("PlaybackInfo = %+v, want nil without a reported source", frame.PlaybackInfo)
	}
}

func TestDecoratePlaybackInfoCarriesElapsedSettlingAfterConvergence(t *testing.T) {
	s := NewServer(nil)
	s.SetSourceModeProvider(func() (string, bool) { return "live", false })
	s.SetSettlingProvider(func() (bool, float32) { return false, 5.9 })

	frame := &FrameBundle{}
	s.decoratePlaybackInfo(frame)

	if frame.PlaybackInfo == nil {
		t.Fatal("live frame received no PlaybackInfo")
	}
	if frame.PlaybackInfo.Settling {
		t.Error("Settling = true after convergence")
	}
	if got := frame.PlaybackInfo.SettlingElapsedSecs; got != 5.9 {
		t.Errorf("SettlingElapsedSecs = %v, want the final 5.9 seconds", got)
	}
}

func TestDecoratePlaybackInfoClearsStaleLiveAnnotations(t *testing.T) {
	s := NewServer(nil)
	s.SetSourceModeProvider(func() (string, bool) { return "live", false })
	s.SetSettlingProvider(func() (bool, float32) { return false, 0 })
	s.SetSensorSilentProvider(func() bool { return false })

	frame := &FrameBundle{PlaybackInfo: &PlaybackInfo{
		Settling:            true,
		SettlingElapsedSecs: 8,
		SensorSilent:        true,
	}}
	s.decoratePlaybackInfo(frame)

	got := frame.PlaybackInfo
	if got.Settling || got.SettlingElapsedSecs != 0 || got.SensorSilent {
		t.Errorf("stale live annotations survived a current healthy frame: %+v", got)
	}
}
