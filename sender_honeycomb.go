package main

import (
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
	log      Logger
}

// make sure it implements Sender
var _ Sender = (*HoneycombSender)(nil)

func NewHoneycombSender(log Logger, opts Options, apihost string) (*HoneycombSender, error) {
	err := libhoney.Init(libhoney.Config{
		WriteKey: opts.APIKey,
		Dataset:  opts.Dataset,
		APIHost:  apihost,
		// Logger:  log,  // uncomment to see libhoney debug logs
	})
	if err != nil {
		return nil, err
	}
	builder := libhoney.NewBuilder()
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
		log.Error("unable to determine hostname: %s, using 'unknown'", err)
	}
	builder.AddField("host_name", host)
	return &HoneycombSender{
		dataset:  opts.Dataset,
		apiKey:   opts.APIKey,
		apiHost:  host,
		maxcount: opts.TraceCount,
		builder:  builder,
		log:      log,
	}, nil
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
					h.log.Error("error sending event -- err: %s  resp: %s", resp.Err, resp.Body)
				}
				// h.log.Printf("%s\n", resp)
			case <-stop:
				libhoney.Close()
				h.log.Printf("stopping error response logger\n")
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
					h.log.Printf("stopping sender after maxcount\n")
					return
				}
			case <-stop:
				h.log.Printf("stopping sender after stop\n")
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
	h.log.Printf("sent event %v\n", event)
}