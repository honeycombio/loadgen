package main

import "sync"

type OTelHoneySender struct {
	dataset string
	apiKey  string
	apiHost string
}

func NewOTelHoneySender(dataset, apiKey, apiHost string, insecure bool) *OTelHoneySender {
	return &OTelHoneySender{
		dataset: dataset,
		apiKey:  apiKey,
		apiHost: apiHost,
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
