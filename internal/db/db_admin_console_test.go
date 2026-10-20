package db

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// consoleQuery sends q to the SQL console's JSON route as a loopback caller
// and returns the status code.
func consoleQuery(mux *http.ServeMux, q string) int {
	req := debugRequest("/debug/tailsql/json?src=" + url.QueryEscape("sqlite://radar.db") + "&q=" + url.QueryEscape(q))
	req.Header.Set("Sec-Tailsql", "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code
}

// The SQL console reads through a read-only handle, so a write that its
// statement filter lets through is refused by SQLite and leaves the data as it
// was.
func TestTailsqlConsoleRefusesWrites(t *testing.T) {
	database, mux := adminMux(t)
	if _, err := database.Exec(`CREATE TABLE console_probe (v INTEGER)`); err != nil {
		t.Fatalf("creating the probe table: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO console_probe (v) VALUES (1)`); err != nil {
		t.Fatalf("seeding the probe table: %v", err)
	}

	if code := consoleQuery(mux, "SELECT v FROM console_probe"); code != http.StatusOK {
		t.Fatalf("reading through the console: status %d, want %d", code, http.StatusOK)
	}
	for _, q := range []string{
		"INSERT INTO console_probe (v) VALUES (2)",
		"UPDATE console_probe SET v = 3",
		"DELETE FROM console_probe",
		"DROP TABLE console_probe",
	} {
		if code := consoleQuery(mux, q); code == http.StatusOK {
			t.Errorf("%q through the console: status %d, want a refusal", q, code)
		}
	}

	var rows, maxV int
	if err := database.QueryRow(`SELECT COUNT(*), MAX(v) FROM console_probe`).Scan(&rows, &maxV); err != nil {
		t.Fatalf("reading the probe table back: %v", err)
	}
	if rows != 1 || maxV != 1 {
		t.Errorf("probe table holds %d rows with max %d after the console's writes, want 1 row of 1", rows, maxV)
	}
}

// A database with no file behind it cannot be reopened read-only, so the
// console is not given a handle rather than being given the writable one.
func TestConsoleHandleNeedsAFile(t *testing.T) {
	database, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("creating an in-memory database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if h, err := database.consoleHandle(); err == nil {
		_ = h.Close()
		t.Fatal("consoleHandle on an in-memory database: want an error, got a handle")
	}
}
