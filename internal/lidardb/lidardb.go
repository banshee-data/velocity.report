package lidardb

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"net"

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

// RecordLidarPacket stores a raw lidar packet in the database
// COMMENTED OUT - not storing raw packets, use wireshark/pcaps instead
/*
func (ldb *LidarDB) RecordLidarPacket(packetData []byte, sourceAddr *net.UDPAddr) error {
	query := `
		INSERT INTO lidar_packets (source_address, packet_size, packet_data, packet_hex)
		VALUES (?, ?, ?, ?)
	`

	hexData := hex.EncodeToString(packetData)

	_, err := ldb.Exec(query, sourceAddr.String(), len(packetData), packetData, hexData)
	if err != nil {
		return fmt.Errorf("failed to insert lidar packet: %v", err)
	}

	return nil
}
*/

// RecordLidarPoint stores an individual lidar point (extracted from packet) in the database
func (ldb *LidarDB) RecordLidarPoint(packetID int64, x, y, z float64, intensity int, timestampNs int64, azimuth, distance float64) error {
	query := `
		INSERT INTO lidar_points (packet_id, x, y, z, intensity, timestamp_ns, azimuth, distance)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := ldb.Exec(query, packetID, x, y, z, intensity, timestampNs, azimuth, distance)
	if err != nil {
		return fmt.Errorf("failed to insert lidar point: %v", err)
	}

	return nil
}

// StartSession creates a new lidar session record
func (ldb *LidarDB) StartSession(sourceAddr *net.UDPAddr, notes string) (int64, error) {
	query := `
		INSERT INTO lidar_sessions (source_address, session_notes)
		VALUES (?, ?)
	`

	result, err := ldb.Exec(query, sourceAddr.String(), notes)
	if err != nil {
		return 0, fmt.Errorf("failed to start lidar session: %v", err)
	}

	sessionID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get session ID: %v", err)
	}

	return sessionID, nil
}

// EndSession closes a lidar session and updates statistics
func (ldb *LidarDB) EndSession(sessionID int64) error {
	// Update session end time and calculate statistics
	query := `
		UPDATE lidar_sessions
		SET
			end_timestamp = UNIXEPOCH('subsec'),
			packet_count = (
				SELECT COUNT(*) FROM lidar_packets
				WHERE source_address = (SELECT source_address FROM lidar_sessions WHERE id = ?)
				AND write_timestamp >= (SELECT start_timestamp FROM lidar_sessions WHERE id = ?)
			),
			points_count = (
				SELECT COUNT(*) FROM lidar_points lp
				JOIN lidar_packets lpk ON lp.packet_id = lpk.id
				WHERE lpk.source_address = (SELECT source_address FROM lidar_sessions WHERE id = ?)
				AND lpk.write_timestamp >= (SELECT start_timestamp FROM lidar_sessions WHERE id = ?)
			)
		WHERE id = ?
	`

	_, err := ldb.Exec(query, sessionID, sessionID, sessionID, sessionID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to end lidar session: %v", err)
	}

	return nil
}

// GetRecentPackets retrieves the most recent lidar packets
// COMMENTED OUT - not storing raw packets
/*
func (ldb *LidarDB) GetRecentPackets(limit int) ([]LidarPacket, error) {
	query := `
		SELECT id, write_timestamp, source_address, packet_size, packet_hex
		FROM lidar_packets
		ORDER BY write_timestamp DESC
		LIMIT ?
	`

	rows, err := ldb.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent packets: %v", err)
	}
	defer rows.Close()

	var packets []LidarPacket
	for rows.Next() {
		var packet LidarPacket
		err := rows.Scan(&packet.ID, &packet.WriteTimestamp, &packet.SourceAddress, &packet.PacketSize, &packet.PacketHex)
		if err != nil {
			return nil, fmt.Errorf("failed to scan packet row: %v", err)
		}
		packets = append(packets, packet)
	}

	return packets, nil
}

// LidarPacket represents a stored lidar packet
// COMMENTED OUT - not storing raw packets
/*
type LidarPacket struct {
	ID             int64   `json:"id"`
	WriteTimestamp float64 `json:"write_timestamp"`
	SourceAddress  string  `json:"source_address"`
	PacketSize     int     `json:"packet_size"`
	PacketHex      string  `json:"packet_hex"`
}
*/

// LidarPoint represents an extracted lidar point
type LidarPoint struct {
	ID             int64   `json:"id"`
	PacketID       int64   `json:"packet_id"`
	WriteTimestamp float64 `json:"write_timestamp"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Z              float64 `json:"z"`
	Intensity      int     `json:"intensity"`
	TimestampNs    int64   `json:"timestamp_ns"`
	Azimuth        float64 `json:"azimuth"`
	Distance       float64 `json:"distance"`
}

// LidarSession represents a lidar data collection session
type LidarSession struct {
	ID             int64    `json:"id"`
	StartTimestamp float64  `json:"start_timestamp"`
	EndTimestamp   *float64 `json:"end_timestamp,omitempty"`
	SourceAddress  string   `json:"source_address"`
	PacketCount    int      `json:"packet_count"`
	PointsCount    int      `json:"points_count"`
	SessionNotes   string   `json:"session_notes"`
}
