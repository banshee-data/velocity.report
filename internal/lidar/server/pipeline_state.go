package server

import (
	"fmt"
	"sync/atomic"
)

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

// PCAPFile returns the active PCAP path, or empty when the source is not a
// PCAP. Matches the legacy currentPCAPFile field, which was cleared whenever
// the pipeline returned to live input.
func (s PipelineState) PCAPFile() string {
	if s.Source != SourceModePCAP {
		return ""
	}
	return s.SourcePath
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

// ReplayActiveFlag exposes whether a replay is driving the pipeline as a
// lock-free flag, for the tracking pipeline's per-frame hot path.
//
// The frame-rate throttle uses it to confine itself to replays. The throttle
// exists so a PCAP replaying far faster than real time cannot flood the
// clustering and tracking stages, and it was assumed a live sensor could never
// reach it because the cap sits above the sensor's rotation rate. That reasons
// from rotation rate, while the throttle measures arrival spacing at the
// callback: frames delivered in a clump trip a 25 fps gate even from a 10 Hz
// sensor, and 5,500 live frames were throttled in sixteen minutes on
// 2026-08-26.
func (ws *Server) ReplayActiveFlag() *atomic.Bool {
	return &ws.replayActiveFlag
}

// PipelineState returns a snapshot of the current pipeline state.
func (ws *Server) PipelineState() PipelineState {
	ws.stateMu.RLock()
	defer ws.stateMu.RUnlock()
	return ws.state
}

// Validate reports the ways a state can be internally inconsistent. Every
// transition is checked against it, so a combination that should be impossible
// is named in the log at the moment it is created rather than being diagnosed
// later from its downstream symptoms.
//
// It deliberately does NOT reject Source=vrlog with ReplayActive: a loaded
// VRLOG that has played to its end stays loaded and seekable, so that pairing
// is legitimate.
func (s PipelineState) Validate() error {
	switch {
	case s.ReplayActive && s.Source == SourceModeLive:
		return fmt.Errorf("ReplayActive with Source=live: a running replay must name its source")
	case s.Pass != ReplayPassNone && !s.ReplayActive:
		return fmt.Errorf("Pass=%q with no active replay", s.Pass)
	case s.TotalPasses < 1:
		return fmt.Errorf("TotalPasses=%d, want at least 1", s.TotalPasses)
	case s.Recording && s.RecordingRunID == "":
		return fmt.Errorf("Recording with no RecordingRunID to attribute it to")
	case s.Source == SourceModeLive && s.SourcePath != "":
		return fmt.Errorf("Source=live with SourcePath=%q left behind", s.SourcePath)
	}
	return nil
}

// String renders the fields that identify a transition, leaving out the
// high-frequency progress counters.
func (s PipelineState) String() string {
	return fmt.Sprintf("source=%s replay=%t pass=%q passes=%d grid=%t recording=%t live_listener=%t path=%q",
		s.Source, s.ReplayActive, s.Pass, s.TotalPasses, s.GridPreserved, s.Recording, s.LiveListenerRunning, s.SourcePath)
}

// sameTransition reports whether two states are equal in every field that
// String renders. Progress counters tick many times a second, so tracing them
// would bury the transitions worth reading.
func sameTransition(a, b PipelineState) bool {
	return a.String() == b.String()
}

// mutateState applies fn to the pipeline state under the state lock, then
// traces the transition and reports any invariant it breaks.
//
// This is the single choke point for pipeline state changes: every setter below
// goes through it, so the trace log is a complete record of how the pipeline
// got into its current state, and reason names the caller responsible.
//
// fn must not block, allocate heavily, or call into another subsystem; see the
// locking contract at the top of this file. Tracing happens after the lock is
// released for the same reason.
func (ws *Server) mutateState(reason string, fn func(*PipelineState)) {
	ws.stateMu.Lock()
	before := ws.state
	fn(&ws.state)
	after := ws.state
	ws.stateMu.Unlock()

	ws.publishStateProjections(after)
	reportStateTransition(reason, before, after)
}

// publishStateProjections mirrors the parts of the state that hot paths read
// without taking the state lock.
//
// Every path that writes the state must call this. tryBeginPCAPReplay does its
// own compare-and-set rather than going through mutateState, so it is the one
// that would otherwise be missed — and it is precisely the transition that
// starts a replay, which is what the throttle needs to know about.
func (ws *Server) publishStateProjections(after PipelineState) {
	ws.replayActiveFlag.Store(after.ReplayActive)
}

// reportStateTransition logs a state change and shouts about a broken
// invariant. Split out so tryBeginPCAPReplay, which needs a compare-and-set
// rather than a plain mutation, reports through the same path.
func reportStateTransition(reason string, before, after PipelineState) {
	if !sameTransition(before, after) {
		diagf("[PipelineState] %s: %s -> %s", reason, before, after)
	}
	if err := after.Validate(); err != nil {
		opsf("[PipelineState] INVARIANT BROKEN after %s: %v (state: %s)", reason, err, after)
	}
}

// setReplayProgress records packet progress for the active replay. Called from
// the PCAP read loop's progress callback, so it stays allocation-free.
func (ws *Server) setReplayProgress(current, total uint64) {
	ws.mutateState("setReplayProgress", func(s *PipelineState) {
		s.CurrentPacket = current
		s.TotalPackets = total
	})
}

// setRecordingFrames records how many frames the active VRLOG recorder has written.
func (ws *Server) setRecordingFrames(n uint64) {
	ws.mutateState("setRecordingFrames", func(s *PipelineState) { s.RecordingFrames = n })
}

// setReplayPass records which pass of a multi-pass replay is running.
func (ws *Server) setReplayPass(p ReplayPass) {
	ws.mutateState("setReplayPass", func(s *PipelineState) { s.Pass = p })
}

// setLiveListenerRunning records whether the live UDP listener is accepting
// packets. Exposed on the status surfaces so an operator can tell "live input"
// from "nothing is being ingested".
func (ws *Server) setLiveListenerRunning(running bool) {
	ws.mutateState("setLiveListenerRunning", func(s *PipelineState) { s.LiveListenerRunning = running })
}

// tryBeginPCAPReplay atomically claims the replay slot for a PCAP replay,
// returning false if a replay is already running.
//
// The claim has to be a compare-and-set on the same lock that publishes the
// state: a caller that checked and then set separately could race a second
// start request into a half-configured replay.
func (ws *Server) tryBeginPCAPReplay(cfg ReplayConfig) (bool, SourceMode) {
	totalPasses := 1
	if cfg.SettleBeforeRecording {
		totalPasses = 2
	}

	ws.stateMu.Lock()
	if ws.state.ReplayActive {
		blocker := ws.state.Source
		ws.stateMu.Unlock()
		return false, blocker
	}
	before := ws.state
	ws.state.Source = SourceModePCAP
	ws.state.ReplayActive = true
	ws.state.GridPreserved = cfg.AnalysisMode
	ws.state.TotalPasses = totalPasses
	ws.state.Pass = ReplayPassNone
	ws.state.SpeedMode = ""
	ws.state.SpeedRatio = 0
	ws.state.LastRunID = ""
	ws.state.CurrentPacket = 0
	ws.state.TotalPackets = 0
	// The previous run's output stops being current once a new replay starts.
	// It is retained after completion so a caller polling later can still see
	// what was written, but carrying it into an unrelated run is just noise.
	ws.state.RecordingPath = ""
	ws.state.RecordingRunID = ""
	ws.state.RecordingFrames = 0
	after := ws.state
	ws.stateMu.Unlock()

	ws.publishStateProjections(after)
	reportStateTransition("tryBeginPCAPReplay", before, after)
	return true, ""
}

// releaseReplaySlot clears the fields that belong to a running replay's
// lifetime, leaving Source and GridPreserved to whoever owns them now.
//
// The PCAP replay goroutine defers this, so the slot it claimed with
// tryBeginPCAPReplay is always released. Teardown used to sit inside an
// `if Source == pcap` branch, so a source switch during a replay skipped it and
// stranded ReplayActive=true — which rejected every later start with "already
// in progress" while every stop answered "not in PCAP mode", leaving no way out
// and no live listener.
func (ws *Server) releaseReplaySlot() {
	ws.mutateState("releaseReplaySlot", func(s *PipelineState) {
		s.ReplayActive = false
		s.Pass = ReplayPassNone
		s.TotalPasses = 1
		s.SpeedMode = ""
		s.SpeedRatio = 0
		s.Recording = false
	})
}

// endReplayAndClaimLive releases the replay slot and names live as the source
// in a single state mutation.
//
// The two have to move together. Releasing the slot first and setting the
// source afterwards leaves the state reading "a VRLOG is the source and
// nothing is replaying" for as long as the teardown takes — and that is
// precisely the condition reconcileLiveOnce acts on, so a teardown already in
// progress would be joined by a second, redundant one fired by the reconciler.
// A 512ms window of it was observed on 2026-08-26, the width of the live
// listener's bind wait.
//
// The listener flag is deliberately not touched here: the listener has not
// been started yet at this point, and claiming otherwise would make the
// reconciler skip the restart it is there to guarantee.
func (ws *Server) endReplayAndClaimLive() {
	ws.mutateState("endReplayAndClaimLive", func(s *PipelineState) {
		s.ReplayActive = false
		s.Pass = ReplayPassNone
		s.TotalPasses = 1
		s.SpeedMode = ""
		s.SpeedRatio = 0
		s.Recording = false

		s.Source = SourceModeLive
		s.SourcePath = ""
		s.GridPreserved = false
		s.CurrentPacket = 0
		s.TotalPackets = 0
	})
}

// abandonReplayStart releases the replay slot claimed by tryBeginPCAPReplay
// when startup fails before the replay goroutine is launched.
func (ws *Server) abandonReplayStart() {
	ws.mutateState("abandonReplayStart", func(s *PipelineState) {
		s.ReplayActive = false
		s.GridPreserved = false
		s.TotalPasses = 1
		s.Pass = ReplayPassNone
		s.SpeedMode = ""
		s.SpeedRatio = 0
		s.LastRunID = ""
	})
}

// markReplayRunning records the pacing and provenance of a replay that has
// been successfully launched.
func (ws *Server) markReplayRunning(path, speedMode string, speedRatio float64, runID string) {
	ws.mutateState("markReplayRunning", func(s *PipelineState) {
		s.SourcePath = path
		s.SpeedMode = speedMode
		s.SpeedRatio = speedRatio
		s.LastRunID = runID
	})
}

// endReplay records that the active replay has stopped. gridPreserved carries
// whether the background grid is being retained for inspection, which is what
// makes the difference between the pcap and pcap_analysis wire tokens.
func (ws *Server) endReplay(gridPreserved bool) {
	ws.mutateState("endReplay", func(s *PipelineState) {
		s.ReplayActive = false
		s.GridPreserved = gridPreserved
		s.Pass = ReplayPassNone
		s.TotalPasses = 1
		s.SpeedMode = ""
		s.SpeedRatio = 0
		s.Recording = false
	})
}

// setSourceLive returns the pipeline to live input. gridPreserved records
// whether the background grid built by a previous replay is being kept, which
// resume-live does and stop-PCAP does not.
func (ws *Server) setSourceLive(gridPreserved bool) {
	ws.mutateState("setSourceLive", func(s *PipelineState) {
		s.Source = SourceModeLive
		s.SourcePath = ""
		s.ReplayActive = false
		s.GridPreserved = gridPreserved
		s.Pass = ReplayPassNone
		s.TotalPasses = 1
		s.SpeedMode = ""
		s.SpeedRatio = 0
		s.CurrentPacket = 0
		s.TotalPackets = 0
	})
}

// setRecording records that a VRLOG recorder is attached, along with where it
// is writing. Previously this lived only as a closure local in the radar
// command and a bool inside the replay goroutine, so no surface could report it.
func (ws *Server) setRecording(runID, path string) {
	ws.mutateState("setRecording", func(s *PipelineState) {
		s.Recording = true
		s.RecordingRunID = runID
		s.RecordingPath = path
		s.RecordingFrames = 0
	})
}

// clearRecording records that the VRLOG recorder has been detached. The path
// is retained so a caller polling after completion can still see what was written.
func (ws *Server) clearRecording() {
	ws.mutateState("clearRecording", func(s *PipelineState) { s.Recording = false })
}

// setSourceVRLog records that a VRLOG replay is driving the visualiser stream.
func (ws *Server) setSourceVRLog(path string) {
	ws.mutateState("setSourceVRLog", func(s *PipelineState) {
		s.Source = SourceModeVRLog
		s.SourcePath = path
		s.ReplayActive = true
		s.Pass = ReplayPassNone
		s.TotalPasses = 1
		s.CurrentPacket = 0
		s.TotalPackets = 0
		// See tryBeginPCAPReplay: a previous run's recording path would be
		// stale context alongside a different replay.
		s.RecordingPath = ""
		s.RecordingRunID = ""
		s.RecordingFrames = 0
	})
}
