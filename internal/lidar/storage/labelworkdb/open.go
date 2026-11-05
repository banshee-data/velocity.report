package labelworkdb

import (
	"database/sql"
	"fmt"
	"os"

	// The ledger database is the server's SQLite file.
	_ "modernc.org/sqlite"
)

// OpenUseLedger opens an existing database's use ledger for an offline tool.
// It never creates or migrates a database: a file without lidar_data_uses is
// refused, and the server migrates it on its next start.
func OpenUseLedger(path string) (UseLedger, func() error, error) {
	if _, err := os.Stat(path); err != nil {
		return UseLedger{}, nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return UseLedger{}, nil, err
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'lidar_data_uses'`).Scan(&n); err != nil || n == 0 {
		db.Close()
		return UseLedger{}, nil, fmt.Errorf("%s has no lidar_data_uses table: start the server on it once to migrate it", path)
	}
	return UseLedger{DB: db}, db.Close, nil
}
