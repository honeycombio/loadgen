package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	tracers  map[string]trace.Tracer
	shutdown func()
}

func otelTracesFromURL(u *url.URL) string {
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

func NewSenderOTel(log Logger, opts *Options) *SenderOTel {
	var protocol otelconfig.Protocol
	switch opts.Output.Protocol {
	case "grpc":
		protocol = otelconfig.ProtocolGRPC
	case "protobuf":
		protocol = otelconfig.ProtocolHTTPProto
	case "json":
		protocol = otelconfig.ProtocolHTTPJSON
	default:
		log.Fatal("unknown protocol: %s", opts.Output.Protocol)
	}

	tracers := make(map[string]trace.Tracer)
	var shutdownFuncs []func()

	services := []string(opts.Telemetry.Services)
	for _, service := range services {
		shutdown, err := otelconfig.ConfigureOpenTelemetry(
			otelconfig.WithExporterProtocol(protocol),
			otelconfig.WithServiceName(service),
			otelconfig.WithTracesExporterEndpoint(otelTracesFromURL(opts.apihost)),
			otelconfig.WithTracesExporterInsecure(opts.Telemetry.Insecure),
			otelconfig.WithMetricsEnabled(false),
			otelconfig.WithLogLevel(opts.Global.LogLevel),
			otelconfig.WithLogger(OtelLogger{log}),
			otelconfig.WithHeaders(map[string]string{
				"x-honeycomb-team": opts.Telemetry.APIKey,
			}),
		)
		if err != nil {
			log.Fatal("failure configuring otel for service %s: %v", service, err)
		}
		shutdownFuncs = append(shutdownFuncs, shutdown)
		tracers[service] = otel.Tracer(ResourceLibrary, trace.WithInstrumentationVersion(ResourceVersion))
	}

	return &SenderOTel{
		tracers: tracers,
		shutdown: func() {
			for _, shutdown := range shutdownFuncs {
				shutdown()
			}
		},
	}
}

func (t *SenderOTel) Close() {
	t.shutdown()
}

func (t *SenderOTel) CreateTrace(ctx context.Context, name string, service string, fielder *Fielder, count int64) (context.Context, Sendable) {
	log := NewLogger(0)
	log.Printf("creating trace %s for service %s.\n", name, service)
	tracer, exists := t.tracers[service]
	if !exists {
		log.Fatal("service %s not found", service)
	}
	ctx, root := tracer.Start(ctx, name)
	fielder.AddFields(root, count, 0)
	var ots OTelSendable
	ots.Span = root
	return ctx, ots
}

func (t *SenderOTel) CreateSpan(ctx context.Context, name string, service string, level int, fielder *Fielder) (context.Context, Sendable) {
	log := NewLogger(0)
	// log.Printf("creating span %s for service %s.\n", name, service)
	tracer, exists := t.tracers[service]
	if !exists {
		log.Fatal("service %s not found", service)
	}
	ctx, span := tracer.Start(ctx, name)
	if rand.Intn(10) == 0 {
		span.AddEvent("exception", trace.WithAttributes(
			attribute.KeyValue{Key: "exception.type", Value: attribute.StringValue("error")},
			attribute.KeyValue{Key: "exception.message", Value: attribute.StringValue("error message")},
			attribute.KeyValue{Key: "exception.stacktrace", Value: attribute.StringValue("stacktrace")},
			attribute.KeyValue{Key: "exception.escaped", Value: attribute.BoolValue(false)},
		))
	}
	fielder.AddFields(span, 0, level)
	var ots OTelSendable
	ots.Span = span
	return ctx, ots
}
