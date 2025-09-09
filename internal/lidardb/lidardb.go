package lidardb

import (
	"database/sql"
	_ "embed"
	"log"

	_ "modernc.org/sqlite"
)

type LidarDB struct {
	*sql.DB
}

// schema.sql contains the SQL statements for creating the lidar database schema.
// It defines tables for storing lidar packets, extracted point data, and session information.
//
//go:embed schema.sql
var schemaSQL string

func NewLidarDB(path string) (*LidarDB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(schemaSQL)
	if err != nil {
		return nil, err
	}

	log.Println("initialized lidar database schema")

	return &LidarDB{db}, nil
}
