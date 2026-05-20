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

	"noco-path-opener/internal/nocodb"
	"noco-path-opener/internal/pathauth"
)

var (
	ErrCurrentPathRequired     = errors.New("current_path is empty")
	ErrPathRequired            = errors.New("path is required")
	ErrPathNotAllowed          = errors.New("path not allowed")
	ErrPathDoesNotExist        = errors.New("path does not exist")
	ErrNocoDBConfigRequired    = errors.New("nocodb_url and nocodb_token must be configured")
	ErrRemoteSyncUnavailable   = errors.New("remote sync is not available")
	ErrRemoteSyncTableMismatch = errors.New("同步配置与当前表不匹配")
	ErrLocalLookupEmpty        = errors.New("本地查询字段为空")
	ErrRemoteRecordNotFound    = errors.New("远端未找到匹配记录")
	ErrRemoteRecordAmbiguous   = errors.New("远端找到多条匹配记录")
)

type Flow struct {
	Runner           Runner
	Opener           Opener
	Updater          Updater
	LocalSyncClient  LocalSyncClient
	RemoteSyncClient RemoteSyncClient
	AllowedRoots     []string
	NocoDBURL        string
	NocoDBToken      string
	SyncProfiles     []SyncProfile
}

func (f *Flow) Run(ctx context.Context, req Request) error {
	if f.Runner == nil {
		return errors.New("runner is not configured")
	}

	req.SyncProfile = strings.TrimSpace(req.SyncProfile)
	req.RemoteSync = f.resolveSyncProfile(req.SyncProfile)

	controller := &flowController{
		flow: f,
		req:  req,
	}
	return f.Runner.Run(ctx, req, controller)
}

func (f *Flow) resolveSyncProfile(name string) *SyncProfile {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	for _, profile := range f.SyncProfiles {
		if strings.TrimSpace(profile.Name) != name {
			continue
		}

		normalized := normalizeSyncProfile(profile)
		return &normalized
	}

	return nil
}

type flowController struct {
	flow *Flow
	req  Request
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
	profile := c.req.RemoteSync
	if profile == nil {
		return ErrRemoteSyncUnavailable
	}

	normalized := normalizeSyncProfile(*profile)
	reqBaseID := strings.TrimSpace(c.req.BaseID)
	reqTableID := strings.TrimSpace(c.req.TableID)
	if reqBaseID != normalized.LocalBaseID || reqTableID != normalized.LocalTableID {
		return ErrRemoteSyncTableMismatch
	}
	if c.flow.LocalSyncClient == nil {
		return errors.New("local NocoDB sync client is not configured")
	}
	if c.flow.RemoteSyncClient == nil {
		return errors.New("remote NocoDB sync client is not configured")
	}

	localRecord, err := c.flow.LocalSyncClient.ReadRecord(ctx, nocodb.ReadRecordRequest{
		BaseID:   normalized.LocalBaseID,
		TableID:  normalized.LocalTableID,
		RecordID: c.req.RecordID,
	})
	if err != nil {
		return err
	}

	lookup, ok := lookupString(localRecord.Fields[normalized.LocalLookupField])
	if !ok {
		return ErrLocalLookupEmpty
	}

	remoteRecords, err := c.flow.RemoteSyncClient.QueryRecords(ctx, nocodb.QueryRecordsRequest{
		BaseID:  normalized.RemoteBaseID,
		TableID: normalized.RemoteTableID,
		Where:   nocodb.EqualWhere(normalized.RemoteLookupField, lookup),
	})
	if err != nil {
		return err
	}
	switch len(remoteRecords) {
	case 0:
		return ErrRemoteRecordNotFound
	case 1:
	default:
		return ErrRemoteRecordAmbiguous
	}

	fields, err := extractSyncFields(remoteRecords[0], normalized.SyncFields)
	if err != nil {
		return err
	}

	return c.flow.LocalSyncClient.UpdateFields(ctx, nocodb.UpdateFieldsRequest{
		BaseID:   normalized.LocalBaseID,
		TableID:  normalized.LocalTableID,
		RecordID: c.req.RecordID,
		Fields:   fields,
	})
}

func normalizeSyncProfile(profile SyncProfile) SyncProfile {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.LocalBaseID = strings.TrimSpace(profile.LocalBaseID)
	profile.LocalTableID = strings.TrimSpace(profile.LocalTableID)
	profile.LocalLookupField = strings.TrimSpace(profile.LocalLookupField)
	profile.RemoteBaseID = strings.TrimSpace(profile.RemoteBaseID)
	profile.RemoteTableID = strings.TrimSpace(profile.RemoteTableID)
	profile.RemoteLookupField = strings.TrimSpace(profile.RemoteLookupField)

	syncFields := make([]string, 0, len(profile.SyncFields))
	for _, field := range profile.SyncFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		syncFields = append(syncFields, field)
	}
	profile.SyncFields = syncFields

	return profile
}

func lookupString(raw json.RawMessage) (string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", false
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		return text, text != ""
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		value := strings.TrimSpace(number.String())
		return value, value != ""
	}

	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		if boolean {
			return "true", true
		}
		return "false", true
	}

	return "", false
}

func extractSyncFields(record nocodb.Record, syncFields []string) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage, len(syncFields))
	missing := make([]string, 0)

	for _, field := range syncFields {
		raw, ok := record.Fields[field]
		if !ok {
			missing = append(missing, field)
			continue
		}
		fields[field] = raw
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("远端记录缺少字段：%s", strings.Join(missing, "、"))
	}

	return fields, nil
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
