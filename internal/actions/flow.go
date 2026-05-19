package actions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"noco-path-opener/internal/nocodb"
	"noco-path-opener/internal/pathauth"
)

var (
	ErrCurrentPathRequired  = errors.New("current_path is empty")
	ErrPathRequired         = errors.New("path is required")
	ErrPathNotAllowed       = errors.New("path not allowed")
	ErrPathDoesNotExist     = errors.New("path does not exist")
	ErrNocoDBConfigRequired = errors.New("nocodb_url and nocodb_token must be configured")
)

type Flow struct {
	Runner       Runner
	Opener       Opener
	Updater      Updater
	AllowedRoots []string
	NocoDBURL    string
	NocoDBToken  string
}

func (f *Flow) Run(ctx context.Context, req Request) error {
	if f.Runner == nil {
		return errors.New("runner is not configured")
	}

	return f.Runner.Run(ctx, req, flowController{
		flow: f,
		req:  req,
	})
}

type flowController struct {
	flow *Flow
	req  Request
}

func (c flowController) OpenCurrent(ctx context.Context) error {
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

func (c flowController) PreparePath(path string) (string, error) {
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

func (c flowController) UpdateSelected(ctx context.Context, path string) error {
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

	return c.flow.Updater.UpdateRecord(ctx, nocodb.UpdateRequest{
		BaseID:    c.req.BaseID,
		TableID:   c.req.TableID,
		RecordID:  c.req.RecordID,
		PathField: c.req.PathField,
		PathValue: absPath,
	})
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
	flow   *Flow
	logger asyncLogger
}

func NewAsyncDispatcher(flow *Flow, logger asyncLogger) *AsyncDispatcher {
	return &AsyncDispatcher{
		flow:   flow,
		logger: logger,
	}
}

func (d *AsyncDispatcher) Dispatch(req Request) {
	go func() {
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
}
