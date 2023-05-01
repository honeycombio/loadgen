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

type ObsoleteSender interface {
	Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{})
}

type Sendable interface {
	Send()
}

type Sender interface {
	CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable)
	CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable)
	Close()
}

type OTelSendable struct {
	trace.Span
}

func (s OTelSendable) Send() {
	(trace.Span)(s).End()
}

type SenderOTel struct {
	tracer trace.Tracer
}

func NewSenderOTel(opts Options) *SenderOTel {
	return &SenderOTel{
		tracer: otel.Tracer("test"),
	}
}

func (t *SenderOTel) Close() {
	beeline.Close()
}

func (t *SenderOTel) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	ctx, root := t.tracer.Start(ctx, "root")
	fielder.AddFields(root, count)
	var ots OTelSendable
	ots.Span = root
	return ctx, ots
}

func (t *SenderOTel) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	span := trace.SpanFromContext(ctx)
	fielder.AddFields(span, 0)
	var ots OTelSendable
	ots.Span = span
	return ctx, ots
}
