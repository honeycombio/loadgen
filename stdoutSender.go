package main

import (
	"fmt"
	"sync"
	"time"
)

type StdoutSender struct {
	spancount int
	rootspans int
}

func NewStdoutSender() *StdoutSender {
	return &StdoutSender{}
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
				fmt.Printf("sent %d spans with %d root spans\n", h.spancount, h.rootspans)
				return
			}
		}
	}()
}

func f(ts time.Time) string {
	return ts.Format("15:04:05.000")
}

func (h *StdoutSender) send(span *Span) {
	if span.ParentId == "" {
		h.rootspans++
	}
	h.spancount++
	fmt.Printf("T:%6.6s S:%4.4s P%4.4s start:%v end:%v %v\n", span.TraceId, span.SpanId, span.ParentId, f(span.StartTime), f(span.EndTime), span.Fields)
}
