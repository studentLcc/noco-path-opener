package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadKeepsExistingConfigWithoutRemoteSyncFieldsValid(t *testing.T) {
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
	if cfg.RemoteNocoDBURL != "" {
		t.Fatalf("RemoteNocoDBURL = %q, want empty string", cfg.RemoteNocoDBURL)
	}
	if cfg.RemoteNocoDBToken != "" {
		t.Fatalf("RemoteNocoDBToken = %q, want empty string", cfg.RemoteNocoDBToken)
	}
	if len(cfg.SyncProfiles) != 0 {
		t.Fatalf("SyncProfiles length = %d, want 0", len(cfg.SyncProfiles))
	}
}

func TestLoadCreatesDefaultConfigWithEmptyRemoteSyncFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RemoteNocoDBURL != "" {
		t.Fatalf("RemoteNocoDBURL = %q, want empty string", cfg.RemoteNocoDBURL)
	}
	if cfg.RemoteNocoDBToken != "" {
		t.Fatalf("RemoteNocoDBToken = %q, want empty string", cfg.RemoteNocoDBToken)
	}
	if len(cfg.SyncProfiles) != 0 {
		t.Fatalf("SyncProfiles length = %d, want 0", len(cfg.SyncProfiles))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("written config is invalid JSON: %v", err)
	}
	if written["remote_nocodb_url"] != "" {
		t.Fatalf("written remote_nocodb_url = %v, want empty string", written["remote_nocodb_url"])
	}
	if written["remote_nocodb_token"] != "" {
		t.Fatalf("written remote_nocodb_token = %v, want empty string", written["remote_nocodb_token"])
	}
	profiles, ok := written["sync_profiles"].([]any)
	if !ok {
		t.Fatalf("written sync_profiles = %T, want array", written["sync_profiles"])
	}
	if len(profiles) != 0 {
		t.Fatalf("written sync_profiles length = %d, want 0", len(profiles))
	}
}

func TestValidateSyncProfilesRequiresRemoteCredentials(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "missing remote url",
			mutate: func(cfg *Config) {
				cfg.RemoteNocoDBURL = ""
				cfg.RemoteNocoDBToken = "token"
			},
			wantErr: "remote_nocodb_url is required when sync_profiles is not empty",
		},
		{
			name: "missing remote token",
			mutate: func(cfg *Config) {
				cfg.RemoteNocoDBURL = "https://remote.example.com"
				cfg.RemoteNocoDBToken = ""
			},
			wantErr: "remote_nocodb_token is required when sync_profiles is not empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validRemoteSyncConfig()
			tt.mutate(&cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("Validate() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateSyncProfilesRequiresProfileFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SyncProfile)
		wantErr string
	}{
		{name: "name", mutate: func(p *SyncProfile) { p.Name = " " }, wantErr: "name"},
		{name: "local base id", mutate: func(p *SyncProfile) { p.LocalBaseID = " " }, wantErr: "local_base_id"},
		{name: "local table id", mutate: func(p *SyncProfile) { p.LocalTableID = " " }, wantErr: "local_table_id"},
		{name: "local lookup field", mutate: func(p *SyncProfile) { p.LocalLookupField = " " }, wantErr: "local_lookup_field"},
		{name: "remote base id", mutate: func(p *SyncProfile) { p.RemoteBaseID = " " }, wantErr: "remote_base_id"},
		{name: "remote table id", mutate: func(p *SyncProfile) { p.RemoteTableID = " " }, wantErr: "remote_table_id"},
		{name: "remote lookup field", mutate: func(p *SyncProfile) { p.RemoteLookupField = " " }, wantErr: "remote_lookup_field"},
		{name: "sync fields", mutate: func(p *SyncProfile) { p.SyncFields = nil }, wantErr: "sync_fields"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validRemoteSyncConfig()
			tt.mutate(&cfg.SyncProfiles[0])

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateSyncProfilesRequiresUniqueTrimmedNames(t *testing.T) {
	cfg := validRemoteSyncConfig()
	duplicate := cfg.SyncProfiles[0]
	duplicate.Name = "  daily files  "
	cfg.SyncProfiles = append(cfg.SyncProfiles, duplicate)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
	wantErr := "sync_profiles[1].name duplicates sync_profiles[0].name"
	if err.Error() != wantErr {
		t.Fatalf("Validate() error = %q, want %q", err.Error(), wantErr)
	}
}

func TestValidateSyncProfilesRequiresNonEmptyTrimmedSyncFields(t *testing.T) {
	cfg := validRemoteSyncConfig()
	cfg.SyncProfiles[0].SyncFields = []string{"Title", " "}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "sync_fields") {
		t.Fatalf("Validate() error = %q, want containing %q", err.Error(), "sync_fields")
	}
}

func TestValidateSyncProfilesRequiresUniqueTrimmedSyncFields(t *testing.T) {
	cfg := validRemoteSyncConfig()
	cfg.SyncProfiles[0].SyncFields = []string{"Title", "Status", " Title "}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
	wantErr := "sync_profiles[0].sync_fields[2] duplicates sync_profiles[0].sync_fields[0]"
	if err.Error() != wantErr {
		t.Fatalf("Validate() error = %q, want %q", err.Error(), wantErr)
	}
}

func TestValidateAcceptsValidSyncProfile(t *testing.T) {
	cfg := validRemoteSyncConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestLoadAcceptsValidSyncProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
		"host": "127.0.0.1",
		"port": 6666,
		"allowed_roots": [],
		"remote_nocodb_url": "https://remote.example.com",
		"remote_nocodb_token": "token",
		"sync_profiles": [
			{
				"name": "daily files",
				"local_base_id": "local-base",
				"local_table_id": "local-table",
				"local_lookup_field": "Path",
				"remote_base_id": "remote-base",
				"remote_table_id": "remote-table",
				"remote_lookup_field": "Path",
				"sync_fields": ["Title", "Status"]
			}
		]
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.RemoteNocoDBURL != "https://remote.example.com" {
		t.Fatalf("RemoteNocoDBURL = %q, want remote URL", cfg.RemoteNocoDBURL)
	}
	if cfg.RemoteNocoDBToken != "token" {
		t.Fatalf("RemoteNocoDBToken = %q, want token", cfg.RemoteNocoDBToken)
	}
	if len(cfg.SyncProfiles) != 1 {
		t.Fatalf("SyncProfiles length = %d, want 1", len(cfg.SyncProfiles))
	}
	profile := cfg.SyncProfiles[0]
	if profile.Name != "daily files" || profile.LocalBaseID != "local-base" || profile.LocalTableID != "local-table" || profile.LocalLookupField != "Path" ||
		profile.RemoteBaseID != "remote-base" || profile.RemoteTableID != "remote-table" || profile.RemoteLookupField != "Path" {
		t.Fatalf("SyncProfiles[0] = %+v, want valid profile from disk", profile)
	}
	if len(profile.SyncFields) != 2 || profile.SyncFields[0] != "Title" || profile.SyncFields[1] != "Status" {
		t.Fatalf("SyncProfiles[0].SyncFields = %#v, want [Title Status]", profile.SyncFields)
	}
}

func validRemoteSyncConfig() Config {
	cfg := Default()
	cfg.RemoteNocoDBURL = "https://remote.example.com"
	cfg.RemoteNocoDBToken = "token"
	cfg.SyncProfiles = []SyncProfile{
		{
			Name:              "daily files",
			LocalBaseID:       "local-base",
			LocalTableID:      "local-table",
			LocalLookupField:  "Path",
			RemoteBaseID:      "remote-base",
			RemoteTableID:     "remote-table",
			RemoteLookupField: "Path",
			SyncFields:        []string{"Title", "Status"},
		},
	}
	return cfg
}
