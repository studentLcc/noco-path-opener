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
	if cfg.MaxGUIWindows != 1 {
		t.Fatalf("MaxGUIWindows = %d, want 1", cfg.MaxGUIWindows)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var written Config
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("written config is invalid JSON: %v", err)
	}
	if written.Host != "0.0.0.0" || written.Port != 6666 || len(written.AllowedRoots) != 0 || written.MaxGUIWindows != 1 {
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
		{name: "negative max gui windows", body: `{"host":"0.0.0.0","port":6666,"allowed_roots":[],"max_gui_windows":-1}`},
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

func TestLoadCreatesDefaultConfigWithNocoDBFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.NocoDBURL != "http://localhost:8080" {
		t.Fatalf("NocoDBURL = %q, want %q", cfg.NocoDBURL, "http://localhost:8080")
	}
	if cfg.NocoDBToken != "" {
		t.Fatalf("NocoDBToken = %q, want empty string", cfg.NocoDBToken)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("written config is invalid JSON: %v", err)
	}
	if written["nocodb_url"] != "http://localhost:8080" {
		t.Fatalf("written nocodb_url = %v, want default URL", written["nocodb_url"])
	}
	if written["nocodb_token"] != "" {
		t.Fatalf("written nocodb_token = %v, want empty string", written["nocodb_token"])
	}
}

func TestLoadKeepsExistingConfigWithoutNocoDBFieldsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"host":"127.0.0.1","port":6666,"allowed_roots":[]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.NocoDBURL != "" {
		t.Fatalf("NocoDBURL = %q, want empty string for existing config", cfg.NocoDBURL)
	}
	if cfg.NocoDBToken != "" {
		t.Fatalf("NocoDBToken = %q, want empty string", cfg.NocoDBToken)
	}
	if cfg.MaxGUIWindows != 1 {
		t.Fatalf("MaxGUIWindows = %d, want default 1 for existing config", cfg.MaxGUIWindows)
	}
}

func TestLoadUsesConfiguredMaxGUIWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"host":"127.0.0.1","port":6666,"allowed_roots":[],"max_gui_windows":3}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxGUIWindows != 3 {
		t.Fatalf("MaxGUIWindows = %d, want 3", cfg.MaxGUIWindows)
	}
}
