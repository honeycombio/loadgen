package main

import (
	"context"
	"sync"
)

type DummySender struct {
	spancount int
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
				h.log.Info("dummysender sent %d spans with %d root spans\n", h.spancount, h.rootspans)
				return
			}
		}
	}()
}

func (h *DummySender) send(span *Span) {
	if span.IsRootSpan() {
		h.rootspans++
	}
	h.spancount++
}

type DummySendable struct{}

func (s DummySendable) Send() {
}

type SenderDummy struct {
	tracecount int
	spancount  int
	log        Logger
}

// make sure it implements Sender
var _ Sender = (*SenderDummy)(nil)

func NewSenderDummy(log Logger, opts Options) Sender {
	return &SenderDummy{log: log}
}

func (t *SenderDummy) Close() {
	t.log.Info("sender sent %d traces with %d spans\n", t.tracecount, t.spancount)
}

func (t *SenderDummy) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	t.tracecount++
	t.spancount++
	return ctx, DummySendable{}
}

func (t *SenderDummy) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	t.spancount++
	return ctx, DummySendable{}
}
