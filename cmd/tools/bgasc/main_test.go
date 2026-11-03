package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyFileWritesTheSameBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.asc")
	dst := filepath.Join(dir, "out", "site.asc")
	if err := os.WriteFile(src, []byte("# Exported points\n1 2 3 0 3.7 300\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Exported points\n1 2 3 0 3.7 300\n" {
		t.Fatalf("copied %q", got)
	}
	if err := copyFile(filepath.Join(dir, "missing.asc"), dst); err == nil {
		t.Fatal("a missing source must fail")
	}
}

func TestASCToPCDKeepsThePointsAndTheirOrder(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.asc")
	dst := filepath.Join(dir, "site.pcd")
	asc := "# Exported points\n# Format: X Y Z Intensity AverageRangeMeters TimesSeenCount\n" +
		"1.000000 2.000000 -1.500000 0 2.692582 300\n\n-4.250000 0.125000 -1.750000 0 4.611979 12\n"
	if err := os.WriteFile(src, []byte(asc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ascToPCD(src, dst); err != nil {
		t.Fatalf("convert: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"VERSION 0.7\n", "FIELDS x y z intensity average_range times_seen\n",
		"TYPE F F F F F U\n", "WIDTH 2\n", "HEIGHT 1\n", "POINTS 2\n", "DATA ascii\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("header lacks %q:\n%s", want, text)
		}
	}
	data := text[strings.Index(text, "DATA ascii\n")+len("DATA ascii\n"):]
	if data != "1.000000 2.000000 -1.500000 0 2.692582 300\n-4.250000 0.125000 -1.750000 0 4.611979 12\n" {
		t.Fatalf("data lines changed or reordered: %q", data)
	}
}

func TestASCToPCDRefusesShortLinesAndEmptyExports(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.asc")
	if err := os.WriteFile(short, []byte("1 2 3 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ascToPCD(short, filepath.Join(dir, "a.pcd")); err == nil {
		t.Fatal("a line without six columns must fail")
	}
	empty := filepath.Join(dir, "empty.asc")
	if err := os.WriteFile(empty, []byte("# Exported points\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ascToPCD(empty, filepath.Join(dir, "b.pcd")); err == nil {
		t.Fatal("an export without points must fail")
	}
}
