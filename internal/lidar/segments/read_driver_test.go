package segments

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

// The database/sql contract permits iteration and close to fail after Query
// succeeds. SQLite fixtures cannot reliably produce those driver boundaries.
type readerFaultDriver struct{ fault string }
type readerFaultConn struct{ fault string }
type readerFaultTx struct{}
type readerFaultRows struct {
	columns []string
	values  [][]driver.Value
	fault   string
	index   int
}

func (d readerFaultDriver) Open(string) (driver.Conn, error) { return readerFaultConn{d.fault}, nil }
func (c readerFaultConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c readerFaultConn) Close() error              { return nil }
func (c readerFaultConn) Begin() (driver.Tx, error) { return readerFaultTx{}, nil }
func (c readerFaultConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return readerFaultTx{}, nil
}
func (readerFaultTx) Commit() error   { return nil }
func (readerFaultTx) Rollback() error { return nil }
func (c readerFaultConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT DISTINCT source_id"):
		return &readerFaultRows{columns: []string{"source_id"}, values: [][]driver.Value{{"source"}}}, nil
	case strings.Contains(query, "SELECT COUNT(*)"):
		return &readerFaultRows{columns: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
	case strings.Contains(query, "SELECT creation_sequence"):
		return &readerFaultRows{columns: []string{"creation_sequence", "frame_unix_nanos", "x", "y", "vx", "vy"}, fault: c.fault}, nil
	default:
		return nil, errors.New("unexpected query")
	}
}
func (r *readerFaultRows) Columns() []string { return r.columns }
func (r *readerFaultRows) Close() error {
	if r.fault == "close" {
		return errors.New("close failed")
	}
	return nil
}
func (r *readerFaultRows) Next(dest []driver.Value) error {
	if r.fault == "next" {
		return errors.New("iteration failed")
	}
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func TestEstimateReaderReportsLateDriverFailures(t *testing.T) {
	for _, fault := range []string{"next", "close"} {
		t.Run(fault, func(t *testing.T) {
			db := sql.OpenDB(readerFaultConnector{driver: readerFaultDriver{fault}})
			t.Cleanup(func() { db.Close() })
			want := "iteration failed"
			if fault == "close" {
				want = "close failed"
			}
			if _, _, err := LoadEstimates(db, "source", "online"); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("lost %s error: %v", fault, err)
			}
		})
	}
}

type readerFaultConnector struct{ driver readerFaultDriver }

func (c readerFaultConnector) Connect(context.Context) (driver.Conn, error) {
	return c.driver.Open("")
}
func (c readerFaultConnector) Driver() driver.Driver { return c.driver }
