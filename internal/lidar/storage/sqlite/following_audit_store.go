package sqlite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// ErrFollowingAuditConflict means an existing evidence binding or record ID has
// different content. A retry cannot revise either; append a new record instead.
var ErrFollowingAuditConflict = errors.New("following audit content conflict")

// ErrFollowingAuditLimit means appending would make the audit too large to reopen
// in the native reviewer. Limits apply independently to each evidence version.
var ErrFollowingAuditLimit = errors.New("following audit size limit")

const followingAuditMaxRecords = 10000

// Leave 64 KiB for the enclosing document's schema, keys and punctuation.
const followingAuditContentBudget = 16*1024*1024 - 64*1024

// FollowingAuditStore retains review records independently per run and canonical
// evidence digest. A newer evidence version never inherits an earlier audit.
type FollowingAuditStore struct{ db DBClient }

func NewFollowingAuditStore(db DBClient) *FollowingAuditStore { return &FollowingAuditStore{db: db} }

// AppendFollowingAudit appends one immutable record atomically with its binding.
// Equivalent JSON retries (including object order and exact numeric spelling)
// return created=false. Arrays retain their order. The caller must validate the
// domain schema and bind the request to its selected canonical evidence.
func (s *FollowingAuditStore) AppendFollowingAudit(runID, digest, recordID string, bindingJSON, recordJSON json.RawMessage) (created bool, err error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(digest) == "" || strings.TrimSpace(recordID) == "" {
		return false, fmt.Errorf("following audit requires run, evidence digest and record ID")
	}
	binding, err := followingAuditJSON(bindingJSON)
	if err != nil {
		return false, fmt.Errorf("binding: %w", err)
	}
	record, err := followingAuditJSON(recordJSON)
	if err != nil {
		return false, fmt.Errorf("record: %w", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Write first: taking the writer lock before reads avoids a deferred
	// transaction's read-to-write upgrade race under concurrent requests.
	if _, err = tx.Exec(`INSERT INTO lidar_following_audit_bindings (run_id,evidence_digest,binding_json) VALUES (?,?,?) ON CONFLICT(run_id,evidence_digest) DO NOTHING`, runID, digest, string(binding)); err != nil {
		return false, err
	}
	var old string
	if err = tx.QueryRow(`SELECT binding_json FROM lidar_following_audit_bindings WHERE run_id=? AND evidence_digest=?`, runID, digest).Scan(&old); err != nil {
		return false, err
	}
	if !followingAuditEquivalent([]byte(old), binding) {
		return false, ErrFollowingAuditConflict
	}
	result, err := tx.Exec(`INSERT INTO lidar_following_audit_records (run_id,evidence_digest,record_id,record_json) VALUES (?,?,?,?) ON CONFLICT(run_id,evidence_digest,record_id) DO NOTHING`, runID, digest, recordID, string(record))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		if err = tx.QueryRow(`SELECT record_json FROM lidar_following_audit_records WHERE run_id=? AND evidence_digest=? AND record_id=?`, runID, digest, recordID).Scan(&old); err != nil {
			return false, err
		}
		if !followingAuditEquivalent([]byte(old), record) {
			return false, ErrFollowingAuditConflict
		}
	}
	if n != 0 {
		var count, total int64
		if err = tx.QueryRow(`SELECT count(*), COALESCE(sum(length(CAST(record_json AS BLOB))),0) FROM lidar_following_audit_records WHERE run_id=? AND evidence_digest=?`, runID, digest).Scan(&count, &total); err != nil {
			return false, err
		}
		// Reserve a comma for every record and include binding bytes. This is
		// conservative, making a successful append safe for the native limit.
		if count > followingAuditMaxRecords || total+count+int64(len(binding)) > followingAuditContentBudget {
			return false, ErrFollowingAuditLimit
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return n != 0, nil
}

// ListFollowingAudit returns a version-isolated history in deterministic record-ID
// order. No records is an empty slice, not a missing run or inherited version.
func (s *FollowingAuditStore) ListFollowingAudit(runID, digest string) ([]json.RawMessage, error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(digest) == "" {
		return nil, fmt.Errorf("following audit requires run and evidence digest")
	}
	rows, err := s.db.Query(`SELECT record_json FROM lidar_following_audit_records WHERE run_id=? AND evidence_digest=? ORDER BY record_id`, runID, digest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(raw))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func followingAuditJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, fmt.Errorf("audit JSON must be an object")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("audit JSON must contain one object")
	}
	return json.Marshal(v)
}

// Keep numeric equality exact, including integers beyond float64 precision.
// A distinct type prevents a JSON number from comparing equal to a string.
type followingAuditNumber string

func followingAuditNormalise(v any) any {
	switch x := v.(type) {
	case json.Number:
		return followingAuditNumeric(x)
	case map[string]any:
		for k, v := range x {
			x[k] = followingAuditNormalise(v)
		}
	case []any:
		for i, v := range x {
			x[i] = followingAuditNormalise(v)
		}
	}
	return v
}
func followingAuditEquivalent(a, b []byte) bool {
	decode := func(raw []byte) any {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil
		}
		return followingAuditNormalise(v)
	}
	return reflect.DeepEqual(decode(a), decode(b))
}

// Canonical scientific notation avoids expanding huge exponents into gigantic
// integers: 1e999999999 is cheap to compare and remains distinct from infinity.
func followingAuditNumeric(v json.Number) followingAuditNumber {
	s := string(v)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign = "-"
		s = s[1:]
	}
	exponent := new(big.Int)
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exponent.SetString(s[i+1:], 10)
		s = s[:i]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		exponent.Sub(exponent, big.NewInt(int64(len(s)-i-1)))
		s = s[:i] + s[i+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	trimmed := strings.TrimRight(s, "0")
	exponent.Add(exponent, big.NewInt(int64(len(s)-len(trimmed))))
	return followingAuditNumber(sign + trimmed + "e" + exponent.String())
}
