package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	modernsqlite "modernc.org/sqlite"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestFollowingAuditDurabilityIsolationAndConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.db")
	db, err := dbpkg.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewFollowingAuditStore(db.DB)
	binding := json.RawMessage(`{"source":"s","n":1}`)
	record := json.RawMessage(`{"id":"r","value":9007199254740993,"a":[true,null,1.0]}`)
	if created, err := store.AppendFollowingAudit("run", "digest", "r", binding, record); err != nil || !created {
		t.Fatalf("append %v %v", created, err)
	}
	if created, err := store.AppendFollowingAudit("run", "digest", "r", json.RawMessage(`{ "n":1e0,"source":"s" }`), json.RawMessage(`{"a":[true,null,1],"value":9007199254740993.0,"id":"r"}`)); err != nil || created {
		t.Fatalf("retry %v %v", created, err)
	}
	for _, changed := range []json.RawMessage{json.RawMessage(`{"id":"r","value":9007199254740992,"a":[true,null,1]}`), json.RawMessage(`{"id":"r","value":"9007199254740993","a":[true,null,1]}`), json.RawMessage(`{"id":"r","value":9007199254740993,"a":[null,true,1]}`)} {
		if _, err := store.AppendFollowingAudit("run", "digest", "r", binding, changed); !errors.Is(err, ErrFollowingAuditConflict) {
			t.Fatalf("record conflict %v", err)
		}
	}
	if _, err := store.AppendFollowingAudit("run", "digest", "new", json.RawMessage(`{"source":"other"}`), record); !errors.Is(err, ErrFollowingAuditConflict) {
		t.Fatalf("binding conflict %v", err)
	}
	if _, err := store.AppendFollowingAudit("other-run", "digest", "r", binding, json.RawMessage(`{"other":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendFollowingAudit("run", "other-digest", "r", json.RawMessage(`{"source":"revised"}`), json.RawMessage(`{"other":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendFollowingAudit("run", "digest", "a", binding, json.RawMessage(`{"id":"a"}`)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = dbpkg.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewFollowingAuditStore(db.DB)
	rows, err := store.ListFollowingAudit("run", "digest")
	if err != nil || len(rows) != 2 {
		t.Fatalf("restart %d %v", len(rows), err)
	}
	if string(rows[0]) != `{"id":"a"}` {
		t.Fatalf("deterministic ordering %s", rows[0])
	}
	rows, err = store.ListFollowingAudit("missing", "digest")
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("missing %v %v", rows, err)
	}
	for _, q := range []string{`UPDATE lidar_following_audit_records SET record_json='{}'`, `DELETE FROM lidar_following_audit_records`, `UPDATE lidar_following_audit_bindings SET binding_json='{}'`, `DELETE FROM lidar_following_audit_bindings`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("append-only trigger allowed %s", q)
		}
	}
}

func TestFollowingAuditConcurrentAppendAndRetry(t *testing.T) {
	db, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	store := NewFollowingAuditStore(db.DB)
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	created := make(chan bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("r%02d", i%16)
			yes, e := store.AppendFollowingAudit("run", "digest", id, json.RawMessage(`{"binding":true}`), json.RawMessage(fmt.Sprintf(`{"id":%q}`, id)))
			errs <- e
			created <- yes
		}(i)
	}
	wg.Wait()
	close(errs)
	close(created)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	n := 0
	for c := range created {
		if c {
			n++
		}
	}
	if n != 16 {
		t.Fatalf("created %d", n)
	}
	rows, err := store.ListFollowingAudit("run", "digest")
	if err != nil || len(rows) != 16 {
		t.Fatalf("lost append %d %v", len(rows), err)
	}
	// Exactly one binding wins when concurrent callers disagree; the losing
	// transaction must not append its otherwise-new record.
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := store.AppendFollowingAudit("race", "digest", fmt.Sprint(i), json.RawMessage(fmt.Sprintf(`{"binding":%d}`, i)), json.RawMessage(`{}`))
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	conflicts := 0
	for e := range errs {
		if errors.Is(e, ErrFollowingAuditConflict) {
			conflicts++
		} else if e != nil {
			t.Fatal(e)
		}
	}
	if conflicts != 1 {
		t.Fatalf("binding races %d", conflicts)
	}
	rows, err = store.ListFollowingAudit("race", "digest")
	if err != nil || len(rows) != 1 {
		t.Fatalf("binding race leaked %d %v", len(rows), err)
	}
}

func TestFollowingAuditValidationAndSQLErrors(t *testing.T) {
	db, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	store := NewFollowingAuditStore(db.DB)
	for _, bad := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`1`), json.RawMessage(`{broken`), json.RawMessage(`{} {}`), json.RawMessage(`{} trailing`)} {
		if _, e := store.AppendFollowingAudit("r", "d", "i", bad, json.RawMessage(`{}`)); e == nil {
			t.Fatalf("binding accepted %s", bad)
		}
		if _, e := store.AppendFollowingAudit("r", "d", "i", json.RawMessage(`{}`), bad); e == nil {
			t.Fatalf("record accepted %s", bad)
		}
	}
	for _, ids := range [][3]string{{"", "d", "i"}, {"r", " ", "i"}, {"r", "d", ""}} {
		if _, e := store.AppendFollowingAudit(ids[0], ids[1], ids[2], json.RawMessage(`{}`), json.RawMessage(`{}`)); e == nil {
			t.Fatal("empty identity")
		}
	}
	if _, e := store.ListFollowingAudit("", "d"); e == nil {
		t.Fatal("empty list identity")
	}
	// Arbitrary exponent magnitude cannot force a giant allocation or overflow.
	a := []byte(`{"x":1e99999999999999999999999999999,"y":-0.000}`)
	b := []byte(`{"y":0,"x":10e99999999999999999999999999998}`)
	if !followingAuditEquivalent(a, b) {
		t.Fatal("exact exponent comparison")
	}
	if followingAuditEquivalent([]byte(`broken`), []byte(`{}`)) {
		t.Fatal("corrupt JSON equal")
	}
	// A failure after binding insertion rolls back the whole transaction.
	if _, e := db.Exec(`CREATE TRIGGER reject_following_record BEFORE INSERT ON lidar_following_audit_records BEGIN SELECT RAISE(ABORT,'test rejection'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e := store.AppendFollowingAudit("rollback", "d", "i", json.RawMessage(`{}`), json.RawMessage(`{}`)); e == nil {
		t.Fatal("SQL error ignored")
	}
	var n int
	if e := db.QueryRow(`SELECT count(*) FROM lidar_following_audit_bindings WHERE run_id='rollback'`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("binding rollback %d %v", n, e)
	}
	if _, e := db.Exec(`DROP TRIGGER reject_following_record; DROP TABLE lidar_following_audit_records`); e != nil {
		t.Fatal(e)
	}
	if _, e := store.ListFollowingAudit("r", "d"); e == nil {
		t.Fatal("query error ignored")
	}
	if _, e := db.Exec(`DROP TABLE lidar_following_audit_bindings`); e != nil {
		t.Fatal(e)
	}
	if _, e := store.AppendFollowingAudit("r", "d", "i", json.RawMessage(`{}`), json.RawMessage(`{}`)); e == nil {
		t.Fatal("insert error ignored")
	}
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := store.AppendFollowingAudit("r", "d", "i", json.RawMessage(`{}`), json.RawMessage(`{}`)); e == nil {
		t.Fatal("begin error ignored")
	}
}

func TestFollowingAuditAtomicLimits(t *testing.T) {
	db, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	store := NewFollowingAuditStore(db.DB)
	binding := json.RawMessage(`{}`)
	if _, err := store.AppendFollowingAudit("count", "d", "first", binding, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`WITH RECURSIVE seq(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM seq WHERE x<9999) INSERT INTO lidar_following_audit_records SELECT 'count','d',CAST(x AS TEXT),'{}' FROM seq`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendFollowingAudit("count", "d", "overflow", binding, json.RawMessage(`{}`)); !errors.Is(err, ErrFollowingAuditLimit) {
		t.Fatalf("count limit %v", err)
	}
	if created, err := store.AppendFollowingAudit("count", "d", "first", binding, json.RawMessage(`{}`)); err != nil || created {
		t.Fatalf("retry at cap %v %v", created, err)
	}
	bigRecord := json.RawMessage(`{"note":"` + strings.Repeat("x", followingAuditContentBudget) + `"}`)
	if _, err := store.AppendFollowingAudit("bytes", "d", "overflow", binding, bigRecord); !errors.Is(err, ErrFollowingAuditLimit) {
		t.Fatalf("byte limit %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM lidar_following_audit_bindings WHERE run_id='bytes'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("limit rollback %d %v", n, err)
	}
}

// Fault-driver checks exercise transaction/read failures that cannot be induced
// by valid schema contents (for example a failed COMMIT or driver Rows.Err).
// All non-failing operations still execute against real SQLite.
type followingAuditFaultDriver struct{ fault string }
type followingAuditFaultConn struct {
	driver.Conn
	fault string
}
type followingAuditFaultTx struct {
	driver.Tx
	fail bool
}
type followingAuditFaultResult struct{ driver.Result }
type followingAuditFaultRows struct {
	failScan bool
	sent     bool
}

var followingAuditDriverID atomic.Int64

func (d followingAuditFaultDriver) Open(name string) (driver.Conn, error) {
	c, e := (&modernsqlite.Driver{}).Open(name)
	if e != nil {
		return nil, e
	}
	return followingAuditFaultConn{Conn: c, fault: d.fault}, nil
}
func (c followingAuditFaultConn) Begin() (driver.Tx, error) {
	tx, e := c.Conn.Begin()
	if e != nil {
		return nil, e
	}
	return followingAuditFaultTx{Tx: tx, fail: c.fault == "commit"}, nil
}
func (t followingAuditFaultTx) Commit() error {
	if t.fail {
		_ = t.Tx.Rollback()
		return errors.New("injected commit")
	}
	return t.Tx.Commit()
}
func (r followingAuditFaultResult) RowsAffected() (int64, error) {
	return 0, errors.New("injected rows affected")
}
func (c followingAuditFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	r, e := c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
	if e == nil && c.fault == "rowsaffected" && strings.Contains(q, "INSERT INTO lidar_following_audit_records") {
		return followingAuditFaultResult{r}, nil
	}
	return r, e
}
func (c followingAuditFaultConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if c.fault == "bindingread" && strings.HasPrefix(q, "SELECT binding_json") || c.fault == "recordread" && strings.HasPrefix(q, "SELECT record_json") || c.fault == "countread" && strings.HasPrefix(q, "SELECT count(*)") {
		return nil, errors.New("injected query")
	}
	if strings.HasPrefix(q, "SELECT record_json") && (c.fault == "listscan" || c.fault == "listnext") {
		return &followingAuditFaultRows{failScan: c.fault == "listscan"}, nil
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}
func (r *followingAuditFaultRows) Columns() []string { return []string{"record_json"} }
func (r *followingAuditFaultRows) Close() error      { return nil }
func (r *followingAuditFaultRows) Next(dest []driver.Value) error {
	if r.sent {
		return io.EOF
	}
	r.sent = true
	if r.failScan {
		dest[0] = nil
		return nil
	}
	return errors.New("injected row iteration")
}

func TestFollowingAuditDriverFailures(t *testing.T) {
	for _, fault := range []string{"bindingread", "recordread", "countread", "rowsaffected", "commit", "listscan", "listnext"} {
		t.Run(fault, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audit.db")
			d, e := dbpkg.NewDB(path)
			if e != nil {
				t.Fatal(e)
			}
			if _, e := NewFollowingAuditStore(d.DB).AppendFollowingAudit("r", "d", "i", json.RawMessage(`{}`), json.RawMessage(`{}`)); e != nil {
				t.Fatal(e)
			}
			if e := d.Close(); e != nil {
				t.Fatal(e)
			}
			name := fmt.Sprintf("following-audit-fault-%d", followingAuditDriverID.Add(1))
			sql.Register(name, followingAuditFaultDriver{fault: fault})
			db, e := sql.Open(name, path)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			store := NewFollowingAuditStore(db)
			if strings.HasPrefix(fault, "list") {
				if _, e := store.ListFollowingAudit("r", "d"); e == nil {
					t.Fatal("list failure ignored")
				}
				return
			}
			id := "new"
			if fault == "recordread" {
				id = "i"
			}
			if _, e := store.AppendFollowingAudit("r", "d", id, json.RawMessage(`{}`), json.RawMessage(`{}`)); e == nil {
				t.Fatal("append failure ignored")
			}
		})
	}
}
