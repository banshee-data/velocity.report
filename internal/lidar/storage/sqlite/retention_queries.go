package sqlite

import (
	"encoding/json"
	"fmt"
)

// Read path for the retention and fragmentation measure
// (perframeeval.MeasureRetention, the geometry plan's W3).

// ClusterSample is one stored cluster's population and the retained sample
// the replay kept of it: the coordinates only, as float32, exactly as the
// record's SamplePoints hold them. The full record also carries every
// retained point's time, intensity and sensor, which this measure does not
// read, so the sample is extracted in SQL rather than through ListBySource.
type ClusterSample struct {
	ObservationID  string
	FrameUnixNanos int64
	ClusterID      int64
	// PointsCount is the cluster's whole population. When it exceeds
	// len(Sample), the sample is a simple random subset of the members
	// (l4perception's uniformSubsample), capped at the replay's
	// -sample-points.
	PointsCount int
	Sample      [][3]float32
}

// ListClusterSamples returns every cluster of a source whose frame lies in
// [fromNanos, toNanos], in frame order, then observation_id order within a
// frame. A record with no sample yields an empty Sample, not an error: the
// caller decides whether a sample-less cluster can be measured.
func (s *ObservationStore) ListClusterSamples(sourceID string, fromNanos, toNanos int64) ([]ClusterSample, error) {
	rows, err := s.db.Query(`
		SELECT observation_id
		     , frame_unix_nanos
		     , cluster_id
		     , COALESCE(json_extract(record_json, '$.raw_cluster.PointsCount'), 0)
		     , COALESCE(json_extract(record_json, '$.raw_cluster.SamplePoints'), '[]')
		  FROM lidar_observations
		 WHERE source_id = ?
		   AND frame_unix_nanos BETWEEN ? AND ?
		 ORDER BY frame_unix_nanos, observation_id`, sourceID, fromNanos, toNanos)
	if err != nil {
		return nil, fmt.Errorf("list cluster samples for source %s: %w", sourceID, err)
	}
	defer rows.Close()
	out := []ClusterSample{}
	for rows.Next() {
		var c ClusterSample
		var sample string
		if err := rows.Scan(&c.ObservationID, &c.FrameUnixNanos, &c.ClusterID, &c.PointsCount, &sample); err != nil {
			return nil, fmt.Errorf("scan cluster sample: %w", err)
		}
		if err := json.Unmarshal([]byte(sample), &c.Sample); err != nil {
			return nil, fmt.Errorf("decode the sample of observation %s: %w", c.ObservationID, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cluster samples: %w", err)
	}
	return out, nil
}
