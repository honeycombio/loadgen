package main

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name       string
		datasets   []string
		depth, nsp int
		wantDepth  int
		wantNSpans int
		wantCount  int
	}{
		{"none", nil, 0, 0, 3, 3, 1},
		{"one", []string{"a"}, 0, 0, 3, 3, 1},
		{"four", []string{"a", "b", "c", "d"}, 0, 0, 4, 4, 4},
		{"explicit depth wins", []string{"a", "b", "c", "d"}, 2, 0, 2, 3, 4},
		{"explicit nspans wins", []string{"a", "b"}, 0, 10, 2, 10, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newOptions()
			o.Telemetry.Dataset = tt.datasets
			o.Format.Depth = tt.depth
			o.Format.NSpans = tt.nsp
			o.ApplyDefaults()
			if o.Format.Depth != tt.wantDepth {
				t.Errorf("depth = %d, want %d", o.Format.Depth, tt.wantDepth)
			}
			if o.Format.NSpans != tt.wantNSpans {
				t.Errorf("nspans = %d, want %d", o.Format.NSpans, tt.wantNSpans)
			}
			if len(o.Telemetry.Dataset) != tt.wantCount {
				t.Errorf("datasets = %v, want %d of them", o.Telemetry.Dataset, tt.wantCount)
			}
		})
	}
}

func TestParseHost(t *testing.T) {
	tests := []struct {
		host     string
		insecure bool
		grpc     bool
		want     string
	}{
		{"https://api.example.com", false, false, "https://api.example.com:443"},
		{"api.example.com", false, false, "https://api.example.com:443"},
		{"api.example.com", true, false, "http://api.example.com:80"},
		{"http://localhost", false, false, "http://localhost:80"},
		{"https://api.example.com", false, true, "https://api.example.com:4317"},
		{"api.example.com", true, true, "http://api.example.com:4317"},
		{"https://api.example.com:9000", false, false, "https://api.example.com:9000"},
		{"https://api.example.com:9000", false, true, "https://api.example.com:9000"},
		{"honeycomb", false, true, "https://api.honeycomb.io:443"},
		{"local", false, false, "http://localhost:8889"},
	}
	for _, tt := range tests {
		if got := parseHost(NewLogger(0), tt.host, tt.insecure, tt.grpc).String(); got != tt.want {
			t.Errorf("parseHost(%q, insecure=%v, grpc=%v) = %s, want %s", tt.host, tt.insecure, tt.grpc, got, tt.want)
		}
	}
}

func TestOTelSenderMultipleDatasets(t *testing.T) {
	datasets := []string{"gateway", "web", "db"}
	opts := newOptions()
	opts.Telemetry.Dataset = datasets
	opts.ApplyDefaults()

	// one in-memory exporter per dataset, in creation order
	var exporters []*tracetest.InMemoryExporter
	sender := newSenderOTelWithExporters(opts, func() sdktrace.SpanExporter {
		e := tracetest.NewInMemoryExporter()
		exporters = append(exporters, e)
		return e
	})

	fielder, err := NewFielder("seed", nil, 0, opts.Format.Depth)
	if err != nil {
		t.Fatal(err)
	}

	ctx, root := sender.CreateTrace(context.Background(), "root", fielder, 1, time.Time{})
	ctx, child := sender.CreateSpan(ctx, "child", 1, fielder, time.Time{})
	_, grandchild := sender.CreateSpan(ctx, "grandchild", 2, fielder, time.Time{})
	grandchild.Send(time.Time{})
	child.Send(time.Time{})
	root.Send(time.Time{})
	// flush rather than Close: shutting down an in-memory exporter discards its spans
	for _, tp := range sender.providers {
		if err := tp.ForceFlush(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	defer sender.Close()

	if len(exporters) != len(datasets) {
		t.Fatalf("got %d exporters, want %d", len(exporters), len(datasets))
	}
	var traceID string
	for i, e := range exporters {
		spans := e.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("%s: got %d spans, want 1", datasets[i], len(spans))
		}
		var svc string
		for _, kv := range spans[0].Resource.Attributes() {
			if kv.Key == "service.name" {
				svc = kv.Value.AsString()
			}
		}
		if svc != datasets[i] {
			t.Errorf("exporter %d: service.name = %q, want %q", i, svc, datasets[i])
		}
		if traceID == "" {
			traceID = spans[0].SpanContext.TraceID().String()
		}
		if got := spans[0].SpanContext.TraceID().String(); got != traceID {
			t.Errorf("%s: trace id %s differs from %s", datasets[i], got, traceID)
		}
	}
	for i := 1; i < len(exporters); i++ {
		parent := exporters[i-1].GetSpans()[0].SpanContext.SpanID()
		if got := exporters[i].GetSpans()[0].Parent.SpanID(); got != parent {
			t.Errorf("%s: parent span id = %s, want %s", datasets[i], got, parent)
		}
	}
}
