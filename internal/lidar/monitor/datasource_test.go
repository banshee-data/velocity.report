package monitor

import (
	"context"
	"errors"
	"testing"
)

func TestMockDataSourceManager_StartLiveListener(t *testing.T) {
	mgr := NewMockDataSourceManager()
	ctx := context.Background()

	err := mgr.StartLiveListener(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !mgr.IsLiveStarted() {
		t.Error("Expected live listener to be started")
	}
	if mgr.CurrentSource() != DataSourceLive {
		t.Errorf("Expected source DataSourceLive, got %s", mgr.CurrentSource())
	}
	if mgr.StartLiveCalls != 1 {
		t.Errorf("Expected 1 StartLiveCalls, got %d", mgr.StartLiveCalls)
	}
	if mgr.LastLiveCtx != ctx {
		t.Error("Expected context to be recorded")
	}
}

func TestMockDataSourceManager_StartLiveListener_Error(t *testing.T) {
	mgr := NewMockDataSourceManager()
	mgr.StartLiveError = errors.New("failed to start")

	err := mgr.StartLiveListener(context.Background())
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "failed to start" {
		t.Errorf("Expected 'failed to start', got: %v", err)
	}
	if mgr.IsLiveStarted() {
		t.Error("Expected live listener to not be started after error")
	}
}

func TestMockDataSourceManager_StopLiveListener(t *testing.T) {
	mgr := NewMockDataSourceManager()
	mgr.StartLiveListener(context.Background())

	err := mgr.StopLiveListener()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if mgr.IsLiveStarted() {
		t.Error("Expected live listener to be stopped")
	}
	if mgr.StopLiveCalls != 1 {
		t.Errorf("Expected 1 StopLiveCalls, got %d", mgr.StopLiveCalls)
	}
}

func TestMockDataSourceManager_StopLiveListener_Error(t *testing.T) {
	mgr := NewMockDataSourceManager()
	mgr.StartLiveListener(context.Background())
	mgr.StopLiveError = errors.New("failed to stop")

	err := mgr.StopLiveListener()
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "failed to stop" {
		t.Errorf("Expected 'failed to stop', got: %v", err)
	}
}

func TestMockDataSourceManager_StartPCAPReplay(t *testing.T) {
	mgr := NewMockDataSourceManager()
	ctx := context.Background()
	config := ReplayConfig{
		StartSeconds:    10.0,
		DurationSeconds: 60.0,
		SpeedMode:       "realtime",
		SpeedRatio:      1.0,
	}

	err := mgr.StartPCAPReplay(ctx, "/path/to/test.pcap", config)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !mgr.IsPCAPInProgress() {
		t.Error("Expected PCAP replay to be in progress")
	}
	if mgr.CurrentSource() != DataSourcePCAP {
		t.Errorf("Expected source DataSourcePCAP, got %s", mgr.CurrentSource())
	}
	if mgr.CurrentPCAPFile() != "/path/to/test.pcap" {
		t.Errorf("Expected PCAP file '/path/to/test.pcap', got '%s'", mgr.CurrentPCAPFile())
	}
	if mgr.StartPCAPCalls != 1 {
		t.Errorf("Expected 1 StartPCAPCalls, got %d", mgr.StartPCAPCalls)
	}
	if mgr.LastPCAPCtx != ctx {
		t.Error("Expected context to be recorded")
	}

	savedConfig := mgr.GetLastPCAPConfig()
	if savedConfig.StartSeconds != 10.0 {
		t.Errorf("Expected StartSeconds 10.0, got %f", savedConfig.StartSeconds)
	}
	if savedConfig.DurationSeconds != 60.0 {
		t.Errorf("Expected DurationSeconds 60.0, got %f", savedConfig.DurationSeconds)
	}
}

func TestMockDataSourceManager_StartPCAPReplay_AnalysisMode(t *testing.T) {
	mgr := NewMockDataSourceManager()
	config := ReplayConfig{
		AnalysisMode: true,
	}

	err := mgr.StartPCAPReplay(context.Background(), "test.pcap", config)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if mgr.CurrentSource() != DataSourcePCAPAnalysis {
		t.Errorf("Expected source DataSourcePCAPAnalysis, got %s", mgr.CurrentSource())
	}
}

func TestMockDataSourceManager_StartPCAPReplay_Error(t *testing.T) {
	mgr := NewMockDataSourceManager()
	mgr.StartPCAPError = errors.New("failed to start pcap")

	err := mgr.StartPCAPReplay(context.Background(), "test.pcap", ReplayConfig{})
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "failed to start pcap" {
		t.Errorf("Expected 'failed to start pcap', got: %v", err)
	}
	if mgr.IsPCAPInProgress() {
		t.Error("Expected PCAP not to be in progress after error")
	}
}

func TestMockDataSourceManager_StopPCAPReplay(t *testing.T) {
	mgr := NewMockDataSourceManager()
	mgr.StartPCAPReplay(context.Background(), "test.pcap", ReplayConfig{})

	err := mgr.StopPCAPReplay()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if mgr.IsPCAPInProgress() {
		t.Error("Expected PCAP replay to be stopped")
	}
	if mgr.CurrentPCAPFile() != "" {
		t.Errorf("Expected empty PCAP file, got '%s'", mgr.CurrentPCAPFile())
	}
	if mgr.CurrentSource() != DataSourceLive {
		t.Errorf("Expected source to reset to DataSourceLive, got %s", mgr.CurrentSource())
	}
	if mgr.StopPCAPCalls != 1 {
		t.Errorf("Expected 1 StopPCAPCalls, got %d", mgr.StopPCAPCalls)
	}
}

func TestMockDataSourceManager_StopPCAPReplay_Error(t *testing.T) {
	mgr := NewMockDataSourceManager()
	mgr.StartPCAPReplay(context.Background(), "test.pcap", ReplayConfig{})
	mgr.StopPCAPError = errors.New("failed to stop pcap")

	err := mgr.StopPCAPReplay()
	if err == nil {
		t.Error("Expected error")
	}
	if err.Error() != "failed to stop pcap" {
		t.Errorf("Expected 'failed to stop pcap', got: %v", err)
	}
}

func TestMockDataSourceManager_Reset(t *testing.T) {
	mgr := NewMockDataSourceManager()

	// Set up state
	mgr.StartLiveListener(context.Background())
	mgr.StartPCAPReplay(context.Background(), "test.pcap", ReplayConfig{})
	mgr.StartLiveError = errors.New("error")
	mgr.StopLiveError = errors.New("error")

	// Reset
	mgr.Reset()

	// Verify reset state
	if mgr.IsLiveStarted() {
		t.Error("Expected live listener not started after reset")
	}
	if mgr.IsPCAPInProgress() {
		t.Error("Expected PCAP not in progress after reset")
	}
	if mgr.CurrentSource() != DataSourceLive {
		t.Errorf("Expected source DataSourceLive, got %s", mgr.CurrentSource())
	}
	if mgr.StartLiveCalls != 0 {
		t.Errorf("Expected 0 StartLiveCalls, got %d", mgr.StartLiveCalls)
	}
	if mgr.StopLiveCalls != 0 {
		t.Errorf("Expected 0 StopLiveCalls, got %d", mgr.StopLiveCalls)
	}
	if mgr.StartPCAPCalls != 0 {
		t.Errorf("Expected 0 StartPCAPCalls, got %d", mgr.StartPCAPCalls)
	}
	if mgr.StopPCAPCalls != 0 {
		t.Errorf("Expected 0 StopPCAPCalls, got %d", mgr.StopPCAPCalls)
	}
	if mgr.StartLiveError != nil {
		t.Error("Expected nil StartLiveError after reset")
	}
	if mgr.StopLiveError != nil {
		t.Error("Expected nil StopLiveError after reset")
	}
}

func TestMockDataSourceManager_SetSource(t *testing.T) {
	mgr := NewMockDataSourceManager()

	mgr.SetSource(DataSourcePCAPAnalysis)

	if mgr.CurrentSource() != DataSourcePCAPAnalysis {
		t.Errorf("Expected source DataSourcePCAPAnalysis, got %s", mgr.CurrentSource())
	}
}

func TestReplayConfig_Fields(t *testing.T) {
	config := ReplayConfig{
		StartSeconds:    5.5,
		DurationSeconds: 120.0,
		SpeedMode:       "fast",
		SpeedRatio:      4.0,
		AnalysisMode:    true,
	}

	if config.StartSeconds != 5.5 {
		t.Errorf("Expected StartSeconds 5.5, got %f", config.StartSeconds)
	}
	if config.DurationSeconds != 120.0 {
		t.Errorf("Expected DurationSeconds 120.0, got %f", config.DurationSeconds)
	}
	if config.SpeedMode != "fast" {
		t.Errorf("Expected SpeedMode 'fast', got '%s'", config.SpeedMode)
	}
	if config.SpeedRatio != 4.0 {
		t.Errorf("Expected SpeedRatio 4.0, got %f", config.SpeedRatio)
	}
	if !config.AnalysisMode {
		t.Error("Expected AnalysisMode true")
	}
}

func TestDataSourceConstants(t *testing.T) {
	if DataSourceLive != "live" {
		t.Errorf("Expected DataSourceLive 'live', got '%s'", DataSourceLive)
	}
	if DataSourcePCAP != "pcap" {
		t.Errorf("Expected DataSourcePCAP 'pcap', got '%s'", DataSourcePCAP)
	}
	if DataSourcePCAPAnalysis != "pcap_analysis" {
		t.Errorf("Expected DataSourcePCAPAnalysis 'pcap_analysis', got '%s'", DataSourcePCAPAnalysis)
	}
}

func TestErrorVariables(t *testing.T) {
	if ErrSourceAlreadyActive.Error() != "data source already active" {
		t.Errorf("ErrSourceAlreadyActive message incorrect: %s", ErrSourceAlreadyActive.Error())
	}
	if ErrNoSourceActive.Error() != "no data source active" {
		t.Errorf("ErrNoSourceActive message incorrect: %s", ErrNoSourceActive.Error())
	}
}

// TestMockDataSourceManager_InterfaceCompliance verifies the mock implements the interface.
func TestMockDataSourceManager_InterfaceCompliance(t *testing.T) {
	var _ DataSourceManager = (*MockDataSourceManager)(nil)
}
