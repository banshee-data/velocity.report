package db

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tailscale/tailsql/server/tailsql"
	_ "modernc.org/sqlite"
	"tailscale.com/tsweb"
)

// TableStats contains size and row count information for a database table.
type TableStats struct {
	Name     string  `json:"name"`
	RowCount int64   `json:"row_count"`
	SizeMB   float64 `json:"size_mb"`
}

// DatabaseStats contains overall database statistics.
type DatabaseStats struct {
	TotalSizeMB float64      `json:"total_size_mb"`
	Tables      []TableStats `json:"tables"`
}

func quoteSQLiteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// GetDatabaseStats returns size and row count information for all tables in the database.
// Uses SQLite's dbstat virtual table to get accurate size information.
func (db *DB) GetDatabaseStats() (*DatabaseStats, error) {
	// Get total database size using page_count * page_size
	var totalPages, pageSize int64
	row := db.QueryRow("SELECT page_count, page_size FROM pragma_page_count(), pragma_page_size()")
	if err := row.Scan(&totalPages, &pageSize); err != nil {
		// Fallback: try individual pragmas
		if err := db.QueryRow("PRAGMA page_count").Scan(&totalPages); err != nil {
			return nil, fmt.Errorf("failed to get page count: %w", err)
		}
		if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
			return nil, fmt.Errorf("failed to get page size: %w", err)
		}
	}
	totalSizeMB := float64(totalPages*pageSize) / (1024 * 1024)

	// Get list of tables
	tablesQuery := `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	rows, err := db.Query(tablesQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tableNames = append(tableNames, name)
	}

	// Get stats for each table
	var tables []TableStats
	for _, tableName := range tableNames {
		var rowCount int64
		// Build the COUNT(*) query dynamically with a quoted SQLite identifier.
		// SQL prepared statements parameterize values, not identifiers, so table
		// names cannot be bound as parameters. Here tableName comes from
		// sqlite_master (trusted metadata), and quoteSQLiteIdentifier applies
		// SQLite's identifier escaping rules by doubling embedded quotes.
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteSQLiteIdentifier(tableName))
		if err := db.QueryRow(countQuery).Scan(&rowCount); err != nil {
			// Table might be empty or have issues, continue with 0
			rowCount = 0
		}

		// Get size using dbstat virtual table (if available)
		var sizeMB float64
		sizeQuery := `SELECT COALESCE(SUM(pgsize), 0) / 1048576.0 FROM dbstat WHERE name = ?`
		if err := db.QueryRow(sizeQuery, tableName).Scan(&sizeMB); err != nil {
			// dbstat might not be available, estimate from row count
			sizeMB = 0
		}

		tables = append(tables, TableStats{
			Name:     tableName,
			RowCount: rowCount,
			SizeMB:   math.Round(sizeMB*100) / 100, // Round to 2 decimal places
		})
	}

	// Sort tables by size descending
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].SizeMB > tables[j].SizeMB
	})

	return &DatabaseStats{
		TotalSizeMB: math.Round(totalSizeMB*100) / 100,
		Tables:      tables,
	}, nil
}

func (db *DB) AttachAdminRoutes(mux *http.ServeMux) {
	debug := tsweb.Debugger(mux)
	// create a tailSQL instance and point it to our DB
	tsql, err := tailsql.NewServer(tailsql.Options{
		RoutePrefix: "/debug/tailsql/",
	})
	if err != nil {
		log.Fatalf("could not create tailsql server: %v", err)
	}
	tsql.SetDB("sqlite://radar.db", db.DB, &tailsql.DBOptions{
		Label: "Radar DB",
	})

	// mount the tailSQL server on the debug /tailsql path
	debug.Handle("tailsql/", "SQL live debugging", tsql.NewMux())

	debug.Handle("db-stats", "Database table sizes and disk usage (JSON)", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats, err := db.GetDatabaseStats()
		if err != nil {
			http.Error(w, fmt.Sprintf("could not read database statistics: %v", err), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			http.Error(w, fmt.Sprintf("could not encode statistics: %v", err), http.StatusInternalServerError)
			return
		}
	}))

	debug.Handle("backup", "Create and download a backup of the database now", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unixTime := time.Now().Unix()
		backupPath := fmt.Sprintf("backup-%d.db", unixTime)
		if _, err := db.DB.Exec("VACUUM INTO ?", backupPath); err != nil {
			http.Error(w, fmt.Sprintf("could not create backup: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", backupPath))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")

		// Send the backup file to the client
		backupFile, err := os.Open(backupPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not open backup file: %v", err), http.StatusInternalServerError)
			return
		}

		// close the backup file after sending it
		// and remove it from the filesystem
		defer func() {
			backupFile.Close()
			if err := os.Remove(backupPath); err != nil {
				log.Printf("Failed to remove backup file: %v", err)
			}
		}()

		gzipWriter := gzip.NewWriter(w)
		defer gzipWriter.Close()
		if _, err := gzipWriter.Write([]byte{}); err != nil {
			// Need to write something to initialize the gzip header
			http.Error(w, fmt.Sprintf("could not start gzip compression: %v", err), http.StatusInternalServerError)
			return
		}

		// Copy the backup file content to the gzip writer
		if _, err := io.Copy(gzipWriter, backupFile); err != nil {
			http.Error(w, fmt.Sprintf("could not write backup data: %v", err), http.StatusInternalServerError)
			return
		}
	}))
}
