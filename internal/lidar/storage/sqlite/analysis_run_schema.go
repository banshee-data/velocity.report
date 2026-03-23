package sqlite

import (
	"fmt"
	"strings"
)

type analysisRunRecordCapabilities struct {
	RunConfigID         bool
	RequestedParamSetID bool
	ReplayCaseID        bool
	CompletedAt         bool
	FrameStartNs        bool
	FrameEndNs          bool
}

func (s *AnalysisRunStore) runRecordCapabilities() (analysisRunRecordCapabilities, error) {
	s.schemaOnce.Do(func() {
		rows, err := s.db.Query(`PRAGMA table_info(lidar_run_records)`)
		if err != nil {
			s.recordCapsErr = fmt.Errorf("inspect lidar_run_records schema: %w", err)
			return
		}
		defer rows.Close()

		var caps analysisRunRecordCapabilities
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
				s.recordCapsErr = fmt.Errorf("scan lidar_run_records schema: %w", err)
				return
			}
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "run_config_id":
				caps.RunConfigID = true
			case "requested_param_set_id":
				caps.RequestedParamSetID = true
			case "replay_case_id":
				caps.ReplayCaseID = true
			case "completed_at":
				caps.CompletedAt = true
			case "frame_start_ns":
				caps.FrameStartNs = true
			case "frame_end_ns":
				caps.FrameEndNs = true
			}
		}
		if err := rows.Err(); err != nil {
			s.recordCapsErr = fmt.Errorf("iterate lidar_run_records schema: %w", err)
			return
		}
		s.recordCaps = caps
	})

	return s.recordCaps, s.recordCapsErr
}
