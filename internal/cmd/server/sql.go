package server

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
)

// runSQL implements `velocity data sql`: a deliberately narrow, read-only
// SQLite inspection path for field diagnostics, replacing the on-device
// `sqlite3` apt dependency. The actual database access lives in internal/db
// (the only layer permitted to import database/sql); this function owns flag
// parsing, the explicit file target, the row cap, and exit codes.
func runSQL(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("data sql", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db-path", "sensor_data.db", "path to the SQLite database file")
	readOnly := fs.Bool("read-only", true, "read-only access (enforced; the only supported mode)")
	limit := fs.Int("limit", 100, "maximum number of rows to print (<= 0 means no limit)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, `usage: velocity data sql [--db-path <file>] [--limit N] [--read-only] "<SQL>"`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*readOnly {
		fmt.Fprintln(stderr, "data sql: only read-only access is supported; do not pass --read-only=false")
		return 2
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		fs.Usage()
		return 2
	}

	switch _, err := db.ReadOnlyQuery(*dbPath, query, *limit, stdout); {
	case errors.Is(err, db.ErrReadOnlyRowLimit):
		fmt.Fprintf(stderr, "data sql: output truncated at --limit=%d rows\n", *limit)
		return 0
	case err != nil:
		fmt.Fprintf(stderr, "data sql: %v\n", err)
		return 1
	}
	return 0
}
