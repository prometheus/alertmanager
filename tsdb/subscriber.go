package tsdb

import (
	"context"
	"log/slog"

	"github.com/oklog/run"
)

type runFunc func(ctx context.Context)

// Subscriber is a generic subscriber that can be used to listen to marker events.
// Events are sent by different stores like Alerts provider and MemMarker.
type subscriber struct {
	cancel context.CancelFunc
	runFn  runFunc
	name   string
	logger *slog.Logger
}

func newSubscriber(name string, runFn runFunc, logger *slog.Logger) *subscriber {
	return &subscriber{
		runFn:  runFn,
		name:   name,
		logger: logger,
	}
}

func (s *subscriber) Run() {
	var (
		g   run.Group
		ctx context.Context
	)

	ctx, s.cancel = context.WithCancel(context.Background())
	runCtx, runCancel := context.WithCancel(ctx)

	g.Add(func() error {
		s.runFn(runCtx)
		return nil
	}, func(err error) {
		runCancel()
	})

	if err := g.Run(); err != nil {
		s.logger.Warn("error running subscriber", "name", s.name, "err", err)
	}
}

func (s *subscriber) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}
