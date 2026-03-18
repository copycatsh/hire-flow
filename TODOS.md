# TODOS

## Pending

### Add optimistic locking to jobs and profiles
- **What:** Add `version` column to jobs/profiles, return 409 on conflict
- **Why:** Concurrent updates currently use last-write-wins with no conflict detection
- **Context:** Not needed for M1 (single-user testing) but will matter when BFFs aggregate requests from multiple clients (M5)
- **Depends on:** M1 complete

### Extract outbox pattern to pkg/outbox/
- **What:** Shared outbox publisher + store for jobs-api, payments, contracts
- **Why:** Payments (M3) and contracts (M4) will need the same outbox pattern
- **Context:** Currently local to jobs-api. Extract when second consumer exists in M3.
- **Depends on:** M1 complete, triggered by M3 start

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

## Completed

### Fix architecture doc contradictions
- **What:** Update docs/architecture/overview.md to fix stale/contradictory info
- **Resolution:** Fully rewritten — translated to English, fixed `docker-compose.yml` → `compose.yaml`, Go version updated, removed hexagonal references from CLAUDE.md template, removed `pkg/` from repo structure, updated Traefik to file provider, marked M0 as complete.
