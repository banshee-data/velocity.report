package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// UnmarshalJSON enforces strict engine-block validation for L3.
func (c *L3Config) UnmarshalJSON(data []byte) error {
	raw, err := parseObject(data, "l3")
	if err != nil {
		return err
	}
	if err := ensureAllowedKeys(raw, "l3", []string{"engine", "ema_baseline_v1", "ema_track_assist_v2"}); err != nil {
		return err
	}
	engine, err := requiredEngine(raw, "l3")
	if err != nil {
		return err
	}
	spec, ok := engineRegistry[engine]
	if !ok || spec.Layer != "l3" {
		return fmt.Errorf("l3: unknown engine %q", engine)
	}

	c.Engine = engine
	switch engine {
	case "ema_baseline_v1":
		block, err := decodeSelectedEngineBlock[L3EmaBaselineV1](raw, "l3", engine)
		if err != nil {
			return err
		}
		c.EmaBaselineV1 = block
	case "ema_track_assist_v2":
		block, err := decodeSelectedEngineBlock[L3EmaTrackAssistV2](raw, "l3", engine)
		if err != nil {
			return err
		}
		c.EmaTrackAssistV2 = block
	}
	return nil
}

// UnmarshalJSON enforces strict engine-block validation for L4.
func (c *L4Config) UnmarshalJSON(data []byte) error {
	raw, err := parseObject(data, "l4")
	if err != nil {
		return err
	}
	if err := ensureAllowedKeys(raw, "l4", []string{"engine", "dbscan_xy_v1", "two_stage_mahalanobis_v2", "hdbscan_adaptive_v1"}); err != nil {
		return err
	}
	engine, err := requiredEngine(raw, "l4")
	if err != nil {
		return err
	}
	spec, ok := engineRegistry[engine]
	if !ok || spec.Layer != "l4" {
		return fmt.Errorf("l4: unknown engine %q", engine)
	}

	c.Engine = engine
	switch engine {
	case "dbscan_xy_v1":
		block, err := decodeSelectedEngineBlock[L4DbscanXyV1](raw, "l4", engine)
		if err != nil {
			return err
		}
		c.DbscanXyV1 = block
	case "two_stage_mahalanobis_v2":
		block, err := decodeSelectedEngineBlock[L4TwoStageMahalanobisV2](raw, "l4", engine)
		if err != nil {
			return err
		}
		c.TwoStageMahalanobisV2 = block
	case "hdbscan_adaptive_v1":
		block, err := decodeSelectedEngineBlock[L4HdbscanAdaptiveV1](raw, "l4", engine)
		if err != nil {
			return err
		}
		c.HdbscanAdaptiveV1 = block
	}
	return nil
}

// UnmarshalJSON enforces strict engine-block validation for L5.
func (c *L5Config) UnmarshalJSON(data []byte) error {
	raw, err := parseObject(data, "l5")
	if err != nil {
		return err
	}
	if err := ensureAllowedKeys(raw, "l5", []string{"engine", "cv_kf_v1", "imm_cv_ca_v2", "imm_cv_ca_rts_eval_v2"}); err != nil {
		return err
	}
	engine, err := requiredEngine(raw, "l5")
	if err != nil {
		return err
	}
	spec, ok := engineRegistry[engine]
	if !ok || spec.Layer != "l5" {
		return fmt.Errorf("l5: unknown engine %q", engine)
	}

	c.Engine = engine
	switch engine {
	case "cv_kf_v1":
		block, err := decodeSelectedEngineBlock[L5CvKfV1](raw, "l5", engine)
		if err != nil {
			return err
		}
		c.CvKfV1 = block
	case "imm_cv_ca_v2":
		block, err := decodeSelectedEngineBlock[L5ImmCvCaV2](raw, "l5", engine)
		if err != nil {
			return err
		}
		c.ImmCvCaV2 = block
	case "imm_cv_ca_rts_eval_v2":
		block, err := decodeSelectedEngineBlock[L5ImmCvCaRtsEvalV2](raw, "l5", engine)
		if err != nil {
			return err
		}
		c.ImmCvCaRtsEvalV2 = block
	}
	return nil
}

func parseObject(data []byte, path string) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: expected object: %w", path, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("%s: expected object", path)
	}
	return raw, nil
}

func requiredEngine(raw map[string]json.RawMessage, path string) (string, error) {
	engineRaw, ok := raw["engine"]
	if !ok {
		return "", fmt.Errorf("%s: missing required key: engine", path)
	}
	var engine string
	if err := json.Unmarshal(engineRaw, &engine); err != nil {
		return "", fmt.Errorf("%s.engine: expected string: %w", path, err)
	}
	if strings.TrimSpace(engine) == "" {
		return "", fmt.Errorf("%s.engine: must be non-empty", path)
	}
	return engine, nil
}

func ensureAllowedKeys(raw map[string]json.RawMessage, path string, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	var unknown []string
	for key := range raw {
		if _, ok := allowedSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%s: unknown keys: %s", path, strings.Join(unknown, ", "))
}

func decodeSelectedEngineBlock[T any](raw map[string]json.RawMessage, path, engine string) (*T, error) {
	for key := range raw {
		if key == "engine" {
			continue
		}
		if key != engine {
			return nil, fmt.Errorf("%s: non-selected engine block %q present while engine=%q", path, key, engine)
		}
	}

	blockRaw, ok := raw[engine]
	if !ok {
		return nil, fmt.Errorf("%s: selected engine block %q missing", path, engine)
	}

	var block T
	if err := strictDecodeObject(blockRaw, &block, path+"."+engine); err != nil {
		return nil, err
	}
	return &block, nil
}

func strictDecodeObject(data []byte, dst interface{}, path string) error {
	raw, err := parseObject(data, path)
	if err != nil {
		return err
	}

	expected := expectedJSONKeys(reflect.TypeOf(dst))
	expectedSet := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		expectedSet[key] = struct{}{}
	}

	var unknown []string
	for key := range raw {
		if _, ok := expectedSet[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%s: unknown keys: %s", path, strings.Join(unknown, ", "))
	}

	var missing []string
	for _, key := range expected {
		if _, ok := raw[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%s: missing required keys: %s", path, strings.Join(missing, ", "))
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func expectedJSONKeys(t reflect.Type) []string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}

	var keys []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name, tagged := jsonFieldName(field)
		if name == "-" {
			continue
		}
		if field.Anonymous && !tagged {
			keys = append(keys, expectedJSONKeys(field.Type)...)
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name, false
	}
	if tag == "-" {
		return "-", true
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		return field.Name, true
	}
	return parts[0], true
}
