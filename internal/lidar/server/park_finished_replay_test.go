package server

import "testing"

// A finished replay stays the data source until an operator says otherwise.
//
// Deciding this automatically is what the previous designs kept getting wrong.
// Choosing from the analysis flag stranded settle-before-recording runs, which
// carry that flag only because the handler requires analysis_mode=true so the
// second pass can record. Choosing from whether the sensor happened to be
// streaming replaced that with a different surprise: the pipeline could be
// taken back to live, and the grid reset, while someone was still looking at
// the replay.

func TestParkFinishedReplayKeepsTheSource(t *testing.T) {
	ws := &Server{state: newPipelineState(), sensorID: "park-test"}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")

	ws.parkFinishedReplay("replay reached the end")

	got := ws.PipelineState()
	if got.Source != SourceModeVRLog {
		t.Errorf("Source = %q, want %q: a finished replay stays the source", got.Source, SourceModeVRLog)
	}
	if got.SourcePath == "" {
		t.Error("SourcePath was cleared; the recording is still loaded")
	}
	if got.ReplayActive {
		t.Error("ReplayActive still set; the slot must be free so another replay can start")
	}
	if got.LiveListenerRunning {
		t.Error("the live listener was started; returning to live is an explicit operator action")
	}
}

// The grid a replay built is left intact — that is the whole point of holding
// the source, so the scene can be inspected after playback ends.
func TestParkFinishedReplayKeepsAnAnalysisGrid(t *testing.T) {
	ws := &Server{state: newPipelineState(), sensorID: "park-test"}
	if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{AnalysisMode: true}); !ok {
		t.Fatal("could not claim the replay slot")
	}

	ws.parkFinishedReplay("replay reached the end")

	got := ws.PipelineState()
	if !got.GridPreserved {
		t.Error("GridPreserved was cleared; the analysis grid is the reason for parking")
	}
	if wire := got.DataSourceWire(); wire != string(DataSourcePCAPAnalysis) {
		t.Errorf("DataSourceWire() = %q, want %q", wire, DataSourcePCAPAnalysis)
	}
}

// Parking must leave the slot free, or the next replay is refused.
func TestAnotherReplayCanStartAfterParking(t *testing.T) {
	ws := &Server{state: newPipelineState(), sensorID: "park-test"}
	if ok, _ := ws.tryBeginPCAPReplay(ReplayConfig{}); !ok {
		t.Fatal("could not claim the replay slot")
	}
	ws.parkFinishedReplay("replay reached the end")

	if ok, blocker := ws.tryBeginPCAPReplay(ReplayConfig{}); !ok {
		t.Errorf("a second replay was refused after parking, blocked by %q", blocker)
	}
}

// Returning to live remains explicit, and it clears the grid.
func TestReturnToLiveIsTheWayOutOfAParkedReplay(t *testing.T) {
	ws := &Server{state: newPipelineState(), sensorID: "park-test"}
	ws.setSourceVRLog("/var/lib/velocity-report/vrlog/run-abc")
	ws.parkFinishedReplay("replay reached the end")

	ws.endReplayAndClaimLive()

	got := ws.PipelineState()
	if got.Source != SourceModeLive {
		t.Errorf("Source = %q, want %q", got.Source, SourceModeLive)
	}
	if got.GridPreserved {
		t.Error("GridPreserved still set; going live clears the grid")
	}
}
