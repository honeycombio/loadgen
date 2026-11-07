package main

import (
	"context"
	"time"

	"github.com/honeycombio/beeline-go"
	beelinetrace "github.com/honeycombio/beeline-go/trace"
	"github.com/honeycombio/libhoney-go"
	"github.com/honeycombio/libhoney-go/transmission"
)

// SenderHoneycomb sends spans through the beeline, which timestamps them
// itself. Spans with an explicit start time (from backfill mode) are built as
// libhoney events instead, because the beeline can't backdate them.
type SenderHoneycomb struct {
	client    *libhoney.Client
	log       Logger
	responses chan map[int]int // status code counts, when backfilling
}

// make sure it implements Sender
var _ Sender = (*SenderHoneycomb)(nil)

func NewSenderHoneycomb(log Logger, opts *Options) *SenderHoneycomb {
	tx := &transmission.Honeycomb{
		EnableMsgpackEncoding: opts.Output.Msgpack,
		MaxBatchSize:          uint(libhoney.DefaultMaxBatchSize),
		BatchTimeout:          libhoney.DefaultBatchTimeout,
	}
	if opts.Output.MaxExportBatchSize != 0 {
		tx.MaxBatchSize = uint(opts.Output.MaxExportBatchSize)
	}
	if opts.Backfill.Enabled() {
		// backfill runs as fast as it can send, so wait for the queue instead of dropping events
		tx.BlockOnSend = true
		tx.PendingWorkCapacity = libhoney.DefaultPendingWorkCapacity
		tx.MaxConcurrentBatches = libhoney.DefaultMaxConcurrentBatches
	}
	sender := newSenderHoneycombWithTransmission(opts, tx)
	sender.log = log
	if opts.Backfill.Enabled() && opts.DebugLevel() <= 2 {
		// count the API's responses so that rejected events don't go unnoticed
		// (in debug mode the beeline reads them instead)
		sender.responses = make(chan map[int]int, 1)
		go func() {
			counts := map[int]int{}
			for r := range sender.client.TxResponses() {
				counts[r.StatusCode]++
			}
			sender.responses <- counts
		}()
	}
	return sender
}

func newSenderHoneycombWithTransmission(opts *Options, tx transmission.Sender) *SenderHoneycomb {
	libhoneyClient, _ := libhoney.NewClient(libhoney.ClientConfig{
		APIKey:       opts.Telemetry.APIKey,
		Dataset:      opts.Telemetry.Dataset[0],
		APIHost:      opts.apihost.String(),
		Transmission: tx,
	})
	beeline.Init(beeline.Config{
		WriteKey:    opts.Telemetry.APIKey,
		APIHost:     opts.apihost.String(),
		ServiceName: opts.Telemetry.Dataset[0],
		Debug:       opts.DebugLevel() > 2,
		Client:      libhoneyClient,
	})
	return &SenderHoneycomb{client: libhoneyClient}
}

func (t *SenderHoneycomb) Close() {
	beeline.Close()
	if t.responses != nil {
		// status 0 means the event never got a response, e.g. a network error
		t.log.Warn("honeycomb responses by status code: %v\n", <-t.responses)
	}
}

type beelineSendable struct {
	*beelinetrace.Span
}

// Send ignores end because the beeline always ends spans now.
func (s beelineSendable) Send(end time.Time) {
	s.Span.Send()
}

// honeySpan is a backfilled span: a libhoney event whose Timestamp is the span's start.
type honeySpan struct {
	ev      *libhoney.Event
	traceID string
	spanID  string
}

type honeySpanKey struct{}

func (s *honeySpan) Send(end time.Time) {
	s.ev.AddField("duration_ms", float64(end.Sub(s.ev.Timestamp))/float64(time.Millisecond))
	s.ev.Send()
}

func (t *SenderHoneycomb) startEvent(ctx context.Context, name string, fields map[string]any, start time.Time) (context.Context, Sendable) {
	span := &honeySpan{ev: t.client.NewEvent(), spanID: randID(8)}
	span.ev.Timestamp = start
	for k, v := range fields {
		span.ev.AddField(k, v)
	}
	span.ev.AddField("name", name)
	span.ev.AddField("trace.span_id", span.spanID)
	if parent, ok := ctx.Value(honeySpanKey{}).(*honeySpan); ok {
		span.traceID = parent.traceID
		span.ev.AddField("trace.parent_id", parent.spanID)
	} else {
		span.traceID = randID(16)
	}
	span.ev.AddField("trace.trace_id", span.traceID)
	return context.WithValue(ctx, honeySpanKey{}, span), span
}

func (t *SenderHoneycomb) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64, start time.Time) (context.Context, Sendable) {
	if !start.IsZero() {
		return t.startEvent(ctx, name, fielder.GetFields(count, 0), start)
	}
	ctx, root := beeline.StartSpan(ctx, name)
	for k, v := range fielder.GetFields(count, 0) {
		root.AddField(k, v)
	}
	return ctx, beelineSendable{root}
}

func (t *SenderHoneycomb) CreateSpan(ctx context.Context, name string, level int, fielder *Fielder, start time.Time) (context.Context, Sendable) {
	if !start.IsZero() {
		return t.startEvent(ctx, name, fielder.GetFields(0, level), start)
	}
	ctx, span := beeline.StartSpan(ctx, name)
	for k, v := range fielder.GetFields(0, level) {
		span.AddField(k, v)
	}
	return ctx, beelineSendable{span}
}
