# TODOS

## Pending

### Add optimistic locking to jobs and profiles
- **What:** Add `version` column to jobs/profiles, return 409 on conflict
- **Why:** Concurrent updates currently use last-write-wins with no conflict detection
- **Context:** Not needed for M1 (single-user testing) but will matter when BFFs aggregate requests from multiple clients (M5)
- **Depends on:** M1 complete

### Add match.found event publishing to ai-matching
- **What:** When match score exceeds threshold, publish to NATS MATCHES stream
- **Why:** Enables real-time notifications when matches are found (consumed by BFFs/Contracts)
- **Context:** Deferred from M2 scope — no consumer exists yet. Consumer + Qdrant store already in place; adding publishing is ~30 lines
- **Depends on:** M2 complete, triggered by M4/M5

### Add OpenAPI spec generation for jobs-api
- **What:** Auto-generate API docs (swaggo or oapi-codegen)
- **Why:** BFF developers (M5) will need API documentation
- **Context:** Deferred from M1 scope (not in merge criteria). Becomes valuable pre-M5.
- **Depends on:** M1 complete

### Add hold expiry background job to payments
- **What:** Background goroutine that expires stale holds (`SET status=expired WHERE expires_at < now() AND status='active'`) and publishes `payment.failed` events
- **Why:** Without this, holds created by a failed/crashed saga could lock funds indefinitely — the expiry job is a safety net for the saga orchestrator's timeout handling
- **Context:** The `expires_at` column exists in the holds table from M3, but no enforcement job runs. The saga orchestrator (M4) is the primary timeout handler; this job catches cases where the orchestrator itself fails.
- **Depends on:** M3 complete, triggered by M4

### Add milestone-level completion to contracts
- **What:** Extend `PUT /contracts/{id}/milestones/{mid}/complete` for individual milestone completion with proportional payment transfer
- **Why:** M4 ships contract-level completion (all-or-nothing). Real contracts need incremental milestone payments.
- **Context:** Milestones table exists with `amount` and `status` columns. Schema is forward-compatible. Saga orchestrator needs per-milestone transfer logic.
- **Depends on:** M4 complete

### Add background retry for stuck saga states
- **What:** Background goroutine retries contracts in COMPLETING/DECLINING for > 5 minutes by re-calling Payments HTTP
- **Why:** M4 uses idempotent manual retry. Without background retry, contracts stay stuck if no one retries. Safety net for saga failures.
- **Context:** Similar to hold expiry TODO — both are saga recovery safety nets. Could be combined into single "saga recovery" job.
- **Depends on:** M4 complete

### Add 48h acceptance timeout for contracts
- **What:** Background job auto-cancels contracts in AWAITING_ACCEPT for > 48 hours, triggering compensation (release hold)
- **Why:** Architecture spec says "WaitForAcceptance (timeout: 48h)". Without this, ignored contracts hold funds indefinitely.
- **Context:** Payments hold `expires_at` column is a safety net. This timeout is business-logic enforcement.
- **Depends on:** M4 complete

### Add rate limiter bucket cleanup to BFF
- **What:** Background goroutine evicts stale rate limiter buckets (users not seen in 1h)
- **Why:** Without cleanup, in-memory map grows unbounded with unique user_ids
- **Context:** MVP uses hardcoded test users (bounded). Production needs this or a distributed rate limiter (Redis).
- **Depends on:** M5 complete

### Add Playwright e2e tests for client and admin frontends
- **What:** End-to-end browser tests for client (login → create job → view matches → propose contract) and admin (login → dashboard → jobs → contracts → wallets) flows
- **Why:** Vitest + RTL tests mock the API. E2e tests verify the real stack works together (CORS, cookies, Traefik routing, role enforcement)
- **Context:** M8/M10 ship with Vitest + RTL + msw. E2e tests require Docker Compose running. Can be added post-M10.
- **Depends on:** M10 complete

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

## Completed

### Extract outbox pattern to pkg/outbox/
- **What:** Shared outbox publisher + store for jobs-api, payments, contracts
- **Resolution:** Extracted in M3. `pkg/outbox` contains Entry, Store interface, PostgresStore, Publisher with EventPublisher interface. jobs-api updated to import from `pkg/outbox`.

### Add CORS middleware to BFFs
- **What:** Add CORS headers (Access-Control-*) to BFF responses for browser requests
- **Resolution:** CORS handled in Traefik middleware (dynamic.yml), not BFF layer. Added in M8.

### Fix architecture doc contradictions
- **What:** Update docs/architecture/overview.md to fix stale/contradictory info
- **Resolution:** Fully rewritten — translated to English, fixed `docker-compose.yml` → `compose.yaml`, Go version updated, removed hexagonal references from CLAUDE.md template, removed `pkg/` from repo structure, updated Traefik to file provider, marked M0 as complete.
