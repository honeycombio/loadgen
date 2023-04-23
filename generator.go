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

type SimpleGenerator struct {
	opts Options
}

func NewSimpleGenerator(opts Options) *SimpleGenerator {
	return &SimpleGenerator{opts: opts}
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
func (s *SimpleGenerator) generate_spans(spans []*Span, depth int, spancount int, timeRemaining time.Duration) []*Span {
	if depth == 0 {
		return spans
	}
	// this is the number of spans at this level
	nspans := 1
	if spancount > depth {
		// there is some chance that this level will have multiple spans based on the difference
		// between spancount and depth. (but we'll override this if it's a root span)
		nspans = 1 + rand.Intn(spancount-depth-1)
	}
	root := spans[0]
	current := spans[len(spans)-1]

	durationRemaining := time.Duration(rand.Intn(int(timeRemaining) / (spancount + 1)))
	durationPerChild := (timeRemaining - durationRemaining) / time.Duration(nspans)

	for i := 0; i < nspans; i++ {
		durationThisSpan := durationRemaining / time.Duration(nspans-i)
		durationRemaining -= durationThisSpan
		time.Sleep(durationThisSpan)
		span := &Span{
			ServiceName: "test",
			TraceId:     root.TraceId,
			ParentId:    current.SpanId,
			SpanId:      randID(8),
			StartTime:   time.Now(),
			Fields:      map[string]interface{}{},
		}
		spans = append(spans, span)
		spans = s.generate_spans(spans, depth-1, spancount-nspans, durationPerChild)
		span.EndTime = time.Now()
		span.Duration = span.EndTime.Sub(span.StartTime)
	}
	time.Sleep(durationRemaining)
	return spans
}

func (s *SimpleGenerator) generate_root(spans []*Span, depth int, spancount int, timeRemaining time.Duration) []*Span {
	root := &Span{
		ServiceName: "test",
		TraceId:     randID(16),
		SpanId:      randID(8),
		StartTime:   time.Now(),
	}
	thisSpanDuration := time.Duration(rand.Intn(int(timeRemaining) / (spancount + 1)))
	childDuration := (timeRemaining - thisSpanDuration)

	time.Sleep(thisSpanDuration / 2)
	spans = append(spans, root)
	spans = s.generate_spans(spans, depth-1, spancount-1, childDuration)
	root.EndTime = time.Now()
	root.Duration = root.EndTime.Sub(root.StartTime)

	time.Sleep(thisSpanDuration / 2)
	return spans
}

func (s *SimpleGenerator) Generate(wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			fmt.Println("stopping generator")
			return
		default:
			// generate a trace
			trace := s.generate_root([]*Span{}, s.opts.Depth, s.opts.SpanCount, s.opts.Duration)
			// send it to the channel
			for _, sp := range trace {
				spans <- sp
			}
		}
	}
}
