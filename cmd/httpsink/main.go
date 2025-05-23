package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/jessevdk/go-flags"
	cuckoo "github.com/panmari/cuckoofilter"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Options defines the command line arguments
type Options struct {
	Port int `long:"port" description:"Port number to listen on for HTTP" default:"4318"`
}

// TraceServer processes incoming trace data
type TraceServer struct {
	traces *cuckoo.Filter
	spans  *cuckoo.Filter
}

func NewTraceServer() *TraceServer {
	return &TraceServer{
		traces: cuckoo.NewFilter(1000000),
		spans:  cuckoo.NewFilter(100000000),
	}
}

// ProcessTraceRequest handles both JSON and Protobuf-encoded trace data
func (t *TraceServer) ProcessTraceRequest(req *collectortrace.ExportTraceServiceRequest) {
	for _, resource := range req.GetResourceSpans() {
		for _, scope := range resource.GetScopeSpans() {
			for _, span := range scope.GetSpans() {
				traceID := span.GetTraceId()
				spanID := span.GetSpanId()
				if !t.traces.Lookup(traceID) {
					t.traces.Insert(traceID)
				}
				if !t.spans.Lookup(spanID) {
					t.spans.Insert(spanID)
				}
			}
		}
	}
}

func initHTTPReceiver(ctx context.Context, opts Options, ts *TraceServer) error {
	mux := http.NewServeMux()

	// Handler for OTLP traces endpoint
	mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Println("received request on /v1/traces")

		// Read request body
		var reader io.ReadCloser = r.Body
		switch r.Header.Get("Content-Encoding") {
		case "gzip":
			var err error
			reader, err = gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to decompress gzip data: "+err.Error(), http.StatusBadRequest)
				return
			}
			defer reader.Close()
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var traceReq collectortrace.ExportTraceServiceRequest

		// Process based on content type
		contentType := r.Header.Get("Content-Type")
		switch contentType {
		case "application/x-protobuf":
			if err := proto.Unmarshal(body, &traceReq); err != nil {
				http.Error(w, "Invalid protobuf data", http.StatusBadRequest)
				return
			}
		case "application/json":
			if err := json.Unmarshal(body, &traceReq); err != nil {
				http.Error(w, "Invalid JSON data", http.StatusBadRequest)
				return
			}
		default:
			// Default to protobuf if content type is not specified
			if err := proto.Unmarshal(body, &traceReq); err != nil {
				http.Error(w, "Invalid data format", http.StatusBadRequest)
				return
			}
		}

		// Process the trace data
		ts.ProcessTraceRequest(&traceReq)

		// Return empty success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	})

	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", opts.Port),
		Handler: mux,
	}

	// Start the server in a goroutine
	go func() {
		log.Printf("HTTP server listening on port %d", opts.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Handle shutdown
	go func() {
		<-ctx.Done()
		log.Println("Stopping HTTP server...")

		// Create a timeout context for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}
	}()

	return nil
}

func main() {
	var opts Options

	// Parse command line arguments
	parser := flags.NewParser(&opts, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		log.Fatalf("Error parsing flags: %v", err)
	}

	log.Printf("Starting HTTP sink server on port %d\n", opts.Port)

	// Create context that listens for interrupt signals
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize trace server
	ts := NewTraceServer()

	// Initialize and start HTTP receiver
	if err := initHTTPReceiver(ctx, opts, ts); err != nil {
		log.Fatalf("Failed to start HTTP receiver: %v", err)
	}

	// Wait for termination signal
	<-ctx.Done()

	fmt.Printf("\n%d traces, %d spans received this session\n", ts.traces.Count(), ts.spans.Count())
	log.Println("Shutting down gracefully...")
}
