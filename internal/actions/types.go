package actions

import (
	"context"
	"encoding/json"
	"strings"

	"noco-path-opener/internal/nocodb"
)

type Request struct {
	BaseID      string
	TableID     string
	RecordID    json.RawMessage
	PathField   string
	CurrentPath string
	BaseDir     string
	FolderName  string
	SyncProfile string
	RemoteSync  *SyncProfile
}

type SyncProfile struct {
	Name              string
	LocalBaseID       string
	LocalTableID      string
	LocalLookupField  string
	RemoteBaseID      string
	RemoteTableID     string
	RemoteLookupField string
	SyncFields        []string
}

func (r Request) HasRemoteSync() bool {
	return r.RemoteSync != nil
}

func (r Request) RowKey() (string, bool) {
	baseID := strings.TrimSpace(r.BaseID)
	tableID := strings.TrimSpace(r.TableID)
	recordID := rowRecordID(r.RecordID)
	if baseID == "" || tableID == "" || recordID == "" {
		return "", false
	}

	key, _ := json.Marshal([]string{baseID, tableID, recordID})
	return string(key), true
}

func (r Request) RowDisplayID() string {
	return rowRecordID(r.RecordID)
}

func rowRecordID(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}

	var value string
	if err := json.Unmarshal([]byte(trimmed), &value); err == nil {
		return strings.TrimSpace(value)
	}

	var number json.Number
	if err := json.Unmarshal([]byte(trimmed), &number); err == nil {
		return number.String()
	}

	return trimmed
}

type Dispatcher interface {
	Dispatch(req Request) error
}

type Controller interface {
	OpenCurrent(ctx context.Context) error
	PreparePath(path string) (string, error)
	UpdateSelected(ctx context.Context, path string) error
	UploadSelected(ctx context.Context, paths []string) error
	SyncRemote(ctx context.Context) error
}

type Runner interface {
	Run(ctx context.Context, req Request, controller Controller) error
}

type Opener interface {
	Open(path string, isDir bool) error
}

type Updater interface {
	UpdateRecord(ctx context.Context, req nocodb.UpdateRequest) error
}

type LocalSyncClient interface {
	ReadRecord(ctx context.Context, req nocodb.ReadRecordRequest) (nocodb.Record, error)
	UpdateFields(ctx context.Context, req nocodb.UpdateFieldsRequest) error
}

type RemoteSyncClient interface {
	QueryRecords(ctx context.Context, req nocodb.QueryRecordsRequest) ([]nocodb.Record, error)
}
