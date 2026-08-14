package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUniquePathsCleansCaseInsensitivelyAndPreservesOrder(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "Data", "One.txt")
	duplicate := filepath.Join(root, "data", "one.txt")
	other := filepath.Join(root, "Other", "Two.txt")
	paths := []string{
		filepath.Join(root, "Data", ".", "One.txt"),
		duplicate,
		other,
	}

	got := uniquePaths(paths)
	want := []string{first, other}
	if len(got) != len(want) {
		t.Fatalf("uniquePaths() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uniquePaths()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMostRecentSelectionDirUsesLastSelectedPath(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "first")
	lastDir := filepath.Join(root, "last")
	if err := os.MkdirAll(firstDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(firstDir) error = %v", err)
	}
	if err := os.MkdirAll(lastDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(lastDir) error = %v", err)
	}
	lastFile := filepath.Join(lastDir, "last.txt")
	if err := os.WriteFile(lastFile, []byte("last"), 0o600); err != nil {
		t.Fatalf("WriteFile(lastFile) error = %v", err)
	}

	if got := mostRecentSelectionDir([]string{firstDir, lastFile}); got != lastDir {
		t.Fatalf("mostRecentSelectionDir() = %q, want %q", got, lastDir)
	}
}

func TestSelectionInitialDirKeepsRecentDirAfterCanceledSelection(t *testing.T) {
	recent := t.TempDir()
	if got := selectionInitialDir(recent, nil); got != recent {
		t.Fatalf("selectionInitialDir() = %q, want %q", got, recent)
	}
}
