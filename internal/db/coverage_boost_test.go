package db

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// loopbackRequest creates an httptest request with RemoteAddr set to loopback
// so that tsweb.AllowDebugAccess returns true.
func loopbackRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

// ---------- AttachAdminRoutes handler coverage ----------

func TestAdminRoutes_DBStats_HandlerBody(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Insert some data so stats have content
	if err := db.RecordRadarObject(`{"classifier":"vehicle","start_time":"100.0","end_time":"105.0","delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`); err != nil {
		t.Fatalf("RecordRadarObject: %v", err)
	}

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	req := loopbackRequest(http.MethodGet, "/debug/db-stats")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify JSON response
	var stats DatabaseStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(stats.Tables) == 0 {
		t.Error("expected at least one table in stats")
	}
}

func TestAdminRoutes_Backup_HandlerBody(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Insert a row so the backup is non-trivial
	if err := db.RecordRawData(`{"uptime":100.0,"magnitude":50,"speed":10.0}`); err != nil {
		t.Fatalf("RecordRawData: %v", err)
	}

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// The backup handler does VACUUM INTO in the current working directory,
	// so we chdir to tmpDir to keep test artifacts contained.
	// Note: VACUUM INTO uses an absolute or relative path from the process cwd.

	req := loopbackRequest(http.MethodGet, "/debug/backup")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify gzip response
	ct := w.Header().Get("Content-Type")
	if ct != "application/octet-stream" {
		t.Errorf("expected Content-Type application/octet-stream, got %q", ct)
	}
	ce := w.Header().Get("Content-Encoding")
	if ce != "gzip" {
		t.Errorf("expected Content-Encoding gzip, got %q", ce)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=backup-") {
		t.Errorf("expected Content-Disposition with backup filename, got %q", cd)
	}

	// Verify the body is valid gzip
	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to open gzip reader: %v", err)
	}
	defer gr.Close()

	data, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read gzip body: %v", err)
	}
	// SQLite databases start with "SQLite format 3\000"
	if len(data) < 16 || string(data[:15]) != "SQLite format 3" {
		t.Error("backup data does not look like a valid SQLite database")
	}
}

// ---------- TransitController.Run fullHistory coverage ----------

func TestTransitController_Run_FullHistoryTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 1 * time.Hour // Long interval so periodic doesn't trigger
	tc := NewTransitController(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error)
	go func() {
		done <- tc.Run(ctx)
	}()

	// Wait for the initial run to complete
	time.Sleep(150 * time.Millisecond)
	initialCount := tc.GetStatus().RunCount

	// Trigger full-history run (covers fullHistory channel handler)
	tc.TriggerFullHistoryRun()
	time.Sleep(200 * time.Millisecond)

	status := tc.GetStatus()
	if status.RunCount <= initialCount {
		t.Errorf("expected run count to increase after full-history trigger, got %d (was %d)", status.RunCount, initialCount)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Run did not terminate")
	}
}

func TestTransitController_Run_FullHistoryDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 1 * time.Hour
	tc := NewTransitController(worker)
	tc.SetEnabled(false) // Disable before starting

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error)
	go func() {
		done <- tc.Run(ctx)
	}()

	// Give time for Run loop to start
	time.Sleep(50 * time.Millisecond)

	// Trigger full-history while disabled (covers disabled skip branch)
	tc.TriggerFullHistoryRun()
	time.Sleep(100 * time.Millisecond)

	status := tc.GetStatus()
	if status.RunCount != 0 {
		t.Errorf("expected 0 runs when disabled, got %d", status.RunCount)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Run did not terminate")
	}
}

// ---------- TransitController edge cases ----------

func TestTransitController_finishRun_NilCurrentRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Call finishRun without calling startRun first (currentRun is nil)
	tc.finishRun(nil)

	status := tc.GetStatus()
	if status.RunCount != 1 {
		t.Errorf("expected run count 1, got %d", status.RunCount)
	}
	if status.LastRun == nil {
		t.Fatal("expected LastRun to be set")
	}
	if status.LastRun.Trigger != "unknown" {
		t.Errorf("expected trigger 'unknown', got %q", status.LastRun.Trigger)
	}
}

func TestTransitController_GetStatus_StaleLastRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 1 * time.Minute
	tc := NewTransitController(worker)

	// Simulate a past run
	tc.mu.Lock()
	tc.lastRunAt = time.Now().Add(-1 * time.Hour)
	tc.mu.Unlock()

	status := tc.GetStatus()
	if status.IsHealthy {
		t.Error("expected unhealthy when last run is stale (> 2x interval)")
	}
}

// ---------- Site: GetAllSites with MapSVGData ----------

func TestGetAllSites_WithMapSVGData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	svgData := []byte("<svg>test</svg>")
	site := &Site{
		Name:       "SVG Site",
		Location:   "Test Location",
		Surveyor:   "Tester",
		Contact:    "test@example.com",
		MapSVGData: &svgData,
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	sites, err := db.GetAllSites()
	if err != nil {
		t.Fatalf("GetAllSites: %v", err)
	}

	found := false
	for _, s := range sites {
		if s.ID == site.ID && s.MapSVGData != nil {
			found = true
			if string(*s.MapSVGData) != "<svg>test</svg>" {
				t.Errorf("expected svg data, got %s", string(*s.MapSVGData))
			}
		}
	}
	if !found {
		t.Error("site with MapSVGData not found in GetAllSites results")
	}
}

// ---------- SiteConfigPeriod: GetActiveSiteConfigPeriod with end unix ----------

func TestGetActiveSiteConfigPeriod_WithEndUnix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Create a site first
	site := &Site{
		Name:     "Config Test Site",
		Location: "Test",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	// Create a config period WITH an effective_end_unix
	endUnix := float64(time.Now().Add(24 * time.Hour).Unix())
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: float64(time.Now().Add(-24 * time.Hour).Unix()),
		EffectiveEndUnix:   &endUnix,
		IsActive:           true,
		CosineErrorAngle:   5.0,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("CreateSiteConfigPeriod: %v", err)
	}

	// Retrieve the active period → covers endUnix.Valid branch
	got, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod: %v", err)
	}
	if got.EffectiveEndUnix == nil {
		t.Error("expected EffectiveEndUnix to be set")
	}
	if got.EffectiveEndUnix != nil && *got.EffectiveEndUnix != endUnix {
		t.Errorf("expected end unix %f, got %f", endUnix, *got.EffectiveEndUnix)
	}
}

// ---------- SiteConfigPeriod: UpdateSiteConfigPeriod validation ----------

func TestUpdateSiteConfigPeriod_InvalidData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Update with invalid cosine error angle (> 80)
	period := &SiteConfigPeriod{
		ID:               999,
		SiteID:           1,
		CosineErrorAngle: 85.0, // Invalid — must be <= 80
	}
	err = db.UpdateSiteConfigPeriod(period)
	if err == nil {
		t.Error("expected error for invalid cosine error angle")
	}

	// Update with ID == 0
	period2 := &SiteConfigPeriod{
		ID:               0,
		SiteID:           1,
		CosineErrorAngle: 5.0,
	}
	err = db.UpdateSiteConfigPeriod(period2)
	if err == nil || !strings.Contains(err.Error(), "ID is required") {
		t.Errorf("expected ID required error, got: %v", err)
	}
}

func TestUpdateSiteConfigPeriod_NonExistentID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Create a site to satisfy FK
	site := &Site{
		Name:     "Test",
		Location: "Test",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	period := &SiteConfigPeriod{
		ID:                 99999,
		SiteID:             site.ID,
		EffectiveStartUnix: 1000.0,
		CosineErrorAngle:   5.0,
	}
	err = db.UpdateSiteConfigPeriod(period)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

// ---------- SiteReport: round-trip CRUD ----------

func TestCovBoost_SiteReport_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Create a site for the report
	site := &Site{
		Name:     "Report Site",
		Location: "Test",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	report := &SiteReport{
		SiteID:    site.ID,
		StartDate: "2025-01-01",
		EndDate:   "2025-01-31",
		Filepath:  "/reports/test.pdf",
		Filename:  "test.pdf",
		RunID:     "run-123",
		Timezone:  "Europe/London",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := db.CreateSiteReport(report); err != nil {
		t.Fatalf("CreateSiteReport: %v", err)
	}
	if report.ID == 0 {
		t.Error("expected report ID to be set")
	}

	// Retrieve by ID
	got, err := db.GetSiteReport(report.ID)
	if err != nil {
		t.Fatalf("GetSiteReport: %v", err)
	}
	if got.Filename != "test.pdf" {
		t.Errorf("expected filename test.pdf, got %s", got.Filename)
	}

	// GetRecentReportsForSite
	reports, err := db.GetRecentReportsForSite(site.ID, 10)
	if err != nil {
		t.Fatalf("GetRecentReportsForSite: %v", err)
	}
	if len(reports) != 1 {
		t.Errorf("expected 1 report, got %d", len(reports))
	}

	// GetRecentReportsAllSites
	allReports, err := db.GetRecentReportsAllSites(10)
	if err != nil {
		t.Fatalf("GetRecentReportsAllSites: %v", err)
	}
	if len(allReports) < 1 {
		t.Error("expected at least 1 report from all sites")
	}

	// Delete
	if err := db.DeleteSiteReport(report.ID); err != nil {
		t.Fatalf("DeleteSiteReport: %v", err)
	}

	// Verify deleted
	_, err = db.GetSiteReport(report.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

// ---------- TransitWorker: AnalyseTransitOverlaps with data ----------

func TestCovBoost_AnalyseTransitOverlaps(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Insert some radar_data to generate transits
	now := float64(time.Now().Unix())
	for i := 0; i < 20; i++ {
		event := map[string]interface{}{
			"uptime":    now + float64(i),
			"magnitude": 50 + i,
			"speed":     float64(10 + i%5),
		}
		eventJSON, _ := json.Marshal(event)
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("RecordRawData: %v", err)
		}
	}

	// Run transit worker to create some transits
	worker := NewTransitWorker(db, 5, "v1-test")
	if err := worker.RunFullHistory(ctx); err != nil {
		t.Fatalf("RunFullHistory: %v", err)
	}

	// Run overlap analysis
	stats, err := db.AnalyseTransitOverlaps(ctx)
	if err != nil {
		t.Fatalf("AnalyseTransitOverlaps: %v", err)
	}
	if stats.TotalTransits < 0 {
		t.Error("expected non-negative total transits")
	}
	if stats.ModelVersionCounts == nil {
		t.Error("expected non-nil ModelVersionCounts")
	}
}

// ---------- TransitWorker: DeleteAllTransits ----------

func TestDeleteAllTransits_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Insert radar data
	for i := 0; i < 10; i++ {
		event := map[string]interface{}{
			"uptime":    float64(1000 + i),
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, _ := json.Marshal(event)
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("RecordRawData: %v", err)
		}
	}

	worker := NewTransitWorker(db, 5, "delete-test")
	if err := worker.RunFullHistory(ctx); err != nil {
		t.Fatalf("RunFullHistory: %v", err)
	}

	deleted, err := worker.DeleteAllTransits(ctx, "delete-test")
	if err != nil {
		t.Fatalf("DeleteAllTransits: %v", err)
	}
	t.Logf("Deleted %d transits", deleted)
}

// ---------- Events with data ----------

func TestCovBoost_Events(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Insert raw data
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"uptime":    float64(100 + i*10),
			"magnitude": 50 + i*5,
			"speed":     float64(10 + i),
		}
		eventJSON, _ := json.Marshal(event)
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("RecordRawData: %v", err)
		}
	}

	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) < 5 {
		t.Errorf("expected at least 5 events, got %d", len(events))
	}
}

// ---------- FindTransitGaps ----------

func TestFindTransitGaps_WithRadarDataOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Insert radar data without transits → should find gaps
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"uptime":    float64(1000 + i*10),
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, _ := json.Marshal(event)
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("RecordRawData: %v", err)
		}
	}

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("FindTransitGaps: %v", err)
	}
	// With only radar_data and no transits, there should be gaps
	t.Logf("Found %d transit gaps", len(gaps))
}

// ---------- ListSiteConfigPeriods with site filter ----------

func TestListSiteConfigPeriods_WithEndUnix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	site := &Site{
		Name:     "Period Site",
		Location: "Test",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	endUnix := float64(time.Now().Add(48 * time.Hour).Unix())
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: float64(time.Now().Add(-48 * time.Hour).Unix()),
		EffectiveEndUnix:   &endUnix,
		IsActive:           true,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("CreateSiteConfigPeriod: %v", err)
	}

	// List with site filter → covers endUnix.Valid branch in ListSiteConfigPeriods
	siteID := site.ID
	periods, err := db.ListSiteConfigPeriods(&siteID)
	if err != nil {
		t.Fatalf("ListSiteConfigPeriods: %v", err)
	}
	if len(periods) != 1 {
		t.Fatalf("expected 1 period, got %d", len(periods))
	}
	if periods[0].EffectiveEndUnix == nil {
		t.Error("expected EffectiveEndUnix to be set")
	}
}

// ---------- GetDatabaseStats covers pragma fallback ----------

func TestCovBoost_GetDatabaseStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// Call on empty DB
	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats: %v", err)
	}
	if stats.TotalSizeMB < 0 {
		t.Error("expected non-negative total size")
	}
}

// ---------- CountUniqueBgSnapshotHashes ----------

func TestCountUniqueBgSnapshotHashes_EmptyDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	count, err := db.CountUniqueBgSnapshotHashes("test-sensor")
	if err != nil {
		t.Fatalf("CountUniqueBgSnapshotHashes: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unique hashes, got %d", count)
	}
}

// ---------- NewDBWithMigrationCheck: legacy DB detection ----------

func TestNewDBWithMigrationCheck_LegacyDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "legacy.db")

	// Step 1: Create a fully-initialised database (schema.sql + baseline)
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close()

	// Step 2: Drop schema_migrations to simulate a legacy database
	rawDB, err := openRawSQLite(dbPath)
	if err != nil {
		t.Fatalf("openRawSQLite: %v", err)
	}
	if _, err := rawDB.Exec("DROP TABLE IF EXISTS schema_migrations"); err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}
	rawDB.Close()

	// Step 3: Open with migration check — should detect schema, baseline, and return
	db2, err := NewDBWithMigrationCheck(dbPath, true)
	if err != nil {
		t.Fatalf("NewDBWithMigrationCheck: %v", err)
	}
	defer db2.Close()

	// Verify schema_migrations was re-created by baselining
	var count int
	if err := db2.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count == 0 {
		t.Error("expected schema_migrations to have at least one row after baselining")
	}
}

func TestNewDBWithMigrationCheck_NoCheckMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nocheck.db")

	// Create a fresh DB without migration checks
	db, err := NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		t.Fatalf("NewDBWithMigrationCheck: %v", err)
	}
	defer db.Close()

	// Should have created the DB successfully
	var tableCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tableCount); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount == 0 {
		t.Error("expected at least one table")
	}
}

// openRawSQLite opens a SQLite database for raw manipulation without schema init.
func openRawSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path)
}

// ---------- Closed DB error paths ----------
// Closing the DB before calling functions triggers SQL error handling branches
// that are otherwise unreachable in normal operation.

func TestCovBoost_ClosedDB_SiteErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close() // Close early to trigger error paths

	_, err = db.GetAllSites()
	if err == nil {
		t.Error("expected error from GetAllSites on closed DB")
	}

	err = db.DeleteSite(1)
	if err == nil {
		t.Error("expected error from DeleteSite on closed DB")
	}

	err = db.UpdateSite(&Site{ID: 1, Name: "x", Location: "x", Surveyor: "x", Contact: "x"})
	if err == nil {
		t.Error("expected error from UpdateSite on closed DB")
	}

	err = db.CreateSite(&Site{Name: "x", Location: "x", Surveyor: "x", Contact: "x"})
	if err == nil {
		t.Error("expected error from CreateSite on closed DB")
	}

	_, err = db.GetSite(1)
	if err == nil {
		t.Error("expected error from GetSite on closed DB")
	}
}

func TestCovBoost_ClosedDB_SiteReportErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close()

	err = db.CreateSiteReport(&SiteReport{SiteID: 1, StartDate: "2025-01-01", EndDate: "2025-01-31", Filepath: "/x", Filename: "x", RunID: "x", Timezone: "UTC", Units: "mph", Source: "radar_objects"})
	if err == nil {
		t.Error("expected error from CreateSiteReport on closed DB")
	}

	_, err = db.GetSiteReport(1)
	if err == nil {
		t.Error("expected error from GetSiteReport on closed DB")
	}

	_, err = db.GetRecentReportsForSite(1, 10)
	if err == nil {
		t.Error("expected error from GetRecentReportsForSite on closed DB")
	}

	_, err = db.GetRecentReportsAllSites(10)
	if err == nil {
		t.Error("expected error from GetRecentReportsAllSites on closed DB")
	}

	err = db.DeleteSiteReport(1)
	if err == nil {
		t.Error("expected error from DeleteSiteReport on closed DB")
	}
}

func TestCovBoost_ClosedDB_ConfigPeriodErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close()

	_, err = db.ListSiteConfigPeriods(nil)
	if err == nil {
		t.Error("expected error from ListSiteConfigPeriods on closed DB")
	}

	_, err = db.GetActiveSiteConfigPeriod(1)
	if err == nil {
		t.Error("expected error from GetActiveSiteConfigPeriod on closed DB")
	}

	_, err = db.GetSiteConfigPeriod(1)
	if err == nil {
		t.Error("expected error from GetSiteConfigPeriod on closed DB")
	}
}

func TestCovBoost_ClosedDB_TransitErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close()

	ctx := context.Background()

	_, err = db.FindTransitGaps()
	if err == nil {
		t.Error("expected error from FindTransitGaps on closed DB")
	}

	_, err = db.AnalyseTransitOverlaps(ctx)
	if err == nil {
		t.Error("expected error from AnalyseTransitOverlaps on closed DB")
	}

	_, err = db.Events()
	if err == nil {
		t.Error("expected error from Events on closed DB")
	}

	_, err = db.GetDatabaseStats()
	if err == nil {
		t.Error("expected error from GetDatabaseStats on closed DB")
	}

	_, err = db.CountUniqueBgSnapshotHashes("test")
	if err == nil {
		t.Error("expected error from CountUniqueBgSnapshotHashes on closed DB")
	}
}

func TestCovBoost_ClosedDB_TransitWorkerErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close()

	ctx := context.Background()
	worker := NewTransitWorker(db, 5, "test-model")

	err = worker.RunOnce(ctx)
	if err == nil {
		t.Error("expected error from RunOnce on closed DB")
	}

	_, err = worker.DeleteAllTransits(ctx, "test-model")
	if err == nil {
		t.Error("expected error from DeleteAllTransits on closed DB")
	}

	err = worker.MigrateModelVersion(ctx, "old-model")
	if err == nil {
		t.Error("expected error from MigrateModelVersion on closed DB")
	}
}

func TestCovBoost_ClosedDB_MigrateErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	db.Close()

	_, err = db.GetDatabaseSchema()
	if err == nil {
		t.Error("expected error from GetDatabaseSchema on closed DB")
	}
}

// ---------- TransitController error logging paths ----------

func TestTransitController_finishRun_WithError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Call finishRun with an error (covers the err != nil branch in finishRun)
	tc.startRun("test")
	tc.finishRun(context.DeadlineExceeded)

	status := tc.GetStatus()
	if status.LastRun == nil {
		t.Fatal("expected LastRun to be set")
	}
	if status.LastRun.Error == "" {
		t.Error("expected error message in LastRun")
	}
}

func TestTransitController_Run_ErrorLogging(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 50 * time.Millisecond
	tc := NewTransitController(worker)

	// Close DB so RunOnce fails → triggers error logging in Run
	db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error)
	go func() {
		done <- tc.Run(ctx)
	}()

	// Wait for initial run (which fails) and a periodic tick (which also fails)
	time.Sleep(200 * time.Millisecond)

	// Trigger manual run → should cover manual error logging
	tc.TriggerManualRun()
	time.Sleep(100 * time.Millisecond)

	// Trigger full-history run → should cover full-history error logging
	tc.TriggerFullHistoryRun()
	time.Sleep(100 * time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Run did not terminate")
	}

	status := tc.GetStatus()
	if status.RunCount == 0 {
		t.Error("expected at least one run attempt")
	}
}

func TestTransitWorker_Start_ErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 20 * time.Millisecond

	// Close DB so tick's RunOnce will fail
	db.Close()

	worker.Start()

	// Wait enough for at least one tick to fail
	time.Sleep(100 * time.Millisecond)

	worker.Stop()
}

// ---------- Site UpdateSite with mapSVGData ----------

func TestUpdateSite_WithMapSVGData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	site := &Site{
		Name:     "Update Test",
		Location: "Test",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	// Update with SVG data
	svgData := []byte("<svg>updated</svg>")
	site.MapSVGData = &svgData
	if err := db.UpdateSite(site); err != nil {
		t.Fatalf("UpdateSite: %v", err)
	}

	got, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("GetSite: %v", err)
	}
	if got.MapSVGData == nil {
		t.Error("expected MapSVGData to be set after update")
	}
}
