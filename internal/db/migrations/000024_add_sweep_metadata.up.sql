-- Migration 000024: Add metadata columns for experiment versioning and explainability
-- Supports sections 9.1 and 9.2 of industry-standard-ml-solver-expansion-plan.md

ALTER TABLE lidar_sweeps ADD COLUMN objective_name TEXT;
ALTER TABLE lidar_sweeps ADD COLUMN objective_version TEXT;
ALTER TABLE lidar_sweeps ADD COLUMN transform_pipeline_name TEXT;
ALTER TABLE lidar_sweeps ADD COLUMN transform_pipeline_version TEXT;
ALTER TABLE lidar_sweeps ADD COLUMN score_components_json TEXT;
ALTER TABLE lidar_sweeps ADD COLUMN recommendation_explanation_json TEXT;
ALTER TABLE lidar_sweeps ADD COLUMN label_provenance_summary_json TEXT;
