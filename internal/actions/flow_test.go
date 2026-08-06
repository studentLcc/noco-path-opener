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

type fakeLocalSyncClient struct {
	readCalls   int
	readReq     nocodb.ReadRecordRequest
	readRecord  nocodb.Record
	readErr     error
	updateCalls int
	updateReq   nocodb.UpdateFieldsRequest
	updateErr   error
}

func (f *fakeLocalSyncClient) ReadRecord(ctx context.Context, req nocodb.ReadRecordRequest) (nocodb.Record, error) {
	f.readCalls++
	f.readReq = req
	return f.readRecord, f.readErr
}

func (f *fakeLocalSyncClient) UpdateFields(ctx context.Context, req nocodb.UpdateFieldsRequest) error {
	f.updateCalls++
	f.updateReq = req
	return f.updateErr
}

type fakeRemoteSyncClient struct {
	queryCalls int
	queryReq   nocodb.QueryRecordsRequest
	records    []nocodb.Record
	err        error
}

func (f *fakeRemoteSyncClient) QueryRecords(ctx context.Context, req nocodb.QueryRecordsRequest) ([]nocodb.Record, error) {
	f.queryCalls++
	f.queryReq = req
	return f.records, f.err
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

func validSyncProfile() SyncProfile {
	return SyncProfile{
		Name:              "docs",
		LocalBaseID:       "local-base",
		LocalTableID:      "local-table",
		LocalLookupField:  "FileKey",
		RemoteBaseID:      "remote-base",
		RemoteTableID:     "remote-table",
		RemoteLookupField: "RemoteKey",
		SyncFields:        []string{"Status", "Owner"},
	}
}

func TestFlowRunResolvesKnownSyncProfileBeforeRunnerStarts(t *testing.T) {
	profile := validSyncProfile()
	flow := &Flow{
		SyncProfiles: []SyncProfile{profile},
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			if req.RemoteSync == nil {
				t.Fatal("RemoteSync = nil, want resolved profile")
			}
			if req.RemoteSync.Name != profile.Name {
				t.Fatalf("RemoteSync.Name = %q, want %q", req.RemoteSync.Name, profile.Name)
			}
			if req.RemoteSync.LocalLookupField != profile.LocalLookupField {
				t.Fatalf("RemoteSync.LocalLookupField = %q, want %q", req.RemoteSync.LocalLookupField, profile.LocalLookupField)
			}
			if !req.HasRemoteSync() {
				t.Fatal("HasRemoteSync() = false, want true")
			}
			return nil
		}},
	}

	err := flow.Run(context.Background(), Request{SyncProfile: " docs "})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestFlowRunLeavesBlankOrUnknownSyncProfileUnresolved(t *testing.T) {
	profile := validSyncProfile()
	tests := []struct {
		name        string
		profileName string
	}{
		{name: "blank", profileName: " "},
		{name: "unknown", profileName: "missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flow := &Flow{
				SyncProfiles: []SyncProfile{profile},
				Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
					if req.RemoteSync != nil {
						t.Fatalf("RemoteSync = %+v, want nil", req.RemoteSync)
					}
					if req.HasRemoteSync() {
						t.Fatal("HasRemoteSync() = true, want false")
					}
					return nil
				}},
			}

			err := flow.Run(context.Background(), Request{SyncProfile: tt.profileName})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
		})
	}
}

func TestFlowRunDoesNotTrustInboundRemoteSyncForUnknownProfile(t *testing.T) {
	profile := validSyncProfile()
	flow := &Flow{
		SyncProfiles: []SyncProfile{profile},
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			if req.RemoteSync != nil {
				t.Fatalf("RemoteSync = %+v, want nil", req.RemoteSync)
			}
			if req.HasRemoteSync() {
				t.Fatal("HasRemoteSync() = true, want false")
			}
			return nil
		}},
	}

	err := flow.Run(context.Background(), Request{
		SyncProfile: "missing",
		RemoteSync:  &profile,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestFlowSyncRemoteRequiresResolvedProfile(t *testing.T) {
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.SyncRemote(ctx)
		}},
	}

	err := flow.Run(context.Background(), Request{})
	if !errors.Is(err, ErrRemoteSyncUnavailable) {
		t.Fatalf("Run() error = %v, want ErrRemoteSyncUnavailable", err)
	}
}

func TestFlowSyncRemoteRequiresMatchingLocalTable(t *testing.T) {
	profile := validSyncProfile()
	t.Run("trimmed mismatch still fails", func(t *testing.T) {
		flow := &Flow{
			SyncProfiles: []SyncProfile{profile},
			Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
				return controller.SyncRemote(ctx)
			}},
		}

		err := flow.Run(context.Background(), Request{
			BaseID:      "  other-base  ",
			TableID:     "  " + profile.LocalTableID + "  ",
			RecordID:    json.RawMessage(`1`),
			SyncProfile: profile.Name,
		})
		if !errors.Is(err, ErrRemoteSyncTableMismatch) {
			t.Fatalf("Run() error = %v, want ErrRemoteSyncTableMismatch", err)
		}
	})

	t.Run("trimmed request values are accepted", func(t *testing.T) {
		local := &fakeLocalSyncClient{readRecord: nocodb.Record{Fields: map[string]json.RawMessage{
			profile.LocalLookupField: json.RawMessage(`"doc-1"`),
		}}}
		remote := &fakeRemoteSyncClient{records: []nocodb.Record{{
			Fields: map[string]json.RawMessage{
				"Status": json.RawMessage(`"ready"`),
				"Owner":  json.RawMessage(`"Ada"`),
			},
		}}}
		flow := &Flow{
			LocalSyncClient:  local,
			RemoteSyncClient: remote,
			SyncProfiles:     []SyncProfile{profile},
			Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
				return controller.SyncRemote(ctx)
			}},
		}

		err := flow.Run(context.Background(), Request{
			BaseID:      "  " + profile.LocalBaseID + "  ",
			TableID:     "  " + profile.LocalTableID + "  ",
			RecordID:    json.RawMessage(`1`),
			SyncProfile: profile.Name,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if local.updateCalls != 1 {
			t.Fatalf("local update calls = %d, want 1", local.updateCalls)
		}
	})
}

func TestFlowSyncRemoteRequiresClients(t *testing.T) {
	profile := validSyncProfile()
	req := Request{
		BaseID:      profile.LocalBaseID,
		TableID:     profile.LocalTableID,
		RecordID:    json.RawMessage(`1`),
		SyncProfile: profile.Name,
	}

	t.Run("missing local client", func(t *testing.T) {
		flow := &Flow{
			RemoteSyncClient: &fakeRemoteSyncClient{},
			SyncProfiles:     []SyncProfile{profile},
			Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
				return controller.SyncRemote(ctx)
			}},
		}

		err := flow.Run(context.Background(), req)
		if err == nil || err.Error() != "local NocoDB sync client is not configured" {
			t.Fatalf("Run() error = %v, want exact local client config error", err)
		}
	})

	t.Run("missing remote client", func(t *testing.T) {
		flow := &Flow{
			LocalSyncClient: &fakeLocalSyncClient{},
			SyncProfiles:    []SyncProfile{profile},
			Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
				return controller.SyncRemote(ctx)
			}},
		}

		err := flow.Run(context.Background(), req)
		if err == nil || err.Error() != "remote NocoDB sync client is not configured" {
			t.Fatalf("Run() error = %v, want exact remote client config error", err)
		}
	})
}

func TestFlowSyncRemoteRequiresLocalLookupValue(t *testing.T) {
	profile := validSyncProfile()
	tests := []struct {
		name   string
		fields map[string]json.RawMessage
	}{
		{name: "missing", fields: map[string]json.RawMessage{}},
		{name: "blank string", fields: map[string]json.RawMessage{"FileKey": json.RawMessage(`"   "`)}},
		{name: "null", fields: map[string]json.RawMessage{"FileKey": json.RawMessage(`null`)}},
		{name: "object", fields: map[string]json.RawMessage{"FileKey": json.RawMessage(`{"value":"doc-1"}`)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := &fakeLocalSyncClient{readRecord: nocodb.Record{Fields: tt.fields}}
			remote := &fakeRemoteSyncClient{}
			flow := &Flow{
				LocalSyncClient:  local,
				RemoteSyncClient: remote,
				SyncProfiles:     []SyncProfile{profile},
				Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
					return controller.SyncRemote(ctx)
				}},
			}

			err := flow.Run(context.Background(), Request{
				BaseID:      profile.LocalBaseID,
				TableID:     profile.LocalTableID,
				RecordID:    json.RawMessage(`1`),
				SyncProfile: profile.Name,
			})
			if !errors.Is(err, ErrLocalLookupEmpty) {
				t.Fatalf("Run() error = %v, want ErrLocalLookupEmpty", err)
			}
			if remote.queryCalls != 0 {
				t.Fatalf("remote query calls = %d, want 0", remote.queryCalls)
			}
			if local.updateCalls != 0 {
				t.Fatalf("local update calls = %d, want 0", local.updateCalls)
			}
		})
	}
}

func TestFlowSyncRemoteRequiresExactlyOneRemoteRecord(t *testing.T) {
	profile := validSyncProfile()
	tests := []struct {
		name    string
		records []nocodb.Record
		wantErr error
	}{
		{name: "none", records: nil, wantErr: ErrRemoteRecordNotFound},
		{name: "multiple", records: []nocodb.Record{{Fields: map[string]json.RawMessage{}}, {Fields: map[string]json.RawMessage{}}}, wantErr: ErrRemoteRecordAmbiguous},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := &fakeLocalSyncClient{readRecord: nocodb.Record{Fields: map[string]json.RawMessage{
				"FileKey": json.RawMessage(`"doc-1"`),
			}}}
			remote := &fakeRemoteSyncClient{records: tt.records}
			flow := &Flow{
				LocalSyncClient:  local,
				RemoteSyncClient: remote,
				SyncProfiles:     []SyncProfile{profile},
				Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
					return controller.SyncRemote(ctx)
				}},
			}

			err := flow.Run(context.Background(), Request{
				BaseID:      profile.LocalBaseID,
				TableID:     profile.LocalTableID,
				RecordID:    json.RawMessage(`1`),
				SyncProfile: profile.Name,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if local.updateCalls != 0 {
				t.Fatalf("local update calls = %d, want 0", local.updateCalls)
			}
		})
	}
}

func TestFlowSyncRemoteRejectsMissingRemoteFieldsWithoutUpdatingLocal(t *testing.T) {
	profile := validSyncProfile()
	local := &fakeLocalSyncClient{readRecord: nocodb.Record{Fields: map[string]json.RawMessage{
		"FileKey": json.RawMessage(`"doc-1"`),
	}}}
	remote := &fakeRemoteSyncClient{records: []nocodb.Record{{
		Fields: map[string]json.RawMessage{
			"Status": json.RawMessage(`"ready"`),
		},
	}}}
	flow := &Flow{
		LocalSyncClient:  local,
		RemoteSyncClient: remote,
		SyncProfiles:     []SyncProfile{profile},
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.SyncRemote(ctx)
		}},
	}

	err := flow.Run(context.Background(), Request{
		BaseID:      profile.LocalBaseID,
		TableID:     profile.LocalTableID,
		RecordID:    json.RawMessage(`1`),
		SyncProfile: profile.Name,
	})
	if err == nil || err.Error() != "远端记录缺少字段：Owner" {
		t.Fatalf("Run() error = %v, want exact missing field error", err)
	}
	if local.updateCalls != 0 {
		t.Fatalf("local update calls = %d, want 0", local.updateCalls)
	}
}

func TestFlowSyncRemoteRejectsMultipleMissingRemoteFieldsWithExactMessage(t *testing.T) {
	profile := validSyncProfile()
	local := &fakeLocalSyncClient{readRecord: nocodb.Record{Fields: map[string]json.RawMessage{
		"FileKey": json.RawMessage(`"doc-1"`),
	}}}
	remote := &fakeRemoteSyncClient{records: []nocodb.Record{{
		Fields: map[string]json.RawMessage{},
	}}}
	flow := &Flow{
		LocalSyncClient:  local,
		RemoteSyncClient: remote,
		SyncProfiles:     []SyncProfile{profile},
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.SyncRemote(ctx)
		}},
	}

	err := flow.Run(context.Background(), Request{
		BaseID:      profile.LocalBaseID,
		TableID:     profile.LocalTableID,
		RecordID:    json.RawMessage(`1`),
		SyncProfile: profile.Name,
	})
	if err == nil || err.Error() != "远端记录缺少字段：Status、Owner" {
		t.Fatalf("Run() error = %v, want exact missing fields error", err)
	}
	if local.updateCalls != 0 {
		t.Fatalf("local update calls = %d, want 0", local.updateCalls)
	}
}

func TestFlowSyncRemoteUpdatesConfiguredFieldsFromRemoteRecord(t *testing.T) {
	profile := validSyncProfile()
	local := &fakeLocalSyncClient{readRecord: nocodb.Record{Fields: map[string]json.RawMessage{
		"FileKey": json.RawMessage(`12345`),
	}}}
	remoteFields := map[string]json.RawMessage{
		"Status":  json.RawMessage(`"ready"`),
		"Owner":   json.RawMessage(`{"name":"Ada"}`),
		"Ignored": json.RawMessage(`"do-not-copy"`),
	}
	remote := &fakeRemoteSyncClient{records: []nocodb.Record{{Fields: remoteFields}}}
	flow := &Flow{
		LocalSyncClient:  local,
		RemoteSyncClient: remote,
		SyncProfiles:     []SyncProfile{profile},
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.SyncRemote(ctx)
		}},
	}
	req := Request{
		BaseID:      profile.LocalBaseID,
		TableID:     profile.LocalTableID,
		RecordID:    json.RawMessage(`1`),
		SyncProfile: profile.Name,
	}

	err := flow.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if local.readCalls != 1 {
		t.Fatalf("local read calls = %d, want 1", local.readCalls)
	}
	if local.readReq.BaseID != profile.LocalBaseID || local.readReq.TableID != profile.LocalTableID || string(local.readReq.RecordID) != string(req.RecordID) {
		t.Fatalf("local read request = %+v, want profile local table and request record ID", local.readReq)
	}
	if remote.queryCalls != 1 {
		t.Fatalf("remote query calls = %d, want 1", remote.queryCalls)
	}
	wantWhere := nocodb.EqualWhere(profile.RemoteLookupField, "12345")
	if remote.queryReq.BaseID != profile.RemoteBaseID || remote.queryReq.TableID != profile.RemoteTableID || remote.queryReq.Where != wantWhere {
		t.Fatalf("remote query request = %+v, want base %q table %q where %q", remote.queryReq, profile.RemoteBaseID, profile.RemoteTableID, wantWhere)
	}
	if local.updateCalls != 1 {
		t.Fatalf("local update calls = %d, want 1", local.updateCalls)
	}
	if local.updateReq.BaseID != profile.LocalBaseID || local.updateReq.TableID != profile.LocalTableID || string(local.updateReq.RecordID) != string(req.RecordID) {
		t.Fatalf("local update request = %+v, want profile local table and request record ID", local.updateReq)
	}
	if len(local.updateReq.Fields) != 2 {
		t.Fatalf("updated fields = %v, want exactly 2 configured fields", local.updateReq.Fields)
	}
	for _, field := range profile.SyncFields {
		if string(local.updateReq.Fields[field]) != string(remoteFields[field]) {
			t.Fatalf("updated field %q = %s, want raw remote value %s", field, local.updateReq.Fields[field], remoteFields[field])
		}
	}
	if _, ok := local.updateReq.Fields["Ignored"]; ok {
		t.Fatal("updated fields include unconfigured field Ignored")
	}
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

func TestFlowUploadSelectedCreatesNamedDirectoryCopiesMultipleSourcesAndUpdatesPath(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "uploads")
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(baseDir) error = %v", err)
	}
	filePath := filepath.Join(root, "one.txt")
	sourceDir := filepath.Join(root, "source-dir")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("one"), 0o600); err != nil {
		t.Fatalf("WriteFile(file) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "two.txt"), []byte("two"), 0o600); err != nil {
		t.Fatalf("WriteFile(source child) error = %v", err)
	}
	sourceInfo, err := os.Stat(filepath.Join(sourceDir, "two.txt"))
	if err != nil {
		t.Fatalf("Stat(source child) error = %v", err)
	}

	updater := &fakeUpdater{}
	req := Request{
		BaseID:     "base-1",
		TableID:    "table-1",
		RecordID:   json.RawMessage(`"rec-1"`),
		PathField:  "LocalPath",
		BaseDir:    baseDir,
		FolderName: "P001",
	}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.UploadSelected(ctx, []string{filePath, sourceDir})
		}},
		Updater:      updater,
		AllowedRoots: []string{root},
		NocoDBURL:    "https://nocodb.example.test",
		NocoDBToken:  "token",
	}

	if err := flow.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	destination := filepath.Join(baseDir, "P001")
	if got, err := os.ReadFile(filepath.Join(destination, "one.txt")); err != nil || string(got) != "one" {
		t.Fatalf("uploaded file = %q, %v; want one", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(destination, "source-dir", "two.txt")); err != nil || string(got) != "two" {
		t.Fatalf("uploaded directory child = %q, %v; want two", got, err)
	}
	if info, err := os.Stat(filepath.Join(destination, "source-dir", "two.txt")); err != nil || info.Mode().Perm() != sourceInfo.Mode().Perm() {
		t.Fatalf("uploaded directory child mode = %v, %v; want source mode %v", info.Mode(), err, sourceInfo.Mode().Perm())
	}
	if updater.calls != 1 || updater.req.PathValue != destination {
		t.Fatalf("updater = %+v, calls=%d; want destination %q once", updater.req, updater.calls, destination)
	}
}

func TestFlowUploadSelectedPreflightsAllConflictsBeforeCopying(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatalf("MkdirAll(destination) error = %v", err)
	}
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatalf("WriteFile(first) error = %v", err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(destination, "second.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	updater := &fakeUpdater{}
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.UploadSelected(ctx, []string{first, second})
		}},
		Updater:      updater,
		AllowedRoots: []string{root},
		NocoDBURL:    "https://nocodb.example.test",
		NocoDBToken:  "token",
	}

	err := flow.Run(context.Background(), Request{CurrentPath: destination, PathField: "LocalPath"})
	if err == nil || !strings.Contains(err.Error(), "upload target already exists") {
		t.Fatalf("Run() error = %v, want target conflict", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "first.txt")); !os.IsNotExist(err) {
		t.Fatalf("first file exists after preflight failure, want no partial copy")
	}
	if updater.calls != 0 {
		t.Fatalf("updater calls = %d, want 0", updater.calls)
	}
}

func TestFlowUploadSelectedRejectsExistingNamedDirectory(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "uploads")
	destination := filepath.Join(baseDir, "P001")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatalf("MkdirAll(destination) error = %v", err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}

	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.UploadSelected(ctx, []string{source})
		}},
		Updater:      &fakeUpdater{},
		AllowedRoots: []string{root},
		NocoDBURL:    "https://nocodb.example.test",
		NocoDBToken:  "token",
	}
	err := flow.Run(context.Background(), Request{BaseDir: baseDir, FolderName: "P001"})
	if !errors.Is(err, ErrUploadDestinationExists) {
		t.Fatalf("Run() error = %v, want ErrUploadDestinationExists", err)
	}
}

func TestFlowUploadSelectedReportsWriteBackFailureAfterCopy(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatalf("MkdirAll(destination) error = %v", err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	wantErr := errors.New("noco update failed")
	flow := &Flow{
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return controller.UploadSelected(ctx, []string{source})
		}},
		Updater:      &fakeUpdater{err: wantErr},
		AllowedRoots: []string{root},
		NocoDBURL:    "https://nocodb.example.test",
		NocoDBToken:  "token",
	}

	err := flow.Run(context.Background(), Request{CurrentPath: destination, PathField: "LocalPath"})
	if !errors.Is(err, ErrUploadWriteBackFailed) || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("Run() error = %v, want write-back context", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "source.txt")); err != nil {
		t.Fatalf("uploaded file missing after write-back failure: %v", err)
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
