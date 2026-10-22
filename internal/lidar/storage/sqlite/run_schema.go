package sqlite

import (
	"fmt"
	"strings"
)

type runRecordCapabilities struct {
	RunConfigID         bool
	RequestedParamSetID bool
	ClipID              bool
	CompletedAt         bool
	FrameStartNs        bool
	FrameEndNs          bool
	StatisticsJSON      bool
}

type runSchemaRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func (s *RunStore) runRecordCapabilities() (runRecordCapabilities, error) {
	s.schemaOnce.Do(func() {
		rows, err := s.db.Query(`PRAGMA table_info(lidar_runs)`)
		if err != nil {
			s.recordCapsErr = fmt.Errorf("inspect lidar_runs schema: %w", err)
			return
		}
		defer rows.Close()

		s.recordCaps, s.recordCapsErr = readRunRecordCapabilitiesRows(rows)
	})

	return s.recordCaps, s.recordCapsErr
}

func readRunRecordCapabilitiesRows(rows runSchemaRows) (runRecordCapabilities, error) {
	var caps runRecordCapabilities
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return runRecordCapabilities{}, fmt.Errorf("scan lidar_runs schema: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "run_config_id":
			caps.RunConfigID = true
		case "requested_param_set_id":
			caps.RequestedParamSetID = true
		case "clip_id":
			caps.ClipID = true
		case "completed_at":
			caps.CompletedAt = true
		case "frame_start_ns":
			caps.FrameStartNs = true
		case "frame_end_ns":
			caps.FrameEndNs = true
		case "statistics_json":
			caps.StatisticsJSON = true
		}
	}
	if err := rows.Err(); err != nil {
		return runRecordCapabilities{}, fmt.Errorf("iterate lidar_runs schema: %w", err)
	}
	return caps, nil
}
