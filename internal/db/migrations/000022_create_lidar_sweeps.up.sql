   CREATE TABLE IF NOT EXISTS lidar_sweeps (
          id INTEGER PRIMARY KEY AUTOINCREMENT
        , sweep_id TEXT NOT NULL UNIQUE
        , sensor_id TEXT NOT NULL
        , mode TEXT NOT NULL DEFAULT 'sweep'
        , status TEXT NOT NULL DEFAULT 'running'
        , request TEXT NOT NULL
        , results TEXT
        , charts TEXT
        , recommendation TEXT
        , round_results TEXT
        , error TEXT
        , started_at DATETIME NOT NULL
        , completed_at DATETIME
        , created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
          );

CREATE INDEX IF NOT EXISTS idx_lidar_sweeps_sensor ON lidar_sweeps (sensor_id);

CREATE INDEX IF NOT EXISTS idx_lidar_sweeps_status ON lidar_sweeps (status);
