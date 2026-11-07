package main

import (
	"context"
	"sync"
	"time"
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

// A Sendable is a span that has been started; Send ends it at the given time
// and sends it. The zero time means now.
type Sendable interface {
	Send(end time.Time)
}

// A Sender creates spans that start at the given time; the zero time means now.
type Sender interface {
	CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64, start time.Time) (context.Context, Sendable)
	CreateSpan(ctx context.Context, name string, level int, fielder *Fielder, start time.Time) (context.Context, Sendable)
	Close()
}
