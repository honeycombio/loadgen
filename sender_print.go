package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// make sure it implements Sender
var _ Sender = (*SenderPrint)(nil)

func ft(ts time.Time) string {
	return ts.Format("15:04:05.000")
}

// randID creates a random byte array of length l and returns it as a hex string.
func randID(l int) string {
	id := make([]byte, l)
	for i := 0; i < l; i++ {
		id[i] = byte(rand.Intn(256))
	}
	return fmt.Sprintf("%x", id)
}

type traceInfo struct {
	TraceId  string
	SpanId   string
	ParentId string
}

func (t *traceInfo) span(parent string) *traceInfo {
	return &traceInfo{
		TraceId:  t.TraceId,
		SpanId:   randID(4),
		ParentId: parent,
	}
}

type PrintSendable struct {
	TInfo     *traceInfo
	Name      string
	StartTime time.Time
	Fields    map[string]interface{}
	log       Logger
}

func (s *PrintSendable) Send() {
	endTime := time.Now()
	s.log.Printf("%s - T:%6.6s S:%4.4s P%4.4s start:%v end:%v %v\n", s.Name, s.TInfo.TraceId, s.TInfo.SpanId, s.TInfo.ParentId, ft(s.StartTime), ft(endTime), s.Fields)
}

type SenderPrint struct {
	tracecount int
	nspans     int
	log        Logger
}

func NewSenderPrint(log Logger, opts *Options) Sender {
	return &SenderPrint{
		log: log,
	}
}

func (t *SenderPrint) Close() {
	t.log.Warn("sender sent %d traces with %d spans\n", t.tracecount, t.nspans)
}

type PrintKey string

func (t *SenderPrint) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	t.tracecount++
	t.nspans++
	tinfo := &traceInfo{
		TraceId:  randID(6),
		SpanId:   randID(4),
		ParentId: "",
	}
	ctx = context.WithValue(ctx, PrintKey("trace"), tinfo)
	return ctx, &PrintSendable{
		Name:      name,
		TInfo:     tinfo,
		StartTime: time.Now(),
		Fields:    fielder.GetFields(count, 0),
		log:       t.log,
	}
}

func (t *SenderPrint) CreateSpan(ctx context.Context, name string, level int, fielder *Fielder) (context.Context, Sendable) {
	t.nspans++
	tinfo := ctx.Value(PrintKey("trace")).(*traceInfo)
	ctx = context.WithValue(ctx, PrintKey("trace"), tinfo.span(tinfo.SpanId))
	return ctx, &PrintSendable{
		Name:      name,
		TInfo:     tinfo.span(tinfo.SpanId),
		StartTime: time.Now(),
		Fields:    fielder.GetFields(0, level),
		log:       t.log,
	}
}
