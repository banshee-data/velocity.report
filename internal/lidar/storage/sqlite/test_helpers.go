package sqlite

import (
	"database/sql"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

// setupTrackingPipelineTestDB creates a test database with proper schema from schema.sql.
// This avoids hardcoded CREATE TABLE statements that can get out of sync with migrations.
func setupTrackingPipelineTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, cleanup := dbpkg.NewTestDB(t)
	return db.DB, cleanup
}
