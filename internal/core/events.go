package core

import (
	"context"

	"github.com/PierreOliveira1/pandora/internal/events"
)

const (
	DefaultReader = "default"
	AIReader      = "ai"
)

func (c *Core) Events() *events.Queue {
	return c.events
}

func (c *Core) RecentEvents(ctx context.Context, f events.Filter) ([]events.Event, error) {
	return c.events.Recent(ctx, f)
}

func (c *Core) PendingEvents(ctx context.Context, reader string, f events.Filter) ([]events.Event, error) {
	return c.events.Pending(ctx, readerOr(reader), f)
}

func (c *Core) AckEvents(ctx context.Context, reader string, seqs []int64) error {
	return c.events.Ack(ctx, readerOr(reader), seqs...)
}

func (c *Core) AckAllEvents(ctx context.Context, reader string, f events.Filter) (int, error) {
	pending, err := c.events.Pending(ctx, readerOr(reader), f)
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}
	seqs := make([]int64, len(pending))
	for i, event := range pending {
		seqs[i] = event.Seq
	}
	if err := c.events.Ack(ctx, readerOr(reader), seqs...); err != nil {
		return 0, err
	}
	return len(seqs), nil
}

func readerOr(reader string) string {
	if reader == "" {
		return DefaultReader
	}
	return reader
}

func (c *Core) SkipPendingEvents(ctx context.Context, reader string) error {
	return c.events.SkipPending(ctx, readerOr(reader))
}
