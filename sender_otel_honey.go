package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
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
	var client otlptrace.Client
	switch opts.Output.Protocol {
	case "grpc":
		client = setupOTELGRPCClient(opts)
	case "http":
		client = setupOTELHTTPClient(opts)
	default:
		log.Fatal("unknown protocol: %s", opts.Output.Protocol)
	}

	exporter, err := otlptrace.New(
		context.Background(),
		client,
	)
	if err != nil {
		log.Fatal("failure configuring otel trace exporter: %v", err)
	}

	var bspOpts []sdktrace.BatchSpanProcessorOption
	if opts.Output.BatchTimeout != 0 {
		bspOpts = append(bspOpts, sdktrace.WithBatchTimeout(opts.Output.BatchTimeout))
	}

	if opts.Output.MaxQueueSize != 0 {
		bspOpts = append(bspOpts, sdktrace.WithMaxQueueSize(opts.Output.MaxQueueSize))
	}
	if opts.Output.MaxExportBatchSize != 0 {
		bspOpts = append(bspOpts, sdktrace.WithMaxExportBatchSize(opts.Output.MaxExportBatchSize))
	}
	if opts.Output.ExportTimeout != 0 {
		bspOpts = append(bspOpts, sdktrace.WithExportTimeout(opts.Output.ExportTimeout))
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter, bspOpts...)
	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceNameKey.String(opts.Telemetry.Dataset))),
	))
	otelshutdown := func() {
		_ = bsp.Shutdown(context.Background())
		_ = exporter.Shutdown(context.Background())
	}

	return &SenderOTel{
		tracer:   otel.Tracer(ResourceLibrary, trace.WithInstrumentationVersion(ResourceVersion)),
		shutdown: otelshutdown,
	}
}

func (t *SenderOTel) Close() {
	t.shutdown()
}

func (t *SenderOTel) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	ctx, root := t.tracer.Start(ctx, name)
	fielder.AddFields(root, count, 0)
	var ots OTelSendable
	ots.Span = root
	return ctx, ots
}

func (t *SenderOTel) CreateSpan(ctx context.Context, name string, level int, fielder *Fielder) (context.Context, Sendable) {
	ctx, span := t.tracer.Start(ctx, name)
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

func setupOTELHTTPClient(opts *Options) otlptrace.Client {
	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(opts.apihost.Host),
		otlptracehttp.WithHeaders(map[string]string{
			"x-honeycomb-team": opts.Telemetry.APIKey,
		}),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	}
	if opts.Telemetry.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	} else {
		options = append(options, otlptracehttp.WithTLSClientConfig(&tls.Config{}))
	}
	return otlptracehttp.NewClient(
		options...,
	)
}

func setupOTELGRPCClient(opts *Options) otlptrace.Client {
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(opts.apihost.Host),
		otlptracegrpc.WithHeaders(map[string]string{
			"x-honeycomb-team": opts.Telemetry.APIKey,
		}),
		otlptracegrpc.WithCompressor(gzip.Name),
	}
	if opts.Telemetry.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	} else {
		options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}
	return otlptracegrpc.NewClient(
		options...,
	)
}
