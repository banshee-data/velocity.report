package configasset

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/version"
	"github.com/google/uuid"
)

const (
	ParamSetTypeRequested = "requested"
	ParamSetTypeEffective = "effective"
	ParamSetTypeLegacy    = "legacy"

	SchemaVersionRequestedV1 = "requested/v1"
	SchemaVersionEffectiveV1 = "effective/v1"
	SchemaVersionLegacyV1    = "legacy/v1"
	SchemaVersionRunConfigV1 = "run_config/v1"

	unknownBuildValue = "unknown"
)

type DBClient interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

type ParamSet struct {
	ParamSetID    string
	ParamsHash    string
	SchemaVersion string
	ParamSetType  string
	ParamsJSON    []byte
	CreatedAt     int64
}

type BuildIdentity struct {
	BuildVersion string
	BuildGitSHA  string
}

type RunConfig struct {
	RunConfigID          string
	ConfigHash           string
	ParamSetID           string
	BuildVersion         string
	BuildGitSHA          string
	CreatedAt            int64
	ComposedJSON         []byte
	ParamSetType         string
	ParamSchemaVersion   string
	ConfigSchemaVersion  string
	ParamsHash           string
	ParamSetEnvelopeJSON []byte
}

type Store struct {
	db DBClient
}

type storedParamSetJSON struct {
	SchemaVersion string          `json:"schema_version"`
	ParamSetType  string          `json:"param_set_type"`
	Params        json.RawMessage `json:"params"`
}

type runConfigJSON struct {
	SchemaVersion string          `json:"schema_version"`
	ParamSetType  string          `json:"param_set_type"`
	Build         buildJSON       `json:"build"`
	Params        json.RawMessage `json:"params"`
}

type buildJSON struct {
	BuildVersion string `json:"build_version"`
	BuildGitSHA  string `json:"build_git_sha"`
}

type paramSetEnvelope struct {
	SchemaVersion string      `json:"schema_version"`
	ParamSetType  string      `json:"param_set_type"`
	Params        interface{} `json:"params"`
}

func NewStore(db DBClient) *Store {
	return &Store{db: db}
}

func ReadBuildIdentity() BuildIdentity {
	return BuildIdentity{
		BuildVersion: normalizeBuildValue(version.Version),
		BuildGitSHA:  normalizeBuildValue(version.GitSHA),
	}
}

func MakeEffectiveParamSet(cfg *cfgpkg.TuningConfig) (*ParamSet, error) {
	if cfg == nil {
		return nil, fmt.Errorf("effective tuning config is required")
	}

	envelope := paramSetEnvelope{
		SchemaVersion: SchemaVersionEffectiveV1,
		ParamSetType:  ParamSetTypeEffective,
		Params:        cfg,
	}
	return makeParamSet(envelope)
}

func MakeRequestedParamSet(raw json.RawMessage) (*ParamSet, error) {
	return makeWrappedRawParamSet(raw, SchemaVersionRequestedV1, ParamSetTypeRequested)
}

func MakeLegacyParamSet(raw json.RawMessage) (*ParamSet, error) {
	return makeWrappedRawParamSet(raw, SchemaVersionLegacyV1, ParamSetTypeLegacy)
}

func ComposeRunConfig(paramSet *ParamSet, build BuildIdentity) ([]byte, error) {
	if paramSet == nil {
		return nil, fmt.Errorf("param set is required")
	}
	if len(paramSet.ParamsJSON) == 0 {
		return nil, fmt.Errorf("param set JSON is required")
	}

	var stored storedParamSetJSON
	if err := json.Unmarshal(paramSet.ParamsJSON, &stored); err != nil {
		return nil, fmt.Errorf("decode param set JSON: %w", err)
	}

	composed := runConfigJSON{
		SchemaVersion: SchemaVersionRunConfigV1,
		ParamSetType:  paramSet.ParamSetType,
		Build: buildJSON{
			BuildVersion: normalizeBuildValue(build.BuildVersion),
			BuildGitSHA:  normalizeBuildValue(build.BuildGitSHA),
		},
		Params: stored.Params,
	}

	data, err := json.Marshal(composed)
	if err != nil {
		return nil, fmt.Errorf("marshal run config JSON: %w", err)
	}
	return data, nil
}

func HashJSON(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (s *Store) EnsureParamSet(paramSet *ParamSet) (*ParamSet, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("config asset store requires a database")
	}
	if paramSet == nil {
		return nil, fmt.Errorf("param set is required")
	}

	if existing, err := s.findParamSetByHash(paramSet.ParamsHash); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	if paramSet.ParamSetID == "" {
		paramSet.ParamSetID = uuid.NewString()
	}
	if paramSet.CreatedAt == 0 {
		paramSet.CreatedAt = time.Now().UnixNano()
	}

	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO lidar_param_sets (
			param_set_id, params_hash, schema_version, param_set_type, params_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		paramSet.ParamSetID,
		paramSet.ParamsHash,
		paramSet.SchemaVersion,
		paramSet.ParamSetType,
		string(paramSet.ParamsJSON),
		paramSet.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert lidar_param_sets: %w", err)
	}

	return s.findParamSetByHash(paramSet.ParamsHash)
}

func (s *Store) EnsureRunConfig(paramSet *ParamSet, build BuildIdentity) (*RunConfig, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("config asset store requires a database")
	}

	build = BuildIdentity{
		BuildVersion: normalizeBuildValue(build.BuildVersion),
		BuildGitSHA:  normalizeBuildValue(build.BuildGitSHA),
	}

	ensuredParamSet, err := s.EnsureParamSet(paramSet)
	if err != nil {
		return nil, err
	}

	composedJSON, err := ComposeRunConfig(ensuredParamSet, build)
	if err != nil {
		return nil, err
	}
	configHash := HashJSON(composedJSON)

	if existing, err := s.findRunConfigByHash(configHash); err == nil {
		existing.ComposedJSON = composedJSON
		return existing, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	runConfig := &RunConfig{
		RunConfigID:          uuid.NewString(),
		ConfigHash:           configHash,
		ParamSetID:           ensuredParamSet.ParamSetID,
		BuildVersion:         build.BuildVersion,
		BuildGitSHA:          build.BuildGitSHA,
		CreatedAt:            time.Now().UnixNano(),
		ComposedJSON:         composedJSON,
		ParamSetType:         ensuredParamSet.ParamSetType,
		ParamSchemaVersion:   ensuredParamSet.SchemaVersion,
		ConfigSchemaVersion:  SchemaVersionRunConfigV1,
		ParamsHash:           ensuredParamSet.ParamsHash,
		ParamSetEnvelopeJSON: append([]byte(nil), ensuredParamSet.ParamsJSON...),
	}

	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO lidar_run_configs (
			run_config_id, config_hash, param_set_id, build_version, build_git_sha, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		runConfig.RunConfigID,
		runConfig.ConfigHash,
		runConfig.ParamSetID,
		runConfig.BuildVersion,
		runConfig.BuildGitSHA,
		runConfig.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert lidar_run_configs: %w", err)
	}

	stored, err := s.findRunConfigByHash(configHash)
	if err != nil {
		return nil, err
	}
	stored.ComposedJSON = composedJSON
	return stored, nil
}

func (s *Store) GetParamSet(paramSetID string) (*ParamSet, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("config asset store requires a database")
	}

	var paramSet ParamSet
	var paramsJSON string
	err := s.db.QueryRow(`
		SELECT param_set_id, params_hash, schema_version, param_set_type, params_json, created_at
		FROM lidar_param_sets
		WHERE param_set_id = ?`,
		paramSetID,
	).Scan(
		&paramSet.ParamSetID,
		&paramSet.ParamsHash,
		&paramSet.SchemaVersion,
		&paramSet.ParamSetType,
		&paramsJSON,
		&paramSet.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	paramSet.ParamsJSON = []byte(paramsJSON)
	return &paramSet, nil
}

func (s *Store) GetRunConfig(runConfigID string) (*RunConfig, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("config asset store requires a database")
	}

	var (
		runConfig  RunConfig
		paramsJSON string
	)
	err := s.db.QueryRow(`
		SELECT
			rc.run_config_id,
			rc.config_hash,
			rc.param_set_id,
			rc.build_version,
			rc.build_git_sha,
			rc.created_at,
			ps.params_hash,
			ps.schema_version,
			ps.param_set_type,
			ps.params_json
		FROM lidar_run_configs rc
		JOIN lidar_param_sets ps ON ps.param_set_id = rc.param_set_id
		WHERE rc.run_config_id = ?`,
		runConfigID,
	).Scan(
		&runConfig.RunConfigID,
		&runConfig.ConfigHash,
		&runConfig.ParamSetID,
		&runConfig.BuildVersion,
		&runConfig.BuildGitSHA,
		&runConfig.CreatedAt,
		&runConfig.ParamsHash,
		&runConfig.ParamSchemaVersion,
		&runConfig.ParamSetType,
		&paramsJSON,
	)
	if err != nil {
		return nil, err
	}

	runConfig.ConfigSchemaVersion = SchemaVersionRunConfigV1
	runConfig.ParamSetEnvelopeJSON = []byte(paramsJSON)

	paramSet := &ParamSet{
		ParamSetID:    runConfig.ParamSetID,
		ParamsHash:    runConfig.ParamsHash,
		SchemaVersion: runConfig.ParamSchemaVersion,
		ParamSetType:  runConfig.ParamSetType,
		ParamsJSON:    runConfig.ParamSetEnvelopeJSON,
	}
	composedJSON, err := ComposeRunConfig(paramSet, BuildIdentity{
		BuildVersion: runConfig.BuildVersion,
		BuildGitSHA:  runConfig.BuildGitSHA,
	})
	if err != nil {
		return nil, err
	}
	runConfig.ComposedJSON = composedJSON
	return &runConfig, nil
}

func makeWrappedRawParamSet(raw json.RawMessage, schemaVersion, paramSetType string) (*ParamSet, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("param set JSON is required")
	}

	params, err := decodeJSONObject(raw)
	if err != nil {
		return nil, err
	}

	envelope := paramSetEnvelope{
		SchemaVersion: schemaVersion,
		ParamSetType:  paramSetType,
		Params:        params,
	}
	return makeParamSet(envelope)
}

func ExtractParamsPayload(data []byte) (json.RawMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("param set JSON is required")
	}

	var stored storedParamSetJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("decode param set JSON: %w", err)
	}
	if len(stored.Params) == 0 {
		return nil, fmt.Errorf("param set payload is empty")
	}
	return append(json.RawMessage(nil), stored.Params...), nil
}

func decodeJSONObject(raw json.RawMessage) (map[string]interface{}, error) {
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode param set JSON: %w", err)
	}

	if encodedString, ok := decoded.(string); ok {
		inner := strings.TrimSpace(encodedString)
		if inner == "" {
			return nil, fmt.Errorf("param set JSON string is empty")
		}
		if err := json.Unmarshal([]byte(inner), &decoded); err != nil {
			return nil, fmt.Errorf("decode stringified param set JSON: %w", err)
		}
	}

	params, ok := decoded.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("param set JSON must decode to an object")
	}
	// Strip non-deterministic keys so legacy blobs with timestamps
	// hash identically to blobs without them.
	delete(params, "timestamp")
	return params, nil
}

func makeParamSet(envelope paramSetEnvelope) (*ParamSet, error) {
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal param set JSON: %w", err)
	}

	return &ParamSet{
		ParamsHash:    HashJSON(data),
		SchemaVersion: envelope.SchemaVersion,
		ParamSetType:  envelope.ParamSetType,
		ParamsJSON:    data,
	}, nil
}

func (s *Store) findParamSetByHash(paramsHash string) (*ParamSet, error) {
	var paramSet ParamSet
	var paramsJSON string
	err := s.db.QueryRow(`
		SELECT param_set_id, params_hash, schema_version, param_set_type, params_json, created_at
		FROM lidar_param_sets
		WHERE params_hash = ?`,
		paramsHash,
	).Scan(
		&paramSet.ParamSetID,
		&paramSet.ParamsHash,
		&paramSet.SchemaVersion,
		&paramSet.ParamSetType,
		&paramsJSON,
		&paramSet.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	paramSet.ParamsJSON = []byte(paramsJSON)
	return &paramSet, nil
}

func (s *Store) findRunConfigByHash(configHash string) (*RunConfig, error) {
	var runConfig RunConfig
	err := s.db.QueryRow(`
		SELECT run_config_id, config_hash, param_set_id, build_version, build_git_sha, created_at
		FROM lidar_run_configs
		WHERE config_hash = ?`,
		configHash,
	).Scan(
		&runConfig.RunConfigID,
		&runConfig.ConfigHash,
		&runConfig.ParamSetID,
		&runConfig.BuildVersion,
		&runConfig.BuildGitSHA,
		&runConfig.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	runConfig.ConfigSchemaVersion = SchemaVersionRunConfigV1

	paramSet, err := s.GetParamSet(runConfig.ParamSetID)
	if err == nil {
		runConfig.ParamsHash = paramSet.ParamsHash
		runConfig.ParamSchemaVersion = paramSet.SchemaVersion
		runConfig.ParamSetType = paramSet.ParamSetType
		runConfig.ParamSetEnvelopeJSON = append([]byte(nil), paramSet.ParamsJSON...)
	}
	return &runConfig, nil
}

func normalizeBuildValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return unknownBuildValue
	}
	return value
}
