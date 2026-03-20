package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type TracedHandler struct {
	inner slog.Handler
}

func NewTracedHandler(inner slog.Handler) *TracedHandler {
	return &TracedHandler{inner: inner}
}

func (h *TracedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *TracedHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h *TracedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TracedHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *TracedHandler) WithGroup(name string) slog.Handler {
	return &TracedHandler{inner: h.inner.WithGroup(name)}
}