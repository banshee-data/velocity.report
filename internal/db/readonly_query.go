package db

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
)

var openReadOnlySQL = sql.Open

// ErrReadOnlyRowLimit is returned by ReadOnlyQuery when output was truncated at
// the requested row limit. It is not a failure: the rows up to the limit have
// already been written.
var ErrReadOnlyRowLimit = errors.New("row limit reached")

// ReadOnlyQuery opens dbPath read-only and writes the result of query to out as
// tab-separated rows (a header row of column names, then up to limit data rows;
// limit <= 0 means no cap). It backs `velocity data sql` and replaces the
// on-device `sqlite3` CLI for field diagnostics.
//
// The database is opened with mode=ro, so an existing database is opened
// read-only (a mistyped path will not create a new one) and writes are refused
// at the SQLite layer regardless of the supplied query. query_only is a
// belt-and-suspenders connection guard and the busy timeout keeps a query
// against the live service from failing instantly.
//
// It returns the number of data rows written. When the row limit is hit the
// returned error is ErrReadOnlyRowLimit; callers should treat that as a
// non-fatal truncation rather than a query failure.
func ReadOnlyQuery(dbPath, query string, limit int, out io.Writer) (int, error) {
	if strings.ContainsAny(dbPath, "?#") {
		return 0, fmt.Errorf("invalid db path %q: must not contain '?' or '#'", dbPath)
	}
	dsn := "file:" + dbPath + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	conn, err := openReadOnlySQL("sqlite", dsn)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer conn.Close()

	rows, err := conn.Query(query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	return writeReadOnlyRows(rows, limit, out)
}

type readOnlyRows interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func writeReadOnlyRows(rows readOnlyRows, limit int, out io.Writer) (int, error) {
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	if len(cols) > 0 {
		fmt.Fprintln(out, strings.Join(cols, "\t"))
	}

	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	printed := 0
	for rows.Next() {
		if limit > 0 && printed >= limit {
			return printed, ErrReadOnlyRowLimit
		}
		if err := rows.Scan(ptrs...); err != nil {
			return printed, err
		}
		fields := make([]string, len(cols))
		for i, v := range vals {
			fields[i] = formatReadOnlyCell(v)
		}
		fmt.Fprintln(out, strings.Join(fields, "\t"))
		printed++
	}
	return printed, rows.Err()
}

func formatReadOnlyCell(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
