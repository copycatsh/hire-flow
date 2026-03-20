package telemetry

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	h := NewTracedHandler(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return slog.New(h)
}

func spanContext(traceID, spanID string) trace.SpanContext {
	tid, _ := trace.TraceIDFromHex(traceID)
	sid, _ := trace.SpanIDFromHex(spanID)
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
}

func TestTracedHandler_WithSpan(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	traceID := "0af7651916cd43dd8448eb211c80319c"
	spanIDVal := "b7ad6b7169203331"
	ctx := trace.ContextWithSpanContext(t.Context(), spanContext(traceID, spanIDVal))

	logger.InfoContext(ctx, "test message")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshaling log output: %v", err)
	}

	if got := m["trace_id"]; got != traceID {
		t.Errorf("trace_id = %v, want %v", got, traceID)
	}
	if got := m["span_id"]; got != spanIDVal {
		t.Errorf("span_id = %v, want %v", got, spanIDVal)
	}
}

func TestTracedHandler_WithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.InfoContext(t.Context(), "no span")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshaling log output: %v", err)
	}

	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id should not be present without span")
	}
	if _, ok := m["span_id"]; ok {
		t.Error("span_id should not be present without span")
	}
}

func TestTracedHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	traceID := "0af7651916cd43dd8448eb211c80319c"
	ctx := trace.ContextWithSpanContext(t.Context(), spanContext(traceID, "b7ad6b7169203331"))

	logger.With("service", "test").InfoContext(ctx, "with attrs")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshaling log output: %v", err)
	}

	if got := m["service"]; got != "test" {
		t.Errorf("service = %v, want test", got)
	}
	if got := m["trace_id"]; got != traceID {
		t.Errorf("trace_id = %v, want %v", got, traceID)
	}
}

func TestTracedHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	traceID := "0af7651916cd43dd8448eb211c80319c"
	ctx := trace.ContextWithSpanContext(t.Context(), spanContext(traceID, "b7ad6b7169203331"))

	logger.WithGroup("req").InfoContext(ctx, "grouped")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshaling log output: %v", err)
	}

	// trace_id should be inside the "req" group since WithGroup wraps the inner handler
	reqGroup, ok := m["req"].(map[string]any)
	if !ok {
		// trace_id added at record level — may appear at top level depending on handler behavior
		if _, exists := m["trace_id"]; !exists {
			t.Error("trace_id should be present")
		}
		return
	}
	if _, exists := reqGroup["trace_id"]; !exists {
		t.Error("trace_id should be present in req group")
	}
}

func TestInit_SmokeTest(t *testing.T) {
	ctx := t.Context()
	shutdown, err := Init(ctx, "test-service", "localhost:4317")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
