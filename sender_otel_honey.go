package main

import "sync"

type OTelHoneySender struct {
	dataset string
	apiKey  string
	apiHost string
	log     Logger
}

// make sure it implements Sender
var _ ObsoleteSender = (*OTelHoneySender)(nil)

func NewOTelHoneySender(log Logger, dataset, apiKey, apiHost string, insecure bool) *OTelHoneySender {
	return &OTelHoneySender{
		dataset: dataset,
		apiKey:  apiKey,
		apiHost: apiHost,
		log:     log,
	}
}

func (h *OTelHoneySender) Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	wg.Add(1)
	defer wg.Done()
	go func() {
		for {
			select {
			case span := <-spans:
				h.send(span)
			case <-stop:
				return
			}
		}
	}()
}

func (h *OTelHoneySender) send(span *Span) {
}
