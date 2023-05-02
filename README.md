# loadgen

## A flexible, Honeycomb-aware Telemetry load generator for traces

`loadgen` generates telemetry loads for performance testing, load testing, and
functionality testing. It allows you to specify the number of spans in a trace,
the depth (nesting level) of traces, the number of different service names, as
well as the number of fields in spans. It supports setting the number of traces
per second it generates, and can generate a specific quanity of traces, or run
for a specific amount of time, or both. It can also control the speed at which
it ramps up and down to the target rate.

It can generate traces in Honeycomb's proprietary protocol as well as all the
OTel-standard protocols, and it can send them to Honeycomb or any OTel agent.

## Quickstart

Assuming you have a recent version of Go installed:

Install:
`go install github.com/honeycombio/loadgen`

Get Usage information:
`loadgen -h`

Generate a single trace, 3 spans deep, and print it to the console:
`

Send 3 traces to Honeycomb's production service, assuming you have an API key in the environment as HONEYCOMB_API_KEY:
`loadgen --dataset=loadtest --tracecount=3`

Send 100 deep traces per second for 10 seconds, with ramp times of 5 seconds
`loadgen --dataset=loadtest --tps=100 --depth=10 --spancount=10 --maxtime=10s --ramp=5s`

## Motivation

We wanted a fast, easy-to-control tool that can send large quantities of traces
of variable shape, using either Honeycomb or OpenTelemetry.

The exact content of the traces isn't that important, but it is important to be
able to control it in a variety of ways. We wanted to be able to send simple
spans, or complex, deeply-nested traces, or shallow-but-wide traces. We wanted
to be able to control the number of fields in a trace, but we don't want them to
be purely random, but to have consistent datatypes and content shape.

We wanted to be able to send large volumes of traces to do load testing. That
also includes being able to ramp up and ramp down the volume at predictable
rates.

And we wanted it to be easy to install and use on a variety of platforms without
a lot of fiddling.

There were alternatives:

* The OTel telemetrygen tool only generates very simple traces and doesn't support Honeycomb format directly.
* The Locust load testing tool is very controllable, but requires installing Python and a virtual environment, and while it's fairly straightforward to generate Honeycomb trace data, it's much harder to make it do OpenTelemetry.
* Honeycomb has several internal tools designed to demonstrate the breadth and variety of our libraries, but they don't have a lot of control over their output and require setting up a complex set of containers.

In short, none of these met most of the goals, so a new tool seemed justified.

## Details

loadgen generates telemetry trace loads for performance testing. It can send
traces to honeycomb or to a local agent, and it can generate OTLP or
Honeycomb-formatted traces. It's highly configurable:

- depth is the depth (nesting level) of a trace.
- spancount is the number of spans in a trace.
- spanwidth is the number of extra fields in a span beyond the standard ones

If spancount is less than depth, the trace will be truncated at the depth of spancount.
If spancount is greater than depth, some of the spans will have siblings.

The names and types of all extra (random) fields will be consistent for a given
dataset, even across runs of loadgen so that datasets have longterm consistency.
Randomness is normally seeded by dataset name but if needed the seed can be set
to ensure consistency across multiple datasets.

Fields in a span will be randomly selected between the following types:
 - int (rectangular min/max)
 - int (gaussian mean/stddev)
 - float (rectangular min/max)
 - float (gaussian mean/stddev)
 - string (from list)
 - string (random min/max length)
 - bool

In addition, every span will always have the following fields:
 - service name
 - trace id
 - span id
 - parent span id
 - duration_ms
 - start_time
 - end_time
 - process_id (the process id of the loadgen process)
 - goroutine_id (the ID of the goroutine producing that particular trace)

- Duration is the average duration of a trace's root span in milliseconds; individual
spans will be randomly assigned durations that will fit within the root span's duration.

- maxTime is the total amount of time to spend generating traces (0 means no limit)
- tracesPerSecond is the number of root spans to generate per second
- traceCount is the maximum number of traces to generate; as soon as TraceCount is reached, the process stops (0 means no limit)
- rampup and rampdown are the number of seconds to spend ramping up and down to the desired TPS

Functionally, the system works by spinning up a number of goroutines, each of
which generates a stream of spans. The number of goroutines needed will equal
tracesPerSecond * avgDuration. Rampup and rampdown are handled only by
increasing or decreasing the number of goroutines.

To mix different kinds of traces, or send traces to multiple datasets, use
multiple loadgen processes.
