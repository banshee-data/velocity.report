package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestSchemaConsistency verifies that running all migrations produces the same schema as schema.sql
func TestSchemaConsistency(t *testing.T) {
	// Create two test databases
	dbFromMigrations := setupEmptyTestDB(t)
	defer cleanupTestDB(t, dbFromMigrations)

	dbFromSchema := setupTestDB(t)
	defer cleanupTestDB(t, dbFromSchema)

	// Apply all migrations to the first database
	migrationsDir := "../../data/migrations"
	absPath, err := filepath.Abs(migrationsDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	err = dbFromMigrations.MigrateUp(absPath)
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Get schema from both databases
	schemaMigrations := getSchemaDefinition(t, dbFromMigrations.DB)
	schemaFromSQL := getSchemaDefinition(t, dbFromSchema.DB)

	// Compare schemas (excluding schema_migrations table which only exists in migrated DB)
	if !schemasMatch(schemaMigrations, schemaFromSQL) {
		t.Errorf("Schema mismatch between migrations and schema.sql")
		t.Logf("Schema from migrations:\n%s", formatSchema(schemaMigrations))
		t.Logf("\nSchema from schema.sql:\n%s", formatSchema(schemaFromSQL))
	}
}

// setupEmptyTestDB creates a test database without running schema.sql
func setupEmptyTestDB(t *testing.T) *DB {
	t.Helper()
	fname := t.Name() + "_migrations.db"
	_ = os.Remove(fname)

	// Open database directly without running schema.sql
	sqlDB, err := sql.Open("sqlite", fname)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}

	return &DB{sqlDB}
}

// getSchemaDefinition extracts table and index definitions from a database
func getSchemaDefinition(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()

	schema := make(map[string]string)

	// Get all tables except sqlite internal tables and schema_migrations
	rows, err := db.Query(`
		SELECT name, sql 
		FROM sqlite_master 
		WHERE type IN ('table', 'index', 'trigger')
		  AND name NOT LIKE 'sqlite_%'
		  AND name != 'schema_migrations'
		  AND name != 'version_unique'
		  AND sql IS NOT NULL
		ORDER BY type, name
	`)
	if err != nil {
		t.Fatalf("Failed to query schema: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, sql string
		if err := rows.Scan(&name, &sql); err != nil {
			t.Fatalf("Failed to scan schema row: %v", err)
		}
		// Normalize whitespace and remove trailing semicolons
		sql = normalizeSQL(sql)
		schema[name] = sql
	}

	return schema
}

// normalizeSQL normalizes SQL statements for comparison
func normalizeSQL(sql string) string {
	// Remove extra whitespace
	sql = strings.TrimSpace(sql)

	// Replace multiple spaces/newlines with single space
	lines := strings.Fields(sql)
	sql = strings.Join(lines, " ")

	// Remove trailing semicolon
	sql = strings.TrimSuffix(sql, ";")

	// Normalize comma spacing - remove spaces before commas
	sql = strings.ReplaceAll(sql, " ,", ",")

	return sql
}

// schemasMatch compares two schema definitions
func schemasMatch(schema1, schema2 map[string]string) bool {
	// Check if all keys in schema1 exist in schema2
	for key, sql1 := range schema1 {
		sql2, exists := schema2[key]
		if !exists {
			return false
		}
		if sql1 != sql2 {
			return false
		}
	}

	// Check if all keys in schema2 exist in schema1
	for key := range schema2 {
		if _, exists := schema1[key]; !exists {
			return false
		}
	}

	return true
}

// formatSchema formats a schema map for display
func formatSchema(schema map[string]string) string {
	var builder strings.Builder

	// Sort keys for consistent output
	keys := make([]string, 0, len(schema))
	for k := range schema {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(":\n  ")
		builder.WriteString(schema[key])
		builder.WriteString("\n\n")
	}

	return builder.String()
}
