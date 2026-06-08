package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// BackupDatabase writes a consistent standalone copy of the SQLite database at
// srcPath to destPath using `VACUUM INTO`.
//
// Unlike a plain file copy, this captures a transactionally consistent snapshot
// even when the source is a live WAL-mode database with un-checkpointed writes
// in its -wal file: a raw copy of just the main `.db` file is stale (missing
// committed frames still in the WAL) and can be torn if a checkpoint runs mid
// copy. The destination is a single self-contained file with no -wal/-shm
// siblings, which is exactly what the upgrade/rollback rescue path needs.
//
// The source is opened read-only, so the backup cannot modify the live
// database.
//
// srcPath is rejected if it contains '?' or '#': those are DSN delimiters, and
// concatenating them into the `file:` URI would let a caller inject query
// parameters (overriding mode=ro) or reintroduce the mistyped-path footgun.
// This mirrors the guard in ReadOnlyQuery.
func BackupDatabase(srcPath, destPath string) error {
	if strings.ContainsAny(srcPath, "?#") {
		return fmt.Errorf("invalid db path %q: must not contain '?' or '#'", srcPath)
	}
	conn, err := sql.Open("sqlite", "file:"+srcPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer conn.Close()

	// VACUUM INTO refuses to overwrite an existing destination file.
	if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing %s: %w", destPath, err)
	}

	if _, err := conn.Exec("VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("VACUUM INTO %s: %w", destPath, err)
	}
	return nil
}
