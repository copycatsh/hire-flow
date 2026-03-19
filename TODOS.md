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

## Completed

### Extract outbox pattern to pkg/outbox/
- **What:** Shared outbox publisher + store for jobs-api, payments, contracts
- **Resolution:** Extracted in M3. `pkg/outbox` contains Entry, Store interface, PostgresStore, Publisher with EventPublisher interface. jobs-api updated to import from `pkg/outbox`.

### Fix architecture doc contradictions
- **What:** Update docs/architecture/overview.md to fix stale/contradictory info
- **Resolution:** Fully rewritten — translated to English, fixed `docker-compose.yml` → `compose.yaml`, Go version updated, removed hexagonal references from CLAUDE.md template, removed `pkg/` from repo structure, updated Traefik to file provider, marked M0 as complete.
