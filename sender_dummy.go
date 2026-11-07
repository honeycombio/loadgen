package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type DummySender struct {
	nspans    int
	rootspans int
	log       Logger
}

func NewDummySender(log Logger) *DummySender {
	return &DummySender{log: log}
}

func (h *DummySender) Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	wg.Add(1)
	defer wg.Done()
	go func() {
		for {
			select {
			case span := <-spans:
				h.send(span)
			case <-stop:
				h.log.Warn("dummysender sent %d spans with %d root spans\n", h.nspans, h.rootspans)
				return
			}
		}
	}()
}

func (h *DummySender) send(span *Span) {
	if span.IsRootSpan() {
		h.rootspans++
	}
	h.nspans++
}

type DummySendable struct{}

func (s DummySendable) Send(end time.Time) {
}

type SenderDummy struct {
	// generators call the sender concurrently
	tracecount atomic.Int64
	nspans     atomic.Int64
	log        Logger
}

// make sure it implements Sender
var _ Sender = (*SenderDummy)(nil)

func NewSenderDummy(log Logger, opts *Options) Sender {
	return &SenderDummy{log: log}
}

func (t *SenderDummy) Close() {
	t.log.Warn("sender sent %d traces with %d spans\n", t.tracecount.Load(), t.nspans.Load())
}

func (t *SenderDummy) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64, start time.Time) (context.Context, Sendable) {
	t.tracecount.Add(1)
	t.nspans.Add(1)
	return ctx, DummySendable{}
}

func (t *SenderDummy) CreateSpan(ctx context.Context, name string, level int, fielder *Fielder, start time.Time) (context.Context, Sendable) {
	t.nspans.Add(1)
	return ctx, DummySendable{}
}
