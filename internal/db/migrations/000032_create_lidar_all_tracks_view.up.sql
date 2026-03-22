-- Create a convenience VIEW that unions live tracks and run-track snapshots.
-- Useful for ad-hoc SQL analysis (sqlite3 / TailSQL) when querying across
-- both transient and permanent track data for a given sensor.
   CREATE VIEW IF NOT EXISTS lidar_all_tracks AS
   SELECT track_id
        , NULL AS run_id
        , sensor_id
        , track_state
        , start_unix_nanos
        , end_unix_nanos
        , observation_count
        , avg_speed_mps
        , max_speed_mps
        , bounding_box_length_avg
        , bounding_box_width_avg
        , bounding_box_height_avg
        , height_p95_max
        , intensity_mean_avg
        , object_class
        , object_confidence
        , classification_model
        , NULL AS user_label
        , NULL AS quality_label
     FROM lidar_tracks
UNION ALL
   SELECT track_id
        , run_id
        , sensor_id
        , track_state
        , start_unix_nanos
        , end_unix_nanos
        , observation_count
        , avg_speed_mps
        , max_speed_mps
        , bounding_box_length_avg
        , bounding_box_width_avg
        , bounding_box_height_avg
        , height_p95_max
        , intensity_mean_avg
        , object_class
        , object_confidence
        , classification_model
        , user_label
        , quality_label
     FROM lidar_run_tracks;
