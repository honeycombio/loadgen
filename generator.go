package main

import (
	"fmt"
	"math/rand"
	"sync"
)

// A Generator generates traces and sends the individual spans to the spans channel. Its
// Generate method should be run in a goroutine, and generates a single trace,
// taking opts.Duration to do so. Its TPS method returns the number of traces
// per second it is currently generating.
type Generator interface {
	Generate(opts Options, wg *sync.WaitGroup, stop chan struct{}, counter chan int64)
	TPS() float64
}

// randID creates a random byte array of length l and returns it as a hex string.
func randID(l int) string {
	id := make([]byte, l)
	for i := 0; i < l; i++ {
		id[i] = byte(rand.Intn(256))
	}
	return fmt.Sprintf("%x", id)
}

type GeneratorState int

const (
	Starting GeneratorState = iota
	Running
	Stopping
)
