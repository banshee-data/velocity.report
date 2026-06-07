package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func newReadOnlyQueryDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sensor_data.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(`CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT, payload BLOB)`); err != nil {
		t.Fatalf("create widgets: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO widgets (name, payload) VALUES ('alpha', x'6162'), (NULL, x'6364')`); err != nil {
		t.Fatalf("insert widgets: %v", err)
	}
	return path
}

func TestReadOnlyQueryFormatsRows(t *testing.T) {
	path := newReadOnlyQueryDB(t)

	var out bytes.Buffer
	n, err := ReadOnlyQuery(path, `SELECT name, payload, id FROM widgets ORDER BY id`, 0, &out)
	if err != nil {
		t.Fatalf("ReadOnlyQuery failed: %v", err)
	}
	if n != 2 {
		t.Fatalf("row count = %d, want 2", n)
	}
	want := "name\tpayload\tid\nalpha\tab\t1\nNULL\tcd\t2\n"
	if out.String() != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", out.String(), want)
	}
}

func TestReadOnlyQueryLimitReturnsSentinelAfterPrintedRows(t *testing.T) {
	path := newReadOnlyQueryDB(t)

	var out bytes.Buffer
	n, err := ReadOnlyQuery(path, `SELECT id FROM widgets ORDER BY id`, 1, &out)
	if !errors.Is(err, ErrReadOnlyRowLimit) {
		t.Fatalf("error = %v, want ErrReadOnlyRowLimit", err)
	}
	if n != 1 {
		t.Fatalf("row count = %d, want 1", n)
	}
	if got := strings.TrimSpace(out.String()); got != "id\n1" {
		t.Fatalf("unexpected limited output: %q", got)
	}
}

func TestReadOnlyQueryRejectsWrites(t *testing.T) {
	path := newReadOnlyQueryDB(t)

	var out bytes.Buffer
	_, err := ReadOnlyQuery(path, `INSERT INTO widgets (name) VALUES ('beta')`, 0, &out)
	if err == nil {
		t.Fatal("expected write query to fail")
	}
	if out.Len() != 0 {
		t.Fatalf("write query should not print rows, got %q", out.String())
	}
}

func TestReadOnlyQueryWrapsOpenError(t *testing.T) {
	old := openReadOnlySQL
	openReadOnlySQL = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	}
	defer func() { openReadOnlySQL = old }()

	_, err := ReadOnlyQuery("broken.db", "SELECT 1", 0, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "open broken.db") {
		t.Fatalf("expected wrapped open error, got %v", err)
	}
}

type fakeReadOnlyRows struct {
	cols      []string
	colsErr   error
	values    [][]any
	scanErr   error
	err       error
	nextIndex int
	scanIndex int
}

func (r *fakeReadOnlyRows) Columns() ([]string, error) {
	if r.colsErr != nil {
		return nil, r.colsErr
	}
	return r.cols, nil
}

func (r *fakeReadOnlyRows) Next() bool {
	if r.nextIndex >= len(r.values) {
		return false
	}
	r.nextIndex++
	return true
}

func (r *fakeReadOnlyRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.values[r.scanIndex]
	r.scanIndex++
	for i := range dest {
		ptr, ok := dest[i].(*any)
		if !ok {
			return fmt.Errorf("unexpected scan destination %T", dest[i])
		}
		*ptr = row[i]
	}
	return nil
}

func (r *fakeReadOnlyRows) Err() error {
	return r.err
}

func TestWriteReadOnlyRowsErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows *fakeReadOnlyRows
		want string
	}{
		{
			name: "columns",
			rows: &fakeReadOnlyRows{colsErr: errors.New("columns failed")},
			want: "columns failed",
		},
		{
			name: "scan",
			rows: &fakeReadOnlyRows{cols: []string{"id"}, values: [][]any{{1}}, scanErr: errors.New("scan failed")},
			want: "scan failed",
		},
		{
			name: "rows err",
			rows: &fakeReadOnlyRows{cols: []string{"id"}, err: errors.New("rows failed")},
			want: "rows failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := writeReadOnlyRows(tc.rows, 0, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestFormatReadOnlyCell(t *testing.T) {
	if got := formatReadOnlyCell(nil); got != "NULL" {
		t.Fatalf("nil = %q, want NULL", got)
	}
	if got := formatReadOnlyCell([]byte("hello")); got != "hello" {
		t.Fatalf("bytes = %q, want hello", got)
	}
	if got := formatReadOnlyCell(12.5); got != "12.5" {
		t.Fatalf("float = %q, want 12.5", got)
	}
}
