package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type Generator interface {
	Generate(spans chan *Span, stop chan struct{})
}

type TraceGenerator struct {
	depth     int
	spanCount int
	duration  time.Duration
	mut       sync.RWMutex
}

func NewTraceGenerator(opts Options) *TraceGenerator {
	return &TraceGenerator{
		depth:     opts.Depth,
		spanCount: opts.SpanCount,
		duration:  opts.Duration,
	}
}

// randID creates a random byte array of length l and returns it as a hex string.
func randID(l int) string {
	id := make([]byte, l)
	for i := 0; i < l; i++ {
		id[i] = byte(rand.Intn(256))
	}
	return fmt.Sprintf("%x", id)
}

// generate_spans generates a list of spans with the given depth and spancount
// it is recursive and expects spans[0] to be the root span
// - depth is the average depth (nesting level) of a trace.
// - spancount is the average number of spans in a trace.
// If spancount is less than depth, the trace will be truncated at spancount.
// If spancount is greater than depth, some of the children will have siblings.
func (s *TraceGenerator) generate_spans(spans chan *Span, tid, pid string, depth int, spancount int, timeRemaining time.Duration) {
	if depth == 0 {
		return
	}
	// this is the number of spans at this level
	nspans := 1
	if spancount > depth {
		// there is some chance that this level will have multiple spans based on the difference
		// between spancount and depth. (but we'll override this if it's a root span)
		nspans = 1 + rand.Intn(spancount-depth-1)
	}

	durationRemaining := time.Duration(rand.Intn(int(timeRemaining) / (spancount + 1)))
	durationPerChild := (timeRemaining - durationRemaining) / time.Duration(nspans)

	for i := 0; i < nspans; i++ {
		durationThisSpan := durationRemaining / time.Duration(nspans-i)
		durationRemaining -= durationThisSpan
		time.Sleep(durationThisSpan / 2)
		span := &Span{
			ServiceName: "test",
			TraceId:     tid,
			ParentId:    pid,
			SpanId:      randID(8),
			StartTime:   time.Now(),
			Fields:      map[string]interface{}{},
		}
		s.generate_spans(spans, tid, span.SpanId, depth-1, spancount-nspans, durationPerChild)
		time.Sleep(durationThisSpan / 2)
		span.EndTime = time.Now()
		span.Duration = span.EndTime.Sub(span.StartTime)
		spans <- span
	}
}

func (s *TraceGenerator) generate_root(spans chan *Span, depth int, spancount int, timeRemaining time.Duration) {
	root := &Span{
		ServiceName: "test",
		TraceId:     randID(16),
		SpanId:      randID(8),
		StartTime:   time.Now(),
	}
	thisSpanDuration := time.Duration(rand.Intn(int(timeRemaining) / (spancount + 1)))
	childDuration := (timeRemaining - thisSpanDuration)

	time.Sleep(thisSpanDuration / 2)
	s.generate_spans(spans, root.TraceId, root.SpanId, depth-1, spancount-1, childDuration)
	time.Sleep(thisSpanDuration / 2)
	root.EndTime = time.Now()
	root.Duration = root.EndTime.Sub(root.StartTime)
	spans <- root
}

func (s *TraceGenerator) Generate(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	s.mut.RLock()
	depth := s.depth
	spanCount := s.spanCount
	duration := s.duration
	s.mut.RUnlock()

	defer wg.Done()
	for {
		select {
		case <-stop:
			fmt.Println("stopping generator")
			return
		default:
			// generate a trace
			s.generate_root(spans, depth, spanCount, duration)
		}
	}
}
