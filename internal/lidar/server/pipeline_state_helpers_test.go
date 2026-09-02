package server

// Test helpers for placing a Server into a specific pipeline state.
//
// These exist because the old currentSource field conflated two axes: setting
// it to DataSourcePCAPAnalysis meant "PCAP source, replay finished, grid kept",
// which is now three fields. Naming the situations keeps the tests readable and
// stops each one re-deriving the mapping.

// setTestSourceLive places the server in live mode with no replay running.
func (ws *Server) setTestSourceLive() {
	ws.setSourceLive(false)
	ws.setLiveListenerRunning(true)
}

// setTestSourcePCAPReplaying places the server mid-PCAP-replay.
func (ws *Server) setTestSourcePCAPReplaying() {
	ws.mutateState("test", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.ReplayActive = true
		s.GridPreserved = false
		s.LiveListenerRunning = false
		s.TotalPasses = 1
	})
}

// setTestSourcePCAPAnalysisReplaying places the server mid-replay in analysis mode.
func (ws *Server) setTestSourcePCAPAnalysisReplaying() {
	ws.mutateState("test", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.ReplayActive = true
		s.GridPreserved = true
		s.LiveListenerRunning = false
		s.TotalPasses = 1
	})
}

// setTestSourcePCAPAnalysis places the server in the terminal analysis state:
// the replay has finished and the grid is retained. This is what the
// pcap_analysis wire token describes.
func (ws *Server) setTestSourcePCAPAnalysis() {
	ws.mutateState("test", func(s *PipelineState) {
		s.Source = SourceModePCAP
		s.ReplayActive = false
		s.GridPreserved = true
		s.LiveListenerRunning = false
		s.TotalPasses = 1
	})
}

// setTestPCAPFile records the active source path.
func (ws *Server) setTestPCAPFile(path string) {
	ws.mutateState("test", func(s *PipelineState) { s.SourcePath = path })
}
