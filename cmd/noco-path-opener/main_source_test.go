package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainUsesConfiguredGUIWindowLimit(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{"actions.NewLimitedRunner", "gui.NewRunner()", "cfg.MaxGUIWindows"} {
		if !strings.Contains(body, token) {
			t.Fatalf("main.go does not contain %s", token)
		}
	}
}

func TestMainRunsHTTPServerBehindTray(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{
		`"noco-path-opener/internal/tray"`,
		"go func()",
		"server.ListenAndServe()",
		"tray.Run(",
		"server.Shutdown(",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("main.go does not contain %s", token)
		}
	}
}

func TestMainWiresLocalAndRemoteSyncClients(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{
		"remoteNocoClient := nocodb.NewClient",
		"BaseURL: cfg.RemoteNocoDBURL",
		"Token:   cfg.RemoteNocoDBToken",
		"LocalSyncClient:  nocoClient",
		"RemoteSyncClient: remoteNocoClient",
		"SyncProfiles:     syncProfilesFromConfig(cfg.SyncProfiles)",
		"func syncProfilesFromConfig(profiles []config.SyncProfile) []actions.SyncProfile",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("main.go does not contain %s", token)
		}
	}
}

func TestReadmeDocumentsWindowsGUISubsystemBuild(t *testing.T) {
	source, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	if !strings.Contains(body, `-ldflags="-H windowsgui"`) {
		t.Fatalf("README.md does not document the windowsgui build flag")
	}
	if !strings.Contains(body, `-ldflags="-s -w -H windowsgui"`) {
		t.Fatalf("README.md does not document the release windowsgui build flag")
	}
}

func TestReleaseMetadataDocumentsRemoteSyncProfiles(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("ReadFile(README.md) error = %v", err)
	}
	readmeBody := string(readme)
	for _, token := range []string{
		`"remote_nocodb_url": "https://remote-nocodb.example.com"`,
		`"remote_nocodb_token": "REMOTE_TOKEN"`,
		`"sync_profiles": [`,
		`"sync_profile": "change-log-main"`,
		"`sync_profile` is optional",
		"同步远端",
		"远端未找到匹配记录",
	} {
		if !strings.Contains(readmeBody, token) {
			t.Fatalf("README.md does not contain %s", token)
		}
	}

	version, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("ReadFile(VERSION) error = %v", err)
	}
	if strings.TrimSpace(string(version)) != "0.3.1" {
		t.Fatalf("VERSION = %q, want 0.3.1", strings.TrimSpace(string(version)))
	}

	changelog, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatalf("ReadFile(CHANGELOG.md) error = %v", err)
	}
	changelogBody := string(changelog)
	for _, token := range []string{
		"## 0.3.1 - 2026-08-14",
		"native multi-select file picking",
		"## 0.3.0 - 2026-05-20",
		"remote field sync profiles",
		"同步远端",
	} {
		if !strings.Contains(changelogBody, token) {
			t.Fatalf("CHANGELOG.md does not contain %s", token)
		}
	}
}
