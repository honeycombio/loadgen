package main

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type GenericTraceGenerator struct {
	depth     int
	spanCount int
	duration  time.Duration
	fielder   *Fielder
	chans     []chan struct{}
	mut       sync.RWMutex
	log       Logger
	tracer    TraceSender
}

// make sure it implements Generator
var _ Generator = (*GenericTraceGenerator)(nil)

func NewGenericTraceGenerator(tsender TraceSender, log Logger, opts Options) *GenericTraceGenerator {
	chans := make([]chan struct{}, 0)
	return &GenericTraceGenerator{
		depth:     opts.Format.Depth,
		spanCount: opts.Format.SpanCount,
		duration:  opts.Format.Duration,
		fielder:   NewFielder("test", opts.Format.SpanWidth),
		chans:     chans,
		log:       log,
		tracer:    tsender,
	}
}

// generate_spans generates a list of spans with the given depth and spancount
// it is recursive and expects spans[0] to be the root span
// - depth is the average depth (nesting level) of a trace.
// - spancount is the average number of spans in a trace.
// If spancount is less than depth, the trace will be truncated at spancount.
// If spancount is greater than depth, some of the children will have siblings.
func (s *GenericTraceGenerator) generate_spans(ctx context.Context, depth int, spancount int, timeRemaining time.Duration) {
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
		_, span := s.tracer.CreateSpan(ctx, "child", s.fielder)
		s.generate_spans(ctx, depth-1, spancount-nspans, durationPerChild)
		time.Sleep(durationThisSpan / 2)
		span.Send()
	}
}

func (s *GenericTraceGenerator) generate_root(count int64, depth int, spancount int, timeRemaining time.Duration) {
	ctx := context.Background()
	ctx, root := s.tracer.CreateTrace(ctx, "root", s.fielder, count)
	thisSpanDuration := time.Duration(rand.Intn(int(timeRemaining) / (spancount + 1)))
	childDuration := (timeRemaining - thisSpanDuration)

	time.Sleep(thisSpanDuration / 2)
	s.generate_spans(ctx, depth-1, spancount-1, childDuration)
	time.Sleep(thisSpanDuration / 2)
	root.Send()
}

// generator is a single goroutine that generates traces and sends them to the spans channel.
// It runs until the stop channel is closed.
// The trace time is determined by the duration, and as soon as one trace is sent the next one is started.
func (s *GenericTraceGenerator) generator(wg *sync.WaitGroup, counter chan int64) {
	s.mut.Lock()
	depth := s.depth
	spanCount := s.spanCount
	duration := s.duration
	stop := make(chan struct{})
	s.chans = append(s.chans, stop)
	s.mut.Unlock()

	ticker := time.NewTicker(duration)
	defer wg.Done()
	for {
		select {
		case <-stop:
			ticker.Stop()
			return
		case <-ticker.C:
			// generate a trace if we haven't been stopped by the counter
			select {
			case count := <-counter:
				s.generate_root(count, depth, spanCount, duration)
			default:
				// do nothing, we're done, and the stop will be caught by the outer select
			}
		}
	}
}

func (s *GenericTraceGenerator) Generate(opts Options, wg *sync.WaitGroup, spans chan *Span, stop chan struct{}, counter chan int64) {
	defer wg.Done()
	ngenerators := float64(opts.Quantity.TPS) / s.TPS()
	uSgeneratorInterval := float64(opts.Quantity.Ramp.Microseconds()) / ngenerators
	generatorInterval := time.Duration(uSgeneratorInterval) * time.Microsecond

	s.log.Debug("ngenerators: %f interval: %s\n", ngenerators, generatorInterval)
	state := Starting

	ticker := time.NewTicker(generatorInterval)
	defer ticker.Stop()

	stopTimer := time.NewTimer(opts.Quantity.MaxTime)
	defer stopTimer.Stop()

	for {
		select {
		case <-stop:
			s.log.Debug("stopping generators from stop signal\n")
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
				if len(s.chans) >= int(ngenerators+0.5) { // make sure we don't get bit by floating point rounding
					s.log.Debug("all generators started, switching to Running state\n")
					state = Running
				} else {
					s.log.Noisy("starting new generator\n")
					wg.Add(1)
					go s.generator(wg, counter)
				}
			case Running:
				// do nothing
			case Stopping:
				s.mut.Lock()
				if len(s.chans) == 0 {
					s.mut.Unlock()
					close(stop)
					return
				}
				s.log.Noisy("killing off a generator\n")
				close(s.chans[0])
				s.chans = s.chans[1:]
				s.mut.Unlock()
			}
		case <-stopTimer.C:
			s.log.Debug("stopping generators from timer\n")
			state = Stopping
		}
	}
}

func (s *GenericTraceGenerator) TPS() float64 {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return 1.0 / s.duration.Seconds()
}
