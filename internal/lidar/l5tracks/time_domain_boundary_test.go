package l5tracks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// wallClockReads are the time-package functions that consult the host's
// clock, or schedule against it. Pure arithmetic and constructors on
// time.Time and time.Duration (Unix, UnixNano, Add, Sub, Duration) stay
// allowed: they compute on timestamps the caller supplied.
var wallClockReads = map[string]bool{
	"Now": true, "Since": true, "Until": true,
	"Sleep": true, "After": true, "AfterFunc": true, "Tick": true,
	"NewTimer": true, "NewTicker": true,
}

// forbiddenClockImports would hand the estimator a clock of its own. The
// timeutil.Clock abstraction exists for runtime code (throttles, pacers,
// cleanup timers) and is exactly the wrong thing to give a tracker whose
// only legitimate source of elapsed time is the capture timestamps it is fed.
var forbiddenClockImports = map[string]bool{
	"github.com/banshee-data/velocity.report/internal/timeutil": true,
}

// scanForWallClock parses every non-test Go source in dir and returns each
// wall-clock read or clock import it finds, as "file:line: what".
func scanForWallClock(dir string) (violations []string, scanned int, err error) {
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, 0, err
	}
	fset := token.NewFileSet()
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, scanned, err
		}
		scanned++
		timeNames := map[string]bool{}
		for _, spec := range file.Imports {
			importPath, _ := strconv.Unquote(spec.Path.Value)
			if forbiddenClockImports[importPath] {
				violations = append(violations, fset.Position(spec.Pos()).String()+": imports "+importPath)
			}
			if importPath != "time" {
				continue
			}
			switch {
			case spec.Name == nil:
				timeNames["time"] = true
			case spec.Name.Name == "_":
			case spec.Name.Name == ".":
				violations = append(violations, fset.Position(spec.Pos()).String()+": dot-imports time, which hides clock reads from this check")
			default:
				timeNames[spec.Name.Name] = true
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && timeNames[pkg.Name] && wallClockReads[sel.Sel.Name] {
				violations = append(violations, fset.Position(sel.Pos()).String()+": "+pkg.Name+"."+sel.Sel.Name)
			}
			return true
		})
	}
	sort.Strings(violations)
	return violations, scanned, nil
}

// TestL5TracksDoesNotReadTheWallClock holds the time-domain boundary in
// time_domain.go. Replay equivalence, the property that the same capture fed
// at any pace yields the same tracks, holds only while nothing in this
// package asks the host what time it is. A single time.Now() in a coast rule
// or a smoother would make every replay pacing-dependent, and no functional
// test on synthetic timestamps would notice. So the rule is checked where it
// can break: in the source.
func TestL5TracksDoesNotReadTheWallClock(t *testing.T) {
	violations, scanned, err := scanForWallClock(".")
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("scanned no non-test sources; the check is not looking at the package")
	}
	if len(violations) > 0 {
		t.Fatalf("l5tracks must take elapsed time only from capture timestamps (see time_domain.go); wall-clock use found:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// The scan must be able to fail. Run it over synthetic sources carrying each
// shape of violation, so a refactor of the walker cannot quietly turn it into
// a check that passes everything. Test files are exempt by design.
func TestWallClockScanDetectsViolations(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("aliased.go", `package probe

import clock "time"

func f() int64 {
	start := clock.Now()
	clock.Sleep(clock.Since(start))
	return clock.Unix(0, 1).UnixNano()
}
`)
	write("dot.go", `package probe

import . "time"

var _ = Second
`)
	write("injected.go", `package probe

import "github.com/banshee-data/velocity.report/internal/timeutil"

var _ timeutil.Clock
`)
	write("arithmetic.go", `package probe

import "time"

func g(ts time.Time) time.Duration { return ts.Sub(time.Unix(0, 0)) + time.Second }
`)
	write("exempt_test.go", `package probe

import "time"

var _ = time.Now()
`)

	violations, scanned, err := scanForWallClock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if scanned != 4 {
		t.Fatalf("scanned %d sources, want 4: test files must be skipped and every other file read", scanned)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{"clock.Now", "clock.Sleep", "clock.Since", "dot-imports time", "imports github.com/banshee-data/velocity.report/internal/timeutil"} {
		if !strings.Contains(joined, want) {
			t.Errorf("scan missed %q; found:\n%s", want, joined)
		}
	}
	for _, v := range violations {
		if strings.Contains(v, "arithmetic.go") || strings.Contains(v, "exempt_test.go") {
			t.Errorf("scan flagged capture-time arithmetic or a test file: %s", v)
		}
	}
}
