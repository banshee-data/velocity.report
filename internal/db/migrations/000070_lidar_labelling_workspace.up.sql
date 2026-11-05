-- The labelling workspace (docs/lidar/operations/labelling-workspace.md).
--
-- lidar_data_uses is the eligibility ledger: one row per input, per
-- measure family, per decision the eligibility service makes for a gated
-- purpose (tuning, development evaluation, developer diagnostics and
-- annotation review or qualification evaluation), allowed or refused. A
-- capture is named by the SHA-256 of its bytes, so a renamed or copied
-- capture keeps its history. Intervals are sample times in nanoseconds;
-- 0 and 0 is the whole capture. Append-only.
   CREATE TABLE lidar_data_uses (
          use_id TEXT PRIMARY KEY
        , request_id TEXT NOT NULL
        , recorded_at_ns INTEGER NOT NULL
        , purpose TEXT NOT NULL
        , measure TEXT NOT NULL
        , decision TEXT NOT NULL
        , held_out_score INTEGER NOT NULL DEFAULT 0
        , source_kind TEXT NOT NULL
        , capture_sha256 TEXT NOT NULL DEFAULT ''
        , capture_basename TEXT NOT NULL DEFAULT ''
        , sensor_id TEXT NOT NULL DEFAULT ''
        , interval_start_ns INTEGER NOT NULL DEFAULT 0
        , interval_end_ns INTEGER NOT NULL DEFAULT 0
        , cohort TEXT NOT NULL
        , site_id TEXT NOT NULL DEFAULT ''
        , experiment TEXT NOT NULL DEFAULT ''
        , run_id TEXT NOT NULL DEFAULT ''
        , actor TEXT NOT NULL
        , entry_point TEXT NOT NULL
        , build_version TEXT NOT NULL DEFAULT ''
        , build_git_sha TEXT NOT NULL DEFAULT ''
        , config_digest TEXT NOT NULL DEFAULT ''
        , policy_digest TEXT NOT NULL
        , split_digest TEXT NOT NULL DEFAULT ''
        , reasons_json TEXT NOT NULL
        , CHECK (
          purpose IN (
          'tuning'
        , 'development_evaluation'
        , 'developer_diagnostic'
        , 'annotation_review'
        , 'qualification_evaluation'
          )
          )
        , CHECK (measure IN ('identity', 'geometry', 'ground', 'settling'))
        , CHECK (decision IN ('allowed', 'denied'))
        , CHECK (held_out_score IN (0, 1))
        , CHECK (source_kind IN ('capture', 'live'))
        , CHECK (
          cohort IN ('development', 'validation', 'qualification', 'unassigned')
          )
        , CHECK (interval_end_ns >= interval_start_ns)
        , CHECK (
          JSON_VALID(reasons_json)
      AND JSON_TYPE(reasons_json) = 'array'
          )
          );

CREATE INDEX idx_lidar_data_uses_capture ON lidar_data_uses (capture_sha256, measure, decision);

CREATE INDEX idx_lidar_data_uses_time ON lidar_data_uses (recorded_at_ns);

CREATE TRIGGER lidar_data_uses_append_only_update BEFORE
   UPDATE ON lidar_data_uses BEGIN
   SELECT RAISE (ABORT, 'data uses are append-only');

END;

CREATE TRIGGER lidar_data_uses_append_only_delete BEFORE DELETE ON lidar_data_uses BEGIN
   SELECT RAISE (ABORT, 'data uses are append-only');

END;

-- Workflow state for one item (a label set, pack, capture, segment or split)
-- and one progress layer: who has it, what it waits on, and notes. It is
-- workflow, not annotation: it never changes a review status, and the
-- annotation truth stays in the label sets and reference stores. revision
-- is the optimistic-concurrency token a write must name.
   CREATE TABLE lidar_workflow_items (
          item_key TEXT NOT NULL
        , layer TEXT NOT NULL
        , state TEXT NOT NULL
        , assignee TEXT NOT NULL DEFAULT ''
        , reviewer TEXT NOT NULL DEFAULT ''
        , blocker TEXT NOT NULL DEFAULT ''
        , tbd_reason TEXT NOT NULL DEFAULT ''
        , note TEXT NOT NULL DEFAULT ''
        , source_revision TEXT NOT NULL DEFAULT ''
        , revision INTEGER NOT NULL
        , updated_by TEXT NOT NULL
        , updated_at_ns INTEGER NOT NULL
        , created_at_ns INTEGER NOT NULL
        , PRIMARY KEY (item_key, layer)
        , CHECK (item_key != '')
        , CHECK (
          layer IN (
          'purpose'
        , 'spans'
        , 'object_review'
        , 'masks'
        , 'mask_review'
        , 'physical'
        , 'occlusion'
        , 'following'
        , 'freeze'
        , 'evaluation'
        , 'general'
          )
          )
        , CHECK (state IN ('open', 'in_progress', 'blocked', 'tbd', 'done'))
        , CHECK (
          tbd_reason IN (
          ''
        , 'unassigned_purpose'
        , 'missing_reference'
        , 'undecided_criterion'
        , 'unavailable_source'
        , 'missing_required_case'
        , 'other'
          )
          )
        , CHECK (
          state != 'tbd'
       OR tbd_reason != ''
          )
        , CHECK (
          state != 'blocked'
       OR blocker != ''
          )
        , CHECK (revision >= 1)
          );

-- Every saved revision of a workflow item, in order. Append-only.
   CREATE TABLE lidar_workflow_events (
          item_key TEXT NOT NULL
        , layer TEXT NOT NULL
        , revision INTEGER NOT NULL
        , state TEXT NOT NULL
        , assignee TEXT NOT NULL
        , reviewer TEXT NOT NULL
        , blocker TEXT NOT NULL
        , tbd_reason TEXT NOT NULL
        , note TEXT NOT NULL
        , source_revision TEXT NOT NULL
        , updated_by TEXT NOT NULL
        , updated_at_ns INTEGER NOT NULL
        , PRIMARY KEY (item_key, layer, revision)
          );

CREATE TRIGGER lidar_workflow_events_append_only_update BEFORE
   UPDATE ON lidar_workflow_events BEGIN
   SELECT RAISE (ABORT, 'workflow events are append-only');

END;

CREATE TRIGGER lidar_workflow_events_append_only_delete BEFORE DELETE ON lidar_workflow_events BEGIN
   SELECT RAISE (ABORT, 'workflow events are append-only');

END;

-- Which window, by workspace item key, each labelling plan in
-- data/experiments/label is run on. A plan's step progress is derived from
-- the bound item's stores; the binding is the operator's statement and is
-- revision-checked. An empty item_key unbinds.
   CREATE TABLE lidar_plan_bindings (
          plan_id TEXT PRIMARY KEY
        , item_key TEXT NOT NULL
        , note TEXT NOT NULL DEFAULT ''
        , revision INTEGER NOT NULL
        , bound_by TEXT NOT NULL
        , bound_at_ns INTEGER NOT NULL
        , CHECK (plan_id != '')
        , CHECK (revision >= 1)
          );

-- Every saved revision of a plan binding. Append-only.
   CREATE TABLE lidar_plan_binding_events (
          plan_id TEXT NOT NULL
        , revision INTEGER NOT NULL
        , item_key TEXT NOT NULL
        , note TEXT NOT NULL
        , bound_by TEXT NOT NULL
        , bound_at_ns INTEGER NOT NULL
        , PRIMARY KEY (plan_id, revision)
          );

CREATE TRIGGER lidar_plan_binding_events_append_only_update BEFORE
   UPDATE ON lidar_plan_binding_events BEGIN
   SELECT RAISE (ABORT, 'plan binding events are append-only');

END;

CREATE TRIGGER lidar_plan_binding_events_append_only_delete BEFORE DELETE ON lidar_plan_binding_events BEGIN
   SELECT RAISE (ABORT, 'plan binding events are append-only');

END;
