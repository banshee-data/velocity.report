package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/server"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

type recordingMetadataSinkStub struct {
	deterministicCalls int
	runConfigID        string
	paramSetID         string
	configHash         string
	paramsHash         string
	schemaVersion      string
	paramSetType       string
	buildVersion       string
	buildGitSHA        string
	executionConfig    []byte
	provenanceSource   string
	provenancePCAP     string
	provenanceHash     string
	provenanceRate     float64
}

func (s *recordingMetadataSinkStub) SetDeterministicConfig(runConfigID, paramSetID, configHash, paramsHash, schemaVersion, paramSetType, buildVersion, buildGitSHA string, executionConfig []byte) {
	s.deterministicCalls++
	s.runConfigID = runConfigID
	s.paramSetID = paramSetID
	s.configHash = configHash
	s.paramsHash = paramsHash
	s.schemaVersion = schemaVersion
	s.paramSetType = paramSetType
	s.buildVersion = buildVersion
	s.buildGitSHA = buildGitSHA
	s.executionConfig = append([]byte(nil), executionConfig...)
}

func (s *recordingMetadataSinkStub) SetProvenance(sourceType, pcapPath, tuningHash string, playbackRate float64) {
	s.provenanceSource = sourceType
	s.provenancePCAP = pcapPath
	s.provenanceHash = tuningHash
	s.provenanceRate = playbackRate
}

type recordingSourceInfoStub struct {
	source    server.DataSource
	pcapPath  string
	speedRate float64
}

func (s recordingSourceInfoStub) CurrentSource() server.DataSource { return s.source }
func (s recordingSourceInfoStub) CurrentPCAPFile() string          { return s.pcapPath }
func (s recordingSourceInfoStub) PCAPSpeedRatio() float64          { return s.speedRate }

type orphanedSweepRecovererStub struct {
	affected int64
	err      error
}

type runConfigQueryInterceptDB struct {
	db               *sql.DB
	runConfigQueryDB *sql.DB
}

func (s orphanedSweepRecovererStub) RecoverOrphanedSweeps() (int64, error) {
	return s.affected, s.err
}

func (d *runConfigQueryInterceptDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *runConfigQueryInterceptDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d *runConfigQueryInterceptDB) QueryRow(query string, args ...any) *sql.Row {
	if strings.Contains(query, "FROM lidar_run_configs") && d.runConfigQueryDB != nil {
		return d.runConfigQueryDB.QueryRow(query, args...)
	}
	return d.db.QueryRow(query, args...)
}

func (d *runConfigQueryInterceptDB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

func TestApplyRecordingMetadata_UsesImmutableRunConfig(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	configStore := configasset.NewStore(testDB.DB)
	paramSet, err := configasset.MakeRequestedParamSet(json.RawMessage(`{"alpha":1}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}
	runConfig, err := configStore.EnsureRunConfig(paramSet, configasset.BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha1"})
	if err != nil {
		t.Fatalf("EnsureRunConfig failed: %v", err)
	}

	runStore := sqlite.NewAnalysisRunStore(testDB.DB)
	if err := runStore.InsertRun(&sqlite.AnalysisRun{
		RunID:       "run-1",
		SourceType:  "pcap",
		SourcePath:  "/tmp/capture.pcap",
		SensorID:    "sensor-1",
		Status:      "completed",
		RunConfigID: runConfig.RunConfigID,
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	var logs bytes.Buffer
	rec := &recordingMetadataSinkStub{}
	applyRecordingMetadata(rec, testDB.DB, recordingSourceInfoStub{
		source:    server.DataSourcePCAP,
		pcapPath:  "/var/data/capture.pcap",
		speedRate: 1.25,
	}, "run-1", "fallback-hash", log.New(&logs, "", 0))

	if rec.deterministicCalls != 1 {
		t.Fatalf("deterministic config calls = %d, want 1", rec.deterministicCalls)
	}
	if rec.runConfigID != runConfig.RunConfigID {
		t.Fatalf("runConfigID = %q, want %q", rec.runConfigID, runConfig.RunConfigID)
	}
	if rec.provenanceSource != "pcap" {
		t.Fatalf("provenance source = %q, want pcap", rec.provenanceSource)
	}
	if rec.provenancePCAP != "capture.pcap" {
		t.Fatalf("provenance PCAP = %q, want capture.pcap", rec.provenancePCAP)
	}
	if rec.provenanceHash != runConfig.ParamsHash {
		t.Fatalf("provenance hash = %q, want %q", rec.provenanceHash, runConfig.ParamsHash)
	}
	if rec.provenanceRate != 1.25 {
		t.Fatalf("provenance rate = %v, want 1.25", rec.provenanceRate)
	}
	if len(logs.String()) != 0 {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
}

func TestApplyRecordingMetadata_LogsRunLookupErrorAndFallsBack(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	var logs bytes.Buffer
	rec := &recordingMetadataSinkStub{}
	applyRecordingMetadata(rec, testDB.DB, nil, "missing-run", "fallback-hash", log.New(&logs, "", 0))

	if rec.deterministicCalls != 0 {
		t.Fatalf("deterministic config calls = %d, want 0", rec.deterministicCalls)
	}
	if rec.provenanceSource != "live" {
		t.Fatalf("provenance source = %q, want live", rec.provenanceSource)
	}
	if rec.provenanceHash != "fallback-hash" {
		t.Fatalf("provenance hash = %q, want fallback-hash", rec.provenanceHash)
	}
	if !strings.Contains(logs.String(), "failed to load run metadata") {
		t.Fatalf("expected run metadata warning, got %q", logs.String())
	}
}

func TestApplyRecordingMetadata_LogsRunConfigLookupError(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	configStore := configasset.NewStore(testDB.DB)
	paramSet, err := configasset.MakeRequestedParamSet(json.RawMessage(`{"alpha":1}`))
	if err != nil {
		t.Fatalf("MakeRequestedParamSet failed: %v", err)
	}
	runConfig, err := configStore.EnsureRunConfig(paramSet, configasset.BuildIdentity{BuildVersion: "v1", BuildGitSHA: "sha1"})
	if err != nil {
		t.Fatalf("EnsureRunConfig failed: %v", err)
	}

	runStore := sqlite.NewAnalysisRunStore(testDB.DB)
	if err := runStore.InsertRun(&sqlite.AnalysisRun{
		RunID:       "run-2",
		SourceType:  "pcap",
		SourcePath:  "/tmp/capture.pcap",
		SensorID:    "sensor-1",
		Status:      "completed",
		RunConfigID: runConfig.RunConfigID,
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	closedDB, closedCleanup := dbpkg.NewTestDB(t)
	defer closedCleanup()
	if err := closedDB.DB.Close(); err != nil {
		t.Fatalf("close secondary db: %v", err)
	}

	var logs bytes.Buffer
	rec := &recordingMetadataSinkStub{}
	applyRecordingMetadata(rec, &runConfigQueryInterceptDB{db: testDB.DB, runConfigQueryDB: closedDB.DB}, recordingSourceInfoStub{source: server.DataSourcePCAPAnalysis, pcapPath: "/tmp/scene.pcap"}, "run-2", "fallback-hash", log.New(&logs, "", 0))

	if rec.deterministicCalls != 0 {
		t.Fatalf("deterministic config calls = %d, want 0", rec.deterministicCalls)
	}
	if rec.provenanceHash != "fallback-hash" {
		t.Fatalf("provenance hash = %q, want fallback-hash", rec.provenanceHash)
	}
	if !strings.Contains(logs.String(), "failed to load immutable config") {
		t.Fatalf("expected immutable config warning, got %q", logs.String())
	}
}

func TestRecoverOrphanedSweepsOnStart_LogsError(t *testing.T) {
	var logs bytes.Buffer
	recoverOrphanedSweepsOnStart(orphanedSweepRecovererStub{err: errors.New("boom")}, log.New(&logs, "", 0))
	if !strings.Contains(logs.String(), "failed to recover orphaned sweeps") {
		t.Fatalf("expected recovery error log, got %q", logs.String())
	}
}

func TestRecoverOrphanedSweepsOnStart_LogsRecoveredCount(t *testing.T) {
	var logs bytes.Buffer
	recoverOrphanedSweepsOnStart(orphanedSweepRecovererStub{affected: 3}, log.New(&logs, "", 0))
	if !strings.Contains(logs.String(), "Recovered 3 orphaned sweep(s)") {
		t.Fatalf("expected recovery count log, got %q", logs.String())
	}
}

func TestHintSceneAdapter_GetScene(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	runStore := sqlite.NewAnalysisRunStore(testDB.DB)
	if err := runStore.InsertRun(&sqlite.AnalysisRun{
		RunID:      "run-1",
		SourceType: "pcap",
		SourcePath: "/tmp/capture.pcap",
		SensorID:   "sensor-1",
		Status:     "completed",
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	store := sqlite.NewReplayCaseStore(testDB.DB)
	startSecs := 1.5
	durationSecs := 2.5
	recommended := json.RawMessage(`{"alpha":1}`)
	if err := store.InsertScene(&sqlite.ReplayCase{
		ReplayCaseID:     "scene-1",
		SensorID:         "sensor-1",
		PCAPFile:         "capture.pcap",
		PCAPStartSecs:    &startSecs,
		PCAPDurationSecs: &durationSecs,
		ReferenceRunID:   "run-1",
	}); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}
	if err := store.SetOptimalParams("scene-1", recommended); err != nil {
		t.Fatalf("SetOptimalParams failed: %v", err)
	}

	adapter := &hintSceneAdapter{store: store}
	scene, err := adapter.GetScene("scene-1")
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
	}
	if scene.ReplayCaseID != "scene-1" || scene.SensorID != "sensor-1" || scene.PCAPFile != "capture.pcap" {
		t.Fatalf("unexpected scene payload: %+v", scene)
	}
	if string(scene.RecommendedParams) != string(recommended) {
		t.Fatalf("recommended params = %s, want %s", scene.RecommendedParams, recommended)
	}
}

func TestHintRunCreator_CreateSweepRun_InvalidJSON(t *testing.T) {
	creator := &hintRunCreator{runner: sweep.NewRunner(nil)}
	_, err := creator.CreateSweepRun("sensor-1", "capture.pcap", json.RawMessage(`{"bad"`), 0, 0)
	if err == nil || !strings.Contains(err.Error(), "parsing paramsJSON for reference run") {
		t.Fatalf("expected paramsJSON parse error, got %v", err)
	}
}
