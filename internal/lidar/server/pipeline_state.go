package server

// PipelineState is the single authoritative answer to two questions: what is
// driving the LiDAR pipeline right now, and what is being captured from it.
//
// Before this type existed the answer was split across three stores that could
// disagree — the monitor server's `currentSource`/`pcapInProgress` pair under two
// separate mutexes, the visualiser gRPC server's `replayMode`/`vrlogMode`
// booleans, and a pair of closure locals in the radar command that held the
// VRLOG recorder. Surfaces rendered whichever store was nearest, so a VRLOG
// replay reported itself as live and recording was unobservable.
//
// Locking contract: the value is guarded by Server.stateMu, which is held only
// for struct copies. It must NEVER be held across a call into another
// subsystem — not ws.onRecordingStart, ws.onPCAPStarted, ws.onVRLogLoad, nor any
// l9endpoints method. onRecordingStart reaches applyRecordingMetadata, which
// calls back into Server.CurrentSource(); holding a state lock across that path
// would deadlock.

// SourceMode names what is driving the pipeline right now.
//
// The vocabulary matches the canonical source-mode set recorded in
// docs/platform/architecture/metrics-registry.md. Note that `pcap_analysis`
// is not a member: it is a derived wire token (see DataSourceWire) covering
// "a PCAP produced this grid, the replay has finished, and the grid is being
// retained for inspection" — a source plus a retention flag, not a source.
type SourceMode string

// Recognised pipeline source modes.
const (
	SourceModeLive  SourceMode = "live"
	SourceModePCAP  SourceMode = "pcap"
	SourceModeVRLog SourceMode = "vrlog"
)

// ReplayPass identifies which pass of a multi-pass replay is running.
//
// A settle-before-recording replay runs the selected window twice: once unpaced
// and unrecorded to settle the background grid, then again from the same offset
// while recording. Both passes report the same source and the same
// in-progress flag, so without this field the two are indistinguishable and
// packet progress appears to run 0-100% twice for no visible reason.
type ReplayPass string

// Recognised replay passes. ReplayPassNone covers single-pass replays.
const (
	ReplayPassNone      ReplayPass = ""
	ReplayPassSettling  ReplayPass = "settling"
	ReplayPassRecording ReplayPass = "recording"
)

// PipelineState describes the active data source and capture state. Callers
// receive a copy from Server.PipelineState and can read it without a lock.
type PipelineState struct {
	// Source and its provenance.
	Source              SourceMode
	SourcePath          string // PCAP file or VRLOG directory; empty when live
	LiveListenerRunning bool

	// Replay progress. ReplayActive distinguishes "a replay is running" from
	// "a replay finished and left state behind", which the legacy
	// pcap/pcap_analysis enum could not express.
	ReplayActive  bool
	Pass          ReplayPass
	TotalPasses   int  // 1, or 2 under settle-before-recording
	GridPreserved bool // analysis-mode grid retained across transitions

	// VRLOG capture.
	Recording       bool
	RecordingPath   string
	RecordingRunID  string
	RecordingFrames uint64

	// Replay pacing and packet progress.
	CurrentPacket uint64
	TotalPackets  uint64
	SpeedMode     string
	SpeedRatio    float64
	LastRunID     string
}

// newPipelineState returns the state a freshly constructed server starts in:
// live input, nothing replaying, nothing recording.
func newPipelineState() PipelineState {
	return PipelineState{
		Source:      SourceModeLive,
		TotalPasses: 1,
	}
}

// DataSourceWire renders the state as the legacy data-source token used by
// GET /api/lidar/data_source, GET /api/lidar/status, and the PCAP control
// responses.
//
// `pcap_analysis` is derived rather than stored because it conflates two
// independent facts. Analysis mode both selects a PCAP source and retains the
// grid afterwards, and resume-live keeps the grid while returning to live
// input — a combination the single enum cannot name, which is why
// handlePCAPResumeLive had to report grid preservation in a separate ad-hoc
// field that then vanished from every subsequent status read.
func (s PipelineState) DataSourceWire() string {
	if s.Source == SourceModePCAP && !s.ReplayActive && s.GridPreserved {
		return string(DataSourcePCAPAnalysis)
	}
	return string(s.Source)
}

// PCAPInProgress reports whether a PCAP replay is currently running.
//
// This is deliberately narrower than "any replay is active": Client.
// WaitForPCAPComplete gates on the pcap_in_progress field this feeds, and the
// sweep runner calls it once per parameter combination. Reporting true during a
// VRLOG replay would make every sweep block until its timeout.
func (s PipelineState) PCAPInProgress() bool {
	return s.Source == SourceModePCAP && s.ReplayActive
}

// AnalysisMode reports whether the active or most recent PCAP replay was
// started in analysis mode, which is what the grid-retention flag records.
func (s PipelineState) AnalysisMode() bool {
	return s.Source == SourceModePCAP && s.GridPreserved
}

// StatusLabel renders the human-readable mode shown on the HTML status page.
func (s PipelineState) StatusLabel() string {
	switch {
	case s.Source == SourceModeVRLog:
		return "VRLOG Replay"
	case s.Source == SourceModePCAP && s.ReplayActive:
		return "PCAP Replay"
	case s.Source == SourceModePCAP:
		return "PCAP Analysis"
	default:
		return "Live UDP"
	}
}

// PipelineState returns a snapshot of the current pipeline state.
func (ws *Server) PipelineState() PipelineState {
	ws.stateMu.RLock()
	defer ws.stateMu.RUnlock()
	return ws.state
}

// mutateState applies fn to the pipeline state under the state lock. fn must
// not block, allocate heavily, or call into another subsystem; see the locking
// contract at the top of this file.
func (ws *Server) mutateState(fn func(*PipelineState)) {
	ws.stateMu.Lock()
	defer ws.stateMu.Unlock()
	fn(&ws.state)
}

// setReplayProgress records packet progress for the active replay. Called from
// the PCAP read loop's progress callback, so it stays allocation-free.
func (ws *Server) setReplayProgress(current, total uint64) {
	ws.mutateState(func(s *PipelineState) {
		s.CurrentPacket = current
		s.TotalPackets = total
	})
}

// setRecordingFrames records how many frames the active VRLOG recorder has written.
func (ws *Server) setRecordingFrames(n uint64) {
	ws.mutateState(func(s *PipelineState) { s.RecordingFrames = n })
}

// setReplayPass records which pass of a multi-pass replay is running.
func (ws *Server) setReplayPass(p ReplayPass) {
	ws.mutateState(func(s *PipelineState) { s.Pass = p })
}
