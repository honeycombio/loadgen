package main

// loadgen generates telemetry trace loads for performance testing. It can send
// traces to honeycomb or to a local agent, and it can generate OTLP or
// Honeycomb-formatted traces. It's highly configurable:
//
// - depth is the average depth (nesting level) of a trace.
// - spancount is the average number of spans in a trace.
// If spancount is less than depth, the trace will be truncated at spancount.
// If spancount is greater than depth, some of the spans will have siblings.
//
// - spanwidth is the average number of fields in a span; this will vary by
// service but will be the same for all calls to a given service, and the names
// and types of all fields for an service will be consistent even across runs of
// loadgen (randomness is seeded by service name).
//
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
// - Duration is the average duration of a trace's root span in milliseconds; individual
// spans will be randomly assigned durations that will fit within the root span's duration.
//
// - maxTime is the total amount of time to spend generating traces (0 means no limit)
// - tracesPerSecond is the number of root spans to generate per second
// - traceCount is the maximum number of traces to generate; as soon as TraceCount is reached, the process stops (0 means no limit)
// - rampup and rampdown are the number of seconds to spend ramping up and down to the desired TPS

// Functionally, the system works by spinning up a number of goroutines, each of which
// generates a stream of spans. The number of goroutines needed will equal tracesPerSecond * avgDuration.
// Rampup and rampdown are handled only by increasing or decreasing the number of goroutines.

// To mix different kinds of traces, or send traces to multiple datasets, use multiple loadgen processes.
