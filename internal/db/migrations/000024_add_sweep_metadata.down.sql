-- Migration 000024: Remove metadata columns
-- modernc.org/sqlite v1.44.3 bundles SQLite 3.51.2 which supports DROP COLUMN

ALTER TABLE lidar_sweeps DROP COLUMN objective_name;
ALTER TABLE lidar_sweeps DROP COLUMN objective_version;
ALTER TABLE lidar_sweeps DROP COLUMN transform_pipeline_name;
ALTER TABLE lidar_sweeps DROP COLUMN transform_pipeline_version;
ALTER TABLE lidar_sweeps DROP COLUMN score_components_json;
ALTER TABLE lidar_sweeps DROP COLUMN recommendation_explanation_json;
ALTER TABLE lidar_sweeps DROP COLUMN label_provenance_summary_json;
