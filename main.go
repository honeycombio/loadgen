package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goware/urlx"
	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v3"
)

var ResourceLibrary = "loadgen"
var ResourceVersion = "dev"

type Options struct {
	Telemetry struct {
		Host     string `long:"host" description:"the url of the host to receive the telemetry (or honeycomb, dogfood, local)" default:"honeycomb"`
		Insecure bool   `long:"insecure" description:"use this for insecure http (not https) connections" yaml:",omitempty"`
		Dataset  string `long:"dataset" description:"sends all traces to the given dataset" env:"HONEYCOMB_DATASET" default:"loadgen"`
		APIKey   string `long:"apikey" description:"the honeycomb API key(*)" env:"HONEYCOMB_API_KEY" yaml:"-"`
	} `group:"Telemetry Options"`
	Format struct {
		Depth     int           `long:"depth" description:"the nesting depth of each trace" default:"3"`
		NSpans    int           `long:"nspans" description:"the total number of spans in a trace" default:"3"`
		Extra     int           `long:"extra" description:"the number of random fields in a span beyond the standard ones" default:"0" yaml:",omitempty"`
		TraceTime time.Duration `long:"tracetime" description:"the duration of a trace" default:"1s"`
	} `group:"Trace Format Options"`
	Quantity struct {
		TPS        int           `long:"tps" description:"the maximum number of traces to generate per second" default:"1"`
		TraceCount int64         `long:"tracecount" description:"the maximum number of traces to generate (0 means no limit, but if runtime is not specified defaults to 1)" default:"0" yaml:",omitempty"`
		RunTime    time.Duration `long:"runtime" description:"the maximum time to spend generating traces at max TPS (0 means no limit)" default:"0s" yaml:",omitempty"`
		RampTime   time.Duration `long:"ramptime" description:"duration to spend ramping up or down to the desired TPS" default:"1s"`
	} `group:"Quantity Options"`
	Output struct {
		Sender             string        `long:"sender" description:"type of sender" choice:"honeycomb" choice:"otel" choice:"print" choice:"dummy" default:"honeycomb"`
		Protocol           string        `long:"protocol" description:"for otel only, protocol to use" choice:"grpc" choice:"http" default:"grpc"`
		MaxQueueSize       int           `long:"maxqueuesize" description:"for otel only, maximum number of spans to queue before dropping" default:"0"`
		MaxExportBatchSize int           `long:"maxexportbatchsize" description:"for otel only, maximum number of spans to export at once" default:"0"`
		BatchTimeout       time.Duration `long:"batchtimeout" description:"for otel only, maximum time to wait before sending a batch" default:"0s"`
		ExportTimeout      time.Duration `long:"exporttimeout" description:"for otel only, maximum time to wait for a batch to be sent" default:"0s"`
	} `group:"Output Options"`
	Global struct {
		LogLevel  string `long:"loglevel" description:"level of logging" choice:"debug" choice:"info" choice:"warn" choice:"error" default:"warn"`
		DebugPort int    `long:"debugport" description:"port to listen on for pprof(*)" default:"-1" yaml:"-"`
		Seed      string `long:"seed" description:"string seed for random number generator (defaults to dataset name)" yaml:",omitempty"`
		Config    string `long:"config" description:"name of config file to load(*)" default:"" yaml:"-"`
		WriteCfg  string `long:"writecfg" description:"write effective YAML config to the specified output file and quit(*)" default:"" yaml:"-"`
	} `group:"Global Options"`
	Fields  map[string]string `yaml:"fields,omitempty"`
	apihost *url.URL
}

func newOptions() *Options {
	return &Options{Fields: make(map[string]string)}
}

func (o *Options) CopyStarredFieldsFrom(other *Options) {
	o.Telemetry.APIKey = other.Telemetry.APIKey
	o.Global.DebugPort = other.Global.DebugPort
	o.Global.Config = other.Global.Config
	o.Global.WriteCfg = other.Global.WriteCfg
}

func (o *Options) DebugLevel() int {
	switch o.Global.LogLevel {
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
		u.Host = fmt.Sprintf("%s:4317", u.Host) // default GRPC port
	}
	return u
}

func ReadConfig(opts *Options, filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(f)
	err = dec.Decode(opts)
	if err != nil {
		return err
	}
	log.Printf("read config from %s\n", filename)
	return nil
}

func WriteConfig(opts *Options, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	enc := yaml.NewEncoder(f)
	err = enc.Encode(opts)
	if err != nil {
		return err
	}
	log.Printf("wrote config to %s\n", filename)
	return nil
}

func main() {
	cmdopts := newOptions()

	parser := flags.NewParser(cmdopts, flags.Default)
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

	You can specify fields to be added to each span. Each field should be specified as
	FIELD=VALUE. The value can be a constant (and will be sent as the appropriate type),
	or a generator function starting with /.
	Allowed generators are /i, /ir, /ig, /f, /fr, /fg, /s, /sx, /sw, /b, /k, optionally
	followed by a single number or a comma-separated pair of numbers.
	Example generators:
		- /s -- alphanumeric string of length 16
		- /sx32 -- hex string of 32 characters
		- /sw12 -- pronounceable words with cardinality 12 with rectangular distribution
		- /sq4 -- pronounceable words with cardinality 4 with quadratic distribution
		- /ir100 -- int in a range of 0 to 100
		- /fg50,30 -- float in a gaussian distribution with mean 50 and stddev 30
		- /b33.3 -- boolean, true or false -- probability of true is 33.3% (default 50%)
		- /u -- https url-like, no query string, two path segments; default cardinality is 10/10 but can be changed like /u3,20
		- /uq -- as /u above, but with query string containing a random key word with a completely random value
		- /st -- an http status code by default reflecting 95% 200s, 4% 400s, 1% 500s. 400s and 500s can be changed like /st10,0.1.
		- /k50,60 -- an intermittent key field with total cardinality 50, but decreasing key frequency. All keys only arrive after 60 seconds

	Field names can be alphanumeric with underscores. If a field name is prefixed with
	a number and a dot (e.g. 1.foo=bar) the field will only be injected into spans at
	that level of nesting (where 0 is the root span).

	Fields can also be specified in the config file as key/value pairs under the "fields" key.

	Options can be set in a config file, or on the command line; to specify them in the
	config file, specify it on the command line with "--config=FILENAME". The config file
	format is YAML; see "example.yml" for an example.

	Note: If a config file is used, it MUST be used for all options, except for the ones
	marked in the help text with (*) -- these fields CANNOT be set in the config file.

	For more detail, see https://github.com/honeycombio/loadgen/
	`

	// read the command line and envvars into cmdargs
	args, err := parser.Parse()
	if err != nil {
		switch flagsErr := err.(type) {
		case *flags.Error:
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
		}
		log.Fatalf("error reading command line: %v", err)
	}

	opts := newOptions()
	if cmdopts.Global.Config != "" {
		if err := ReadConfig(opts, cmdopts.Global.Config); err != nil {
			log.Fatalf("err %v -- unable to read config file %s", err, cmdopts.Global.Config)
		}
		opts.CopyStarredFieldsFrom(cmdopts)
	} else {
		opts = cmdopts // we don't have to read from a file
	}

	// split the args into opts.Fields, potentially overwriting
	for _, arg := range args {
		s := strings.SplitN(arg, "=", 2)
		if len(s) < 2 {
			log.Fatalf("field `%s` missing required '='", s)
		}
		opts.Fields[s[0]] = s[1]
	}

	if opts.Global.WriteCfg != "" {
		err := WriteConfig(opts, opts.Global.WriteCfg)
		if err != nil {
			log.Fatalf("unable to write config: %s\n", err)
		}
		os.Exit(0)
	}

	if opts.Global.Seed == "" {
		opts.Global.Seed = opts.Telemetry.Dataset
	}

	if opts.Global.DebugPort > 0 {
		go func() {
			http.ListenAndServe(fmt.Sprintf("localhost:%d", opts.Global.DebugPort), nil)
		}()
	}

	// if we're not given a trace count or a runtime, send only 1 trace
	if opts.Quantity.TraceCount == 0 && opts.Quantity.RunTime == 0 {
		opts.Quantity.TraceCount = 1
	}

	log := NewLogger(opts.DebugLevel())

	getFielderFn := func() *Fielder {
		getFielder, err := NewFielder(opts.Global.Seed, opts.Fields, opts.Format.Extra, opts.Format.Depth)
		if err != nil {
			log.Fatal("unable to create fields as specified: %s\n", err)
		}
		return getFielder
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
	var generator Generator = NewTraceGenerator(sender, getFielderFn, log, opts)
	wg.Add(1)
	go generator.Generate(opts, wg, stop, counterChan)

	// wait for things to finish
	wg.Wait()
	sender.Close()
}
