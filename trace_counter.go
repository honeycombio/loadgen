package main

// TraceCounter reads spans from src and writes them to dest, stopping
// when it has read maxcount spans or when it receives a value on stop.
// If maxcount is 0, it will run until it receives a value on stop.
// It returns true if it stopped because of a value on stop, false otherwise.
func TraceCounter(log Logger, src, dest chan *Span, maxcount int64, stop chan struct{}) bool {
	var count int64

	defer func() {
		log.Printf("span counter exiting after %d spans\n", count)
	}()

	for {
		select {
		case <-stop:
			return true
		case span := <-src:
			dest <- span
			if span.IsRootSpan() {
				count++
			}
			if maxcount > 0 && count >= maxcount {
				return false
			}
		}
	}
}
