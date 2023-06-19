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
		Depth     int           `long:"depth" description:"the nesting depth of each trace" default:"3"`
		NSpans    int           `long:"nspans" description:"the total number of spans in a trace" default:"3"`
		Extra     int           `long:"extra" description:"the number of random fields in a span beyond the standard ones" default:"0"`
		TraceTime time.Duration `long:"tracetime" description:"the duration of a trace" default:"1s"`
	} `group:"Trace Format Options"`
	Quantity struct {
		TPS        int           `long:"tps" description:"the maximum number of traces to generate per second" default:"1"`
		TraceCount int64         `long:"tracecount" description:"the maximum number of traces to generate (0 means no limit, but if runtime is not specified defaults to 1)" default:"0"`
		RunTime    time.Duration `long:"runtime" description:"the maximum time to spend generating traces at max TPS (0 means no limit)" default:"0s"`
		RampTime   time.Duration `long:"ramptime" description:"duration to spend ramping up or down to the desired TPS" default:"1s"`
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
	var opts Options

	parser := flags.NewParser(&opts, flags.Default)
	parser.Usage = `[OPTIONS] [FIELD=VALUE]...

	loadgen generates telemetry loads for performance testing, load testing, and
	functionality testing. It allows you to specify the number of spans in a trace,
	the depth (nesting level) of traces, the duration of traces, as well as the
	number of fields in spans. It supports setting the number of traces per second
	it generates, and can generate a specific quanity of traces, or run for a
	specific amount of time, or both. It can also control the speed at which it
	ramps up and down to the target rate.

	It can generate OTLP or Honeycomb-formatted traces, and send them to Honeycomb
	or (for OTLP) to any OTel agent.

	You can specify fields to be added to each span by specifying them on the command
	line. Each field should be specified as FIELD=VALUE. The value can be a constant
	(and will be sent as the appropriate type), or a generator function starting with /.
	Allowed generators are /i, /ir, /ig, /f, /fr, /fg, /s, /sx, /sw, /b, optionally
	followed by a single number or a comma-separated pair of numbers.
	Example generators:
		- /s -- alphanumeric string of length 16
		- /sx32 -- hex string of 32 characters
		- /sw12 -- pronounceable words with cardinality 12 with rectangular distribution
		- /sq4 -- pronounceable words with cardinality 4 with quadratic distribution
		- /ir100 -- int in a range of 0 to 100
		- /fg50,30 -- float in a gaussian distribution with mean 50 and stddev 30
		- /b -- boolean, true or false

	Field names can be alphanumeric with underscores. If a field name is prefixed with
	a number and a dot (e.g. 1.foo=bar) the field will only be injected into spans at
	that level of nesting (where 0 is the root span).

	For full details, see https://github.com/honeycombio/loadgen/
	`

	args, err := parser.Parse()
	if err != nil {
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

	// if we're not given a trace count or a runtime, send only 1 trace
	if opts.Quantity.TraceCount == 0 && opts.Quantity.RunTime == 0 {
		opts.Quantity.TraceCount = 1
	}

	log := NewLogger(opts.DebugLevel())

	fielder, err := NewFielder(opts.Seed, args, opts.Format.Extra, opts.Format.Depth)
	if err != nil {
		log.Fatal("unable to create fields as specified: %s\n", err)
	}

	opts.apihost = parseHost(log, opts.Telemetry.Host, opts.Telemetry.Insecure)

	log.Info("host: %s, dataset: %s, apikey: ...%4.4s\n", opts.apihost.String(), opts.Telemetry.Dataset, opts.Telemetry.APIKey)

	var sender Sender
	switch opts.Output.Sender {
	case "dummy":
		sender = NewSenderDummy(log, opts)
	case "print":
		sender = NewSenderPrint(log, opts)
	case "honeycomb":
		sender = NewSenderHoneycomb(opts)
	case "otel":
		sender = NewSenderOTel(log, opts)
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

	// Start the trace counter to keep track of how many traces we've sent and
	// stop the generator when we've reached the limit. We don't want to close
	// counterChan until we're done with everything else because the generators
	// block on it and we want that.
	wg.Add(1)
	counterChan := make(chan int64)
	defer close(counterChan)
	go func() {
		if !TraceCounter(log, opts.Quantity.TraceCount, counterChan, stop) {
			// give the senders a chance to finish sending
			time.Sleep(1 * time.Second)
			close(stop)
		}
		wg.Done()
	}()

	// start the load generator to create spans and send them on the source chan
	var generator Generator = NewTraceGenerator(sender, fielder, log, opts)
	wg.Add(1)
	go generator.Generate(opts, wg, stop, counterChan)

	// wait for things to finish
	wg.Wait()
	sender.Close()
}
