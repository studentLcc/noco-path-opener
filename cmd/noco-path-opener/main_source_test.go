package main

import (
	"encoding/json"
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
	source, err := os.ReadFile("exit_tray_windows.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{
		`"noco-path-opener/internal/tray"`,
		"tray.Run(",
		"shutdownServer(server)",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("exit_tray_windows.go does not contain %s", token)
		}
	}
}

func TestConsoleDebugBuildWaitsForSignalWithoutTray(t *testing.T) {
	source, err := os.ReadFile("exit_console.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{
		"//go:build !windows || consoledebug",
		`signal.Notify(stop, os.Interrupt, syscall.SIGTERM)`,
		`log.Printf("console debug mode active; press Ctrl+C to exit")`,
		"shutdownServer(server)",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("exit_console.go does not contain %s", token)
		}
	}
	if strings.Contains(body, "tray.Run") {
		t.Fatal("console debug exit handler must not initialize the tray")
	}
}

func TestMainWiresDynamicRemoteSyncClient(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{
		"LocalSyncClient:",
		"DynamicRemoteSyncClient:",
		"remoteSyncClient := remotesync.NewClient",
		`filepath.Join(filepath.Dir(exePath), "remote_sync_params.json")`,
		`filepath.Join(filepath.Dir(exePath), "remote_sync_headers.json")`,
		"LogResponseBodies:",
		"logRemoteResponseBodies",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("main.go does not contain %s", token)
		}
	}
	for _, removed := range []string{"RemoteNocoDB", "\n\t\tRemoteSyncClient:", "SyncProfiles:", "syncProfilesFromConfig"} {
		if strings.Contains(body, removed) {
			t.Fatalf("main.go still contains legacy synchronization code %s", removed)
		}
	}
}

func TestConsoleDebugBuildEnablesRemoteResponseBodyLogging(t *testing.T) {
	source, err := os.ReadFile("remote_log_bodies_debug.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(source), "const logRemoteResponseBodies = true") {
		t.Fatal("console debug build does not enable remote response body logging")
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

func TestReleaseMetadataDocumentsDynamicRemoteSyncOnly(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("ReadFile(README.md) error = %v", err)
	}
	readmeBody := string(readme)
	for _, token := range []string{
		"`remote_sync_headers.json`",
		"`{token}`",
		"同步远端",
		"`current_path`",
	} {
		if !strings.Contains(readmeBody, token) {
			t.Fatalf("README.md does not contain %s", token)
		}
	}
	for _, removed := range []string{"sync_profiles", "sync_profile", "remote_nocodb_url", "remote_nocodb_token"} {
		if strings.Contains(readmeBody, removed) {
			t.Fatalf("README.md still documents legacy synchronization setting %s", removed)
		}
	}

	version, err := os.ReadFile("../../VERSION")
	if err != nil {
		t.Fatalf("ReadFile(VERSION) error = %v", err)
	}
	if strings.TrimSpace(string(version)) != "0.5.0" {
		t.Fatalf("VERSION = %q, want 0.5.0", strings.TrimSpace(string(version)))
	}

	changelog, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatalf("ReadFile(CHANGELOG.md) error = %v", err)
	}
	changelogBody := string(changelog)
	for _, token := range []string{
		"## 0.5.0 - 2026-08-14",
		"remote_sync_headers.json",
		"current_path",
		"sync_profiles",
	} {
		if !strings.Contains(changelogBody, token) {
			t.Fatalf("CHANGELOG.md does not contain %s", token)
		}
	}
}

func TestConfigurationExamplesAreValidJSON(t *testing.T) {
	for _, name := range []string{
		"config.example.json",
		"remote_sync_params.example.json",
		"remote_sync_headers.example.json",
	} {
		data, err := os.ReadFile("../../" + name)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		if !json.Valid(data) {
			t.Fatalf("%s is not valid JSON", name)
		}
	}
}
