package typst

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// importPattern finds the root-relative files a template pulls in.
var importPattern = regexp.MustCompile(`#(?:import|include)\s+"/([^"]+)"`)

// TestTemplateSetsAreClosedOverImports holds the stated sets to the templates
// themselves: every embedded template belongs to some set, and every file a
// set member imports is in the same set, so a render and its source archive
// never lack a file.
func TestTemplateSetsAreClosedOverImports(t *testing.T) {
	embedded := map[string]bool{}
	if err := fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			embedded[strings.TrimPrefix(path, "templates/")] = false
		}
		return nil
	}); err != nil {
		t.Fatalf("walk templates: %v", err)
	}

	for entry, files := range templateSets {
		if len(files) == 0 || files[0] != entry {
			t.Errorf("set %s must list its entry first, got %v", entry, files)
		}
		inSet := map[string]bool{}
		for _, f := range files {
			inSet[f] = true
			if _, ok := embedded[f]; !ok {
				t.Errorf("set %s names %s, which is not embedded", entry, f)
				continue
			}
			embedded[f] = true
		}
		for _, f := range files {
			body, err := templatesFS.ReadFile("templates/" + f)
			if err != nil {
				continue // reported above
			}
			for _, m := range importPattern.FindAllStringSubmatch(string(body), -1) {
				if !inSet[m[1]] {
					t.Errorf("set %s: %s imports /%s, which is not in the set", entry, f, m[1])
				}
			}
		}
	}
	for name, used := range embedded {
		if !used {
			t.Errorf("embedded template %s belongs to no entry set", name)
		}
	}
}

func TestSourcesForReturnsExactlyTheSet(t *testing.T) {
	for entry, files := range templateSets {
		got, err := SourcesFor(entry)
		if err != nil {
			t.Fatalf("SourcesFor(%s): %v", entry, err)
		}
		var names []string
		for name := range got {
			names = append(names, name)
		}
		sort.Strings(names)
		want := append([]string(nil), files...)
		sort.Strings(want)
		if strings.Join(names, ",") != strings.Join(want, ",") {
			t.Errorf("SourcesFor(%s) = %v, want %v", entry, names, want)
		}
	}
	if _, err := SourcesFor("../escape.typ"); err == nil {
		t.Fatal("SourcesFor should refuse an unregistered entry")
	}
}

func TestRenderRejectsUnknownEntry(t *testing.T) {
	withMockTypstResolver(t, testMetadataFixturePDF(), 0)
	err := Render(&bytes.Buffer{}, Options{Entry: "missing.typ", Data: map[string]any{"ok": true}})
	if err == nil || !strings.Contains(err.Error(), "unknown template entry") {
		t.Fatalf("Render unknown entry error = %v, want unknown template entry", err)
	}
}

// TestRenderBootstrapsTheEntry checks that the document typst compiles is the
// requested entry, and that only its set is materialised beside it.
func TestRenderBootstrapsTheEntry(t *testing.T) {
	restoreRenderDeps(t)
	dir := t.TempDir()
	stdin := filepath.Join(dir, "stdin.typ")
	listing := filepath.Join(dir, "listing.txt")
	script := filepath.Join(dir, "typst")
	// The mock records its stdin and the .typ files in the compile root, the
	// value following --root, then emits a PDF.
	body := "#!/bin/sh\ncat > " + stdin + "\n" +
		"root=\nprev=\nfor a in \"$@\"; do if [ \"$prev\" = \"--root\" ]; then root=\"$a\"; fi; prev=\"$a\"; done\n" +
		"ls \"$root\" | grep '\\.typ$' > " + listing + "\n" +
		"cat <<'EOF'\n" + string(testMetadataFixturePDF()) + "EOF\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write mock typst: %v", err)
	}
	renderResolveTypst = func() (string, func(), error) { return script, func() {}, nil }

	if err := Render(&bytes.Buffer{}, Options{Data: map[string]any{"ok": true}}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got, err := os.ReadFile(stdin)
	if err != nil {
		t.Fatalf("read recorded stdin: %v", err)
	}
	if string(got) != `#include "/report.typ"` {
		t.Fatalf("bootstrap = %q, want the default report entry", got)
	}
	files, err := os.ReadFile(listing)
	if err != nil {
		t.Fatalf("read recorded listing: %v", err)
	}
	want := append([]string(nil), templateSets[EntryReport]...)
	sort.Strings(want)
	if strings.Join(strings.Fields(string(files)), ",") != strings.Join(want, ",") {
		t.Fatalf("materialised templates = %q, want %v", files, want)
	}
}
