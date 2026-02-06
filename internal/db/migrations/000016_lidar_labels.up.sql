-- Add lidar_labels table for manual labelling workflow
-- Labels can be applied to tracks for classifier training and validation

CREATE TABLE IF NOT EXISTS lidar_labels (
    label_id TEXT PRIMARY KEY,
    track_id TEXT NOT NULL,
    class_label TEXT NOT NULL,
    start_timestamp_ns INTEGER NOT NULL,
    end_timestamp_ns INTEGER,
    confidence REAL,
    created_by TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    notes TEXT,
    FOREIGN KEY (track_id) REFERENCES lidar_tracks(track_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_lidar_labels_track ON lidar_labels(track_id);
CREATE INDEX IF NOT EXISTS idx_lidar_labels_time ON lidar_labels(start_timestamp_ns, end_timestamp_ns);
CREATE INDEX IF NOT EXISTS idx_lidar_labels_class ON lidar_labels(class_label);
