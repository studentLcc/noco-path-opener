package profilegen

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"noco-path-opener/internal/config"
)

func TestRunPrintModePromptsCredentialsAndWritesOnlyProfileJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := `{
  "host": "127.0.0.1",
  "port": 6666,
  "allowed_roots": [],
  "max_gui_windows": 1,
  "nocodb_url": "",
  "nocodb_token": "",
  "remote_nocodb_url": "",
  "remote_nocodb_token": "",
  "sync_profiles": []
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	local := &fakeFieldLister{fields: []Field{
		{ID: "l1", Title: "Change ID"},
		{ID: "l2", Title: "Status"},
		{ID: "l3", Title: "Owner"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "Change ID"},
		{ID: "r2", Title: "Status"},
		{ID: "r3", Title: "Owner"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"https://local.example.com",
		"local-token-from-prompt",
		"https://remote.example.com",
		"remote-token-from-prompt",
		"change-log-main",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"2,3",
	}, "\n") + "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), Options{
		ConfigPath:        path,
		In:                input,
		Out:               &stdout,
		Err:               &stderr,
		LocalFieldLister:  local,
		RemoteFieldLister: remote,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var profile config.SyncProfile
	if err := json.Unmarshal(stdout.Bytes(), &profile); err != nil {
		t.Fatalf("stdout is not valid profile JSON: %v\nstdout: %s", err, stdout.String())
	}
	if profile.Name != "change-log-main" ||
		profile.LocalBaseID != "p_local" ||
		profile.LocalTableID != "m_local" ||
		profile.LocalLookupField != "Change ID" ||
		profile.RemoteBaseID != "p_remote" ||
		profile.RemoteTableID != "m_remote" ||
		profile.RemoteLookupField != "Change ID" {
		t.Fatalf("profile = %+v, want generated profile", profile)
	}
	if !reflect.DeepEqual(profile.SyncFields, []string{"Status", "Owner"}) {
		t.Fatalf("SyncFields = %#v, want selected fields", profile.SyncFields)
	}
	for _, unwanted := range []string{"Local NocoDB URL:", "Local fields:", "Remote fields:"} {
		if strings.Contains(stdout.String(), unwanted) {
			t.Fatalf("stdout = %q, want only JSON without %q", stdout.String(), unwanted)
		}
	}
	for _, want := range []string{
		"Local NocoDB URL:",
		"Local NocoDB token:",
		"Remote NocoDB URL:",
		"Remote NocoDB token:",
		"Local fields:",
		"Remote fields:",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want containing %q", stderr.String(), want)
		}
	}
	if !reflect.DeepEqual(local.calls, []fieldCall{{baseID: "p_local", tableID: "m_local"}}) {
		t.Fatalf("local calls = %#v, want local metadata call", local.calls)
	}
	if !reflect.DeepEqual(remote.calls, []fieldCall{{baseID: "p_remote", tableID: "m_remote"}}) {
		t.Fatalf("remote calls = %#v, want remote metadata call", remote.calls)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(after) != initial {
		t.Fatalf("config changed in print mode\nbefore: %s\nafter: %s", initial, after)
	}
	for _, token := range []string{"local-token-from-prompt", "remote-token-from-prompt"} {
		if bytes.Contains(after, []byte(token)) {
			t.Fatalf("config contains prompted token %q", token)
		}
	}
}

func TestRunWriteModeAppendsProfileWithConfiguredRemoteCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := `{
  "host": "127.0.0.1",
  "port": 6666,
  "allowed_roots": [],
  "max_gui_windows": 1,
  "nocodb_url": "",
  "nocodb_token": "",
  "remote_nocodb_url": " https://remote.example.com ",
  "remote_nocodb_token": " remote-token ",
  "sync_profiles": []
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	local := &fakeFieldLister{fields: []Field{
		{ID: "l1", Title: "Change ID"},
		{ID: "l2", Title: "Status"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "Change ID"},
		{ID: "r2", Title: "Status"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"https://local.example.com",
		"local-token-from-prompt",
		"write-profile",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"2",
	}, "\n") + "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := Run(context.Background(), Options{
		ConfigPath:        path,
		Write:             true,
		In:                input,
		Out:               &stdout,
		Err:               &stderr,
		LocalFieldLister:  local,
		RemoteFieldLister: remote,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty in write mode", stdout.String())
	}
	for _, want := range []string{"Local NocoDB URL:", "Local NocoDB token:", "Local fields:", "Remote fields:"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want containing %q", stderr.String(), want)
		}
	}
	for _, unwanted := range []string{"Remote NocoDB URL:", "Remote NocoDB token:"} {
		if strings.Contains(stderr.String(), unwanted) {
			t.Fatalf("stderr = %q, want no prompt for configured %q", stderr.String(), unwanted)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if bytes.Contains(data, []byte("local-token-from-prompt")) {
		t.Fatal("config contains prompted local token")
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(cfg.SyncProfiles) != 1 {
		t.Fatalf("SyncProfiles length = %d, want 1", len(cfg.SyncProfiles))
	}
	if cfg.SyncProfiles[0].Name != "write-profile" {
		t.Fatalf("profile name = %q, want write-profile", cfg.SyncProfiles[0].Name)
	}
	if cfg.NocoDBURL != "" || cfg.NocoDBToken != "" {
		t.Fatalf("local credentials = %q/%q, want not persisted", cfg.NocoDBURL, cfg.NocoDBToken)
	}
	if cfg.RemoteNocoDBURL != " https://remote.example.com " || cfg.RemoteNocoDBToken != " remote-token " {
		t.Fatalf("remote credentials = %q/%q, want preserved bytes values", cfg.RemoteNocoDBURL, cfg.RemoteNocoDBToken)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("written config Validate() error = %v", err)
	}
}

func TestRunWriteModeFailsWhenRemoteCredentialsOnlyPromptedAndDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := `{
  "host": "127.0.0.1",
  "port": 6666,
  "allowed_roots": [],
  "max_gui_windows": 1,
  "nocodb_url": "https://local.example.com",
  "nocodb_token": "local-token",
  "remote_nocodb_url": "",
  "remote_nocodb_token": "",
  "sync_profiles": []
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	local := &fakeFieldLister{fields: []Field{
		{ID: "l1", Title: "Change ID"},
		{ID: "l2", Title: "Status"},
	}}
	remote := &fakeFieldLister{fields: []Field{
		{ID: "r1", Title: "Change ID"},
		{ID: "r2", Title: "Status"},
	}}
	input := strings.NewReader(strings.Join([]string{
		"https://remote.example.com",
		"remote-token-from-prompt",
		"prompted-remote",
		"p_local",
		"m_local",
		"p_remote",
		"m_remote",
		"1",
		"1",
		"2",
	}, "\n") + "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), Options{
		ConfigPath:        path,
		Write:             true,
		In:                input,
		Out:               &stdout,
		Err:               &stderr,
		LocalFieldLister:  local,
		RemoteFieldLister: remote,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want persisted remote credential validation error")
	}
	if !strings.Contains(err.Error(), "remote_nocodb_url is required when sync_profiles is not empty") {
		t.Fatalf("Run() error = %q, want remote credential validation error", err.Error())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failed write mode", stdout.String())
	}
	for _, want := range []string{"Remote NocoDB URL:", "Remote NocoDB token:", "Local fields:", "Remote fields:"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want containing %q", stderr.String(), want)
		}
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(after) != initial {
		t.Fatalf("config changed after failed write\nbefore: %s\nafter: %s", initial, after)
	}
}
