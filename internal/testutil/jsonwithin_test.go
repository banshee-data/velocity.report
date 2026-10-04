package testutil

import (
	"strings"
	"testing"
)

func TestJSONWithin(t *testing.T) {
	cases := []struct {
		name, got, want, err string
	}{
		{"identical", `{"a":[1,"x",true,null]}`, `{"a":[1,"x",true,null]}`, ""},
		{"last digit", `{"x":0.16657716470365075}`, `{"x":0.1665771647036507}`, ""},
		{"large number, relative", `{"t":1700000000000000001}`, `{"t":1700000000000000000}`, ""},
		{"number too far", `{"x":1.0}`, `{"x":1.001}`, "$.x: got 1, want 1.001"},
		{"number against string", `{"x":"1"}`, `{"x":1}`, "$.x: got 1, want number 1"},
		{"string differs", `{"s":"a"}`, `{"s":"b"}`, "$.s: got a, want b"},
		{"missing key", `{"a":1,"c":2}`, `{"a":1,"b":2}`, "$.b: missing"},
		{"extra key", `{"a":1,"b":2}`, `{"a":1}`, "$: got 2 keys, want 1"},
		{"object against array", `[]`, `{}`, "$: got []interface {}, want object"},
		{"array length", `[1,2]`, `[1]`, "$: got 2 elements, want 1"},
		{"array element", `[1,[2,3]]`, `[1,[2,4]]`, "$[1][1]: got 3, want 4"},
		{"array against object", `{}`, `[]`, "$: got map[string]interface {}, want array"},
		{"bad got", `{`, `{}`, "got: "},
		{"bad want", `{}`, `{`, "want: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := JSONWithin([]byte(tc.got), []byte(tc.want), 1e-9)
			if tc.err == "" {
				if err != nil {
					t.Fatalf("want agreement, got %v", err)
				}
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.err) {
				t.Fatalf("want error starting %q, got %v", tc.err, err)
			}
		})
	}
}
