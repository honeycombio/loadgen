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

type Sendable interface {
	Send()
}

type Sender interface {
	CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable)
	CreateSpan(ctx context.Context, name string, level int, fielder *Fielder) (context.Context, Sendable)
	Close()
}
