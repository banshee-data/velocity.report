-- Rollback: selections no longer name the selector that chose them.
--
-- The record goes with the column. A selection that was clipped keeps it in
-- its pack's segment.json, which this does not touch.
     DROP TRIGGER IF EXISTS lidar_segment_selections_keep_selector;

     DROP TRIGGER IF EXISTS lidar_segment_selections_name_selector;

    ALTER TABLE lidar_segment_selections
     DROP COLUMN selector_json;
