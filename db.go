package main

import "database/sql"

type DB struct {
	*sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS data (
			uptime DOUBLE,
			magnitude DOUBLE,
			speed DOUBLE,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS commands (
			command_id BIGINT PRIMARY KEY,
			command TEXT,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS log (
			log_id BIGINT PRIMARY KEY,
			command_id BIGINT,
			log_data TEXT,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(command_id) REFERENCES commands(command_id)
		);
	`)
	if err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) RecordObservation(uptime, magnitude, speed float64) error {
	_, err := db.Exec("INSERT INTO data (uptime, magnitude, speed) VALUES (?, ?, ?)", uptime, magnitude, speed)
	if err != nil {
		return err
	}
	return nil
}
