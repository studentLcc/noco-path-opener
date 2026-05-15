package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreatesDefaultConfigWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Fatalf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 6666 {
		t.Fatalf("Port = %d, want %d", cfg.Port, 6666)
	}
	if len(cfg.AllowedRoots) != 0 {
		t.Fatalf("AllowedRoots length = %d, want 0", len(cfg.AllowedRoots))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var written Config
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("written config is invalid JSON: %v", err)
	}
	if written.Host != "0.0.0.0" || written.Port != 6666 || len(written.AllowedRoots) != 0 {
		t.Fatalf("written config = %+v, want default config", written)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"host":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want invalid JSON error")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty host", body: `{"host":"","port":6666,"allowed_roots":[]}`},
		{name: "zero port", body: `{"host":"0.0.0.0","port":0,"allowed_roots":[]}`},
		{name: "negative port", body: `{"host":"0.0.0.0","port":-1,"allowed_roots":[]}`},
		{name: "port too high", body: `{"host":"0.0.0.0","port":70000,"allowed_roots":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
		})
	}
}
