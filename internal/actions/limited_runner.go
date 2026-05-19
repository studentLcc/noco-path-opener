package actions

import (
	"context"
	"errors"
)

type LimitedRunner struct {
	delegate Runner
	slots    chan struct{}
}

func NewLimitedRunner(delegate Runner, maxConcurrent int) *LimitedRunner {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &LimitedRunner{
		delegate: delegate,
		slots:    make(chan struct{}, maxConcurrent),
	}
}

func (r *LimitedRunner) Run(ctx context.Context, req Request, controller Controller) error {
	if r == nil || r.delegate == nil {
		return errors.New("runner is not configured")
	}

	select {
	case r.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() {
		<-r.slots
	}()

	return r.delegate.Run(ctx, req, controller)
}
