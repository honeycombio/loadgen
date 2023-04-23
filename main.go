package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/goware/urlx"
	"github.com/jessevdk/go-flags"
)

var ResourceLibrary = "loadgen"
var ResourceVersion = "dev"

// loadgen generates telemetry loads for performance testing It can generate
// traces (and eventually metrics and logs) It can send them to honeycomb or to
// a local agent, and it can generate OTLP or Honeycomb-formatted traces. It's
// highly configurable:
//
// - nservices is the number of communicating services to simulate; they will be
// divided into a triangular tree of services where each service will only call its siblings and the next rank.
// Services are named for spices.
//
// - depth is the average depth (nesting level) of a trace.
// - spancount is the average number of spans in a trace.
// If spancount is less than depth, the trace will be truncated at spancount.
// If spancount is greater than depth, some of the spans will have siblings.
//
// - spanwidth is the average number of fields in a span; this will vary by
// service but will be the same for all calls to a given service, and the names
// and types of all fields for an service will be consistent even across runs of loadgen (randomness is seeded by service name).
// Fields in a span will be randomly selected between the following types:
// #   - int (rectangular min/max)
// #   - int (gaussian mean/stddev)
// #   - int upcounter
// #   - int updowncounter (min/max)
// #   - float (rectangular min/max)
// #   - float (gaussian mean/stddev)
// #   - string (from list)
// #   - string (random min/max length)
// #   - bool
// In addition, every span will always have the following fields:
// #   - service name
// #   - trace id
// #   - span id
// #   - parent span id
// #   - duration_ms
// #   - start_time
// #   - end_time
// #   - process_id (the process id of the loadgen process)
//
// - avgDuration is the average duration of a trace's root span in milliseconds; individual
// spans will be randomly assigned durations that will fit within the root span's duration.
//
// - maxTime is the total amount of time to spend generating traces (0 means no limit)
// - tracesPerSecond is the number of root spans to generate per second
// - traceCount is the maximum number of traces to generate; as soon as TraceCount is reached, the process stops (0 means no limit)
// - rampup and rampdown are the number of seconds to spend ramping up and down to the desired TPS

// Functionally, the system works by spinning up a number of goroutines, each of which
// generates a stream of spans. The number of goroutines needed will equal tracesPerSecond * avgDuration.
// Rampup and rampdown are handled only by increasing or decreasing the number of goroutines.

// If a mix of different kinds of traces is desired, multiple loadgen processes can be run.

// servicenames is a list of common spices
var servicenames = []string{
	"allspice", "anise", "basil", "bay", "black pepper", "cardamom", "cayenne",
	"cinnamon", "cloves", "coriander", "cumin", "curry", "dill", "fennel", "fenugreek",
	"garlic", "ginger", "marjoram", "mustard", "nutmeg", "oregano", "paprika", "parsley",
	"pepper", "rosemary", "saffron", "sage", "salt", "tarragon", "thyme", "turmeric", "vanilla",
	"caraway", "chili", "masala", "lemongrass", "mint", "poppy", "sesame", "sumac", "mace",
	"nigella", "peppercorn", "wasabi",
}

type Options struct {
	Host       string        `long:"host" description:"the url of the host to receive the metrics (or honeycomb, dogfood, localhost)" default:"honeycomb"`
	Insecure   bool          `long:"insecure" description:"use this for http connections"`
	UseEvents  bool          `long:"useevents" description:"use this to send honeycomb-formatted data instead of otlp traces"`
	Dataset    string        `long:"dataset" description:"if set, sends all traces to the given dataset; otherwise, sends them to the dataset named for the service"`
	APIKey     string        `long:"apikey" description:"the honeycomb API key"`
	NServices  int           `long:"nservices" description:"the number of services to simulate" default:"1"`
	Depth      int           `long:"depth" description:"the average depth of a trace" default:"3"`
	SpanCount  int           `long:"spancount" description:"the average number of spans in a trace" default:"3"`
	SpanWidth  int           `long:"spanwidth" description:"the average number of random fields in a span beyond the standard ones" default:"10"`
	Duration   time.Duration `long:"duration" description:"the duration of a trace" default:"1s"`
	MaxTime    int           `long:"maxtime" description:"the maximum time to spend generating traces (0 means no limit)" default:"0"`
	TPS        int           `long:"tracespersecond" description:"the number of traces to generate per second" default:"1"`
	TraceCount int           `long:"tracecount" description:"the maximum number of traces to generate (0 means no limit)" default:"1"`
	Rampup     int           `long:"rampup" description:"the number of seconds to spend ramping up to the desired TPS" default:"0"`
	Rampdown   int           `long:"rampdown" description:"the number of seconds to spend ramping down to 0 TPS" default:"0"`
	Verbose    bool          `long:"verbose" description:"set to print status and progress messages"`
}

// parses the host information and returns a cleaned-up version to make
// it easier to make sure that things are properly specified
// exits if it can't make sense of it
func parseHost(host string, insecure bool) (string, bool) {
	switch host {
	case "honeycomb":
		return "api.honeycomb.io:443", false
	case "dogfood":
		return "api-dogfood.honeycomb.io:443", false
	case "localhost":
		return "localhost:8889", true
	default:
		// if the scheme is not specified, fall back to the value of the insecure flag
		defaultScheme := "https"
		if insecure {
			defaultScheme = "http"
		}
		u, err := urlx.ParseWithDefaultScheme(host, defaultScheme)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		port := u.Port()
		if port == "" {
			port = "4317" // default GRPC port
		}

		// it's insecure if it's not https
		return fmt.Sprintf("%s:%s", u.Hostname(), u.Port()), u.Scheme != "https"
	}
}

func main() {
	var args Options

	_, err := flags.Parse(&args)
	if err != nil {
		// fmt.Println(err)
		os.Exit(1)
	}

	host, insecure := parseHost(args.Host, args.Insecure)

	if args.Verbose {
		fmt.Printf("host: %s, dataset: %s, apikey: %s, insecure: %t\n\n", host, args.Dataset, args.APIKey, insecure)
	}

	var sender Sender
	if args.UseEvents {
		sender = NewHoneycombSender(args, host)
	} else {
		// ctx := context.Background()

		// var headers = map[string]string{
		// 	"x-honeycomb-team":    args.APIKey,
		// 	"x-honeycomb-dataset": args.Dataset,
		// }
		sender = NewOTelHoneySender(args.Dataset, args.APIKey, host, insecure)
	}
	spans := make(chan *Span, 1000)
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	sender.Run(wg, spans, stop)

	// start the load generator
	generator := NewSimpleGenerator(args)
	wg.Add(1)
	go generator.Generate(wg, spans, stop)

	// wait for things to finish
	wg.Wait()
	time.Sleep(1 * time.Second)
}
