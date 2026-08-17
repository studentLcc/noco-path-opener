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
	if cfg.Host != "0.0.0.0" || cfg.Port != 6666 || cfg.MaxGUIWindows != 1 {
		t.Fatalf("config = %+v, want default host, port, and GUI limit", cfg)
	}
	if len(cfg.AllowedRoots) != 0 {
		t.Fatalf("AllowedRoots = %#v, want empty", cfg.AllowedRoots)
	}
}

func TestLoadCreatesDefaultConfigWithLocalNocoDBFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if _, err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("written config is invalid JSON: %v", err)
	}
	if written["nocodb_url"] != "http://localhost:8080" || written["nocodb_token"] != "" {
		t.Fatalf("local NocoDB defaults = %#v", written)
	}
	for _, legacyKey := range []string{"remote_nocodb_url", "remote_nocodb_token", "sync_profiles"} {
		if _, found := written[legacyKey]; found {
			t.Fatalf("written config still contains legacy setting %q", legacyKey)
		}
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"host":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want invalid JSON error")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []string{
		`{"host":"","port":6666,"allowed_roots":[]}`,
		`{"host":"0.0.0.0","port":0,"allowed_roots":[]}`,
		`{"host":"0.0.0.0","port":70000,"allowed_roots":[]}`,
		`{"host":"0.0.0.0","port":6666,"allowed_roots":[],"max_gui_windows":-1}`,
	}
	for _, body := range tests {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("Load(%s) error = nil, want validation error", body)
		}
	}
}

func TestLoadDefaultsMissingGUIWindowLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"host":"127.0.0.1","port":6666,"allowed_roots":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxGUIWindows != 1 {
		t.Fatalf("MaxGUIWindows = %d, want 1", cfg.MaxGUIWindows)
	}
}
