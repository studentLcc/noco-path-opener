//go:build !windows

package gui

import (
	"context"
	"fmt"

	"noco-path-opener/internal/actions"
)

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (Runner) Run(ctx context.Context, req actions.Request, controller actions.Controller) error {
	return fmt.Errorf("GUI is only supported on Windows")
}
