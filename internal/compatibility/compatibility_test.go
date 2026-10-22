package compatibility

import (
	"bytes"
	"flag"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReadRegister(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		wantError  bool
	}{
		{"valid", "## Register\n| Surviving name | Where | Who | Treatment | Remove in |\n| --- | --- | --- | --- | --- |\n| `old` | here | app | alias | v1.2.3 |\n## Other\n| ignored |", false},
		{"no table", "## Other", true},
		{"bad row", "## Register\n| bad |", true},
		{"no removal", "## Register\n| `old` | here | app | alias | |", true},
		{"no name", "## Register\n| | here | app | alias | kept |", true},
		{"bad version", "## Register\n| `old` | here | app | alias | v1.2.b |", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := ReadRegister(tc.text)
			if (err != nil) != tc.wantError {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
		})
	}
}

func TestExpiry(t *testing.T) {
	entries := []Entry{{Name: "event", RemoveIn: "next DMG"}, {Name: "old", RemoveIn: "v0.5.9"}}
	for _, tc := range []struct {
		version string
		expired bool
	}{
		{"0.5.8", false}, {"v0.5.8-pre1", false}, {"0.5.9", true}, {"0.5.9-pre1", true},
		{"0.5.10", true}, {"0.6.0", true}, {"1.0.0", true}, {"broken", true},
	} {
		if err := CheckExpiry(entries, tc.version); (err != nil) != tc.expired {
			t.Errorf("%s: %v", tc.version, err)
		}
	}
	if err := CheckExpiry(entries[:1], "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if !Registered(entries, "missing") && Registered([]Entry{{Name: "`old` and `older`"}}, "old") && !Registered(entries, "ol") {
		return
	}
	t.Fatal("registration must match complete code spellings")
}

func TestTombstoneFlagRefusesWork(t *testing.T) {
	for _, args := range [][]string{{"-tuning", "old.json"}, {"-tuning=old.json"}, {"-config", "new.json", "-tuning=old.json"}, {"-tuning="}} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var out bytes.Buffer
		fs.SetOutput(&out)
		current := fs.String("config", "default.json", "")
		TombstoneFlag(fs, "tuning", "-config")
		if err := fs.Parse(args); err == nil || !strings.Contains(err.Error(), "use -config") {
			t.Fatalf("%v: %v", args, err)
		}
		if *current == "old.json" {
			t.Fatal("tombstone changed the live config")
		}
	}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	current := fs.String("config", "", "")
	TombstoneFlag(fs, "tuning", "-config")
	if err := fs.Parse([]string{"-config=new.json"}); err != nil || *current != "new.json" {
		t.Fatalf("canonical flag: %v", err)
	}
	if AliasRoute("old", "new") != "old" {
		t.Fatal("alias pattern changed")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func register(t *testing.T) []Entry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "docs/platform/operations/compatibility-register.md"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ReadRegister(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestRegisterExpiry(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^VERSION\s*:=\s*(\S+)`).FindStringSubmatch(string(data))
	if len(match) != 2 {
		t.Fatal("Makefile VERSION not found")
	}
	if err := CheckExpiry(register(t), match[1]); err != nil {
		t.Fatal(err)
	}
}

// Inspect declarations, rather than maintain a second manually synchronised list.
// Repeated handlers in the route table must declare their canonical pattern.
func TestRouteAliasesRegistered(t *testing.T) {
	entries := register(t)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(repositoryRoot(t), "internal/lidar/server/routes.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	type declaration struct{ pattern, target, handler string }
	var routes []declaration
	source := func(n ast.Node) string { var b bytes.Buffer; _ = printer.Fprint(&b, fset, n); return b.String() }
	literal := func(e ast.Expr) string {
		s, ok := e.(*ast.BasicLit)
		if !ok {
			return ""
		}
		return strings.Trim(s.Value, `"`)
	}
	add := func(pattern, handler ast.Expr) {
		d := declaration{handler: source(handler)}
		if call, ok := pattern.(*ast.CallExpr); ok && source(call.Fun) == "compatibility.AliasRoute" {
			if len(call.Args) != 2 {
				t.Fatal("alias needs old and canonical patterns")
			}
			d.pattern, d.target = literal(call.Args[0]), literal(call.Args[1])
			path := d.pattern
			if i := strings.IndexByte(path, ' '); i >= 0 {
				path = path[i+1:]
			}
			if !Registered(entries, path) {
				t.Errorf("unregistered route alias %s", d.pattern)
			}
		} else {
			d.pattern = literal(pattern)
		}
		if strings.Contains(d.pattern, "/api/") {
			routes = append(routes, d)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CompositeLit:
			// All route groups use []route literals with unkeyed two-field entries.
			if a, ok := v.Type.(*ast.ArrayType); ok && source(a.Elt) == "route" {
				for _, e := range v.Elts {
					r := e.(*ast.CompositeLit)
					add(r.Elts[0], r.Elts[1])
				}
			}
		case *ast.CallExpr:
			if source(v.Fun) == "mux.HandleFunc" && len(v.Args) == 2 {
				add(v.Args[0], v.Args[1])
			}
		}
		return true
	})
	handlers := map[string]string{}
	for _, r := range routes {
		if r.target == "" {
			if first, ok := handlers[r.handler]; ok {
				t.Errorf("%s repeats handler for %s without AliasRoute", r.pattern, first)
			}
			handlers[r.handler] = r.pattern
		}
	}
	for _, r := range routes {
		if r.target != "" && handlers[r.handler] != r.target {
			t.Errorf("alias %s targets %s, handler belongs to %s", r.pattern, r.target, handlers[r.handler])
		}
	}
}

func TestFlagTombstonesRegistered(t *testing.T) {
	entries := register(t)
	count := 0
	err := filepath.WalkDir(filepath.Join(repositoryRoot(t), "internal/cmd"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "TombstoneFlag" {
				return true
			}
			count++
			name, ok := c.Args[1].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: tombstone name must be literal", path)
				return true
			}
			if !Registered(entries, "-"+strings.Trim(name.Value, `"`)) {
				t.Errorf("%s: unregistered flag %s", path, name.Value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no tombstone declarations found")
	}
}

func TestClipDMGNamesHaveRegisterRows(t *testing.T) {
	entries := register(t)
	for _, name := range []string{"replay_case_id", "replay_case_name", "pcap", "/lidar/replay-cases"} {
		if !Registered(entries, name) {
			t.Errorf("unregistered clip compatibility name %s", name)
		}
	}
}
