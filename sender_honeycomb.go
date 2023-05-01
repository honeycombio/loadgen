package main

import (
	"context"

	"github.com/honeycombio/beeline-go"
)

// type HoneycombSender struct {
// 	dataset  string
// 	apiKey   string
// 	apiHost  string
// 	maxcount int64
// 	builder  *libhoney.Builder
// 	log      Logger
// }

// func NewHoneycombSender(log Logger, opts Options, apihost string) (*HoneycombSender, error) {
// 	err := libhoney.Init(libhoney.Config{
// 		WriteKey: opts.Telemetry.APIKey,
// 		Dataset:  opts.Telemetry.Dataset,
// 		APIHost:  apihost,
// 		// Logger:  log,  // uncomment to see libhoney debug logs
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	builder := libhoney.NewBuilder()
// 	host, err := os.Hostname()
// 	if err != nil {
// 		host = "unknown"
// 		log.Error("unable to determine hostname: %s, using 'unknown'", err)
// 	}
// 	builder.AddField("host_name", host)
// 	return &HoneycombSender{
// 		dataset:  opts.Telemetry.Dataset,
// 		apiKey:   opts.Telemetry.APIKey,
// 		apiHost:  host,
// 		maxcount: opts.Quantity.TraceCount,
// 		builder:  builder,
// 		log:      log,
// 	}, nil
// }

// func (h *HoneycombSender) Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
// 	wg.Add(2)
// 	// one goroutine to log errors when they occur
// 	go func() {
// 		defer wg.Done()
// 		responses := libhoney.TxResponses()
// 		for {
// 			select {
// 			case resp := <-responses:
// 				if resp.Err != nil {
// 					h.log.Error("error sending event -- err: %s  resp: %s", resp.Err, resp.Body)
// 				}
// 				// h.log.Printf("%s\n", resp)
// 			case <-stop:
// 				libhoney.Close()
// 				h.log.Info("stopping error response logger\n")
// 				return
// 			}
// 		}
// 	}()

// 	go func() {
// 		defer wg.Done()
// 		for {
// 			select {
// 			case span := <-spans:
// 				h.send(span)
// 			case <-stop:
// 				h.log.Info("stopping sender after stop\n")
// 				return
// 			}
// 		}
// 	}()
// }

// func (h *HoneycombSender) send(span *Span) {
// 	event := h.builder.NewEvent()
// 	event.Timestamp = span.StartTime
// 	if h.dataset != "" {
// 		event.AddField("service_name", span.ServiceName)
// 	} else {
// 		event.Dataset = span.ServiceName
// 	}
// 	event.AddField("trace.trace_id", span.TraceId)
// 	event.AddField("trace.span_id", span.SpanId)
// 	if span.ParentId != "" {
// 		event.AddField("trace.parent_id", span.ParentId)
// 	}
// 	event.AddField("duration_ms", span.Duration.Milliseconds())
// 	event.AddField("start_time", span.StartTime)
// 	event.AddField("end_time", span.EndTime)
// 	for k, v := range span.Fields {
// 		event.AddField(k, v)
// 	}
// 	event.Send()
// 	h.log.Info("sent event %v\n", event)
// }

type SenderHoneycomb struct{}

// make sure it implements Sender
var _ Sender = (*SenderHoneycomb)(nil)

func NewSenderHoneycomb(opts Options) *SenderHoneycomb {
	beeline.Init(beeline.Config{
		WriteKey:    opts.Telemetry.APIKey,
		APIHost:     opts.apihost.String(),
		ServiceName: "loadtest",
		Debug:       len(opts.Verbose) > 2,
		// Dataset:     opts.Telemetry.Dataset,
		// STDOUT: true,
	})
	return &SenderHoneycomb{}
}

func (t *SenderHoneycomb) Close() {
	beeline.Close()
}

func (t *SenderHoneycomb) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	// a beeline span is already a Sendable
	ctx, root := beeline.StartSpan(ctx, "root")
	for k, v := range fielder.GetFields(count) {
		root.AddField(k, v)
	}
	return ctx, root
}

func (t *SenderHoneycomb) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	// a beeline span is already a Sendable
	ctx, span := beeline.StartSpan(ctx, name)
	for k, v := range fielder.GetFields(0) {
		span.AddField(k, v)
	}
	return ctx, span
}
