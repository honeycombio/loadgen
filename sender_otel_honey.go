package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// make sure it implements Sender
var _ Sender = (*SenderOTel)(nil)

type OTelSendable struct {
	trace.Span
}

func (s OTelSendable) Send() {
	(trace.Span)(s).End()
}

type SenderOTel struct {
	tracer   trace.Tracer
	shutdown func()
}

func otelTracesFromURL(u url.URL) string {
	target := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	return target
}

type OtelLogger struct {
	Logger
}

func (l OtelLogger) Debugf(format string, args ...interface{}) {
	l.Logger.Debug(format, args...)
}

func (l OtelLogger) Fatalf(format string, args ...interface{}) {
	l.Logger.Fatal(format, args...)
}

func NewSenderOTel(log Logger, opts Options) *SenderOTel {
	u := *opts.apihost
	u.Path = "/v1/traces"

	otelshutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName("loadtest"),
		otelconfig.WithServiceVersion("0.0.1"),
		otelconfig.WithTracesExporterEndpoint(otelTracesFromURL(u)),
		otelconfig.WithTracesExporterInsecure(opts.Telemetry.Insecure),
		otelconfig.WithMetricsEnabled(false),
		otelconfig.WithLogLevel(opts.LogLevel),
		otelconfig.WithLogger(OtelLogger{log}),
		otelconfig.WithHeaders(map[string]string{
			"x-honeycomb-team": opts.Telemetry.APIKey,
		}),
	)
	if err != nil {
		log.Fatal("failure configuring otel: %v", err)
	}
	return &SenderOTel{
		tracer:   otel.Tracer("test"),
		shutdown: otelshutdown,
	}
}

func (t *SenderOTel) Close() {
	t.shutdown()
}

func (t *SenderOTel) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	ctx, root := t.tracer.Start(ctx, "root")
	fielder.AddFields(root, count)
	var ots OTelSendable
	ots.Span = root
	return ctx, ots
}

func (t *SenderOTel) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	span := trace.SpanFromContext(ctx)
	fielder.AddFields(span, 0)
	var ots OTelSendable
	ots.Span = span
	return ctx, ots
}

// type OTelHoneySender struct {
// 	dataset string
// 	apiKey  string
// 	apiHost string
// 	log     Logger
// }

// func NewOTelHoneySender(log Logger, dataset, apiKey, apiHost string, insecure bool) *OTelHoneySender {
// 	return &OTelHoneySender{
// 		dataset: dataset,
// 		apiKey:  apiKey,
// 		apiHost: apiHost,
// 		log:     log,
// 	}
// }

// func (h *OTelHoneySender) Run(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
// 	wg.Add(1)
// 	defer wg.Done()
// 	go func() {
// 		for {
// 			select {
// 			case span := <-spans:
// 				h.send(span)
// 			case <-stop:
// 				return
// 			}
// 		}
// 	}()
// }

// func (h *OTelHoneySender) send(span *Span) {
// }
