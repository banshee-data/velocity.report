-- Region persistence for settling time optimization
-- Stores identified region data with scene hash for quick restoration
-- when loading PCAPs from the same location
   CREATE TABLE lidar_bg_regions (
          region_set_id INTEGER PRIMARY KEY AUTOINCREMENT
        , snapshot_id INTEGER REFERENCES lidar_bg_snapshot (snapshot_id)
        , sensor_id TEXT NOT NULL
        , created_unix_nanos INTEGER NOT NULL
        , region_count INTEGER NOT NULL
        , regions_json TEXT NOT NULL
        , variance_data_json TEXT
        , settling_frames INTEGER NOT NULL
        , scene_hash TEXT NOT NULL
          );

CREATE INDEX idx_bg_regions_sensor ON lidar_bg_regions (sensor_id);

CREATE INDEX idx_bg_regions_scene_hash ON lidar_bg_regions (scene_hash);
