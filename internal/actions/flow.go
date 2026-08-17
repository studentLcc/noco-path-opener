package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"noco-path-opener/internal/nocodb"
	"noco-path-opener/internal/pathauth"
	"noco-path-opener/internal/remotesync"
)

var (
	ErrCurrentPathRequired               = errors.New("current_path is empty")
	ErrPathRequired                      = errors.New("path is required")
	ErrPathNotAllowed                    = errors.New("path not allowed")
	ErrPathDoesNotExist                  = errors.New("path does not exist")
	ErrNocoDBConfigRequired              = errors.New("nocodb_url and nocodb_token must be configured")
	ErrRemoteSyncUnavailable             = errors.New("remote sync is not available")
	ErrRemoteSyncTokenRequired           = errors.New("snc-token is required")
	ErrRemoteSyncDownloadDirectoryExists = errors.New("远程同步下载目录已存在")
	ErrRemoteSyncDownloadExists          = errors.New("远程同步下载目标已存在")
	ErrUploadDestinationNotDirectory     = errors.New("upload destination is not a directory")
	ErrUploadDestinationExists           = errors.New("upload destination already exists")
	ErrUploadFolderNameInvalid           = errors.New("upload folder name is invalid")
	ErrUploadSourceConflict              = errors.New("upload source conflicts with destination")
	ErrUploadWriteBackFailed             = errors.New("files uploaded but path write-back failed")
)

type Flow struct {
	Runner                  Runner
	Opener                  Opener
	Updater                 Updater
	LocalSyncClient         LocalSyncClient
	DynamicRemoteSyncClient DynamicRemoteSyncClient
	AllowedRoots            []string
	NocoDBURL               string
	NocoDBToken             string
	Logger                  asyncLogger
}

func (f *Flow) Run(ctx context.Context, req Request) error {
	if f.Runner == nil {
		return errors.New("runner is not configured")
	}

	if req.DynamicRemoteSync != nil {
		f.logf("remote sync mode=dynamic post_url=%q process_code=%q", req.DynamicRemoteSync.PostURL, req.DynamicRemoteSync.ProcessCode)
	} else {
		f.logf("remote sync mode=none")
	}

	controller := &flowController{
		flow: f,
		req:  req,
	}
	return f.Runner.Run(ctx, req, controller)
}

type flowController struct {
	flow              *Flow
	req               Request
	remoteToken       string
	pendingRemoteSync *pendingRemoteSync
}

type pendingRemoteSync struct {
	token  string
	result remotesync.Result
}

type RemoteSyncDirectoryExistsError struct {
	Directory string
}

func (e *RemoteSyncDirectoryExistsError) Error() string {
	return fmt.Sprintf("%s：%s", ErrRemoteSyncDownloadDirectoryExists, e.Directory)
}

func (e *RemoteSyncDirectoryExistsError) Unwrap() error {
	return ErrRemoteSyncDownloadDirectoryExists
}

func (c *flowController) SetRemoteToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrRemoteSyncTokenRequired
	}
	if token != c.remoteToken {
		c.pendingRemoteSync = nil
	}
	c.remoteToken = token
	return nil
}

func (c *flowController) OpenCurrent(ctx context.Context) error {
	path := strings.TrimSpace(c.req.CurrentPath)
	if path == "" {
		return ErrCurrentPathRequired
	}

	allowed, err := isAllowed(path, c.flow.AllowedRoots)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPathNotAllowed
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrPathDoesNotExist
		}
		return err
	}
	if c.flow.Opener == nil {
		return errors.New("opener is not configured")
	}

	return c.flow.Opener.Open(path, info.IsDir())
}

func (c *flowController) PreparePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ErrPathRequired
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	allowed, err := isAllowed(absPath, c.flow.AllowedRoots)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", ErrPathNotAllowed
	}

	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return "", ErrPathDoesNotExist
		}
		return "", err
	}

	return absPath, nil
}

func (c *flowController) UpdateSelected(ctx context.Context, path string) error {
	absPath, err := c.PreparePath(path)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.flow.NocoDBURL) == "" || strings.TrimSpace(c.flow.NocoDBToken) == "" {
		return ErrNocoDBConfigRequired
	}
	if c.flow.Updater == nil {
		return errors.New("updater is not configured")
	}

	if err := c.flow.Updater.UpdateRecord(ctx, nocodb.UpdateRequest{
		BaseID:    c.req.BaseID,
		TableID:   c.req.TableID,
		RecordID:  c.req.RecordID,
		PathField: c.req.PathField,
		PathValue: absPath,
	}); err != nil {
		return err
	}

	c.req.CurrentPath = absPath
	return nil
}

func (c *flowController) SyncRemote(ctx context.Context) error {
	return c.syncRemote(ctx, RemoteSyncDirectoryPrompt)
}

func (c *flowController) SyncRemoteWithDirectoryAction(ctx context.Context, action RemoteSyncDirectoryAction) error {
	switch action {
	case RemoteSyncDirectoryOverwrite, RemoteSyncDirectorySkip:
		return c.syncRemote(ctx, action)
	default:
		return errors.New("remote sync directory action is invalid")
	}
}

func (c *flowController) syncRemote(ctx context.Context, action RemoteSyncDirectoryAction) error {
	if c.req.DynamicRemoteSync == nil {
		return ErrRemoteSyncUnavailable
	}
	return c.syncDynamicRemote(ctx, *c.req.DynamicRemoteSync, action)
}

func (c *flowController) syncDynamicRemote(ctx context.Context, spec remotesync.Spec, action RemoteSyncDirectoryAction) error {
	if c.flow.LocalSyncClient == nil {
		return errors.New("local NocoDB sync client is not configured")
	}
	if c.flow.DynamicRemoteSyncClient == nil {
		return errors.New("dynamic remote sync client is not configured")
	}
	token := strings.TrimSpace(c.remoteToken)
	if token == "" {
		return ErrRemoteSyncTokenRequired
	}
	spec = normalizeDynamicRemoteSpec(spec)
	result, err := c.fetchRemoteSync(ctx, spec, token, action)
	if err != nil {
		return err
	}
	fields, err := remotesync.BuildMappedFields(result, spec.FieldMapping)
	if err != nil {
		return err
	}

	if len(result.Files) > 0 {
		directory, existingDirectory, err := c.remoteSyncDownloadDestination()
		if err != nil {
			return err
		}
		if existingDirectory && action == RemoteSyncDirectoryPrompt {
			c.pendingRemoteSync = &pendingRemoteSync{token: token, result: result}
			return &RemoteSyncDirectoryExistsError{Directory: directory}
		}
		if action == RemoteSyncDirectorySkip {
			return c.updateRemoteFields(ctx, fields)
		}
		overwrite := action == RemoteSyncDirectoryOverwrite
		if err := preflightRemoteDownloads(directory, result.Files, overwrite); err != nil {
			return err
		}
		if !existingDirectory {
			if err := os.Mkdir(directory, 0o755); err != nil {
				return fmt.Errorf("create remote sync destination: %w", err)
			}
		}
		for _, file := range result.Files {
			if err := c.flow.DynamicRemoteSyncClient.Download(ctx, remotesync.DownloadRequest{
				URL:       spec.DownloadURL,
				File:      file,
				Token:     token,
				Directory: directory,
				Overwrite: overwrite,
			}); err != nil {
				return err
			}
		}
		if strings.TrimSpace(c.req.CurrentPath) == "" {
			pathValue, err := json.Marshal(directory)
			if err != nil {
				return fmt.Errorf("encode remote sync directory: %w", err)
			}
			fields[c.req.PathField] = pathValue
		}
		if err := c.updateRemoteFields(ctx, fields); err != nil {
			return err
		}
		c.req.CurrentPath = directory
		return nil
	}

	return c.updateRemoteFields(ctx, fields)
}

func (c *flowController) fetchRemoteSync(ctx context.Context, spec remotesync.Spec, token string, action RemoteSyncDirectoryAction) (remotesync.Result, error) {
	if c.pendingRemoteSync != nil {
		pending := c.pendingRemoteSync
		c.pendingRemoteSync = nil
		if action != RemoteSyncDirectoryPrompt && pending.token == token {
			return pending.result, nil
		}
	}
	return c.flow.DynamicRemoteSyncClient.Fetch(ctx, remotesync.FetchRequest{
		PostURL:        spec.PostURL,
		GetURL:         spec.GetURL,
		ProcessCode:    spec.ProcessCode,
		InputField:     spec.InputField,
		Token:          token,
		RequestTimeout: requestTimeout(spec.RequestTimeoutSeconds),
	})
}

func (c *flowController) updateRemoteFields(ctx context.Context, fields map[string]json.RawMessage) error {
	if len(fields) == 0 {
		return nil
	}
	return c.flow.LocalSyncClient.UpdateFields(ctx, nocodb.UpdateFieldsRequest{
		BaseID:   c.req.BaseID,
		TableID:  c.req.TableID,
		RecordID: c.req.RecordID,
		Fields:   fields,
	})
}

func requestTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (f *Flow) logf(format string, values ...any) {
	if f != nil && f.Logger != nil {
		f.Logger.Printf(format, values...)
	}
}

func preflightRemoteDownloads(directory string, files []remotesync.File, overwrite bool) error {
	names := make(map[string]string, len(files))
	for _, file := range files {
		name, err := remotesync.ValidateFilename(file.Name)
		if err != nil {
			return err
		}
		key := strings.ToLower(name)
		if previous, exists := names[key]; exists {
			return fmt.Errorf("%w：%s 与 %s", ErrRemoteSyncDownloadExists, previous, name)
		}
		names[key] = name

		destination := filepath.Join(directory, name)
		info, err := os.Lstat(destination)
		if err == nil {
			if !overwrite {
				return fmt.Errorf("%w：%s", ErrRemoteSyncDownloadExists, destination)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("remote sync overwrite target is not a regular file: %s", destination)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (c *flowController) remoteSyncDownloadDestination() (string, bool, error) {
	current := strings.TrimSpace(c.req.CurrentPath)
	if current != "" {
		prepared, err := c.PreparePath(current)
		if err != nil {
			return "", false, err
		}
		info, err := os.Stat(prepared)
		if err != nil {
			return "", false, err
		}
		if !info.IsDir() {
			return "", false, ErrUploadDestinationNotDirectory
		}
		return prepared, true, nil
	}

	baseDir := strings.TrimSpace(c.req.BaseDir)
	if baseDir == "" {
		return "", false, errors.New("base_dir is required for remote sync downloads")
	}
	folderName := strings.TrimSpace(c.req.FolderName)
	if !validUploadFolderName(folderName) {
		return "", false, ErrUploadFolderNameInvalid
	}
	base, err := c.PreparePath(baseDir)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(base)
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, ErrUploadDestinationNotDirectory
	}

	directory := filepath.Join(base, folderName)
	allowed, err := isAllowed(directory, c.flow.AllowedRoots)
	if err != nil {
		return "", false, err
	}
	if !allowed {
		return "", false, ErrPathNotAllowed
	}
	info, err = os.Stat(directory)
	if err == nil {
		if !info.IsDir() {
			return "", false, ErrUploadDestinationNotDirectory
		}
		return directory, true, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	return directory, false, nil
}

func normalizeDynamicRemoteSpec(spec remotesync.Spec) remotesync.Spec {
	spec.PostURL = strings.TrimSpace(spec.PostURL)
	spec.GetURL = strings.TrimSpace(spec.GetURL)
	spec.DownloadURL = strings.TrimSpace(spec.DownloadURL)
	spec.ProcessCode = strings.TrimSpace(spec.ProcessCode)
	spec.InputField = strings.TrimSpace(spec.InputField)
	return spec
}

func isAllowed(path string, allowedRoots []string) (bool, error) {
	if len(allowedRoots) == 0 || !filepath.IsAbs(path) {
		return pathauth.IsAllowed(path, allowedRoots)
	}

	normalizedRoots := make([]string, 0, len(allowedRoots))
	for _, root := range allowedRoots {
		if root == "" || filepath.IsAbs(root) {
			normalizedRoots = append(normalizedRoots, root)
			continue
		}

		absRoot, err := filepath.Abs(root)
		if err != nil {
			return false, err
		}
		normalizedRoots = append(normalizedRoots, absRoot)
	}

	return pathauth.IsAllowed(path, normalizedRoots)
}

type asyncLogger interface {
	Printf(format string, v ...any)
}

type AsyncDispatcher struct {
	flow     *Flow
	logger   asyncLogger
	mu       sync.Mutex
	openRows map[string]struct{}
}

func NewAsyncDispatcher(flow *Flow, logger asyncLogger) *AsyncDispatcher {
	return &AsyncDispatcher{
		flow:     flow,
		logger:   logger,
		openRows: make(map[string]struct{}),
	}
}

func (d *AsyncDispatcher) Dispatch(req Request) error {
	if d == nil {
		return errors.New("dispatcher is not configured")
	}

	rowKey, hasRowKey := req.RowKey()
	if hasRowKey && !d.reserveRow(rowKey) {
		FocusRowWindow(rowKey)
		return nil
	}

	go func() {
		if hasRowKey {
			defer d.releaseRow(rowKey)
		}
		defer func() {
			if recovered := recover(); recovered != nil && d.logger != nil {
				d.logger.Printf("action flow panicked: %v", recovered)
			}
		}()

		if d.flow == nil {
			if d.logger != nil {
				d.logger.Printf("action flow is not configured")
			}
			return
		}

		if err := d.flow.Run(context.Background(), req); err != nil && d.logger != nil {
			d.logger.Printf("action flow failed: %v", err)
		}
	}()

	return nil
}

func (d *AsyncDispatcher) reserveRow(rowKey string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.openRows == nil {
		d.openRows = make(map[string]struct{})
	}
	if _, exists := d.openRows[rowKey]; exists {
		return false
	}
	d.openRows[rowKey] = struct{}{}
	return true
}

func (d *AsyncDispatcher) releaseRow(rowKey string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.openRows, rowKey)
}
