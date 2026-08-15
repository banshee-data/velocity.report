package db

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// failingWriter is an http.ResponseWriter whose Write always fails, so the
// handlers' response-writing error branches can be reached. Header and
// WriteHeader still work, and the status is recorded for assertions.
type failingWriter struct {
	header http.Header
	status int
}

func newFailingWriter() *failingWriter {
	return &failingWriter{header: make(http.Header)}
}

func (f *failingWriter) Header() http.Header { return f.header }

func (f *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated response write failure")
}

func (f *failingWriter) WriteHeader(status int) {
	if f.status == 0 {
		f.status = status
	}
}

// debugRequest builds a request that passes tsweb's debug-access gate.
// httptest.NewRequest defaults RemoteAddr to a TEST-NET address, which tsweb
// rejects with 403 before the handler ever runs.
func debugRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

// adminMux returns a mux with the admin routes attached to a fresh database.
func adminMux(t *testing.T) (*DB, *http.ServeMux) {
	t.Helper()
	database, err := NewDB(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("creating database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	mux := http.NewServeMux()
	database.AttachAdminRoutes(mux)
	return database, mux
}

func TestGetDatabaseStatsFailsOnClosedDatabase(t *testing.T) {
	database, err := NewDB(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("creating database: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing database: %v", err)
	}

	// Every pragma query fails once the handle is closed, including the
	// individual-pragma fallback path.
	if _, err := database.GetDatabaseStats(); err == nil {
		t.Fatal("GetDatabaseStats on a closed database succeeded, want an error")
	}
}

func TestDBStatsEndpointReportsStatsFailure(t *testing.T) {
	database, mux := adminMux(t)
	// Close the handle after the routes are attached so the handler's own
	// error path runs rather than failing at construction.
	if err := database.Close(); err != nil {
		t.Fatalf("closing database: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, debugRequest("/debug/db-stats"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %q)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "could not read database statistics") {
		t.Errorf("body = %q, want the stats-failure diagnostic", rec.Body)
	}
}

func TestDBStatsEndpointReportsEncodeFailure(t *testing.T) {
	_, mux := adminMux(t)

	// The stats gather succeeds; the JSON encode fails because the response
	// writer refuses every Write.
	w := newFailingWriter()
	mux.ServeHTTP(w, debugRequest("/debug/db-stats"))

	if w.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.status)
	}
}

func TestBackupEndpointReportsVacuumFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	database, mux := adminMux(t)
	// A closed handle makes VACUUM INTO fail, so the backup never gets as far
	// as opening or streaming a file.
	if err := database.Close(); err != nil {
		t.Fatalf("closing database: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, debugRequest("/debug/backup"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %q)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "could not create backup") {
		t.Errorf("body = %q, want the backup-failure diagnostic", rec.Body)
	}
}

func TestBackupEndpointReportsStreamFailure(t *testing.T) {
	// Backup writes into the working directory, so run somewhere disposable.
	t.Chdir(t.TempDir())
	_, mux := adminMux(t)

	// VACUUM INTO succeeds and the file opens, but streaming the gzip body to
	// the client fails on the first Write.
	w := newFailingWriter()
	mux.ServeHTTP(w, debugRequest("/debug/backup"))

	if w.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.status)
	}
}
