package db

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func resetProdSchemaConsistencyCacheForTest(t *testing.T) {
	t.Helper()
	prodSchemaConsistencyOnce = sync.Once{}
	prodSchemaConsistencyErr = nil
	prodLatestVersion = 0
	t.Cleanup(func() {
		prodSchemaConsistencyOnce = sync.Once{}
		prodSchemaConsistencyErr = nil
		prodLatestVersion = 0
	})
}

func withSchemaSQLForTest(t *testing.T, sql string) {
	t.Helper()
	original := schemaSQL
	schemaSQL = sql
	t.Cleanup(func() {
		schemaSQL = original
	})
}

func withDevModeForTest(t *testing.T, enabled bool) {
	t.Helper()
	original := DevMode
	DevMode = enabled
	t.Cleanup(func() {
		DevMode = original
	})
}

func withTempDevMigrationsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	migrationsDir := filepath.Join(root, "internal", "db", "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	for name, content := range files {
		path := filepath.Join(migrationsDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write migration file %s: %v", name, err)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("failed to chdir to temp repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	return root
}

func singleMigrationFS(upSQL string) fs.FS {
	return fstest.MapFS{
		"000001_test.up.sql":   &fstest.MapFile{Data: []byte(upSQL)},
		"000001_test.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS demo_table;")},
	}
}

func TestGetSchemaConsistencyResult_DevModeRechecksEveryCall(t *testing.T) {
	resetProdSchemaConsistencyCacheForTest(t)
	withDevModeForTest(t, true)
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	validFS := singleMigrationFS(`CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)
	version, err := getSchemaConsistencyResult(validFS)
	if err != nil {
		t.Fatalf("first getSchemaConsistencyResult failed: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1 on first call, got %d", version)
	}

	// Should re-check and fail (no stale cached success in DevMode).
	_, err = getSchemaConsistencyResult(fstest.MapFS{})
	if err == nil {
		t.Fatal("expected error on second call with empty migrations in DevMode")
	}

	// Should re-check again and recover with valid migrations.
	version, err = getSchemaConsistencyResult(validFS)
	if err != nil {
		t.Fatalf("third getSchemaConsistencyResult failed: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1 on third call, got %d", version)
	}
}

func TestGetSchemaConsistencyResult_ProdCachesSuccess(t *testing.T) {
	resetProdSchemaConsistencyCacheForTest(t)
	withDevModeForTest(t, false)
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	validFS := singleMigrationFS(`CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)
	version, err := getSchemaConsistencyResult(validFS)
	if err != nil {
		t.Fatalf("first getSchemaConsistencyResult failed: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1 on first call, got %d", version)
	}

	// Should return cached success in production mode despite invalid second FS.
	version, err = getSchemaConsistencyResult(fstest.MapFS{})
	if err != nil {
		t.Fatalf("expected cached success on second call, got error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected cached version 1 on second call, got %d", version)
	}
}

func TestGetSchemaConsistencyResult_ProdCachesError(t *testing.T) {
	resetProdSchemaConsistencyCacheForTest(t)
	withDevModeForTest(t, false)
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	_, err := getSchemaConsistencyResult(fstest.MapFS{})
	if err == nil {
		t.Fatal("expected first call to fail with empty migrations")
	}

	validFS := singleMigrationFS(`CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)
	_, err = getSchemaConsistencyResult(validFS)
	if err == nil {
		t.Fatal("expected cached error to persist on second call in production mode")
	}
}

func TestValidateSchemaSQLConsistency_GetLatestVersionError(t *testing.T) {
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	_, err := validateSchemaSQLConsistency(fstest.MapFS{})
	if err == nil {
		t.Fatal("expected error for empty migrations FS")
	}
	if !strings.Contains(err.Error(), "failed to get latest migration version") {
		t.Fatalf("expected latest migration version error, got: %v", err)
	}
}

func TestValidateSchemaSQLConsistency_InvalidSchemaSQL(t *testing.T) {
	withSchemaSQLForTest(t, `CREATE TABLE broken (`) // invalid SQL
	validFS := singleMigrationFS(`CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	_, err := validateSchemaSQLConsistency(validFS)
	if err == nil {
		t.Fatal("expected invalid schema.sql error")
	}
	if !strings.Contains(err.Error(), "failed to initialize temp schema database") {
		t.Fatalf("expected temp schema init error, got: %v", err)
	}
}

func TestValidateSchemaSQLConsistency_GetSchemaAtMigrationError(t *testing.T) {
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)
	badFS := fstest.MapFS{
		"000001_bad.up.sql":   &fstest.MapFile{Data: []byte("THIS IS NOT SQL;")},
		"000001_bad.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS demo_table;")},
	}

	_, err := validateSchemaSQLConsistency(badFS)
	if err == nil {
		t.Fatal("expected migration application error")
	}
	if !strings.Contains(err.Error(), "failed to get schema at migration v1") {
		t.Fatalf("expected get schema at migration error, got: %v", err)
	}
}

func TestValidateSchemaSQLConsistency_MismatchError(t *testing.T) {
	withSchemaSQLForTest(t, `CREATE TABLE schema_table (id INTEGER PRIMARY KEY);`)
	mismatchFS := singleMigrationFS(`CREATE TABLE migration_table (id INTEGER PRIMARY KEY);`)

	_, err := validateSchemaSQLConsistency(mismatchFS)
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
	if !strings.Contains(err.Error(), "schema.sql is out of sync with migration v1") {
		t.Fatalf("expected out-of-sync error, got: %v", err)
	}
}

func TestValidateSchemaSQLConsistency_Success(t *testing.T) {
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)
	validFS := singleMigrationFS(`CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	version, err := validateSchemaSQLConsistency(validFS)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
}

func TestNewDBWithMigrationCheck_SchemaConsistencyErrorPath(t *testing.T) {
	resetProdSchemaConsistencyCacheForTest(t)
	withDevModeForTest(t, true)
	withSchemaSQLForTest(t, `CREATE TABLE schema_table (id INTEGER PRIMARY KEY);`)

	withTempDevMigrationsDir(t, map[string]string{
		"000001_demo.up.sql":   "CREATE TABLE migration_table (id INTEGER PRIMARY KEY);",
		"000001_demo.down.sql": "DROP TABLE IF EXISTS migration_table;",
	})

	dbPath := filepath.Join(t.TempDir(), "schema_mismatch.db")
	db, err := NewDBWithMigrationCheck(dbPath, false)
	if err == nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatal("expected schema consistency error from NewDBWithMigrationCheck")
	}
	if !strings.Contains(err.Error(), "schema.sql is out of sync") {
		t.Fatalf("expected out-of-sync error, got: %v", err)
	}
}

func TestNewDBWithMigrationCheck_SuccessPathInDevMode(t *testing.T) {
	resetProdSchemaConsistencyCacheForTest(t)
	withDevModeForTest(t, true)
	withSchemaSQLForTest(t, `CREATE TABLE demo_table (id INTEGER PRIMARY KEY);`)

	withTempDevMigrationsDir(t, map[string]string{
		"000001_demo.up.sql":   "CREATE TABLE demo_table (id INTEGER PRIMARY KEY);",
		"000001_demo.down.sql": "DROP TABLE IF EXISTS demo_table;",
	})

	dbPath := filepath.Join(t.TempDir(), "success.db")
	db, err := NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	defer db.Close()

	var version uint
	if err := db.QueryRow("SELECT version FROM schema_migrations LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("failed to read baseline version: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected baseline version 1, got %d", version)
	}
}
