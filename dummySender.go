package main

import (
	"fmt"
	"sync"
)

type DummySender struct {
	spancount int
	rootspans int
}

// make sure it implements Sender
var _ Sender = (*DummySender)(nil)

func NewDummySender() *DummySender {
	return &DummySender{}
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
				fmt.Printf("sent %d spans with %d root spans\n", h.spancount, h.rootspans)
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
