package main

import (
	"context"
	"math"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/honeycombio/libhoney-go/transmission"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// lognormalCDF is the CDF of a log-normal distribution with the given median and sigma.
func lognormalCDF(x, median time.Duration, sigma float64) float64 {
	return 0.5 * math.Erfc(-math.Log(float64(x)/float64(median))/(sigma*math.Sqrt2))
}

func TestBackfillPickTrace(t *testing.T) {
	tests := []struct {
		name   string
		b      BackfillOptions
		checks []time.Duration // compare the empirical CDF with the truncated log-normal CDF here
	}{
		{
			// a long tail, with a small fraction of traces longer than a 2h lookback
			name:   "long tail",
			b:      BackfillOptions{Window: 2 * time.Hour, DurationMedian: 30 * time.Second, DurationSigma: 2, DurationMax: 4 * time.Hour},
			checks: []time.Duration{time.Second, 30 * time.Second, 10 * time.Minute, time.Hour, 2 * time.Hour, 3 * time.Hour},
		},
		{
			// the max cuts off half the distribution; clamping would put half the samples at the max
			name:   "truncated at median",
			b:      BackfillOptions{Window: time.Hour, DurationMedian: time.Hour, DurationSigma: 1, DurationMax: time.Hour},
			checks: []time.Duration{5 * time.Minute, 20 * time.Minute, 40 * time.Minute, 59 * time.Minute},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const n = 200000
			now := time.Now()
			durations := make([]time.Duration, n)
			minEnd, maxEnd := now, now.Add(-tt.b.Window)
			atMax := 0
			for i := range durations {
				start, d := tt.b.pickTrace(now)
				end := start.Add(d)
				if end.Before(now.Add(-tt.b.Window)) || end.After(now) {
					t.Fatalf("end %v is outside the window [%v, %v]", end, now.Add(-tt.b.Window), now)
				}
				if d < time.Millisecond || d > tt.b.DurationMax {
					t.Fatalf("duration %v is outside [1ms, %v]", d, tt.b.DurationMax)
				}
				if d == tt.b.DurationMax {
					atMax++
				}
				if end.Before(minEnd) {
					minEnd = end
				}
				if end.After(maxEnd) {
					maxEnd = end
				}
				durations[i] = d
			}
			// the ends cover the whole window
			if slack := tt.b.Window / 100; minEnd.Sub(now.Add(-tt.b.Window)) > slack || now.Sub(maxEnd) > slack {
				t.Errorf("ends span only [%v, %v] of the window", now.Sub(minEnd), now.Sub(maxEnd))
			}
			if atMax > n/1000 {
				t.Errorf("%d of %d durations are exactly the max", atMax, n)
			}
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			fmax := lognormalCDF(tt.b.DurationMax, tt.b.DurationMedian, tt.b.DurationSigma)
			for _, x := range tt.checks {
				got := float64(sort.Search(n, func(i int) bool { return durations[i] > x })) / n
				want := lognormalCDF(x, tt.b.DurationMedian, tt.b.DurationSigma) / fmax
				if math.Abs(got-want) > 0.01 {
					t.Errorf("P(duration <= %v) = %.4f, want %.4f", x, got, want)
				}
			}
		})
	}
}

// recordedSpan and recordingSender capture the timestamps the generator passes to a sender.
type recordedSpan struct {
	parent     *recordedSpan
	start, end time.Time
}

type recordingSender struct {
	mut   sync.Mutex
	spans []*recordedSpan
}

type recordedSpanKey struct{}

func (r *recordingSender) start(ctx context.Context, start time.Time) (context.Context, Sendable) {
	parent, _ := ctx.Value(recordedSpanKey{}).(*recordedSpan)
	span := &recordedSpan{parent: parent, start: start}
	return context.WithValue(ctx, recordedSpanKey{}, span), recordingSendable{r, span}
}

func (r *recordingSender) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64, start time.Time) (context.Context, Sendable) {
	return r.start(ctx, start)
}

func (r *recordingSender) CreateSpan(ctx context.Context, name string, level int, fielder *Fielder, start time.Time) (context.Context, Sendable) {
	return r.start(ctx, start)
}

func (r *recordingSender) Close() {}

type recordingSendable struct {
	r    *recordingSender
	span *recordedSpan
}

func (s recordingSendable) Send(end time.Time) {
	s.r.mut.Lock()
	defer s.r.mut.Unlock()
	s.span.end = end
	s.r.spans = append(s.r.spans, s.span)
}

func TestBackfillTraces(t *testing.T) {
	opts := newOptions()
	opts.Format.Depth = 4
	opts.Format.NSpans = 10
	opts.Format.TraceTime = time.Second
	opts.Quantity.TPS = 1000
	opts.Quantity.TraceCount = 500
	opts.Quantity.RampTime = 10 * time.Millisecond
	opts.Backfill = BackfillOptions{Window: 2 * time.Hour, DurationMedian: 30 * time.Minute, DurationSigma: 1, DurationMax: 3 * time.Hour}
	log := NewLogger(0)
	sender := &recordingSender{}
	getFielder := func() *Fielder {
		f, err := NewFielder("seed", nil, 0, opts.Format.Depth)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}

	// run the generator as main does; the traces last up to hours, so this
	// only finishes promptly if nothing sleeps through them
	before := time.Now()
	stop := make(chan struct{})
	counter := make(chan int64)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go NewTraceGenerator(sender, getFielder, log, opts).Generate(opts, wg, stop, counter)
	TraceCounter(log, opts.Quantity.TraceCount, counter, stop)
	close(stop)
	wg.Wait()
	elapsed := time.Since(before)
	after := time.Now()

	// the traces are still paced at --tps
	if want := 500 * time.Millisecond; elapsed < want*8/10 || elapsed > 5*want {
		t.Errorf("generating 500 traces at 1000 tps took %v, want about %v", elapsed, want)
	}
	roots := 0
	for _, s := range sender.spans {
		if s.start.IsZero() || s.end.Before(s.start) {
			t.Fatalf("span has start %v and end %v", s.start, s.end)
		}
		if p := s.parent; p != nil {
			if s.start.Before(p.start) || s.end.After(p.end) {
				t.Errorf("child [%v, %v] is not inside its parent [%v, %v]", s.start, s.end, p.start, p.end)
			}
			continue
		}
		roots++
		if s.end.Before(before.Add(-opts.Backfill.Window)) || s.end.After(after) {
			t.Errorf("root ends at %v, outside the backfill window", s.end)
		}
		if d := s.end.Sub(s.start); d < time.Millisecond || d > opts.Backfill.DurationMax {
			t.Errorf("root lasts %v", d)
		}
	}
	if roots != 500 {
		t.Errorf("got %d traces, want 500", roots)
	}
	if len(sender.spans) <= roots {
		t.Errorf("got only %d spans in %d traces", len(sender.spans), roots)
	}
}

func TestSendersUseExplicitTimestamps(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	childStart, childEnd := start.Add(time.Minute), start.Add(time.Hour+30*time.Second)
	end := start.Add(2*time.Hour + 30*time.Minute)

	opts := newOptions()
	opts.Telemetry.APIKey = "test"
	opts.apihost = &url.URL{Scheme: "http", Host: "localhost:1"}
	opts.ApplyDefaults()
	fielder, err := NewFielder("seed", nil, 0, opts.Format.Depth)
	if err != nil {
		t.Fatal(err)
	}
	sendTrace := func(sender Sender) {
		ctx, root := sender.CreateTrace(context.Background(), "root", fielder, 1, start)
		_, child := sender.CreateSpan(ctx, "child", 1, fielder, childStart)
		child.Send(childEnd)
		root.Send(end)
	}

	t.Run("otel", func(t *testing.T) {
		exporter := tracetest.NewInMemoryExporter()
		sender := newSenderOTelWithExporters(opts, func() sdktrace.SpanExporter { return exporter })
		sendTrace(sender)
		if err := sender.providers[0].ForceFlush(context.Background()); err != nil {
			t.Fatal(err)
		}
		spans := exporter.GetSpans()
		if len(spans) != 2 {
			t.Fatalf("got %d spans, want 2", len(spans))
		}
		child, root := spans[0], spans[1]
		if !root.StartTime.Equal(start) || !root.EndTime.Equal(end) {
			t.Errorf("root is [%v, %v], want [%v, %v]", root.StartTime, root.EndTime, start, end)
		}
		if !child.StartTime.Equal(childStart) || !child.EndTime.Equal(childEnd) {
			t.Errorf("child is [%v, %v], want [%v, %v]", child.StartTime, child.EndTime, childStart, childEnd)
		}
		for _, ev := range child.Events {
			if !ev.Time.Equal(childStart) {
				t.Errorf("child event at %v, want %v", ev.Time, childStart)
			}
		}
		if child.Parent.SpanID() != root.SpanContext.SpanID() {
			t.Errorf("child's parent is %v, want %v", child.Parent.SpanID(), root.SpanContext.SpanID())
		}
	})

	t.Run("honeycomb", func(t *testing.T) {
		tx := &transmission.MockSender{}
		sender := newSenderHoneycombWithTransmission(opts, tx)
		sendTrace(sender)
		events := tx.Events()
		if len(events) != 2 {
			t.Fatalf("got %d events, want 2", len(events))
		}
		child, root := events[0], events[1]
		if !root.Timestamp.Equal(start) || root.Data["duration_ms"] != 150.0*60*1000 {
			t.Errorf("root has timestamp %v and duration_ms %v, want %v and %v", root.Timestamp, root.Data["duration_ms"], start, 150.0*60*1000)
		}
		if !child.Timestamp.Equal(childStart) || child.Data["duration_ms"] != 59.5*60*1000 {
			t.Errorf("child has timestamp %v and duration_ms %v, want %v and %v", child.Timestamp, child.Data["duration_ms"], childStart, 59.5*60*1000)
		}
		if child.Data["trace.trace_id"] != root.Data["trace.trace_id"] || child.Data["trace.parent_id"] != root.Data["trace.span_id"] {
			t.Errorf("child %v is not linked to root %v", child.Data, root.Data)
		}
		if _, ok := root.Data["trace.parent_id"]; ok {
			t.Errorf("root has a parent: %v", root.Data)
		}
		if root.Data["service.name"] != "loadgen" || root.Data["name"] != "root" {
			t.Errorf("root is missing its service or span name: %v", root.Data)
		}
	})
}
