package main

// TraceCounter sends an incrementing int64 on its channel, stopping
// when it has generated maxcount values or when it receives a value on stop.
// If maxcount is 0, it will run until it receives a value on stop.
// It returns true if it stopped because of a value on stop, false otherwise.
func TraceCounter(log Logger, maxcount int64, output chan int64, stop chan struct{}) bool {
	var count int64

	defer func() {
		log.Info("trace counter exiting after %d traces\n", count)
	}()

	for {
		if maxcount > 0 && count >= maxcount {
			return false
		}
		count++
		select {
		case <-stop:
			return true
		case output <- count:
			// do nothing
		}
	}
}
