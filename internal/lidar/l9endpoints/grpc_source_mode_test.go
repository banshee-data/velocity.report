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
		// An empty or unknown token leaves older or unwired servers on the
		// client's is_live/seekable inference rather than asserting a mode.
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
			IsLive:     false,
			Seekable:   true,
			SourceMode: "vrlog",
			Recording:  true,
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
	// is_live stays populated so older clients keep working.
	if pbFrame.PlaybackInfo.IsLive {
		t.Error("IsLive = true for a VRLOG replay")
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
