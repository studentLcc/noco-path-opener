package actions

import (
	"context"
	"encoding/json"

	"noco-path-opener/internal/nocodb"
)

type Request struct {
	BaseID      string
	TableID     string
	RecordID    json.RawMessage
	PathField   string
	CurrentPath string
}

type Dispatcher interface {
	Dispatch(req Request)
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
