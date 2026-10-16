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
	duration   time.Duration // the duration of each trace, unless backfilling
	interval   time.Duration // how often each generator goroutine starts a trace
	backfill   BackfillOptions
	getFielder func() *Fielder
	chans      []chan struct{}
	mut        sync.RWMutex
	log        Logger
	tracer     Sender
}

// make sure it implements Generator
var _ Generator = (*TraceGenerator)(nil)

// backfillTick is the shortest interval at which a generator goroutine starts
// backfilled traces.
const backfillTick = 10 * time.Millisecond

func NewTraceGenerator(tsender Sender, getFielder func() *Fielder, log Logger, opts *Options) *TraceGenerator {
	chans := make([]chan struct{}, 0)
	// Normally a generator spends its whole interval generating one trace.
	interval := opts.Format.TraceTime
	if opts.Backfill.Enabled() {
		// Backfilled traces take no time to generate, so the interval only paces
		// them; use the fewest generators that each start a trace at most every backfillTick.
		tps := float64(max(opts.Quantity.TPS, 1))
		ngenerators := math.Ceil(tps * backfillTick.Seconds())
		interval = time.Duration(ngenerators / tps * float64(time.Second))
	}
	return &TraceGenerator{
		depth:      opts.Format.Depth,
		nspans:     opts.Format.NSpans,
		duration:   opts.Format.TraceTime,
		interval:   interval,
		backfill:   opts.Backfill,
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
// - numOfSpansGenerated is a pointer to the number of spans generated so far. This is only used for debugging.
// If nspans is less than depth, the trace will be truncated at nspans.
// If nspans is greater than depth, some of the children will have siblings.
func (s *TraceGenerator) generate_spans(ctx context.Context, clk *spanClock, fielder *Fielder, level int, depth int, nspans int, timeRemaining time.Duration) int {
	if timeRemaining <= 0 {
		return 0
	}

	if nspans == 0 {
		// if there's still time remaining, sleep for the remainder of the time
		clk.Sleep(timeRemaining)
		return 0
	}

	// if we are at the bottom of the tree, we need to generate all the spans
	if depth == 0 && nspans > 0 {
		durationRemaining := time.Duration(rand.Intn(int(math.Ceil((float64(timeRemaining) / float64(nspans+1))))))
		spansCreated := 0
		for i := 0; i < nspans; i++ {
			durationThisSpan := durationRemaining / time.Duration(nspans-i)
			durationRemaining -= durationThisSpan
			clk.Sleep(durationThisSpan / 2)
			_, span := s.tracer.CreateSpan(ctx, fielder.GetServiceName(depth), level, fielder, clk.Now())
			spansCreated++

			clk.Sleep(durationThisSpan / 2)
			span.Send(clk.Now())
		}

		return spansCreated
	}

	spansAtThisLevel := 1
	if nspans > depth {
		// there is some chance that this level will have multiple spans based on the difference
		// between nspans and depth. (but we'll override this if it's a root span)
		// spansAtThisLevel is always between 1 and nspans
		spansAtThisLevel = 1 + rand.Intn(nspans-depth)
	}

	spancountsPerSpanAtThisLevel := make([]int, 0, spansAtThisLevel)
	if spansAtThisLevel == 1 {
		// this means we need to generate all spans at this level
		// so we can just give it all the counts
		spancountsPerSpanAtThisLevel = append(spancountsPerSpanAtThisLevel, nspans)
	} else {
		// split the counts among the spans at this level
		// we take a random portion of the counts for each span, then put the leftovers in a random span
		count := nspans
		spansPerPeer := nspans / spansAtThisLevel // always at least 1
		for i := 0; i < spansAtThisLevel; i++ {
			spancountsPerSpanAtThisLevel = append(spancountsPerSpanAtThisLevel, rand.Intn(spansPerPeer)+1)
			count -= spancountsPerSpanAtThisLevel[i]
		}
		// add the leftovers to a random span
		randomSpan := rand.Intn(spansAtThisLevel)
		spancountsPerSpanAtThisLevel[randomSpan] += count
	}

	durationRemaining := time.Duration(rand.Intn(int(math.Ceil((float64(timeRemaining) / float64(spansAtThisLevel+1))))))
	durationPerChild := (timeRemaining - durationRemaining) / time.Duration(spansAtThisLevel)

	spansCreated := 0
	for i := 0; i < spansAtThisLevel; i++ {
		durationThisSpan := durationRemaining / time.Duration(spansAtThisLevel-i)
		durationRemaining -= durationThisSpan
		clk.Sleep(durationThisSpan / 2)
		childctx, span := s.tracer.CreateSpan(ctx, fielder.GetServiceName(depth), level, fielder, clk.Now())
		spansCreated++

		nextDepth := depth - 1
		nextSpanCount := spancountsPerSpanAtThisLevel[i] - 1
		childSpansCreated := s.generate_spans(childctx, clk, fielder, level+1, nextDepth, nextSpanCount, durationPerChild)
		clk.Sleep(durationThisSpan / 2)
		spansCreated += childSpansCreated
		span.Send(clk.Now())
	}

	return spansCreated
}

// generate_root generates a trace lasting timeRemaining. If clk is a backfill
// clock, the trace starts at clk.Now() and nothing sleeps.
func (s *TraceGenerator) generate_root(clk *spanClock, fielder *Fielder, count int64, depth int, nspans int, timeRemaining time.Duration) {
	ctx := context.Background()
	start := clk.Now()
	ctx, root := s.tracer.CreateTrace(ctx, fielder.GetServiceName(depth), fielder, count, start)
	thisSpanDuration := time.Duration(rand.Intn(int(timeRemaining) / (nspans + 1)))
	childDuration := (timeRemaining - thisSpanDuration)

	now := time.Now()
	clk.Sleep(thisSpanDuration / 2)
	totalSpanCreated := s.generate_spans(ctx, clk, fielder, 1, depth-1, nspans-1, childDuration)
	clk.Sleep(thisSpanDuration / 2)
	totalSpanCreated++
	end := clk.Now()
	if !start.IsZero() {
		// the children may not use all of their time, so end a backfilled root
		// exactly when its sampled duration is up
		end = start.Add(timeRemaining)
	}
	root.Send(end)
	s.log.Debug("generated %d spans within %v\n", totalSpanCreated, time.Since(now))
}

// generator is a single goroutine that generates traces and sends them to the spans channel.
// It runs until the stop channel is closed.
// The trace time is determined by the duration, and as soon as one trace is sent the next one is started.
func (s *TraceGenerator) generator(wg *sync.WaitGroup, counter chan int64, stop chan struct{}) {
	s.mut.RLock()
	depth := s.depth
	nspans := s.nspans
	duration := s.duration
	interval := s.interval
	s.mut.RUnlock()

	ticker := time.NewTicker(interval)
	fielder := s.getFielder()
	defer wg.Done()

	for {
		select {
		case <-stop:
			ticker.Stop()
			return
		case <-ticker.C:
			if !s.backfill.Enabled() {
				// generate a trace if we haven't been stopped by the counter
				select {
				case count := <-counter:
					s.generate_root(&spanClock{}, fielder, count, depth, nspans, duration)
				default:
					// do nothing, we're done, and the stop will be caught by the outer select
				}
				continue
			}
			// Backfilled traces are quick to make, so many ticks compete for the
			// counter; wait for it rather than skip the tick when it isn't ready.
			select {
			case count := <-counter:
				start, d := s.backfill.pickTrace(time.Now())
				s.generate_root(&spanClock{now: start}, fielder, count, depth, nspans, d)
			case <-stop:
				ticker.Stop()
				return
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
					// register the generator's stop channel before starting it, so that
					// stopping during the ramp-up can't miss it
					genstop := make(chan struct{})
					s.mut.Lock()
					s.chans = append(s.chans, genstop)
					s.mut.Unlock()
					wg.Add(1)
					go s.generator(wg, counter, genstop)
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
	return 1.0 / s.interval.Seconds()
}
