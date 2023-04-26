package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// A Generator generates traces and sends them to the spans channel. Its
// Generate method should be run in a goroutine, and generates a single trace,
// taking opts.Duration to do so. Its TPS method returns the number of traces
// per second it is currently generating.
type Generator interface {
	Generate(opts Options, wg *sync.WaitGroup, spans chan *Span, stop chan struct{})
	TPS() float64
}

type TraceGenerator struct {
	depth     int
	spanCount int
	duration  time.Duration
	chans     []chan struct{}
	mut       sync.RWMutex
}

// make sure it implements Generator
var _ Generator = (*TraceGenerator)(nil)

func NewTraceGenerator(opts Options) *TraceGenerator {
	chans := make([]chan struct{}, 0)
	return &TraceGenerator{
		depth:     opts.Depth,
		spanCount: opts.SpanCount,
		duration:  opts.Duration,
		chans:     chans,
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

// generator is a single goroutine that generates traces and sends them to the spans channel.
// It runs until the stop channel is closed.
// The trace time is determined by the duration, and as soon as one trace is sent the next one is started.
func (s *TraceGenerator) generator(wg *sync.WaitGroup, spans chan *Span) {
	s.mut.Lock()
	depth := s.depth
	spanCount := s.spanCount
	duration := s.duration
	stop := make(chan struct{})
	s.chans = append(s.chans, stop)
	s.mut.Unlock()

	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:
			// generate a trace
			s.generate_root(spans, depth, spanCount, duration)
		}
	}
}

type GeneratorState int

const (
	Starting GeneratorState = iota
	Running
	Stopping
)

func (s *TraceGenerator) Generate(opts Options, wg *sync.WaitGroup, spans chan *Span, stop chan struct{}) {
	defer wg.Done()
	ngenerators := float64(opts.TPS) / s.TPS()
	uSgeneratorInterval := float64(opts.Ramp.Microseconds()) / ngenerators
	generatorInterval := time.Duration(uSgeneratorInterval) * time.Microsecond

	fmt.Println("ngenerators:", ngenerators, "interval:", generatorInterval)
	state := Starting

	ticker := time.NewTicker(generatorInterval)
	defer ticker.Stop()

	stopTimer := time.NewTimer(opts.MaxTime)
	defer stopTimer.Stop()

	for {
		select {
		case <-stop:
			fmt.Println("stopping generators from stop signal")
			state = Stopping
			s.mut.Lock()
			for _, ch := range s.chans {
				close(ch)
			}
			s.mut.Unlock()
			return
		case <-ticker.C:
			switch state {
			case Starting:
				if len(s.chans) >= int(ngenerators+0.5) {
					fmt.Println("switching to Running state")
					state = Running
				} else {
					fmt.Println("starting new generator")
					wg.Add(1)
					go s.generator(wg, spans)
				}
			case Running:
				// do nothing
			case Stopping:
				s.mut.Lock()
				if len(s.chans) == 0 {
					close(stop)
					return
				}
				fmt.Println("killing off a generator")
				close(s.chans[0])
				s.chans = s.chans[1:]
				s.mut.Unlock()
			}
		case <-stopTimer.C:
			fmt.Println("stopping generators from timer")
			state = Stopping
		}
	}
}

func (s *TraceGenerator) TPS() float64 {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return 1.0 / s.duration.Seconds()
}
