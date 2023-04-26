package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/honeycombio/libhoney-go"
)

type HoneycombSender struct {
	dataset  string
	apiKey   string
	apiHost  string
	maxcount int
	builder  *libhoney.Builder
}

// make sure it implements Sender
var _ Sender = (*HoneycombSender)(nil)

func NewHoneycombSender(opts Options, host string) *HoneycombSender {
	libhoney.Init(libhoney.Config{
		WriteKey: opts.APIKey,
		Dataset:  opts.Dataset,
		APIHost:  host,
	})
	builder := libhoney.NewBuilder()
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
		fmt.Fprintf(os.Stderr, "unable to determine hostname: %s", err)
	}
	builder.AddField("host_name", host)
	return &HoneycombSender{
		dataset:  opts.Dataset,
		apiKey:   opts.APIKey,
		apiHost:  host,
		maxcount: opts.TraceCount,
		builder:  builder,
	}
}

func (h *HoneycombSender) Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	wg.Add(2)
	// one goroutine to log errors when they occur
	go func() {
		defer wg.Done()
		responses := libhoney.TxResponses()
		for {
			select {
			case resp := <-responses:
				if resp.Err != nil {
					fmt.Fprintf(os.Stderr, "error sending event -- err: %s  resp: %s", resp.Err, resp.Body)
				}
				fmt.Println(resp)
			case <-stop:
				fmt.Println("stopping logger")
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		count := 0
		for {
			select {
			case span := <-spans:
				h.send(span)
				count++
				if h.maxcount > 0 && count >= h.maxcount {
					close(stop)
					fmt.Println("stopping sender after maxcount")
					return
				}
			case <-stop:
				fmt.Println("stopping sender after stop")
				return
			}
		}
	}()
}

func (h *HoneycombSender) send(span *Span) {
	event := h.builder.NewEvent()
	event.Timestamp = span.StartTime
	if h.dataset != "" {
		event.AddField("service_name", span.ServiceName)
	} else {
		event.Dataset = span.ServiceName
	}
	event.AddField("trace.trace_id", span.TraceId)
	event.AddField("trace.span_id", span.SpanId)
	if span.ParentId != "" {
		event.AddField("trace.parent_id", span.ParentId)
	}
	event.AddField("duration_ms", span.Duration.Milliseconds())
	event.AddField("start_time", span.StartTime)
	event.AddField("end_time", span.EndTime)
	for k, v := range span.Fields {
		event.AddField(k, v)
	}
	event.Send()
}
