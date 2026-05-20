package profilegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"noco-path-opener/internal/config"
)

func TestAppendProfileToConfigFilePreservesExistingFieldsAndUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
  "host": "127.0.0.1",
  "port": 7777,
  "allowed_roots": ["/tmp/noco"],
  "max_gui_windows": 2,
  "nocodb_url": "http://local.example.com",
  "nocodb_token": "local-token",
  "remote_nocodb_url": "https://remote.example.com",
  "remote_nocodb_token": "remote-token",
  "sync_profiles": [
    {
      "name": "existing",
      "local_base_id": "p_local_existing",
      "local_table_id": "m_local_existing",
      "local_lookup_field": "Change ID",
      "remote_base_id": "p_remote_existing",
      "remote_table_id": "m_remote_existing",
      "remote_lookup_field": "Change ID",
      "sync_fields": ["Status"]
    }
  ],
  "custom_setting": {"enabled": true}
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := appendProfileToConfigFile(path, validGeneratedProfile(" generated ")); err != nil {
		t.Fatalf("appendProfileToConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("written config is invalid JSON: %v", err)
	}
	if _, ok := raw["custom_setting"]; !ok {
		t.Fatal("custom_setting missing after append")
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("written config Validate() error = %v", err)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 7777 || cfg.MaxGUIWindows != 2 {
		t.Fatalf("written base config = %+v, want existing host, port, and max_gui_windows", cfg)
	}
	if len(cfg.AllowedRoots) != 1 || cfg.AllowedRoots[0] != "/tmp/noco" {
		t.Fatalf("AllowedRoots = %#v, want preserved root", cfg.AllowedRoots)
	}
	if cfg.NocoDBURL != "http://local.example.com" || cfg.NocoDBToken != "local-token" {
		t.Fatalf("local NocoDB fields = %q/%q, want preserved", cfg.NocoDBURL, cfg.NocoDBToken)
	}
	if cfg.RemoteNocoDBURL != "https://remote.example.com" || cfg.RemoteNocoDBToken != "remote-token" {
		t.Fatalf("remote NocoDB fields = %q/%q, want preserved", cfg.RemoteNocoDBURL, cfg.RemoteNocoDBToken)
	}
	if len(cfg.SyncProfiles) != 2 {
		t.Fatalf("SyncProfiles length = %d, want 2", len(cfg.SyncProfiles))
	}
	if cfg.SyncProfiles[0].Name != "existing" {
		t.Fatalf("first profile name = %q, want existing", cfg.SyncProfiles[0].Name)
	}
	appended := cfg.SyncProfiles[1]
	if appended.Name != "generated" ||
		appended.LocalBaseID != "p_local" ||
		appended.LocalTableID != "m_local" ||
		appended.LocalLookupField != "Change ID" ||
		appended.RemoteBaseID != "p_remote" ||
		appended.RemoteTableID != "m_remote" ||
		appended.RemoteLookupField != "Change ID" {
		t.Fatalf("appended profile = %+v, want normalized generated profile", appended)
	}
	if len(appended.SyncFields) != 2 || appended.SyncFields[0] != "Status" || appended.SyncFields[1] != "Owner" {
		t.Fatalf("appended SyncFields = %#v, want trimmed fields", appended.SyncFields)
	}
}

func TestAppendProfileToConfigFileRejectsDuplicateNameAndDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
  "host": "127.0.0.1",
  "port": 6666,
  "allowed_roots": [],
  "remote_nocodb_url": "https://remote.example.com",
  "remote_nocodb_token": "remote-token",
  "sync_profiles": [
    {
      "name": "duplicate",
      "local_base_id": "p_local_existing",
      "local_table_id": "m_local_existing",
      "local_lookup_field": "Change ID",
      "remote_base_id": "p_remote_existing",
      "remote_table_id": "m_remote_existing",
      "remote_lookup_field": "Change ID",
      "sync_fields": ["Status"]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() before error = %v", err)
	}

	err = appendProfileToConfigFile(path, validGeneratedProfile(" duplicate "))
	if err == nil {
		t.Fatal("appendProfileToConfigFile() error = nil, want duplicate name error")
	}
	if !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("appendProfileToConfigFile() error = %q, want duplicate error", err.Error())
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("config file changed after validation failure\nbefore: %s\nafter: %s", before, after)
	}
}

func TestLoadExistingConfigFileRequiresExistingValidJSON(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "missing.json")

	_, _, err := loadExistingConfigFile(missingPath)
	if err == nil {
		t.Fatal("loadExistingConfigFile() missing error = nil, want read error")
	}
	if !strings.Contains(err.Error(), "read config:") {
		t.Fatalf("loadExistingConfigFile() missing error = %q, want read config", err.Error())
	}

	invalidPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(`{"host":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, _, err = loadExistingConfigFile(invalidPath)
	if err == nil {
		t.Fatal("loadExistingConfigFile() invalid JSON error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), "parse config:") {
		t.Fatalf("loadExistingConfigFile() invalid JSON error = %q, want parse config", err.Error())
	}
}

func validGeneratedProfile(name string) config.SyncProfile {
	return config.SyncProfile{
		Name:              name,
		LocalBaseID:       " p_local ",
		LocalTableID:      " m_local ",
		LocalLookupField:  " Change ID ",
		RemoteBaseID:      " p_remote ",
		RemoteTableID:     " m_remote ",
		RemoteLookupField: " Change ID ",
		SyncFields:        []string{" Status ", " Owner "},
	}
}
