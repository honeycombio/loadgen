package main

import (
	"context"
	"sync"
	"time"

	"github.com/honeycombio/beeline-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type Span struct {
	ServiceName string
	TraceId     string
	SpanId      string
	ParentId    string
	Duration    time.Duration
	StartTime   time.Time
	EndTime     time.Time
	Fields      map[string]interface{}
}

func (s *Span) IsRootSpan() bool {
	return s.ParentId == ""
}

type Sender interface {
	Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{})
}

type Sendable interface {
	Send()
}

type TraceSender interface {
	CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable)
	CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable)
}

type TraceSenderHoneycomb struct{}

func NewTraceSenderHoneycomb(opts Options) *TraceSenderHoneycomb {
	beeline.Init(beeline.Config{
		WriteKey:    opts.Telemetry.APIKey,
		APIHost:     opts.Telemetry.Host,
		ServiceName: "loadtest",
		// Dataset:     opts.Telemetry.Dataset,
		// STDOUT: true,
	})
	return &TraceSenderHoneycomb{}
}

func (t *TraceSenderHoneycomb) Close() {
	beeline.Close()
}

func (t *TraceSenderHoneycomb) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	ctx, root := beeline.StartSpan(ctx, "root")
	for k, v := range fielder.GetFields(count) {
		root.AddField(k, v)
	}
	return ctx, root
}

func (t *TraceSenderHoneycomb) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	ctx, span := beeline.StartSpan(ctx, name)
	for k, v := range fielder.GetFields(0) {
		span.AddField(k, v)
	}
	return ctx, span
}

type OTelSendable struct {
	trace.Span
}

func (s OTelSendable) Send() {
	(trace.Span)(s).End()
}

type TraceSenderOTel struct {
	tracer trace.Tracer
}

func NewTraceSenderOTel(opts Options) *TraceSenderOTel {
	return &TraceSenderOTel{
		tracer: otel.Tracer("test"),
	}
}

func (t *TraceSenderOTel) Close() {
	beeline.Close()
}

func (t *TraceSenderOTel) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	ctx, root := t.tracer.Start(ctx, "root")
	fielder.AddFields(root, count)
	var ots OTelSendable
	ots.Span = root
	return ctx, ots
}

func (t *TraceSenderOTel) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	span := trace.SpanFromContext(ctx)
	fielder.AddFields(span, 0)
	var ots OTelSendable
	ots.Span = span
	return ctx, ots
}

type TraceSenderDummy struct{}

func (t TraceSenderDummy) Send() {}

func NewTraceSenderDummy(opts Options) TraceSender {
	return TraceSenderDummy{}
}

func (t TraceSenderDummy) Close() {
}

func (t TraceSenderDummy) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	return ctx, TraceSenderDummy{}
}

func (t TraceSenderDummy) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	return ctx, TraceSenderDummy{}
}
