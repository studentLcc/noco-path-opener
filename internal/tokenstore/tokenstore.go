package tokenstore

import (
	"os"
	"path/filepath"
)

const tokenFileName = "snc-token.dat"

type Store struct {
	path string
}

func New() *Store {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return newStore(filepath.Join(tempDir, "noco-path-opener", tokenFileName))
}

func newStore(path string) *Store {
	return &Store{path: path}
}
