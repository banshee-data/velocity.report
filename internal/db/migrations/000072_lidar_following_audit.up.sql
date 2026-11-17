-- Append-only operator review of one run's canonical following evidence.
   CREATE TABLE lidar_following_audit_bindings (
          run_id TEXT NOT NULL CHECK (LENGTH(TRIM(run_id)) > 0)
        , evidence_digest TEXT NOT NULL CHECK (LENGTH(TRIM(evidence_digest)) > 0)
        , binding_json TEXT NOT NULL CHECK (
          JSON_VALID(binding_json)
      AND JSON_TYPE(binding_json) = 'object'
          )
        , PRIMARY KEY (run_id, evidence_digest)
          );

   CREATE TABLE lidar_following_audit_records (
          run_id TEXT NOT NULL
        , evidence_digest TEXT NOT NULL
        , record_id TEXT NOT NULL CHECK (LENGTH(TRIM(record_id)) > 0)
        , record_json TEXT NOT NULL CHECK (
          JSON_VALID(record_json)
      AND JSON_TYPE(record_json) = 'object'
          )
        , PRIMARY KEY (run_id, evidence_digest, record_id)
        , FOREIGN KEY (run_id, evidence_digest) REFERENCES lidar_following_audit_bindings (run_id, evidence_digest)
          );

CREATE TRIGGER lidar_following_audit_bindings_no_update BEFORE
   UPDATE ON lidar_following_audit_bindings BEGIN
   SELECT RAISE (ABORT, 'following audit bindings are append-only');

END;

CREATE TRIGGER lidar_following_audit_bindings_no_delete BEFORE DELETE ON lidar_following_audit_bindings BEGIN
   SELECT RAISE (ABORT, 'following audit bindings are append-only');

END;

CREATE TRIGGER lidar_following_audit_records_no_update BEFORE
   UPDATE ON lidar_following_audit_records BEGIN
   SELECT RAISE (ABORT, 'following audit records are append-only');

END;

CREATE TRIGGER lidar_following_audit_records_no_delete BEFORE DELETE ON lidar_following_audit_records BEGIN
   SELECT RAISE (ABORT, 'following audit records are append-only');

END;
