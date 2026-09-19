package sqlite

import (
	"database/sql"
	"path/filepath"
)

// SQLDB is a type alias for sql.DB, exported so that packages outside the
// storage layer can reference the database connection type without importing
// database/sql directly. This keeps the database/sql import boundary narrow:
// only internal/db and the internal/lidar/storage tree should import it.
type SQLDB = sql.DB

// SQLTx is a type alias for sql.Tx, exported so that packages outside the
// storage layer can reference the transaction type without importing
// database/sql directly.
type SQLTx = sql.Tx

// DBClient is the minimal query/exec surface shared by *sql.DB and *db.DB.
// It standardises store constructors on behaviour rather than a concrete type.
type DBClient interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Begin() (*sql.Tx, error)
}

// ErrNotFound is returned when a queried record does not exist.
// Callers outside the storage layer should check against this sentinel
// instead of importing database/sql for sql.ErrNoRows.
var ErrNotFound = sql.ErrNoRows

// OpenReadOnly opens an existing SQLite database file in read-only mode, for
// tools that inspect immutable evidence without risking a write. Callers
// outside the storage layer use this instead of sql.Open so the
// database/sql import boundary holds.
func OpenReadOnly(path string) (*SQLDB, error) {
	return sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(5000)")
}
