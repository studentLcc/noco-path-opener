package actions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"noco-path-opener/internal/remotesync"
)

type fakeDynamicRemoteSyncClient struct {
	fetchCalls   int
	fetchRequest remotesync.FetchRequest
	result       remotesync.Result
	fetchErr     error
	downloads    []remotesync.DownloadRequest
	downloadErr  error
	download     func(remotesync.DownloadRequest) error
}

func (f *fakeDynamicRemoteSyncClient) Fetch(ctx context.Context, req remotesync.FetchRequest) (remotesync.Result, error) {
	f.fetchCalls++
	f.fetchRequest = req
	return f.result, f.fetchErr
}

func (f *fakeDynamicRemoteSyncClient) Download(ctx context.Context, req remotesync.DownloadRequest) error {
	f.downloads = append(f.downloads, req)
	if f.download != nil {
		return f.download(req)
	}
	return f.downloadErr
}

func dynamicSpec() *remotesync.Spec {
	return &remotesync.Spec{
		PostURL:     "https://remote.test/list",
		GetURL:      "https://remote.test/detail/{id}",
		DownloadURL: "https://remote.test/file/{file_id}/{file_name}",
		ProcessCode: "process-1",
		InputField:  "input",
		FieldMapping: map[string]string{
			"name":         "本地名称",
			"designName":   "设计名称",
			"input_value":  "输入值",
			"file_uploads": "附件",
		},
	}
}

func dynamicResult(files ...remotesync.File) remotesync.Result {
	return remotesync.Result{
		Name:       "项目A",
		ID:         "remote-1",
		DesignName: "设计A",
		InputValue: json.RawMessage(`"input-value"`),
		Files:      files,
	}
}

func dynamicFlow(local *fakeLocalSyncClient, remote *fakeDynamicRemoteSyncClient, run func(actions Controller, ctx context.Context) error) *Flow {
	return &Flow{
		LocalSyncClient:         local,
		DynamicRemoteSyncClient: remote,
		Runner: fakeRunner{run: func(ctx context.Context, req Request, controller Controller) error {
			return run(controller, ctx)
		}},
	}
}

func setDynamicToken(controller Controller, ctx context.Context) error {
	if err := controller.SetRemoteToken("secret-token"); err != nil {
		return err
	}
	return controller.SyncRemote(ctx)
}

func setDynamicTokenWithDirectoryAction(action RemoteSyncDirectoryAction) func(Controller, context.Context) error {
	return func(controller Controller, ctx context.Context) error {
		if err := controller.SetRemoteToken("secret-token"); err != nil {
			return err
		}
		return controller.SyncRemoteWithDirectoryAction(ctx, action)
	}
}

func TestFlowDynamicRemoteSyncUsesCurrentPathForDownloads(t *testing.T) {
	directory := t.TempDir()
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "img001", Name: "图片1.png"})}
	flow := dynamicFlow(local, remote, setDynamicTokenWithDirectoryAction(RemoteSyncDirectoryOverwrite))

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		CurrentPath:       directory,
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if local.readCalls != 0 {
		t.Fatalf("local read calls = %d, want 0", local.readCalls)
	}
	if len(remote.downloads) != 1 || remote.downloads[0].Directory != directory {
		t.Fatalf("downloads = %#v, want current_path directory", remote.downloads)
	}
	if !remote.downloads[0].Overwrite {
		t.Fatalf("download request = %#v, want overwrite enabled", remote.downloads[0])
	}
	if local.updateCalls != 1 {
		t.Fatalf("local update calls = %d, want 1", local.updateCalls)
	}
	if string(local.updateReq.Fields["本地名称"]) != `"项目A"` || string(local.updateReq.Fields["设计名称"]) != `"设计A"` {
		t.Fatalf("updated identity fields = %#v", local.updateReq.Fields)
	}
	if string(local.updateReq.Fields["输入值"]) != `"input-value"` {
		t.Fatalf("updated input field = %s, want input value", local.updateReq.Fields["输入值"])
	}
}

func TestFlowDynamicRemoteSyncCreatesAndWritesCurrentPathWhenEmpty(t *testing.T) {
	base := t.TempDir()
	destination := filepath.Join(base, "P001")
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "1", Name: "report.pdf"})}
	remote.download = func(req remotesync.DownloadRequest) error {
		return os.WriteFile(filepath.Join(req.Directory, req.File.Name), []byte("content"), 0o600)
	}
	flow := dynamicFlow(local, remote, setDynamicToken)

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		PathField:         "本地文件路径",
		BaseDir:           base,
		FolderName:        "P001",
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if info, err := os.Stat(destination); err != nil || !info.IsDir() {
		t.Fatalf("destination = %q stat error = %v, want created directory", destination, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "report.pdf")); err != nil {
		t.Fatalf("downloaded file error = %v", err)
	}
	wantPathBytes, err := json.Marshal(destination)
	if err != nil {
		t.Fatalf("Marshal(destination) error = %v", err)
	}
	wantPath := string(wantPathBytes)
	if got := string(local.updateReq.Fields["本地文件路径"]); got != wantPath {
		t.Fatalf("path write-back = %s, want %s", got, wantPath)
	}
}

func TestFlowDynamicRemoteSyncKeepsCreatedFilesAndDirectoryAfterFailure(t *testing.T) {
	base := t.TempDir()
	destination := filepath.Join(base, "P001")
	local := &fakeLocalSyncClient{updateErr: errors.New("NocoDB patch failed")}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "1", Name: "first.txt"})}
	remote.download = func(req remotesync.DownloadRequest) error {
		return os.WriteFile(filepath.Join(req.Directory, req.File.Name), []byte("content"), 0o600)
	}
	flow := dynamicFlow(local, remote, setDynamicToken)

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		PathField:         "本地文件路径",
		BaseDir:           base,
		FolderName:        "P001",
		DynamicRemoteSync: dynamicSpec(),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want NocoDB update failure")
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("destination was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "first.txt")); err != nil {
		t.Fatalf("downloaded file was removed: %v", err)
	}
}

func TestFlowDynamicRemoteSyncPromptsBeforeUsingExistingDirectory(t *testing.T) {
	directory := t.TempDir()
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(
		remotesync.File{ID: "new", Name: "new.txt"},
	)}
	flow := dynamicFlow(local, remote, setDynamicToken)

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		CurrentPath:       directory,
		DynamicRemoteSync: dynamicSpec(),
	})
	if !errors.Is(err, ErrRemoteSyncDownloadDirectoryExists) {
		t.Fatalf("Run() error = %v, want ErrRemoteSyncDownloadDirectoryExists", err)
	}
	if len(remote.downloads) != 0 || local.updateCalls != 0 {
		t.Fatalf("downloads = %#v, updates = %d, want no side effects", remote.downloads, local.updateCalls)
	}
}

func TestFlowDynamicRemoteSyncReusesFetchedResultAfterDirectoryDecision(t *testing.T) {
	directory := t.TempDir()
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "new", Name: "new.txt"})}
	flow := dynamicFlow(local, remote, func(controller Controller, ctx context.Context) error {
		if err := controller.SetRemoteToken("secret-token"); err != nil {
			return err
		}
		if err := controller.SyncRemote(ctx); !errors.Is(err, ErrRemoteSyncDownloadDirectoryExists) {
			return err
		}
		return controller.SyncRemoteWithDirectoryAction(ctx, RemoteSyncDirectoryOverwrite)
	})

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		CurrentPath:       directory,
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if remote.fetchCalls != 1 {
		t.Fatalf("fetch calls = %d, want one remote fetch", remote.fetchCalls)
	}
	if len(remote.downloads) != 1 {
		t.Fatalf("downloads = %#v, want one download", remote.downloads)
	}
}

func TestFlowDynamicRemoteSyncSkipsExistingDirectoryDownloads(t *testing.T) {
	directory := t.TempDir()
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "new", Name: "new.txt"})}
	flow := dynamicFlow(local, remote, setDynamicTokenWithDirectoryAction(RemoteSyncDirectorySkip))

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		CurrentPath:       directory,
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(remote.downloads) != 0 {
		t.Fatalf("downloads = %#v, want no downloads", remote.downloads)
	}
	if local.updateCalls != 1 {
		t.Fatalf("update calls = %d, want mapped fields to be written", local.updateCalls)
	}
}

func TestFlowDynamicRemoteSyncOverwritesExistingFiles(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "existing.txt")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "existing", Name: "existing.txt"})}
	remote.download = func(req remotesync.DownloadRequest) error {
		if !req.Overwrite {
			t.Fatal("download overwrite = false, want true")
		}
		return os.WriteFile(filepath.Join(req.Directory, req.File.Name), []byte("remote"), 0o600)
	}
	flow := dynamicFlow(local, remote, setDynamicTokenWithDirectoryAction(RemoteSyncDirectoryOverwrite))

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		CurrentPath:       directory,
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "remote" {
		t.Fatalf("file content = %q, want remote", got)
	}
}

func TestFlowDynamicRemoteSyncPromptsForExistingGeneratedDirectory(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "P001")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "new", Name: "new.txt"})}
	flow := dynamicFlow(local, remote, setDynamicToken)

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		PathField:         "本地文件路径",
		BaseDir:           base,
		FolderName:        "P001",
		DynamicRemoteSync: dynamicSpec(),
	})
	if !errors.Is(err, ErrRemoteSyncDownloadDirectoryExists) {
		t.Fatalf("Run() error = %v, want ErrRemoteSyncDownloadDirectoryExists", err)
	}
}

func TestFlowDynamicRemoteSyncOverwritesExistingGeneratedDirectory(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "P001")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	destination := filepath.Join(directory, "report.pdf")
	if err := os.WriteFile(destination, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult(remotesync.File{ID: "1", Name: "report.pdf"})}
	remote.download = func(req remotesync.DownloadRequest) error {
		if !req.Overwrite {
			t.Fatal("download overwrite = false, want true")
		}
		return os.WriteFile(filepath.Join(req.Directory, req.File.Name), []byte("remote"), 0o600)
	}
	flow := dynamicFlow(local, remote, setDynamicTokenWithDirectoryAction(RemoteSyncDirectoryOverwrite))

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		PathField:         "本地文件路径",
		BaseDir:           base,
		FolderName:        "P001",
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "remote" {
		t.Fatalf("file content = %q, want remote", got)
	}
	pathValue, err := json.Marshal(directory)
	if err != nil {
		t.Fatalf("Marshal(directory) error = %v", err)
	}
	if got := string(local.updateReq.Fields["本地文件路径"]); got != string(pathValue) {
		t.Fatalf("path write-back = %s, want %s", got, pathValue)
	}
}

func TestFlowDynamicRemoteSyncReusesConfiguredToken(t *testing.T) {
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult()}
	flow := dynamicFlow(local, remote, func(controller Controller, ctx context.Context) error {
		if err := controller.SetRemoteToken("secret-token"); err != nil {
			return err
		}
		if err := controller.SyncRemote(ctx); err != nil {
			return err
		}
		return controller.SyncRemote(ctx)
	})

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if remote.fetchCalls != 2 {
		t.Fatalf("fetch calls = %d, want token reuse for two synchronizations", remote.fetchCalls)
	}
}

func TestFlowDynamicRemoteSyncWithoutFilesDoesNotCreateDirectory(t *testing.T) {
	base := t.TempDir()
	destination := filepath.Join(base, "P001")
	local := &fakeLocalSyncClient{}
	remote := &fakeDynamicRemoteSyncClient{result: dynamicResult()}
	flow := dynamicFlow(local, remote, setDynamicToken)

	err := flow.Run(context.Background(), Request{
		BaseID:            "base",
		TableID:           "table",
		RecordID:          json.RawMessage(`1`),
		PathField:         "本地文件路径",
		BaseDir:           base,
		FolderName:        "P001",
		DynamicRemoteSync: dynamicSpec(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination = %v, want no directory for a field-only synchronization", err)
	}
	if local.updateCalls != 1 {
		t.Fatalf("local update calls = %d, want 1", local.updateCalls)
	}
	if _, exists := local.updateReq.Fields["本地文件路径"]; exists {
		t.Fatalf("fields = %#v, want no path write-back without files", local.updateReq.Fields)
	}
}
