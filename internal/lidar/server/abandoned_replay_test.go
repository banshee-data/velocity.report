package server

import "testing"

// A PCAP start claims the replay slot and names PCAP as the source before it
// resolves the file. When the start then fails — a path the server cannot see,
// most often — the claim has to be given back in full.
//
// Leaving the source on PCAP stranded every client on "REPLAY (PCAP)" while
// live packets flowed, and nothing later corrected it: no replay had ended, so
// no teardown ran and the parked-replay watcher was never armed. Five failed
// starts in a row on 2026-08-31 left the pipeline in exactly that state.

func TestAbandonedStartRestoresTheDisplacedSource(t *testing.T) {
	for _, before := range []SourceMode{SourceModeLive, SourceModeVRLog} {
		t.Run(string(before), func(t *testing.T) {
			ws := &Server{}
			ws.state.Source = before
			ws.state.SourcePath = "/some/recording"

			ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{})
			if !ok {
				t.Fatal("the claim was refused from an idle pipeline")
			}
			if ws.PipelineState().Source != SourceModePCAP {
				t.Fatal("the claim did not name PCAP")
			}

			ws.abandonReplayStart()

			if got := ws.PipelineState().Source; got != before {
				t.Errorf("source is %q after an abandoned start, want the displaced %q", got, before)
			}
			if got := ws.PipelineState().ReplayActive; got {
				t.Error("ReplayActive stayed set after an abandoned start")
			}
		})
	}
}

// Abandoning twice must not resurrect a stale source: the second call has
// nothing to restore.
func TestAbandonIsIdempotent(t *testing.T) {
	ws := &Server{}
	ws.state.Source = SourceModeLive

	ws.tryBeginPCAPReplay(ReplayConfig{})
	ws.abandonReplayStart()

	// A later, unrelated source change must survive a second abandon.
	ws.mutateState("test", func(s *PipelineState) { s.Source = SourceModeVRLog })
	ws.abandonReplayStart()

	if got := ws.PipelineState().Source; got != SourceModeVRLog {
		t.Errorf("source is %q, want vrlog: a second abandon restored a stale source", got)
	}
}

// A successful start keeps PCAP: the claim was not abandoned.
func TestSuccessfulClaimKeepsPCAP(t *testing.T) {
	ws := &Server{}
	ws.state.Source = SourceModeLive

	ws.tryBeginPCAPReplay(ReplayConfig{})
	ws.markReplayRunning("/capture.pcap", "scaled", 0.1, "run-1")

	if got := ws.PipelineState().Source; got != SourceModePCAP {
		t.Errorf("source is %q after a successful start, want pcap", got)
	}
}
