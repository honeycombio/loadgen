package main

import (
	"errors"
	"math"
	"time"

	"pgregory.net/rand"
)

// BackfillOptions configure backfill mode, in which loadgen generates traces
// that have already finished instead of waiting for them to happen: each trace
// ends at a random time in the last Window and lasts a long-tailed random
// duration, and its spans carry explicit timestamps.
type BackfillOptions struct {
	Window         time.Duration `long:"backfillwindow" description:"if nonzero, generate already-finished traces without sleeping: each trace ends at a random time in this window before now, and its duration comes from durationmedian, durationsigma and durationmax (tracetime is ignored)" default:"0s" yaml:",omitempty"`
	DurationMedian time.Duration `long:"durationmedian" description:"for backfill only, the median trace duration (of a log-normal distribution)" default:"30s" yaml:",omitempty"`
	DurationSigma  float64       `long:"durationsigma" description:"for backfill only, the standard deviation of the natural log of the trace duration; larger values give a longer tail" default:"2" yaml:",omitempty"`
	DurationMax    time.Duration `long:"durationmax" description:"for backfill only, the maximum trace duration; the distribution is truncated here, not clamped" default:"4h" yaml:",omitempty"`
}

func (b *BackfillOptions) Enabled() bool {
	return b.Window > 0
}

func (b *BackfillOptions) Validate() error {
	if !b.Enabled() {
		return nil
	}
	if b.DurationMedian <= 0 || b.DurationMax <= 0 || b.DurationSigma <= 0 {
		return errors.New("backfill requires positive durationmedian, durationsigma and durationmax")
	}
	return nil
}

// pickTrace chooses when a backfilled trace happened: it ends at a uniformly
// random time in [now-Window, now] and lasts a duration from sampleDuration.
func (b *BackfillOptions) pickTrace(now time.Time) (start time.Time, duration time.Duration) {
	end := now.Add(-time.Duration(rand.Float64() * float64(b.Window)))
	duration = b.sampleDuration(rand.Float64())
	return end.Add(-duration), duration
}

// sampleDuration maps u, uniform in [0, 1), to a duration from a log-normal
// distribution with the configured median and sigma, truncated at DurationMax.
// Truncating (scaling u into [0, F(max)) before inverting the CDF) keeps the
// shape of the tail instead of piling clamped samples up at the maximum.
// Durations are at least 1ms so that every span in the trace gets some time.
func (b *BackfillOptions) sampleDuration(u float64) time.Duration {
	mu := math.Log(float64(b.DurationMedian))
	sigma := b.DurationSigma
	fmax := 0.5 * math.Erfc(-(math.Log(float64(b.DurationMax))-mu)/(sigma*math.Sqrt2))
	z := math.Sqrt2 * math.Erfinv(2*u*fmax-1)
	d := time.Duration(math.Exp(mu + sigma*z))
	return min(max(d, time.Millisecond), b.DurationMax)
}

// spanClock supplies the timestamps that the generator passes to the sender.
// The zero value follows the wall clock: Now returns the zero time, which tells
// senders to stamp spans with the current time, and Sleep really sleeps. A
// backfill clock starts at a trace's backdated start time, and Sleep only
// advances it.
type spanClock struct {
	now time.Time
}

func (c *spanClock) Now() time.Time {
	return c.now
}

func (c *spanClock) Sleep(d time.Duration) {
	if c.now.IsZero() {
		time.Sleep(d)
		return
	}
	c.now = c.now.Add(d)
}
