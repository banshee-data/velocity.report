package headway

import (
	"encoding"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/typst"
)

// TestReportDataPassesSurfaceAudit: the data file is a behaviour surface, so
// every key it writes is a registered metric id, reason or structural name,
// and no key or value is a metric alias (Section 10.4, "canonical registry
// names across storage, API and report").
func TestReportDataPassesSurfaceAudit(t *testing.T) {
	for _, r := range []Report{oracleReport(t), provisionalReport(t)} {
		rendered, _ := assembled(t, r)
		data, err := typst.MarshalData(rendered)
		if err != nil {
			t.Fatal(err)
		}
		if err := l8behaviour.AuditSurfaceJSON(data); err != nil {
			t.Errorf("%s data.json: %v", r.Status, err)
		}
	}
}

// contractKeys collects every object key the contract can write, following
// struct fields through pointers, slices, arrays and maps. A type that
// marshals itself is a leaf: the vocabularies write a token, not an object.
func contractKeys(rt reflect.Type, seen map[reflect.Type]bool, out map[string]string) {
	for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Array || rt.Kind() == reflect.Map {
		rt = rt.Elem()
	}
	if seen[rt] || rt.Kind() != reflect.Struct {
		return
	}
	seen[rt] = true
	marshaler := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	text := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	if rt.Implements(marshaler) || rt.Implements(text) {
		return
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = rt.Name() + "." + f.Name
		contractKeys(f.Type, seen, out)
	}
}

// TestContractNamesAreRegistered covers the fields the rendered reports
// leave empty or omit, so a new field cannot reach a data file before its
// name is registered.
func TestContractNamesAreRegistered(t *testing.T) {
	keys := map[string]string{}
	contractKeys(reflect.TypeOf(Report{}), map[reflect.Type]bool{}, keys)
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	doc := map[string]int{}
	for _, n := range names {
		doc[n] = 0
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := l8behaviour.AuditSurfaceJSON(raw); err != nil {
		t.Errorf("contract fields fail the surface audit: %v", err)
	}
	if len(names) < 150 {
		t.Errorf("walked %d names; the walk no longer reaches the whole contract", len(names))
	}
}
