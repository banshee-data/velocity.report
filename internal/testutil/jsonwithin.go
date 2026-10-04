package testutil

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// JSONWithin reports whether two JSON documents hold the same values, with
// numbers allowed to differ by a relative tolerance. Golden files of derived
// geometry need it: Go fuses multiply-adds on arm64 but not on amd64, so the
// same code can print a float that differs in its last digit between a Mac
// and CI. It returns nil when the documents agree, and otherwise an error
// naming the first path that does not.
func JSONWithin(got, want []byte, relTol float64) error {
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		return fmt.Errorf("got: %w", err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		return fmt.Errorf("want: %w", err)
	}
	return valueWithin("$", g, w, relTol)
}

func valueWithin(path string, g, w any, relTol float64) error {
	switch wv := w.(type) {
	case float64:
		gv, ok := g.(float64)
		if !ok {
			return fmt.Errorf("%s: got %v, want number %v", path, g, wv)
		}
		scale := math.Max(1, math.Max(math.Abs(gv), math.Abs(wv)))
		if math.Abs(gv-wv) > relTol*scale {
			return fmt.Errorf("%s: got %v, want %v", path, gv, wv)
		}
		return nil
	case map[string]any:
		gv, ok := g.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: got %T, want object", path, g)
		}
		if len(gv) != len(wv) {
			return fmt.Errorf("%s: got %d keys, want %d", path, len(gv), len(wv))
		}
		keys := make([]string, 0, len(wv))
		for k := range wv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			gk, ok := gv[k]
			if !ok {
				return fmt.Errorf("%s.%s: missing", path, k)
			}
			if err := valueWithin(path+"."+k, gk, wv[k], relTol); err != nil {
				return err
			}
		}
		return nil
	case []any:
		gv, ok := g.([]any)
		if !ok {
			return fmt.Errorf("%s: got %T, want array", path, g)
		}
		if len(gv) != len(wv) {
			return fmt.Errorf("%s: got %d elements, want %d", path, len(gv), len(wv))
		}
		for i := range wv {
			if err := valueWithin(fmt.Sprintf("%s[%d]", path, i), gv[i], wv[i], relTol); err != nil {
				return err
			}
		}
		return nil
	default:
		if g != w {
			return fmt.Errorf("%s: got %v, want %v", path, g, w)
		}
		return nil
	}
}
