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
