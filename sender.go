package main

import (
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

type Sender interface {
	Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{})
}
