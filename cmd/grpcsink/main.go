package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	cuckoo "github.com/panmari/cuckoofilter"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	_ "google.golang.org/grpc/encoding/gzip"
)

// Options defines the command line arguments
type Options struct {
	Port int `long:"port" description:"Port number to listen on for grpc" default:"4317"`
}

const (
	// Default values for gRPC configuration
	DefaultMaxSendMsgSize        = 4 * 1024 * 1024  // 4 MB
	DefaultMaxRecvMsgSize        = 15 * 1024 * 1024 // 15 MB
	DefaultMaxConnectionIdle     = 30 * 60 * 1000   // 30 minutes
	DefaultMaxConnectionAge      = 60 * 60 * 1000   // 1 hour
	DefaultMaxConnectionAgeGrace = 5 * 60 * 1000    // 5 minutes
	DefaultKeepAlive             = 2 * 60 * 1000    // 2 minutes
	DefaultKeepAliveTimeout      = 20 * 1000        // 20 seconds
)

type TraceServer struct {
	traces     *cuckoo.Filter
	spans      *cuckoo.Filter
	traceCount int
	spanCount  int
	collectortrace.UnimplementedTraceServiceServer
}

func NewTraceServer() *TraceServer {
	traceServer := TraceServer{
		traces:     cuckoo.NewFilter(1000000),
		spans:      cuckoo.NewFilter(1000000),
		traceCount: 0,
		spanCount:  0,
	}
	return &traceServer
}

func (t *TraceServer) Export(ctx context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	// ctx, span := otelutil.StartSpan(ctx, t.router.Tracer, "ExportOTLPTrace")
	// defer span.End()

	// ri := huskyotlp.GetRequestInfoFromGrpcMetadata(ctx)
	// apicfg := t.router.Config.GetAccessKeyConfig()
	// if err := apicfg.IsAccepted(ri.ApiKey); err != nil {
	// 	return nil, status.Error(codes.Unauthenticated, err.Error())
	// }

	// keyToUse, _ := apicfg.GetReplaceKey(ri.ApiKey)

	// if err := ri.ValidateTracesHeaders(); err != nil && err != huskyotlp.ErrMissingAPIKeyHeader {
	// 	return nil, huskyotlp.AsGRPCError(err)
	// }

	// ri.ApiKey = keyToUse

	// result, err := huskyotlp.TranslateTraceRequest(ctx, req, ri)
	// if err != nil {
	// 	return nil, huskyotlp.AsGRPCError(err)
	// }

	// if err := t.router.processOTLPRequest(ctx, result.Batches, keyToUse, ri.UserAgent); err != nil {
	// 	return nil, huskyotlp.AsGRPCError(err)
	// }
	// log.Printf("%+v", req)
	for _, resource := range req.GetResourceSpans() {
		for _, scope := range resource.GetScopeSpans() {
			for _, span := range scope.GetSpans() {
				traceID := span.GetTraceId()
				spanID := span.GetSpanId()
				if !t.traces.Lookup(traceID) {
					t.traces.Insert(traceID)
					t.traceCount++
				}
				if !t.spans.Lookup(spanID) {
					t.spans.Insert(spanID)
					t.spanCount++
				}
			}
		}
	}

	return &collectortrace.ExportTraceServiceResponse{}, nil
}

// initGRPCReceiver initializes and starts a trace server on localhost with the specified options
func initGRPCReceiver(ctx context.Context, opts Options) (*TraceServer, error) {
	addr := fmt.Sprintf("localhost:%d", opts.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	serverOpts := []grpc.ServerOption{
		grpc.MaxSendMsgSize(int(DefaultMaxSendMsgSize)),
		grpc.MaxRecvMsgSize(int(DefaultMaxRecvMsgSize)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     time.Duration(DefaultMaxConnectionIdle),
			MaxConnectionAge:      time.Duration(DefaultMaxConnectionAge),
			MaxConnectionAgeGrace: time.Duration(DefaultMaxConnectionAgeGrace),
			Time:                  time.Duration(DefaultKeepAlive),
			Timeout:               time.Duration(DefaultKeepAliveTimeout),
		}),
	}

	// Create a new gRPC server with default options
	srv := grpc.NewServer(serverOpts...)

	traceServer := NewTraceServer()
	collectortrace.RegisterTraceServiceServer(srv, traceServer)

	// Start the server in a separate goroutine
	go func() {
		log.Printf("gRPC server listening on %s", addr)
		if err := srv.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Set up graceful shutdown when context is cancelled
	go func() {
		<-ctx.Done()
		log.Println("Stopping gRPC server...")
		srv.GracefulStop()
	}()

	return traceServer, nil
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

	log.Printf("Starting sink server on port %d\n", opts.Port)

	// Create context that listens for interrupt signals
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize and start our trace server
	var ts *TraceServer
	ts, err = initGRPCReceiver(ctx, opts)
	if err != nil {
		log.Fatalf("Failed to start gRPC receiver: %v", err)
	}

	// Wait for termination signal
	<-ctx.Done()

	fmt.Printf("\n%d traces, %d spans received this session\n", ts.traceCount, ts.spanCount)

	log.Println("Shutting down gracefully...")
}
