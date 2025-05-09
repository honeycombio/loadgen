package main

import (
	"context"
	"math"
	"sync"
	"time"

	"pgregory.net/rand"
)

// A Generator generates traces and sends the individual spans to the spans channel. Its
// Generate method should be run in a goroutine, and generates a single trace,
// taking opts.Duration to do so. Its TPS method returns the number of traces
// per second it is currently generating.
type Generator interface {
	Generate(opts *Options, wg *sync.WaitGroup, stop chan struct{}, counter chan int64)
	TPS() float64
}

type GeneratorState int

const (
	Starting GeneratorState = iota
	Running
	Stopping
)

type TraceGenerator struct {
	depth      int
	nspans     int
	duration   time.Duration
	getFielder func() *Fielder
	chans      []chan struct{}
	mut        sync.RWMutex
	log        Logger
	tracer     Sender
}

// make sure it implements Generator
var _ Generator = (*TraceGenerator)(nil)

func NewTraceGenerator(tsender Sender, getFielder func() *Fielder, log Logger, opts *Options) *TraceGenerator {
	chans := make([]chan struct{}, 0)
	return &TraceGenerator{
		depth:      opts.Format.Depth,
		nspans:     opts.Format.NSpans,
		duration:   opts.Format.TraceTime,
		getFielder: getFielder,
		chans:      chans,
		log:        log,
		tracer:     tsender,
	}
}

// generate_spans generates a list of spans with the given depth and spancount
// it is recursive and expects spans[0] to be the root span
// - level is the current depth of this span where 0 is the root span
// - depth is the maximum depth (nesting level) of a trace -- how much deeper this trace will go
// - nspans is the number of spans in a trace.
// If nspans is less than depth, the trace will be truncated at nspans.
// If nspans is greater than depth, some of the children will have siblings.
func (s *TraceGenerator) generate_spans(ctx context.Context, fielder *Fielder, level int, depth int, nspans int, timeRemaining time.Duration, numOfSpansGenerated *int) {
	if timeRemaining <= 0 {
		return
	}

	if nspans == 0 {
		// if there's still time remaining, sleep for the remainder of the time
		if timeRemaining > 0 {
			time.Sleep(timeRemaining)
		}
		return
	}

	spansAtThisLevel := 1
	if nspans > depth && depth >= 0 {
		// there is some chance that this level will have multiple spans based on the difference
		// between nspans and depth. (but we'll override this if it's a root span)
		// spanAtThisLevel is always between 1 and nspans
		spansAtThisLevel = 1 + rand.Intn(nspans-depth)
	}

	spancounts := make([]int, 0, spansAtThisLevel)
	if spansAtThisLevel == 1 {
		// if there's only one span, give it all the counts
		spancounts = append(spancounts, nspans)
	} else {
		// split the counts among the spans at this level
		// we take a random portion of the counts for each span, then put the leftovers in a random span
		count := nspans
		spansPerPeer := nspans / spansAtThisLevel // always at least 1
		for i := 0; i < spansAtThisLevel; i++ {
			spancounts = append(spancounts, rand.Intn(spansPerPeer)+1)
			count -= spancounts[i]
		}
		spancounts[rand.Intn(spansAtThisLevel)] += count
	}

	durationRemaining := time.Duration(rand.Intn(int(math.Ceil((float64(timeRemaining) / float64(nspans+1))))))
	durationPerChild := (timeRemaining - durationRemaining) / time.Duration(spansAtThisLevel)

	for i := 0; i < spansAtThisLevel; i++ {
		durationThisSpan := durationRemaining / time.Duration(spansAtThisLevel-i)
		durationRemaining -= durationThisSpan
		time.Sleep(durationThisSpan / 2)
		childctx, span := s.tracer.CreateSpan(ctx, fielder.GetServiceName(i+1), level, fielder)
		*numOfSpansGenerated = *numOfSpansGenerated + 1
		s.generate_spans(childctx, fielder, level+1, depth-1, spancounts[i]-1, durationPerChild, numOfSpansGenerated)
		time.Sleep(durationThisSpan / 2)
		span.Send()
	}
}

func (s *TraceGenerator) generate_root(fielder *Fielder, count int64, depth int, nspans int, timeRemaining time.Duration) {
	ctx := context.Background()
	ctx, root := s.tracer.CreateTrace(ctx, fielder.GetServiceName(depth), fielder, count)
	thisSpanDuration := time.Duration(rand.Intn(int(timeRemaining) / (nspans + 1)))
	childDuration := (timeRemaining - thisSpanDuration)

	numOfSpansGenerated := 1
	time.Sleep(thisSpanDuration / 2)
	s.generate_spans(ctx, fielder, 1, depth-1, nspans-1, childDuration, &numOfSpansGenerated)
	time.Sleep(thisSpanDuration / 2)
	root.Send()
	s.log.Debug("generated %d spans\n", numOfSpansGenerated)
}

// generator is a single goroutine that generates traces and sends them to the spans channel.
// It runs until the stop channel is closed.
// The trace time is determined by the duration, and as soon as one trace is sent the next one is started.
func (s *TraceGenerator) generator(wg *sync.WaitGroup, counter chan int64) {
	s.mut.Lock()
	depth := s.depth
	nspans := s.nspans
	duration := s.duration
	stop := make(chan struct{})
	s.chans = append(s.chans, stop)
	s.mut.Unlock()

	ticker := time.NewTicker(duration)
	fielder := s.getFielder()
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
				s.generate_root(fielder, count, depth, nspans, duration)
			default:
				// do nothing, we're done, and the stop will be caught by the outer select
			}
		}
	}
}

func (s *TraceGenerator) Generate(opts *Options, wg *sync.WaitGroup, stop chan struct{}, counter chan int64) {
	defer wg.Done()
	ngenerators := float64(opts.Quantity.TPS) / s.TPS()
	uSgeneratorInterval := float64(opts.Quantity.RampTime.Microseconds()) / ngenerators
	generatorInterval := time.Duration(uSgeneratorInterval) * time.Microsecond

	s.log.Info("ngenerators: %f interval: %s\n", ngenerators, generatorInterval)
	state := Starting

	ticker := time.NewTicker(generatorInterval)
	defer ticker.Stop()

	// Create a long timer but stop it immediately so that we have a valid channel.
	// We'll Reset it in the Starting state if they specified a max time.
	stopTimer := time.NewTimer(time.Hour)
	stopTimer.Stop()

	for {
		select {
		case <-stop:
			s.log.Info("stopping generators from stop signal\n")
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
					s.log.Info("all generators started, switching to Running state\n")
					// if they want a timer, start it now
					if opts.Quantity.RunTime > 0 {
						// could have used AfterFunc, but we're already in a goroutine with a select
						// and it would have required a mutex to protect the state
						stopTimer.Reset(opts.Quantity.RunTime)
						defer stopTimer.Stop()
					}
					// and change to run state
					state = Running
				} else {
					s.log.Debug("starting new generator\n")
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
				s.log.Debug("killing off a generator\n")
				close(s.chans[0])
				s.chans = s.chans[1:]
				s.mut.Unlock()
			}
		case <-stopTimer.C:
			s.log.Info("stopping generators from timer\n")
			state = Stopping
		}
	}
}

func (s *TraceGenerator) TPS() float64 {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return 1.0 / s.duration.Seconds()
}
