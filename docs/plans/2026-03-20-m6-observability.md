# M6 — Observability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add structured logging (JSON slog + structlog), OpenTelemetry tracing across services, Prometheus metrics, and Grafana dashboards to the hire-flow platform.

**Architecture:** Hybrid export — each service exposes `/metrics` for Prometheus to scrape directly (pull model). Traces go through an OTel Collector via OTLP gRPC. A shared `pkg/telemetry/` package handles OTel SDK init + a custom slog handler that enriches logs with trace/span IDs. Framework-specific OTel middleware (otelecho, otelchi, otelgin) handles server spans + metrics. BFF uses `otelhttp.NewTransport()` for trace propagation to downstream services.

**Tech Stack:** OpenTelemetry Go SDK, Prometheus Go client, otelecho/otelchi/otelgin middleware, OTel Collector (Docker), Prometheus (Docker), Grafana (Docker), structlog + opentelemetry-python + prometheus-client (Python).

**Merge criteria:** Can trace a request across 3+ services. Grafana shows basic metrics.

**Eng Review Decisions (2026-03-20):**
1. Shared `pkg/telemetry/` for OTel init (like `pkg/outbox/`)
2. Hybrid export: direct `/metrics` + OTel Collector for traces
3. HTTP-only trace propagation (NATS deferred)
4. Official OTel middleware for Echo/Chi/Gin + custom for stdlib BFFs
5. JSON slog with trace ID handler in `pkg/telemetry/`
6. Migrate all slog calls to context-aware (`slog.InfoContext`/`slog.ErrorContext`)
7. `otelhttp.NewTransport()` for BFF HTTP client trace propagation
8. Instrument contracts outbox publisher independently (no unification)
9. Full tests for TracedHandler + smoke for InitTelemetry

---

## Task 1: Create `pkg/telemetry/` — OTel Init + Traced slog Handler

**Files:**
- Create: `pkg/telemetry/go.mod`
- Create: `pkg/telemetry/telemetry.go`
- Create: `pkg/telemetry/handler.go`
- Create: `pkg/telemetry/handler_test.go`
- Modify: `go.work`

### Step 1: Create go.mod

Create `pkg/telemetry/go.mod`:

```go
module github.com/copycatsh/hire-flow/pkg/telemetry

go 1.25.0

require (
	go.opentelemetry.io/otel v1.41.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.41.0
	go.opentelemetry.io/otel/exporters/prometheus v0.63.0
	go.opentelemetry.io/otel/sdk v1.41.0
	go.opentelemetry.io/otel/sdk/metric v1.41.0
	go.opentelemetry.io/otel/trace v1.41.0
	github.com/prometheus/client_golang v1.22.0
)
```

### Step 2: Create `telemetry.go` — OTel SDK init

Create `pkg/telemetry/telemetry.go`:

```go
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Init sets up OpenTelemetry tracing (OTLP export) and metrics (Prometheus export).
// Returns a shutdown function that must be called on service exit.
//
//	shutdown, err := telemetry.Init(ctx, "jobs-api", "otel-collector:4317")
//	if err != nil { ... }
//	defer shutdown(ctx)
func Init(ctx context.Context, serviceName, otelEndpoint string) (shutdown func(context.Context) error, err error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}

	// Trace exporter (OTLP gRPC → OTel Collector)
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otelEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
	)
	otel.SetTracerProvider(tracerProvider)

	// Propagation (W3C TraceContext)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Metrics exporter (Prometheus pull)
	promExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("telemetry prometheus exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExporter),
	)
	otel.SetMeterProvider(meterProvider)

	shutdown = func(ctx context.Context) error {
		var errs []error
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return fmt.Errorf("telemetry shutdown: %v", errs)
		}
		return nil
	}

	return shutdown, nil
}

// MetricsHandler returns an http.Handler that serves Prometheus metrics at /metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
```

### Step 3: Create `handler.go` — Traced slog handler

Create `pkg/telemetry/handler.go`:

```go
package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TracedHandler wraps an slog.Handler to inject trace_id and span_id
// from the context into every log record.
type TracedHandler struct {
	inner slog.Handler
}

// NewTracedHandler wraps the given handler with trace ID enrichment.
func NewTracedHandler(inner slog.Handler) *TracedHandler {
	return &TracedHandler{inner: inner}
}

func (h *TracedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *TracedHandler) Handle(ctx context.Context, record slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		record.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, record)
}

func (h *TracedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewTracedHandler(h.inner.WithAttrs(attrs))
}

func (h *TracedHandler) WithGroup(name string) slog.Handler {
	return NewTracedHandler(h.inner.WithGroup(name))
}
```

### Step 4: Write tests for TracedHandler

Create `pkg/telemetry/handler_test.go`:

```go
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTracedHandler_WithSpan(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewTracedHandler(inner)
	logger := slog.New(handler)

	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), spanCtx)

	logger.InfoContext(ctx, "test message", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}

	if got := entry["trace_id"]; got != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace_id = %q, want %q", got, "0af7651916cd43dd8448eb211c80319c")
	}
	if got := entry["span_id"]; got != "b7ad6b7169203331" {
		t.Errorf("span_id = %q, want %q", got, "b7ad6b7169203331")
	}
	if got := entry["key"]; got != "value" {
		t.Errorf("key = %q, want %q", got, "value")
	}
}

func TestTracedHandler_WithoutSpan(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewTracedHandler(inner)
	logger := slog.New(handler)

	logger.InfoContext(t.Context(), "no span")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}

	if _, ok := entry["trace_id"]; ok {
		t.Error("trace_id should not be present without span")
	}
	if _, ok := entry["span_id"]; ok {
		t.Error("span_id should not be present without span")
	}
}

func TestTracedHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewTracedHandler(inner)
	logger := slog.New(handler).With("service", "test")

	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), spanCtx)

	logger.InfoContext(ctx, "with attrs")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}

	if got := entry["service"]; got != "test" {
		t.Errorf("service = %q, want %q", got, "test")
	}
	if got := entry["trace_id"]; got != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace_id missing with WithAttrs")
	}
}

func TestTracedHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	handler := NewTracedHandler(inner)
	logger := slog.New(handler).WithGroup("req")

	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(t.Context(), spanCtx)

	logger.InfoContext(ctx, "grouped", "method", "GET")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log: %v", err)
	}

	// trace_id should be at top level even with group
	if _, ok := entry["trace_id"]; !ok {
		t.Error("trace_id should be present at top level")
	}
}

func TestInit_SmokeTest(t *testing.T) {
	ctx := t.Context()
	shutdown, err := Init(ctx, "test-service", "localhost:4317")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := shutdown(ctx); err != nil {
		t.Logf("shutdown warning (expected without collector): %v", err)
	}
}
```

### Step 5: Run tests to verify they pass

```bash
cd pkg/telemetry && go mod tidy && go test -v ./...
```

Expected: all tests pass (Init smoke test may log a warning about missing collector — that's OK).

### Step 6: Update go.work

Add `./pkg/telemetry` to the `use` block in `go.work`:

```go
go 1.25.0

use (
	./pkg/outbox
	./pkg/telemetry
	./services/bff/admin
	./services/bff/client
	./services/bff/freelancer
	./services/contracts
	./services/jobs-api
	./services/payments
)
```

### Step 7: Commit

```bash
git add pkg/telemetry/ go.work
git commit -m "feat(m6): add pkg/telemetry — OTel init + traced slog handler"
```

---

## Task 2: Add Observability Infra to Docker Compose

**Files:**
- Create: `infra/otel-collector/config.yaml`
- Create: `infra/prometheus/prometheus.yml`
- Create: `infra/grafana/provisioning/datasources/datasource.yml`
- Create: `infra/grafana/provisioning/dashboards/dashboard.yml`
- Create: `infra/grafana/dashboards/services.json`
- Modify: `compose.yaml`

### Step 1: Create OTel Collector config

Create `infra/otel-collector/config.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 5s
    send_batch_size: 1024

exporters:
  debug:
    verbosity: detailed

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
```

### Step 2: Create Prometheus scrape config

Create `infra/prometheus/prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: "jobs-api"
    static_configs:
      - targets: ["jobs-api:8001"]

  - job_name: "ai-matching"
    static_configs:
      - targets: ["ai-matching:8002"]

  - job_name: "contracts"
    static_configs:
      - targets: ["contracts:8003"]

  - job_name: "payments"
    static_configs:
      - targets: ["payments:8004"]

  - job_name: "bff-client"
    static_configs:
      - targets: ["bff-client:8010"]

  - job_name: "bff-freelancer"
    static_configs:
      - targets: ["bff-freelancer:8011"]

  - job_name: "bff-admin"
    static_configs:
      - targets: ["bff-admin:8012"]
```

### Step 3: Create Grafana provisioning

Create `infra/grafana/provisioning/datasources/datasource.yml`:

```yaml
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: false
```

Create `infra/grafana/provisioning/dashboards/dashboard.yml`:

```yaml
apiVersion: 1

providers:
  - name: "default"
    orgId: 1
    folder: ""
    type: file
    disableDeletion: false
    updateIntervalSeconds: 30
    options:
      path: /var/lib/grafana/dashboards
      foldersFromFilesStructure: false
```

### Step 4: Create Grafana dashboard JSON

Create `infra/grafana/dashboards/services.json` — a Grafana dashboard with 4 panels:
1. **Request Rate** — `sum(rate(http_server_request_duration_seconds_count[1m])) by (job)`
2. **Request Latency (p95)** — `histogram_quantile(0.95, sum(rate(http_server_request_duration_seconds_bucket[5m])) by (le, job))`
3. **Error Rate** — `sum(rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[1m])) by (job)`
4. **Requests by Status** — `sum(rate(http_server_request_duration_seconds_count[1m])) by (http_response_status_code, job)`

```json
{
  "annotations": { "list": [] },
  "editable": true,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 1,
  "id": null,
  "links": [],
  "panels": [
    {
      "title": "Request Rate (req/s)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_count[1m])) by (job)",
          "legendFormat": "{{job}}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Request Latency p95 (s)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(http_server_request_duration_seconds_bucket[5m])) by (le, job))",
          "legendFormat": "{{job}}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "s",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    },
    {
      "title": "Error Rate (5xx/s)",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 8 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_count{http_response_status_code=~\"5..\"}[1m])) by (job)",
          "legendFormat": "{{job}}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 },
          "color": { "mode": "fixed", "fixedColor": "red" }
        },
        "overrides": []
      }
    },
    {
      "title": "Requests by Status Code",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 8 },
      "datasource": { "type": "prometheus", "uid": "prometheus" },
      "targets": [
        {
          "expr": "sum(rate(http_server_request_duration_seconds_count[1m])) by (http_response_status_code, job)",
          "legendFormat": "{{job}} {{http_response_status_code}}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "reqps",
          "custom": { "drawStyle": "line", "fillOpacity": 10 }
        },
        "overrides": []
      }
    }
  ],
  "schemaVersion": 39,
  "tags": ["hire-flow"],
  "templating": { "list": [] },
  "time": { "from": "now-15m", "to": "now" },
  "title": "hire-flow Services",
  "uid": "hire-flow-services"
}
```

### Step 5: Add containers to compose.yaml

Add these services to `compose.yaml` in the Infrastructure section, and add `prometheus-data` and `grafana-data` volumes:

Add to `volumes:` section:
```yaml
  prometheus-data:
  grafana-data:
```

Add these services after `qdrant`:
```yaml
  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.120.0
    container_name: hire-flow-otel-collector
    command: ["--config", "/etc/otelcol/config.yaml"]
    volumes:
      - ./infra/otel-collector/config.yaml:/etc/otelcol/config.yaml:ro
    ports:
      - "4317:4317"
      - "4318:4318"
    networks:
      - hire-flow
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:13133/ > /dev/null 2>&1"]
      interval: 5s
      timeout: 3s
      retries: 5

  prometheus:
    image: prom/prometheus:v3.4.0
    container_name: hire-flow-prometheus
    volumes:
      - ./infra/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    ports:
      - "9090:9090"
    networks:
      - hire-flow
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:9090/-/healthy > /dev/null 2>&1"]
      interval: 5s
      timeout: 3s
      retries: 5

  grafana:
    image: grafana/grafana:11.6.0
    container_name: hire-flow-grafana
    environment:
      GF_SECURITY_ADMIN_USER: admin
      GF_SECURITY_ADMIN_PASSWORD: admin
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: Viewer
    volumes:
      - grafana-data:/var/lib/grafana
      - ./infra/grafana/provisioning:/etc/grafana/provisioning:ro
      - ./infra/grafana/dashboards:/var/lib/grafana/dashboards:ro
    ports:
      - "3000:3000"
    networks:
      - hire-flow
    depends_on:
      prometheus:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:3000/api/health > /dev/null 2>&1"]
      interval: 5s
      timeout: 3s
      retries: 5
```

Add `OTEL_ENDPOINT: "otel-collector:4317"` env var and `otel-collector` dependency to all 7 service definitions in compose.yaml. For each service, add:
```yaml
    environment:
      OTEL_ENDPOINT: "otel-collector:4317"
    depends_on:
      otel-collector:
        condition: service_healthy
```

(Merge with existing `environment` and `depends_on` blocks — don't duplicate keys.)

### Step 6: Commit

```bash
git add infra/otel-collector/ infra/prometheus/ infra/grafana/ compose.yaml
git commit -m "feat(m6): add OTel Collector + Prometheus + Grafana to compose"
```

---

## Task 3: Instrument jobs-api (Echo + OTel)

**Files:**
- Modify: `services/jobs-api/go.mod` — add `pkg/telemetry`, `otelecho`, `otelhttp` deps
- Modify: `services/jobs-api/cmd/server/main.go` — init telemetry, JSON slog, add middleware + /metrics
- Modify: `services/jobs-api/cmd/server/middleware.go` — use `slog.InfoContext`
- Modify: `services/jobs-api/Dockerfile` — add `pkg/telemetry` COPY

### Step 1: Update go.mod

Add dependencies:

```bash
cd services/jobs-api
go get github.com/copycatsh/hire-flow/pkg/telemetry
go get go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
go mod tidy
```

### Step 2: Update main.go — add telemetry init, JSON slog, /metrics, OTel middleware

At the top of `main()`, after env var parsing and before DB connection, add:

```go
	// Telemetry
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	shutdownTelemetry, err := telemetry.Init(ctx, "jobs-api", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry(context.Background())
```

Add imports: `"github.com/copycatsh/hire-flow/pkg/telemetry"` and `"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"`.

After `e.Use(requestLogger)`, add:

```go
	e.Use(otelecho.Middleware("jobs-api"))
```

After the `/health` route, add:

```go
	e.GET("/metrics", echo.WrapHandler(telemetry.MetricsHandler()))
```

### Step 3: Migrate slog calls to context-aware

In `main.go`, change all `slog.Info(...)` to `slog.InfoContext(ctx, ...)` and `slog.Error(...)` to `slog.ErrorContext(ctx, ...)` where `ctx` is available.

Startup messages that don't have a context can remain as `slog.Info(...)` (they run before any request context exists).

In `middleware.go`, change:

```go
// In requestLogger:
slog.Info("request", ...)
// →
slog.InfoContext(req.Context(), "request", ...)

// In customErrorHandler:
slog.Error("unhandled error", "error", err)
// →
slog.ErrorContext(c.Request().Context(), "unhandled error", "error", err)
```

Also grep through all `.go` files in `services/jobs-api/` for `slog.Error` and `slog.Info` calls in handlers, stores, etc. and migrate them to context-aware variants where the handler/method receives a context parameter.

### Step 4: Update Dockerfile

The Dockerfile already copies `pkg/outbox/`. Add `pkg/telemetry/` the same way:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/outbox/go.mod pkg/outbox/go.sum ./pkg/outbox/
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/jobs-api/go.mod services/jobs-api/go.sum ./services/jobs-api/
RUN cd services/jobs-api && go mod download
COPY pkg/outbox/ ./pkg/outbox/
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/jobs-api/ ./services/jobs-api/
RUN cd services/jobs-api && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8001
CMD ["/server"]
```

### Step 5: Run tests

```bash
cd services/jobs-api && go test -v ./...
```

Expected: all existing tests pass (no behavior change, only log format and middleware added).

### Step 6: Commit

```bash
git add services/jobs-api/ pkg/telemetry/go.mod pkg/telemetry/go.sum
git commit -m "feat(m6): instrument jobs-api with OTel tracing + Prometheus metrics + JSON slog"
```

---

## Task 4: Instrument payments (Gin + OTel)

**Files:**
- Modify: `services/payments/go.mod`
- Modify: `services/payments/cmd/server/main.go`
- Modify: `services/payments/Dockerfile`
- Migrate: all `slog.Info`/`slog.Error` calls in payments service to context-aware

### Step 1: Update go.mod

```bash
cd services/payments
go get github.com/copycatsh/hire-flow/pkg/telemetry
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
go mod tidy
```

### Step 2: Update main.go

Add after env var parsing, before DB:

```go
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	shutdownTelemetry, err := telemetry.Init(ctx, "payments", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry(context.Background())
```

Add OTel middleware after `r.Use(gin.Recovery())`:

```go
	r.Use(otelgin.Middleware("payments"))
```

Add metrics endpoint after `/health`:

```go
	r.GET("/metrics", gin.WrapH(telemetry.MetricsHandler()))
```

Add imports: `telemetry`, `otelgin`.

### Step 3: Migrate slog calls to context-aware

Grep all `.go` files in `services/payments/` for `slog.Info` and `slog.Error`. In handler methods that receive `*gin.Context`, use `c.Request.Context()`:

```go
slog.Error("hold: begin tx", "error", err)
// →
slog.ErrorContext(c.Request.Context(), "hold: begin tx", "error", err)
```

In `outbox_publisher.go` (via `pkg/outbox/publisher.go`), the `ctx` is already available in `Run()` and `PublishBatch()`.

### Step 4: Update Dockerfile

Add `pkg/telemetry/` copying (same pattern as `pkg/outbox/`):

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/outbox/go.mod pkg/outbox/go.sum ./pkg/outbox/
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/payments/go.mod services/payments/go.sum ./services/payments/
RUN cd services/payments && go mod download
COPY pkg/outbox/ ./pkg/outbox/
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/payments/ ./services/payments/
RUN cd services/payments && CGO_ENABLED=0 go build -o /server ./cmd/server
RUN cd services/payments && CGO_ENABLED=0 go build -o /seed ./cmd/seed

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
COPY --from=build /seed /seed
EXPOSE 8004
CMD ["/server"]
```

### Step 5: Run tests

```bash
cd services/payments && go test -v ./...
```

### Step 6: Commit

```bash
git add services/payments/
git commit -m "feat(m6): instrument payments with OTel tracing + Prometheus metrics + JSON slog"
```

---

## Task 5: Instrument contracts (Chi + OTel)

**Files:**
- Modify: `services/contracts/go.mod`
- Modify: `services/contracts/cmd/server/main.go`
- Modify: `services/contracts/cmd/server/outbox_publisher.go` — migrate slog calls
- Modify: `services/contracts/Dockerfile`

### Step 1: Update go.mod

```bash
cd services/contracts
go get github.com/copycatsh/hire-flow/pkg/telemetry
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
go mod tidy
```

Note: Chi uses `otelhttp` middleware (Chi is stdlib-compatible, so `otelhttp.NewHandler` works). There is no dedicated `otelchi` in the official OTel contrib — use `otelhttp.NewHandler(r, "contracts")`.

### Step 2: Update main.go

Add after env var parsing, before MySQL:

```go
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	shutdownTelemetry, err := telemetry.Init(ctx, "contracts", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry(context.Background())
```

Add `/metrics` route after `/health`:

```go
	r.Handle("/metrics", telemetry.MetricsHandler())
```

Wrap the Chi router with OTel handler when creating the server:

```go
	srv := &http.Server{Addr: port, Handler: otelhttp.NewHandler(r, "contracts")}
```

Add imports: `telemetry`, `otelhttp`.

### Step 3: Migrate slog calls

In `main.go`, `outbox_publisher.go`, `saga.go`, handler files — change `slog.Info`/`slog.Error` to `slog.InfoContext`/`slog.ErrorContext` where `ctx` is available.

### Step 4: Update Dockerfile

Add `pkg/telemetry/`:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/contracts/go.mod services/contracts/go.sum ./services/contracts/
RUN cd services/contracts && go mod download
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/contracts/ ./services/contracts/
RUN cd services/contracts && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8003
CMD ["/server"]
```

### Step 5: Run tests

```bash
cd services/contracts && go test -v ./...
```

### Step 6: Commit

```bash
git add services/contracts/
git commit -m "feat(m6): instrument contracts with OTel tracing + Prometheus metrics + JSON slog"
```

---

## Task 6: Instrument bff-client (stdlib + otelhttp + trace propagation)

**Files:**
- Modify: `services/bff/client/go.mod`
- Modify: `services/bff/client/cmd/server/main.go`
- Modify: `services/bff/client/cmd/server/middleware.go` — migrate slog calls
- Modify: `services/bff/client/Dockerfile`

### Step 1: Update go.mod

```bash
cd services/bff/client
go get github.com/copycatsh/hire-flow/pkg/telemetry
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
go mod tidy
```

### Step 2: Update main.go

Add after env var parsing, before auth config:

```go
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	shutdownTelemetry, err := telemetry.Init(ctx, "bff-client", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry(context.Background())
```

**Critical:** Wrap the `http.Client` transport with `otelhttp.NewTransport()` for trace propagation to downstream services:

```go
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
```

Add `/metrics` endpoint alongside `/health`:

```go
	mux.Handle("GET /metrics", telemetry.MetricsHandler())
```

Wrap the top-level handler with OTel:

```go
	handler := otelhttp.NewHandler(RequestLogger(mux), "bff-client")
```

(Move `RequestLogger` inside `otelhttp.NewHandler` so spans are created before request logging.)

Add imports: `telemetry`, `otelhttp`.

### Step 3: Migrate slog calls

In `middleware.go`:

```go
// RequestLogger:
slog.Info("request", ...)
// →
slog.InfoContext(r.Context(), "request", ...)
```

In `client.go`:

```go
slog.Error("failed to create upstream request", ...)
// →
slog.ErrorContext(ctx, "failed to create upstream request", ...)
```

### Step 4: Update Dockerfile

The BFF Dockerfile uses a simple local build (no monorepo context). Since bff-client now depends on `pkg/telemetry/`, the Dockerfile needs to be updated to copy from monorepo root, or use the same pattern as other services:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/bff/client/go.mod services/bff/client/go.sum ./services/bff/client/
RUN cd services/bff/client && go mod download
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/bff/client/ ./services/bff/client/
RUN cd services/bff/client && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8010
CMD ["/server"]
```

**Important:** Update the `build` section in `compose.yaml` for `bff-client` to use monorepo root as context:

```yaml
  bff-client:
    build:
      context: .
      dockerfile: services/bff/client/Dockerfile
```

### Step 5: Run tests

```bash
cd services/bff/client && go test -v ./...
```

### Step 6: Commit

```bash
git add services/bff/client/ compose.yaml
git commit -m "feat(m6): instrument bff-client with OTel tracing + trace propagation + Prometheus metrics"
```

---

## Task 7: Instrument bff-freelancer and bff-admin (stub services)

**Files:**
- Modify: `services/bff/freelancer/go.mod`
- Modify: `services/bff/freelancer/cmd/server/main.go`
- Modify: `services/bff/freelancer/Dockerfile`
- Modify: `services/bff/admin/go.mod`
- Modify: `services/bff/admin/cmd/server/main.go`
- Modify: `services/bff/admin/Dockerfile`
- Modify: `compose.yaml` — update build contexts

Both are stub services with only a `/health` endpoint. They need minimal instrumentation: JSON slog, OTel init, `/metrics` endpoint, `otelhttp.NewHandler` wrapper.

### Step 1: Update both go.mod files

```bash
cd services/bff/freelancer
go get github.com/copycatsh/hire-flow/pkg/telemetry
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
go mod tidy

cd ../admin
go get github.com/copycatsh/hire-flow/pkg/telemetry
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
go mod tidy
```

### Step 2: Update both main.go files

For `bff-freelancer/cmd/server/main.go`:

```go
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/copycatsh/hire-flow/pkg/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	port := cmp.Or(os.Getenv("PORT"), ":8011")
	if port[0] != ':' {
		port = ":" + port
	}
	otelEndpoint := cmp.Or(os.Getenv("OTEL_ENDPOINT"), "localhost:4317")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.SetDefault(slog.New(
		telemetry.NewTracedHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	shutdownTelemetry, err := telemetry.Init(ctx, "bff-freelancer", otelEndpoint)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry init", "error", err)
		os.Exit(1)
	}
	defer shutdownTelemetry(context.Background())

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.Handle("GET /metrics", telemetry.MetricsHandler())

	srv := &http.Server{
		Addr:    port,
		Handler: otelhttp.NewHandler(mux, "bff-freelancer"),
	}

	go func() {
		slog.InfoContext(ctx, "starting bff-freelancer", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.InfoContext(ctx, "shutting down bff-freelancer")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(shutdownCtx, "shutdown error", "error", err)
	}
}
```

For `bff-admin/cmd/server/main.go` — identical pattern, replace `bff-freelancer` → `bff-admin` and port `8011` → `8012`.

### Step 3: Update Dockerfiles

Both Dockerfiles need monorepo context for `pkg/telemetry/`:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY pkg/telemetry/go.mod pkg/telemetry/go.sum ./pkg/telemetry/
COPY services/bff/freelancer/go.mod services/bff/freelancer/go.sum ./services/bff/freelancer/
RUN cd services/bff/freelancer && go mod download
COPY pkg/telemetry/ ./pkg/telemetry/
COPY services/bff/freelancer/ ./services/bff/freelancer/
RUN cd services/bff/freelancer && CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=build /server /server
EXPOSE 8011
CMD ["/server"]
```

(Same for admin, with `freelancer` → `admin` and port `8011` → `8012`.)

### Step 4: Update compose.yaml build contexts

Change both BFF services to use monorepo root:

```yaml
  bff-freelancer:
    build:
      context: .
      dockerfile: services/bff/freelancer/Dockerfile
    # ... rest stays the same, add OTEL_ENDPOINT env

  bff-admin:
    build:
      context: .
      dockerfile: services/bff/admin/Dockerfile
    # ... rest stays the same, add OTEL_ENDPOINT env
```

### Step 5: Run tests

```bash
go test ./services/bff/freelancer/... ./services/bff/admin/...
```

### Step 6: Commit

```bash
git add services/bff/freelancer/ services/bff/admin/ compose.yaml
git commit -m "feat(m6): instrument bff-freelancer + bff-admin with OTel + Prometheus + JSON slog"
```

---

## Task 8: Instrument ai-matching (Python — structlog + OTel + Prometheus)

**Files:**
- Modify: `services/ai-matching/pyproject.toml` — add structlog, OTel, prometheus deps
- Modify: `services/ai-matching/src/main.py` — structlog config, OTel init, /metrics
- Modify: `services/ai-matching/src/config.py` — add otel_endpoint setting
- Create: `services/ai-matching/src/telemetry.py` — OTel + structlog setup
- Modify: `services/ai-matching/src/api.py` — migrate logging
- Modify: `services/ai-matching/src/consumer.py` — migrate logging

### Step 1: Add dependencies to pyproject.toml

Add to `dependencies`:

```toml
dependencies = [
    "fastapi>=0.115.0",
    "uvicorn[standard]>=0.34.0",
    "sentence-transformers>=3.0.0",
    "qdrant-client>=1.12.0",
    "nats-py>=2.9.0",
    "pydantic-settings>=2.0.0",
    "numpy>=1.26.0",
    "structlog>=24.0.0",
    "opentelemetry-api>=1.30.0",
    "opentelemetry-sdk>=1.30.0",
    "opentelemetry-exporter-otlp-proto-grpc>=1.30.0",
    "opentelemetry-instrumentation-fastapi>=0.51b0",
    "prometheus-client>=0.22.0",
]
```

### Step 2: Add otel_endpoint to config.py

```python
class Settings(BaseSettings):
    nats_url: str = "nats://localhost:4222"
    qdrant_host: str = "localhost"
    qdrant_port: int = 6333
    embedding_model: str = "all-MiniLM-L6-v2"
    embedding_dim: int = 384
    consumer_batch_size: int = 10
    consumer_max_deliver: int = 5
    batch_concurrency: int = 3
    otel_endpoint: str = "localhost:4317"
```

### Step 3: Create src/telemetry.py

```python
import logging
import structlog
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource, SERVICE_NAME
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor


def init_telemetry(service_name: str, otel_endpoint: str) -> None:
    """Initialize OTel tracing + structlog with JSON output."""
    # OTel tracer
    resource = Resource.create({SERVICE_NAME: service_name})
    exporter = OTLPSpanExporter(endpoint=otel_endpoint, insecure=True)
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)

    # structlog with JSON
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.stdlib.add_log_level,
            _add_trace_context,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )

    # Route stdlib logging through structlog
    logging.basicConfig(
        format="%(message)s",
        level=logging.INFO,
        handlers=[structlog.stdlib.ProcessorFormatter.wrap_for_formatter(
            logging.StreamHandler(),
            formatter=structlog.stdlib.ProcessorFormatter(
                processor=structlog.processors.JSONRenderer(),
            ),
        )],
        force=True,
    )


def shutdown_telemetry() -> None:
    provider = trace.get_tracer_provider()
    if hasattr(provider, "shutdown"):
        provider.shutdown()


def _add_trace_context(
    logger: structlog.types.WrappedLogger,
    method_name: str,
    event_dict: structlog.types.EventDict,
) -> structlog.types.EventDict:
    span = trace.get_current_span()
    ctx = span.get_span_context()
    if ctx.is_valid:
        event_dict["trace_id"] = format(ctx.trace_id, "032x")
        event_dict["span_id"] = format(ctx.span_id, "016x")
    return event_dict
```

### Step 4: Update main.py

Replace `logging.basicConfig(...)` and `logger = logging.getLogger(...)` with structlog + OTel:

```python
import structlog
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from prometheus_client import make_asgi_app

from src.telemetry import init_telemetry, shutdown_telemetry

logger = structlog.get_logger()


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings: Settings = app.state.settings

    # Initialize telemetry
    init_telemetry("ai-matching", settings.otel_endpoint)

    # ... rest of existing lifespan code ...

    logger.info("AI Matching service started")
    yield

    # Shutdown
    logger.info("Shutting down...")
    # ... existing shutdown code ...
    shutdown_telemetry()
    logger.info("AI Matching service stopped")


def create_app(settings: Settings | None = None) -> FastAPI:
    if settings is None:
        settings = Settings()

    app = FastAPI(title="ai-matching", lifespan=lifespan)
    app.state.settings = settings

    # OTel instrumentation
    FastAPIInstrumentor.instrument_app(app)

    router = create_router()
    app.include_router(router, prefix="/api/v1")

    @app.get("/health")
    async def health():
        return {"status": "ok"}

    # Prometheus metrics
    metrics_app = make_asgi_app()
    app.mount("/metrics", metrics_app)

    return app
```

### Step 5: Migrate logging in api.py and consumer.py

In `api.py`:
```python
import structlog
logger = structlog.get_logger()
```

In `consumer.py`:
```python
import structlog
logger = structlog.get_logger()
```

Replace all `logging.getLogger(__name__)` with `structlog.get_logger()`. The `.info()`, `.error()`, `.warning()` API is the same.

### Step 6: Run tests

```bash
cd services/ai-matching && pip install -e ".[dev]" && python -m pytest tests/ -v --ignore=tests/test_integration.py
```

### Step 7: Commit

```bash
git add services/ai-matching/
git commit -m "feat(m6): instrument ai-matching with OTel tracing + structlog + Prometheus metrics"
```

---

## Task 9: Migrate slog calls in pkg/outbox/

**Files:**
- Modify: `pkg/outbox/publisher.go`

### Step 1: Change slog calls to context-aware

In `publisher.go`:

```go
// Line 53: in PublishBatch error log
slog.Error("outbox publish batch", "error", err)
// →
slog.ErrorContext(ctx, "outbox publish batch", "error", err)

// Line 77: in publish loop error
slog.Error("outbox: publish to nats", "error", err, "entry_id", e.ID)
// →
slog.ErrorContext(ctx, "outbox: publish to nats", "error", err, "entry_id", e.ID)
```

### Step 2: Run tests

```bash
cd pkg/outbox && go test -v ./...
```

### Step 3: Commit

```bash
git add pkg/outbox/publisher.go
git commit -m "feat(m6): migrate pkg/outbox slog calls to context-aware"
```

---

## Task 10: Update Makefile + TODOS.md + Verify Everything Works

**Files:**
- Modify: `Makefile`
- Modify: `TODOS.md`

### Step 1: Update Makefile

Add a `health` check that also verifies observability infra, and a `trace-demo` target:

After the existing `health-traefik` target, add:

```makefile
# Check observability stack
health-obs:
	@echo "Checking observability stack..."
	@failed=0; \
	for svc in \
		"prometheus:9090/-/healthy" \
		"grafana:3000/api/health" \
		"otel-collector:4317"; \
	do \
		name=$${svc%%:*}; \
		url="http://localhost:$${svc#*:}"; \
		if [ "$$name" = "otel-collector" ]; then \
			if curl -sf "http://localhost:4318/v1/traces" -X POST -d '{}' > /dev/null 2>&1; then \
				printf "  %-20s ✓ healthy\n" "$$name"; \
			else \
				printf "  %-20s ✗ unhealthy\n" "$$name"; \
				failed=1; \
			fi; \
		else \
			if curl -sf "$$url" > /dev/null 2>&1; then \
				printf "  %-20s ✓ healthy\n" "$$name"; \
			else \
				printf "  %-20s ✗ unhealthy\n" "$$name"; \
				failed=1; \
			fi; \
		fi; \
	done; \
	echo ""; \
	if [ "$$failed" = "1" ]; then \
		echo "FAIL: some observability services are unhealthy"; \
		exit 1; \
	else \
		echo "OK: observability stack healthy"; \
	fi
```

Update the `.PHONY` line to include `health-obs`.

### Step 2: Update TODOS.md

Add these two TODOs (from eng review decisions) to the Pending section:

```markdown
### Add NATS message trace propagation
- **What:** Inject W3C trace context into NATS message headers in outbox publisher, extract in consumers
- **Why:** M6 implements HTTP-only trace propagation. Adding NATS tracing enables end-to-end async flow tracing (e.g., job.created → AI matching consumes → embedding stored)
- **Context:** ~30 lines in pkg/outbox/ publisher + Python consumer. OTel SDK already initialized in both. Need to add trace context injection/extraction to NATS message headers.
- **Depends on:** M6 complete

### Add database query tracing
- **What:** Add OTel tracing to database queries via pgx.QueryTracer (PostgreSQL) and otelsql (MySQL)
- **Why:** Shows DB query duration inside traces — useful for debugging slow requests. Currently traces stop at HTTP handler level.
- **Context:** pgx supports OTel tracing via QueryTracer interface. go-sql-driver/mysql has otelsql wrapper. Requires modifying connection setup in each service's main.go (~10 lines per service).
- **Depends on:** M6 complete
```

### Step 3: Full integration test

```bash
make up
make health
make health-obs
```

Then verify trace propagation by calling the BFF which fans out to multiple services:

```bash
# Create a test job via BFF → jobs-api (2 services)
curl -s http://localhost:8010/api/v1/jobs -X POST \
  -H "Content-Type: application/json" \
  -H "Cookie: access_token=<valid-jwt>" \
  -d '{"title":"Test","description":"Test job for tracing"}'

# Check OTel Collector logs for traces spanning services:
docker compose logs otel-collector --tail=50

# Check Prometheus scrapes work:
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Check Grafana dashboard loads:
curl -s http://localhost:3000/api/dashboards/uid/hire-flow-services | jq '.dashboard.title'
```

Expected:
- `make health` — all 7 services green
- `make health-obs` — prometheus, grafana, otel-collector green
- OTel Collector logs show traces with spans from multiple services
- Prometheus shows all 7 targets as "up"
- Grafana dashboard returns "hire-flow Services"

### Step 4: Commit

```bash
git add Makefile TODOS.md
git commit -m "feat(m6): update Makefile with health-obs target + add observability TODOs"
```

---

## Summary of All Files Changed

| Category | Files |
|---|---|
| **New package** | `pkg/telemetry/go.mod`, `telemetry.go`, `handler.go`, `handler_test.go` |
| **New infra** | `infra/otel-collector/config.yaml`, `infra/prometheus/prometheus.yml`, `infra/grafana/provisioning/datasources/datasource.yml`, `infra/grafana/provisioning/dashboards/dashboard.yml`, `infra/grafana/dashboards/services.json` |
| **New Python** | `services/ai-matching/src/telemetry.py` |
| **Modified Go** | 7× `main.go`, 7× `go.mod`, 7× `Dockerfile`, `middleware.go` (jobs-api, bff-client), `outbox_publisher.go` (contracts), `publisher.go` (pkg/outbox) |
| **Modified Python** | `main.py`, `config.py`, `api.py`, `consumer.py`, `pyproject.toml` |
| **Modified config** | `compose.yaml`, `go.work`, `Makefile`, `TODOS.md` |