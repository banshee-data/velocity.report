package fsutil

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystem_Exists(t *testing.T) {
	fs := OSFileSystem{}

	if !fs.Exists("filesystem.go") {
		t.Error("expected filesystem.go to exist")
	}

	if fs.Exists("nonexistent_file_xyz.go") {
		t.Error("expected nonexistent file to not exist")
	}
}

func TestOSFileSystem_ReadFile(t *testing.T) {
	fs := OSFileSystem{}

	data, err := fs.ReadFile("filesystem.go")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty file content")
	}
}

func TestMemoryFileSystem_WriteAndRead(t *testing.T) {
	mfs := NewMemoryFileSystem()

	testData := []byte("hello, world")
	err := mfs.WriteFile("/test.txt", testData, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, err := mfs.ReadFile("/test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("expected %q, got %q", testData, data)
	}
}

func TestMemoryFileSystem_CreateAndWrite(t *testing.T) {
	mfs := NewMemoryFileSystem()

	w, err := mfs.Create("/created.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err = w.Write([]byte("created content"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := mfs.ReadFile("/created.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != "created content" {
		t.Errorf("expected 'created content', got %q", data)
	}
}

func TestMemoryFileSystem_Open(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.WriteFile("/opentest.txt", []byte("open me"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	f, err := mfs.Open("/opentest.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(data) != "open me" {
		t.Errorf("expected 'open me', got %q", data)
	}
}

func TestMemoryFileSystem_OpenNonExistent(t *testing.T) {
	mfs := NewMemoryFileSystem()

	_, err := mfs.Open("/nonexistent.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestMemoryFileSystem_Stat(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.WriteFile("/stattest.txt", []byte("stat content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, err := mfs.Stat("/stattest.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Name() != "stattest.txt" {
		t.Errorf("expected name 'stattest.txt', got %q", info.Name())
	}

	if info.Size() != int64(len("stat content")) {
		t.Errorf("expected size %d, got %d", len("stat content"), info.Size())
	}

	if info.IsDir() {
		t.Error("expected file, not directory")
	}
}

func TestMemoryFileSystem_StatDir(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.MkdirAll("/testdir/subdir", 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	info, err := mfs.Stat("/testdir/subdir")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestMemoryFileSystem_StatNonExistent(t *testing.T) {
	mfs := NewMemoryFileSystem()

	_, err := mfs.Stat("/nonexistent.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestMemoryFileSystem_MkdirAll(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.MkdirAll("/a/b/c", 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if !mfs.Exists("/a/b/c") {
		t.Error("expected directory to exist")
	}

	if !mfs.Exists("/a/b") {
		t.Error("expected parent directory to exist")
	}

	if !mfs.Exists("/a") {
		t.Error("expected grandparent directory to exist")
	}
}

func TestMemoryFileSystem_Remove(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.WriteFile("/removeme.txt", []byte("delete"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !mfs.Exists("/removeme.txt") {
		t.Error("expected file to exist before removal")
	}

	err = mfs.Remove("/removeme.txt")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if mfs.Exists("/removeme.txt") {
		t.Error("expected file to not exist after removal")
	}
}

func TestMemoryFileSystem_RemoveNonExistent(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.Remove("/nonexistent.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestMemoryFileSystem_RemoveDir(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.MkdirAll("/dirtoremove", 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	err = mfs.Remove("/dirtoremove")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if mfs.Exists("/dirtoremove") {
		t.Error("expected directory to not exist after removal")
	}
}

func TestMemoryFileSystem_RemoveAll(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.MkdirAll("/parent/child", 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	err = mfs.WriteFile("/parent/file1.txt", []byte("file1"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err = mfs.WriteFile("/parent/child/file2.txt", []byte("file2"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err = mfs.RemoveAll("/parent")
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	if mfs.Exists("/parent") {
		t.Error("expected /parent to not exist")
	}

	if mfs.Exists("/parent/file1.txt") {
		t.Error("expected /parent/file1.txt to not exist")
	}

	if mfs.Exists("/parent/child") {
		t.Error("expected /parent/child to not exist")
	}

	if mfs.Exists("/parent/child/file2.txt") {
		t.Error("expected /parent/child/file2.txt to not exist")
	}
}

func TestMemoryFileSystem_Exists(t *testing.T) {
	mfs := NewMemoryFileSystem()

	if mfs.Exists("/nonexistent") {
		t.Error("expected non-existent path to not exist")
	}

	err := mfs.WriteFile("/exists.txt", []byte("data"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !mfs.Exists("/exists.txt") {
		t.Error("expected file to exist")
	}

	err = mfs.MkdirAll("/existsdir", 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if !mfs.Exists("/existsdir") {
		t.Error("expected directory to exist")
	}
}

func TestMemoryFileSystem_PathCleaning(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.WriteFile("./dirty/../clean.txt", []byte("clean"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, err := mfs.ReadFile("clean.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != "clean" {
		t.Errorf("expected 'clean', got %q", data)
	}
}

func TestMemFileReader_Stat(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.WriteFile("/readable.txt", []byte("readable content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	f, err := mfs.Open("/readable.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Name() != "readable.txt" {
		t.Errorf("expected name 'readable.txt', got %q", info.Name())
	}
}

func TestMemFileWriter_UpdateExisting(t *testing.T) {
	mfs := NewMemoryFileSystem()

	err := mfs.WriteFile("/update.txt", []byte("initial"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	w, err := mfs.Create("/update.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err = w.Write([]byte("updated"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := mfs.ReadFile("/update.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != "updated" {
		t.Errorf("expected 'updated', got %q", data)
	}
}

func TestOSFileSystem_TempFileOperations(t *testing.T) {
	fs := OSFileSystem{}
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	err := fs.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !fs.Exists(testFile) {
		t.Error("expected file to exist")
	}

	data, err := fs.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != "test content" {
		t.Errorf("expected 'test content', got %q", data)
	}

	info, err := fs.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Name() != "test.txt" {
		t.Errorf("expected name 'test.txt', got %q", info.Name())
	}

	err = fs.Remove(testFile)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if fs.Exists(testFile) {
		t.Error("expected file to not exist after removal")
	}
}

func TestOSFileSystem_MkdirAll(t *testing.T) {
	fs := OSFileSystem{}
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "a", "b", "c")

	err := fs.MkdirAll(nestedDir, 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if !fs.Exists(nestedDir) {
		t.Error("expected nested directory to exist")
	}
}

func TestOSFileSystem_CreateAndOpen(t *testing.T) {
	fs := OSFileSystem{}
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "created.txt")

	w, err := fs.Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err = w.Write([]byte("created via Create"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	f, err := fs.Open(testFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(data) != "created via Create" {
		t.Errorf("expected 'created via Create', got %q", data)
	}
}

func TestOSFileSystem_RemoveAll(t *testing.T) {
	fs := OSFileSystem{}
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "parent")
	childDir := filepath.Join(parentDir, "child")

	err := fs.MkdirAll(childDir, 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	err = fs.WriteFile(filepath.Join(childDir, "file.txt"), []byte("data"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err = fs.RemoveAll(parentDir)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	if fs.Exists(parentDir) {
		t.Error("expected parent directory to not exist")
	}
}

func TestMemoryFileSystem_DataIsolation(t *testing.T) {
	mfs := NewMemoryFileSystem()

	original := []byte("original")
	err := mfs.WriteFile("/isolated.txt", original, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	original[0] = 'X'

	data, err := mfs.ReadFile("/isolated.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if data[0] != 'o' {
		t.Error("expected data to be isolated from original slice")
	}

	data[0] = 'Y'

	data2, err := mfs.ReadFile("/isolated.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if data2[0] != 'o' {
		t.Error("expected read data to be isolated")
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s      string
		prefix string
		want   bool
	}{
		{"/a/b/c", "/a/b", true},
		{"/a/b/c", "/a/b/c", true},
		{"/a/b/c", "/a/b/c/", false},
		{"/a/b", "/a/b/c", false},
		{"", "", true},
		{"a", "", true},
		{"", "a", false},
	}

	for _, tt := range tests {
		got := hasPrefix(tt.s, tt.prefix)
		if got != tt.want {
			t.Errorf("hasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
		}
	}
}

func TestMemFileInfo(t *testing.T) {
	info := &memFileInfo{
		name:  "test.txt",
		size:  100,
		mode:  0644,
		isDir: false,
	}

	if info.Name() != "test.txt" {
		t.Errorf("Name() = %q, want 'test.txt'", info.Name())
	}

	if info.Size() != 100 {
		t.Errorf("Size() = %d, want 100", info.Size())
	}

	if info.Mode() != 0644 {
		t.Errorf("Mode() = %v, want 0644", info.Mode())
	}

	if info.IsDir() {
		t.Error("IsDir() = true, want false")
	}

	if info.Sys() != nil {
		t.Error("Sys() should return nil")
	}

	if !info.ModTime().IsZero() {
		t.Error("ModTime() should return zero time")
	}
}

func TestMemoryFileSystem_ReadNonExistent(t *testing.T) {
	mfs := NewMemoryFileSystem()

	_, err := mfs.ReadFile("/nonexistent.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}

	pathErr, ok := err.(*os.PathError)
	if !ok {
		t.Errorf("expected *os.PathError, got %T", err)
	}

	if pathErr.Op != "read" {
		t.Errorf("expected Op 'read', got %q", pathErr.Op)
	}
}
