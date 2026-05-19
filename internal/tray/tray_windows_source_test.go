package tray

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsTrayShowsNotifyIconWithExitAction(t *testing.T) {
	source, err := os.ReadFile("tray_windows.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	body := string(source)
	for _, token := range []string{
		"walk.NewNotifyIcon",
		"notifyIcon.SetVisible(true)",
		`exitAction.SetText("退出")`,
		"notifyIcon.ContextMenu().Actions().Add(exitAction)",
		"walk.App().Exit(0)",
		"onExit()",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("tray_windows.go does not contain %s", token)
		}
	}
}
