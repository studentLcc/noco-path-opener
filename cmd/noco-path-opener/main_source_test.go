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
