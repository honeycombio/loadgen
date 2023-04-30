package main

import (
	"sync"
	"time"
)

type StdoutSender struct {
	spancount int
	rootspans int
	log       Logger
}

// make sure it implements Sender
var _ Sender = (*StdoutSender)(nil)

func NewStdoutSender(log Logger) *StdoutSender {
	return &StdoutSender{log: log}
}

func (h *StdoutSender) Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	wg.Add(1)
	defer wg.Done()
	go func() {
		for {
			select {
			case span := <-spans:
				h.send(span)
			case <-stop:
				h.log.Printf("sent %d spans with %d root spans\n", h.spancount, h.rootspans)
				return
			}
		}
	}()
}

func f(ts time.Time) string {
	return ts.Format("15:04:05.000")
}

func (h *StdoutSender) send(span *Span) {
	if span.IsRootSpan() {
		h.rootspans++
	}
	h.spancount++
	h.log.Printf("T:%6.6s S:%4.4s P%4.4s start:%v end:%v %v\n", span.TraceId, span.SpanId, span.ParentId, f(span.StartTime), f(span.EndTime), span.Fields)
}