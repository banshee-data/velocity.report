package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	radar "github.com/banshee-data/velocity.report"
)

// TestWebBuildEmbedKeepsUnderscoreFiles pins the embed directive behind
// radar.WebBuildFiles. A plain directory pattern (web/build or web/build/*)
// makes Go skip every file below web/build whose name starts with "_" or
// ".", and SvelteKit can give a chunk a content-hash name that starts with
// "_". No committed build contains such a file to prove the point at run
// time, so the directive itself is what is checked.
func TestWebBuildEmbedKeepsUnderscoreFiles(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	path := filepath.Join(root, "assets.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var directives []string
	found := false
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR || gen.Doc == nil {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				if name.Name != "WebBuildFiles" {
					continue
				}
				found = true
				for _, c := range gen.Doc.List {
					if pattern, ok := strings.CutPrefix(c.Text, "//go:embed "); ok {
						directives = append(directives, strings.TrimSpace(pattern))
					}
				}
			}
		}
	}

	if !found {
		t.Fatalf("%s declares no WebBuildFiles", path)
	}
	if len(directives) != 1 || directives[0] != "all:web/build" {
		t.Fatalf("WebBuildFiles embeds %q, want exactly %q: without all:, "+
			"_-prefixed chunks below web/build are left out of the binary", directives, "all:web/build")
	}
}

// TestWebBuildEmbedMatchesDisk checks that the binary carries every file of
// the web build it was compiled with. In CI's integration job that is a real
// SvelteKit build; elsewhere it may be only the stub index.html.
func TestWebBuildEmbedMatchesDisk(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	buildDir := filepath.Join(root, "web", "build")
	if _, err := os.Stat(buildDir); err != nil {
		t.Skipf("no web build on disk: %v", err)
	}

	checked := 0
	err = filepath.WalkDir(buildDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(buildDir, path)
		if err != nil {
			return err
		}
		embedPath := "web/build/" + filepath.ToSlash(rel)
		if _, err := radar.WebBuildFiles.ReadFile(embedPath); err != nil {
			t.Errorf("%s is on disk but not embedded: %v", embedPath, err)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", buildDir, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no files; the embed needs at least index.html", buildDir)
	}
}
