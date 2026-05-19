package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"noco-path-opener/internal/nocodb"
)

type fakeRunner struct {
	run func(context.Context, Request, Controller) error
}

func (f fakeRunner) Run(ctx context.Context, req Request, controller Controller) error {
	return f.run(ctx, req, controller)
}

type fakeOpener struct {
	calls int
	path  string
	isDir bool
	err   error
}

func (f *fakeOpener) Open(path string, isDir bool) error {
	f.calls++
	f.path = path
	f.isDir = isDir
	return f.err
}

type fakeUpdater struct {
	calls int
	req   nocodb.UpdateRequest
	err   error
}

type fakeLogger struct {
	messages chan string
}

func newFakeLogger() *fakeLogger {
	return &fakeLogger{messages: make(chan string, 1)}
}

func (f *fakeLogger) Printf(format string, v ...any) {
	f.messages <- fmt.Sprintf(format, v...)
}

func (f *fakeUpdater) UpdateRecord(ctx context.Context, req nocodb.UpdateRequest) error {
	f.calls++
	f.req = req
	return f.err
}

func TestFlowOpenCurrentChecksAllowedRootsBeforeOpening(t *testing.T) {
	allowedRoot := t.TempDir()
	outsideRoot := t.TempDir()
	currentPath := filepath.Join(outsideRoot, "current.txt")
	if err := os.WriteFile(currentPath, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opener := &fakeOpener{}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.OpenCurrent(ctx)
		}},
		Opener:       opener,
		AllowedRoots: []string{allowedRoot},
	}

	err := flow.Run(context.Background(), Request{CurrentPath: currentPath})
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Fatalf("Run() error = %v, want ErrPathNotAllowed", err)
	}
	if opener.calls != 0 {
		t.Fatalf("opener calls = %d, want 0", opener.calls)
	}
}

func TestFlowOpenCurrentOpensExistingAllowedPath(t *testing.T) {
	root := t.TempDir()
	currentPath := filepath.Join(root, "current.txt")
	if err := os.WriteFile(currentPath, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opener := &fakeOpener{}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.OpenCurrent(ctx)
		}},
		Opener:       opener,
		AllowedRoots: []string{root},
	}

	err := flow.Run(context.Background(), Request{CurrentPath: "  " + currentPath + "  "})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want 1", opener.calls)
	}
	if opener.path != currentPath {
		t.Fatalf("opener path = %q, want %q", opener.path, currentPath)
	}
	if opener.isDir {
		t.Fatalf("opener isDir = true, want false")
	}
}

func TestFlowPreparePathConvertsSelectedPathToAbsolute(t *testing.T) {
	relPath, absPath := writeRelativeTempFile(t)

	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			got, err := controller.PreparePath("  " + relPath + "  ")
			if err != nil {
				return err
			}
			if got != absPath {
				t.Fatalf("PreparePath() = %q, want %q", got, absPath)
			}
			return nil
		}},
		AllowedRoots: []string{filepath.Dir(absPath)},
	}

	if err := flow.Run(context.Background(), Request{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestFlowPreparePathAllowsRelativeAllowedRootAfterAbsoluteConversion(t *testing.T) {
	relPath, absPath := writeRelativeTempFile(t)
	relRoot := filepath.Dir(relPath)

	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			got, err := controller.PreparePath(relPath)
			if err != nil {
				return err
			}
			if got != absPath {
				t.Fatalf("PreparePath() = %q, want %q", got, absPath)
			}
			return nil
		}},
		AllowedRoots: []string{relRoot},
	}

	if err := flow.Run(context.Background(), Request{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestFlowUpdateSelectedSendsNocoDBUpdate(t *testing.T) {
	relPath, absPath := writeRelativeTempFile(t)
	updater := &fakeUpdater{}
	req := Request{
		BaseID:    "base-1",
		TableID:   "table-1",
		RecordID:  json.RawMessage(`"rec-1"`),
		PathField: "LocalPath",
	}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.UpdateSelected(ctx, relPath)
		}},
		Updater:      updater,
		AllowedRoots: []string{filepath.Dir(absPath)},
		NocoDBURL:    " https://nocodb.example.test ",
		NocoDBToken:  " token ",
	}

	if err := flow.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if updater.calls != 1 {
		t.Fatalf("updater calls = %d, want 1", updater.calls)
	}
	if updater.req.BaseID != req.BaseID {
		t.Fatalf("BaseID = %q, want %q", updater.req.BaseID, req.BaseID)
	}
	if updater.req.TableID != req.TableID {
		t.Fatalf("TableID = %q, want %q", updater.req.TableID, req.TableID)
	}
	if string(updater.req.RecordID) != string(req.RecordID) {
		t.Fatalf("RecordID = %s, want %s", updater.req.RecordID, req.RecordID)
	}
	if updater.req.PathField != req.PathField {
		t.Fatalf("PathField = %q, want %q", updater.req.PathField, req.PathField)
	}
	if updater.req.PathValue != absPath {
		t.Fatalf("PathValue = %q, want %q", updater.req.PathValue, absPath)
	}
}

func TestFlowUpdateSelectedRequiresNocoDBConfig(t *testing.T) {
	relPath, absPath := writeRelativeTempFile(t)
	updater := &fakeUpdater{}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.UpdateSelected(ctx, relPath)
		}},
		Updater:      updater,
		AllowedRoots: []string{filepath.Dir(absPath)},
		NocoDBURL:    " ",
		NocoDBToken:  "token",
	}

	err := flow.Run(context.Background(), Request{})
	if !errors.Is(err, ErrNocoDBConfigRequired) {
		t.Fatalf("Run() error = %v, want ErrNocoDBConfigRequired", err)
	}
	if updater.calls != 0 {
		t.Fatalf("updater calls = %d, want 0", updater.calls)
	}
}

func TestFlowSurfacesOpenAndUpdateErrors(t *testing.T) {
	t.Run("open error", func(t *testing.T) {
		root := t.TempDir()
		currentPath := filepath.Join(root, "current.txt")
		if err := os.WriteFile(currentPath, []byte("content"), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		wantErr := errors.New("open failed")
		flow := &Flow{
			Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
				return controller.OpenCurrent(ctx)
			}},
			Opener:       &fakeOpener{err: wantErr},
			AllowedRoots: []string{root},
		}

		err := flow.Run(context.Background(), Request{CurrentPath: currentPath})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Run() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("update error", func(t *testing.T) {
		relPath, absPath := writeRelativeTempFile(t)
		wantErr := errors.New("update failed")
		flow := &Flow{
			Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
				return controller.UpdateSelected(ctx, relPath)
			}},
			Updater:      &fakeUpdater{err: wantErr},
			AllowedRoots: []string{filepath.Dir(absPath)},
			NocoDBURL:    "https://nocodb.example.test",
			NocoDBToken:  "token",
		}

		err := flow.Run(context.Background(), Request{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("Run() error = %v, want %v", err, wantErr)
		}
	})
}

func TestAsyncDispatcherLogsNilFlow(t *testing.T) {
	logger := newFakeLogger()
	dispatcher := NewAsyncDispatcher(nil, logger)

	dispatcher.Dispatch(Request{})

	got := waitForLog(t, logger)
	if got != "action flow is not configured" {
		t.Fatalf("log = %q, want nil flow message", got)
	}
}

func TestAsyncDispatcherLogsReturnedErrors(t *testing.T) {
	logger := newFakeLogger()
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return errors.New("runner failed")
		}},
	}
	dispatcher := NewAsyncDispatcher(flow, logger)

	dispatcher.Dispatch(Request{})

	got := waitForLog(t, logger)
	if !strings.Contains(got, "action flow failed: runner failed") {
		t.Fatalf("log = %q, want returned error", got)
	}
}

func TestAsyncDispatcherRecoversAndLogsPanics(t *testing.T) {
	logger := newFakeLogger()
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			panic("runner exploded")
		}},
	}
	dispatcher := NewAsyncDispatcher(flow, logger)

	dispatcher.Dispatch(Request{})

	got := waitForLog(t, logger)
	if !strings.Contains(got, "action flow panicked: runner exploded") {
		t.Fatalf("log = %q, want panic recovery message", got)
	}
}

func waitForLog(t *testing.T, logger *fakeLogger) string {
	t.Helper()

	select {
	case got := <-logger.messages:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for log message")
		return ""
	}
}

func writeRelativeTempFile(t *testing.T) (string, string) {
	t.Helper()

	dir, err := os.MkdirTemp(".", "flow-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("RemoveAll() error = %v", err)
		}
	})

	relPath := filepath.Join(dir, "selected.txt")
	if err := os.WriteFile(relPath, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	return relPath, absPath
}
