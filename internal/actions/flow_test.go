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

type blockingRunner struct {
	enter    chan string
	release  chan struct{}
	enterAck chan struct{}
}

func (b blockingRunner) Run(ctx context.Context, req Request, controller Controller) error {
	b.enter <- req.CurrentPath
	<-b.release
	b.enterAck <- struct{}{}
	return nil
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

func TestFlowUpdateSelectedRefreshesCurrentPathForSubsequentOpen(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile(old) error = %v", err)
	}
	newPath := filepath.Join(root, "new.txt")
	if err := os.WriteFile(newPath, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFile(new) error = %v", err)
	}

	opener := &fakeOpener{}
	updater := &fakeUpdater{}
	req := Request{
		BaseID:      "base-1",
		TableID:     "table-1",
		RecordID:    json.RawMessage(`123`),
		PathField:   "LocalPath",
		CurrentPath: oldPath,
	}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			if err := controller.UpdateSelected(ctx, newPath); err != nil {
				return err
			}
			return controller.OpenCurrent(ctx)
		}},
		Opener:       opener,
		Updater:      updater,
		AllowedRoots: []string{root},
		NocoDBURL:    "https://nocodb.example.test",
		NocoDBToken:  "token",
	}

	if err := flow.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if opener.calls != 1 {
		t.Fatalf("opener calls = %d, want 1", opener.calls)
	}
	if opener.path != newPath {
		t.Fatalf("opener path = %q, want updated path %q", opener.path, newPath)
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

func TestLimitedRunnerLimitsConcurrentRuns(t *testing.T) {
	enter := make(chan string, 3)
	release := make(chan struct{})
	enterAck := make(chan struct{}, 3)
	runner := NewLimitedRunner(blockingRunner{
		enter:    enter,
		release:  release,
		enterAck: enterAck,
	}, 2)

	errs := make(chan error, 3)
	for i := 1; i <= 3; i++ {
		go func(i int) {
			errs <- runner.Run(context.Background(), Request{CurrentPath: fmt.Sprintf("req-%d", i)}, nil)
		}(i)
	}

	first := waitForEnter(t, enter)
	second := waitForEnter(t, enter)
	if first == second {
		t.Fatalf("first two entries should be distinct, got %q twice", first)
	}

	select {
	case got := <-enter:
		t.Fatalf("third run entered too early: %q", got)
	case <-time.After(150 * time.Millisecond):
	}

	release <- struct{}{}
	third := waitForEnter(t, enter)
	if third == first || third == second {
		t.Fatalf("third entry = %q, want a new request", third)
	}

	for i := 0; i < 2; i++ {
		release <- struct{}{}
	}
	for i := 0; i < 3; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for runner completion")
		}
	}

	if len(enterAck) != 3 {
		t.Fatalf("enter acknowledgements = %d, want 3", len(enterAck))
	}
}

func TestLimitedRunnerDefaultsToOneConcurrentRun(t *testing.T) {
	enter := make(chan string, 2)
	release := make(chan struct{})
	runner := NewLimitedRunner(blockingRunner{
		enter:    enter,
		release:  release,
		enterAck: make(chan struct{}, 2),
	}, 0)

	errs := make(chan error, 2)
	for i := 1; i <= 2; i++ {
		go func(i int) {
			errs <- runner.Run(context.Background(), Request{CurrentPath: fmt.Sprintf("req-%d", i)}, nil)
		}(i)
	}

	first := waitForEnter(t, enter)
	select {
	case got := <-enter:
		t.Fatalf("second run entered too early: %q after %q", got, first)
	case <-time.After(150 * time.Millisecond):
	}

	release <- struct{}{}
	second := waitForEnter(t, enter)
	if second == first {
		t.Fatalf("second entry = %q, want a different request", second)
	}

	release <- struct{}{}
	for i := 0; i < 2; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for runner completion")
		}
	}
}

func TestAsyncDispatcherIgnoresDuplicateRowWithoutStartingAnotherRun(t *testing.T) {
	entries := make(chan Request, 2)
	release := make(chan struct{})
	defer close(release)

	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			entries <- req
			<-release
			return nil
		}},
	}
	dispatcher := NewAsyncDispatcher(flow, nil)
	req := Request{BaseID: "base", TableID: "tbl", RecordID: json.RawMessage(`123`)}

	if err := dispatcher.Dispatch(req); err != nil {
		t.Fatalf("Dispatch() error = %v, want nil", err)
	}
	waitForRequest(t, entries)

	err := dispatcher.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch(duplicate) error = %v, want nil", err)
	}

	select {
	case got := <-entries:
		t.Fatalf("duplicate row entered runner: %+v", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAsyncDispatcherFocusesOpenWindowForDuplicateRow(t *testing.T) {
	entries := make(chan Request, 2)
	release := make(chan struct{})
	defer close(release)

	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			entries <- req
			<-release
			return nil
		}},
	}
	dispatcher := NewAsyncDispatcher(flow, nil)
	req := Request{BaseID: "base", TableID: "tbl", RecordID: json.RawMessage(`123`)}
	rowKey, ok := req.RowKey()
	if !ok {
		t.Fatal("request did not produce a row key")
	}
	focused := make(chan struct{}, 1)
	unregister := RegisterRowWindow(rowKey, func() {
		focused <- struct{}{}
	})
	defer unregister()

	if err := dispatcher.Dispatch(req); err != nil {
		t.Fatalf("Dispatch() error = %v, want nil", err)
	}
	waitForRequest(t, entries)

	if err := dispatcher.Dispatch(req); err != nil {
		t.Fatalf("Dispatch(duplicate) error = %v, want nil", err)
	}

	select {
	case <-focused:
	case <-time.After(2 * time.Second):
		t.Fatal("duplicate row did not focus the registered window")
	}
	select {
	case got := <-entries:
		t.Fatalf("duplicate row entered runner: %+v", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAsyncDispatcherAllowsDifferentRowsWhileOneRowIsActive(t *testing.T) {
	entries := make(chan Request, 2)
	release := make(chan struct{})
	defer close(release)

	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			entries <- req
			<-release
			return nil
		}},
	}
	dispatcher := NewAsyncDispatcher(flow, nil)

	if err := dispatcher.Dispatch(Request{BaseID: "base", TableID: "tbl", RecordID: json.RawMessage(`1`)}); err != nil {
		t.Fatalf("Dispatch(first) error = %v, want nil", err)
	}
	waitForRequest(t, entries)

	if err := dispatcher.Dispatch(Request{BaseID: "base", TableID: "tbl", RecordID: json.RawMessage(`2`)}); err != nil {
		t.Fatalf("Dispatch(second row) error = %v, want nil", err)
	}
	waitForRequest(t, entries)
}

func TestRequestRowDisplayIDUsesOnlyRecordID(t *testing.T) {
	req := Request{BaseID: "base", TableID: "tbl", RecordID: json.RawMessage(`"rec-001"`)}

	if got := req.RowDisplayID(); got != "rec-001" {
		t.Fatalf("RowDisplayID() = %q, want %q", got, "rec-001")
	}
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

func waitForEnter(t *testing.T, enter <-chan string) string {
	t.Helper()

	select {
	case got := <-enter:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner entry")
		return ""
	}
}

func waitForRequest(t *testing.T, entries <-chan Request) Request {
	t.Helper()

	select {
	case got := <-entries:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner entry")
		return Request{}
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
