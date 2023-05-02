package main

import (
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/goware/urlx"
	"github.com/jessevdk/go-flags"
)

var ResourceLibrary = "loadgen"
var ResourceVersion = "dev"

type Options struct {
	Telemetry struct {
		Host     string `long:"host" description:"the url of the host to receive the telemetry (or honeycomb, dogfood, local)" default:"honeycomb"`
		Insecure bool   `long:"insecure" description:"use this for insecure http (not https) connections"`
		Dataset  string `long:"dataset" description:"sends all traces to the given dataset" env:"HONEYCOMB_DATASET" default:"loadgen"`
		APIKey   string `long:"apikey" description:"the honeycomb API key" env:"HONEYCOMB_API_KEY"`
	} `group:"Telemetry Options"`
	Format struct {
		Depth     int           `long:"depth" description:"the average depth of a trace" default:"3"`
		SpanCount int           `long:"spancount" description:"the average number of spans in a trace" default:"3"`
		SpanWidth int           `long:"spanwidth" description:"the average number of random fields in a span beyond the standard ones" default:"5"`
		Duration  time.Duration `long:"duration" description:"the duration of a trace" default:"1s"`
	} `group:"Trace Format Options"`
	Quantity struct {
		TPS        int           `long:"tps" description:"the maximum number of traces to generate per second" default:"1"`
		TraceCount int64         `long:"tracecount" description:"the maximum number of traces to generate (0 means no limit, but if maxtime is not specified defaults to 1)" default:"0"`
		MaxTime    time.Duration `long:"maxtime" description:"the maximum time to spend generating traces at max TPS (0 means no limit)" default:"0s"`
		Ramp       time.Duration `long:"ramp" description:"seconds to spend ramping up or down to the desired TPS" default:"1s"`
	} `group:"Quantity Options"`
	Output struct {
		Sender   string `long:"sender" description:"type of sender" choice:"honeycomb" choice:"otel" choice:"print" choice:"dummy" default:"honeycomb"`
		Protocol string `long:"protocol" description:"for otel only, protocol to use" choice:"grpc" choice:"protobuf" choice:"json" default:"grpc"`
	} `group:"Output Options"`
	LogLevel string `long:"loglevel" description:"level of logging" choice:"debug" choice:"info" choice:"warn" choice:"error" default:"warn"`
	Seed     string `long:"seed" description:"string seed for random number generator (defaults to dataset name)"`
	apihost  *url.URL
}

func (o *Options) DebugLevel() int {
	switch o.LogLevel {
	case "debug":
		return 3
	case "info":
		return 2
	case "warn":
		return 1
	case "error":
		return 0
	default:
		return 0
	}
}

// parses the host information and returns a cleaned-up version to make
// it easier to make sure that things are properly specified
// exits if it can't make sense of it
func parseHost(log Logger, host string, insecure bool) *url.URL {
	switch host {
	case "honeycomb":
		host = "https://api.honeycomb.io:443"
	case "dogfood":
		host = "https://api-dogfood.honeycomb.io:443"
	case "local":
		host = "http://localhost:8889"
	default:
	}

	// if the scheme is not specified, fall back to the value of the insecure flag
	defaultScheme := "https"
	if insecure {
		defaultScheme = "http"
	}
	u, err := urlx.ParseWithDefaultScheme(host, defaultScheme)
	if err != nil {
		log.Fatal("unable to parse host: %s\n", err)
	}
	port := u.Port()
	if port == "" {
		port = "4317" // default GRPC port
	}
	return u
}

func main() {
	var args Options

	parser := flags.NewParser(&args, flags.Default)
	parser.Usage = `[OPTIONS]

	loadgen generates telemetry loads for performance testing, load testing, and functionality testing.
	It allows you to specify the number of spans in a trace, the depth (nesting level) of traces, the
	number of different service names, as well as the number of fields in spans.
	It supports setting the number of traces per second it generates, and can generate a specific
	quanity of traces, or run for a specific amount of time, or both. It can also control the speed at
	which it ramps up and down to the target rate.
	It can generate OTLP or Honeycomb-formatted traces, and send them to Honeycomb or (for OTLP) to
	any OTel agent.
	`

	if _, err := parser.Parse(); err != nil {
		switch flagsErr := err.(type) {
		case *flags.Error:
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
			os.Exit(1)
		default:
			os.Exit(1)
		}
	}

	// if we're not given a trace count or a max time, send only 1 trace
	if args.Quantity.TraceCount == 0 && args.Quantity.MaxTime == 0 {
		args.Quantity.TraceCount = 1
	}

	log := NewLogger(args.DebugLevel())
	args.apihost = parseHost(log, args.Telemetry.Host, args.Telemetry.Insecure)

	log.Warn("host: %s, dataset: %s, apikey: ...%4.4s\n", args.apihost.String(), args.Telemetry.Dataset, args.Telemetry.APIKey)

	var sender Sender
	switch args.Output.Sender {
	case "dummy":
		sender = NewSenderDummy(log, args)
	case "print":
		sender = NewSenderPrint(log, args)
	case "honeycomb":
		sender = NewSenderHoneycomb(args)
	case "otel":
		sender = NewSenderOTel(log, args)
	}

	// create a stop channel so we can shut down gracefully
	stop := make(chan struct{})
	// and a waitgroup so we can wait for everything to finish
	wg := &sync.WaitGroup{}

	// catch ctrl-c and close the stop channel
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt, syscall.SIGTERM)
	// we don't want a wait group for this one, or we'll never exit
	go func() {
		select {
		case <-sigch:
			log.Warn("\nshutting down from operating system signal\n")
			close(stop)
			return
		case <-stop:
			return
		}
	}()

	// start the sender to receive spans and forward them appropriately
	// sender.Run(wg, dest, stop)

	// Start the trace counter to keep track of how many traces we've sent and
	// stop the generator when we've reached the limit. We don't want to close
	// counterChan until we're done with everything else because the generators
	// block on it and we want that.
	wg.Add(1)
	counterChan := make(chan int64)
	defer close(counterChan)
	go func() {
		if !TraceCounter(log, args.Quantity.TraceCount, counterChan, stop) {
			// give the senders a chance to finish sending
			time.Sleep(1 * time.Second)
			close(stop)
		}
		wg.Done()
	}()

	// start the load generator to create spans and send them on the source chan
	var generator Generator = NewTraceGenerator(sender, log, args)
	wg.Add(1)
	go generator.Generate(args, wg, stop, counterChan)

	// wait for things to finish
	wg.Wait()
	sender.Close()
}
