//go:build windows

package tokenstore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewUsesTEMPDirectory(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "temp")
	tmpDir := filepath.Join(t.TempDir(), "tmp")
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMP", tmpDir)

	store := New()
	want := filepath.Join(tempDir, "noco-path-opener", tokenFileName)
	if store.path != want {
		t.Fatalf("New() path = %q, want %q", store.path, want)
	}
}

func TestStoreLoadMissingFileReturnsEmptyToken(t *testing.T) {
	store := newStore(filepath.Join(t.TempDir(), "snc-token.dat"))

	token, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if token != "" {
		t.Fatalf("Load() token = %q, want empty", token)
	}
}

func TestStoreSaveEncryptsAndLoadsToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "snc-token.dat")
	store := newStore(path)
	const token = "secret-snc-token"

	if err := store.Save(token); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	encrypted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(encrypted) == 0 {
		t.Fatal("saved token file is empty")
	}
	if bytes.Contains(encrypted, []byte(token)) {
		t.Fatal("saved token file contains the plaintext token")
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != token {
		t.Fatalf("Load() token = %q, want %q", got, token)
	}
}

func TestStoreSaveOverwritesPreviousToken(t *testing.T) {
	store := newStore(filepath.Join(t.TempDir(), "snc-token.dat"))

	if err := store.Save("old-token"); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save("new-token"); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != "new-token" {
		t.Fatalf("Load() token = %q, want new-token", got)
	}
}

func TestStoreLoadRejectsCorruptedCiphertext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snc-token.dat")
	if err := os.WriteFile(path, []byte("not-dpapi-ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := newStore(path)
	if _, err := store.Load(); err == nil {
		t.Fatal("Load() error = nil, want corrupted ciphertext error")
	}
}
