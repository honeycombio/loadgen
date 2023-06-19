package main

import (
	"context"

	"github.com/honeycombio/beeline-go"
)

type SenderHoneycomb struct{}

// make sure it implements Sender
var _ Sender = (*SenderHoneycomb)(nil)

func NewSenderHoneycomb(opts Options) *SenderHoneycomb {
	beeline.Init(beeline.Config{
		WriteKey:    opts.Telemetry.APIKey,
		APIHost:     opts.apihost.String(),
		ServiceName: opts.Telemetry.Dataset,
		Debug:       opts.DebugLevel() > 2,
	})
	return &SenderHoneycomb{}
}

func (t *SenderHoneycomb) Close() {
	beeline.Close()
}

func (t *SenderHoneycomb) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	// a beeline span is already a Sendable
	ctx, root := beeline.StartSpan(ctx, name)
	for k, v := range fielder.GetFields(count, 0) {
		root.AddField(k, v)
	}
	return ctx, root
}

func (t *SenderHoneycomb) CreateSpan(ctx context.Context, name string, level int, fielder *Fielder) (context.Context, Sendable) {
	// a beeline span is already a Sendable
	ctx, span := beeline.StartSpan(ctx, name)
	for k, v := range fielder.GetFields(0, level) {
		span.AddField(k, v)
	}
	return ctx, span
}
