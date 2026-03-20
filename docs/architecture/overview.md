# Project 1 — AI Hiring Platform

> Codename: `hire-flow`
> Type: Microservices platform (Go + Python + React)
> Goal: Portfolio + skill building (distributed patterns, multi-framework Go, AI/ML integration)

---

## Legend

Platform for matching freelancers with job offers via AI-powered embeddings.
Client publishes a job posting → AI analyzes and matches with freelancer profiles →
both parties confirm → contract is created → payment through payment service.

### Actors

- **Client** — publishes jobs, selects freelancers, pays
- **Freelancer** — creates profile with skills, receives matches, accepts contracts
- **Admin** — moderation, analytics, dispute resolution

---

## Architecture overview

```
                         ┌─────────────┐
                         │   Traefik   │
                         │ API Gateway │
                         └──────┬──────┘
                                │
                 ┌──────────────┼──────────────┐
                 │              │              │
          ┌──────┴──────┐ ┌────┴────┐ ┌───────┴──────┐
          │  BFF Layer  │ │   BFF   │ │     BFF      │
          │  (client)   │ │(freelan)│ │   (admin)    │
          └──────┬──────┘ └────┬────┘ └───────┬──────┘
                 │              │              │
        ─────────┴──────────────┴──────────────┴─────────
        │            Internal service mesh              │
        ─────────────────────────────────────────────────
           │            │            │           │
     ┌─────┴────┐ ┌─────┴────┐ ┌────┴─────┐ ┌──┴───────┐
     │ Jobs API │ │AI Matchin│ │Contracts │ │ Payments │
     │  (Echo)  │ │ (FastAPI)│ │  (Chi)   │ │  (Gin)   │
     │ Postgres │ │  Qdrant  │ │  MySQL   │ │ Postgres │
     └─────┬────┘ └─────┬────┘ └────┬─────┘ └──┬───────┘
           │            │            │           │
        ─────────────────────────────────────────────────
        │              NATS JetStream                   │
        │         (events / commands / CDC)              │
        ─────────────────────────────────────────────────
```

---

## Services detail

### 1. Jobs API (Go — Echo)

**Responsibility:** CRUD for job postings, freelancer profiles, skill taxonomy.

| Aspect | Decision |
|--------|----------|
| Framework | Echo v4 — middleware-rich, clean context API |
| Database | PostgreSQL 16 — jobs, profiles, skills |
| Patterns | Outbox pattern (events to NATS via polling) |
| API | REST (JSON), OpenAPI spec |

**Key endpoints:**
- `POST /api/v1/jobs` — create job posting
- `GET /api/v1/jobs` — list/filter jobs (pagination, filters)
- `POST /api/v1/profiles` — create freelancer profile
- `GET /api/v1/profiles/{id}` — get profile with skills
- `PUT /api/v1/profiles/{id}/skills` — update skills

**Events published:**
- `job.created` → triggers AI matching
- `job.updated` → re-triggers matching
- `profile.updated` → CDC → re-index embeddings in Qdrant

### 2. AI Matching Service (Python — FastAPI)

**Responsibility:** Embedding generation, vector search, match scoring.

| Aspect | Decision |
|--------|----------|
| Framework | FastAPI + uvicorn |
| Database | Qdrant (vector DB for embeddings) |
| ML | sentence-transformers (all-MiniLM-L6-v2 for MVP) |
| Patterns | CDC consumer (profile changes → re-embed) |

**Key endpoints:**
- `POST /api/v1/match/job/{id}` — find matching freelancers for job
- `POST /api/v1/match/profile/{id}` — find matching jobs for freelancer
- `GET /api/v1/match/score` — get match score between job + profile

**Events consumed:**
- `job.created` → generate job embedding → store in Qdrant
- `profile.updated` → re-generate profile embedding (CDC)

**Events published:**
- `match.found` → notify both parties

### 3. Contracts Service (Go — Chi)

**Responsibility:** Contract lifecycle: proposal → negotiation → active → completed/cancelled.

| Aspect | Decision |
|--------|----------|
| Framework | Chi v5 — idiomatic, stdlib-compatible router |
| Database | MySQL 8 — contracts, terms, milestones |
| Patterns | Saga orchestrator (contract creation flow) |

**Key endpoints:**
- `POST /api/v1/contracts` — create contract proposal
- `PUT /api/v1/contracts/{id}/accept` — freelancer accepts
- `PUT /api/v1/contracts/{id}/complete` — mark milestone complete
- `PUT /api/v1/contracts/{id}/cancel` — cancel with compensation logic

**Saga: Contract creation flow:**
```
1. Client selects freelancer from matches
2. → Contracts: create contract (PENDING)
3. → Payments: reserve funds (HOLD)
4. → Notify freelancer
5. Freelancer accepts
6. → Contracts: activate (ACTIVE)
7. → Payments: confirm hold

Compensation (if freelancer declines):
7c. → Payments: release hold
8c. → Contracts: mark DECLINED
9c. → Notify client
```

**Events published:**
- `contract.created`, `contract.accepted`, `contract.completed`, `contract.cancelled`

### 4. Payments Service (Go — Gin)

**Responsibility:** Payment holds, releases, transfers. Mock payment provider for MVP.

| Aspect | Decision |
|--------|----------|
| Framework | Gin — most popular, best for portfolio recognition |
| Database | PostgreSQL 16 — transactions, wallets, ledger |
| Patterns | Outbox + Saga participant |

**Key endpoints:**
- `POST /api/v1/payments/hold` — reserve funds
- `POST /api/v1/payments/release` — release hold
- `POST /api/v1/payments/transfer` — execute payment
- `GET /api/v1/payments/wallet/{user_id}` — balance

**Events published:**
- `payment.held`, `payment.released`, `payment.transferred`, `payment.failed`

> **TODO: production-ready** — Stripe/PayPal integration. MVP uses mock wallet with fake balance.

---

## BFF Layer (Backend for Frontend)

Three lightweight BFF services (Go — stdlib `net/http` with Go 1.22+ routing).

The choice of stdlib for BFFs is intentional: demonstrates Go proficiency without frameworks,
and BFFs don't need a middleware-heavy framework.

| BFF | Consumers | Aggregates from |
|-----|-----------|-----------------|
| Client BFF | Web/mobile client app | Jobs + Matching + Contracts + Payments |
| Freelancer BFF | Freelancer app | Jobs + Matching + Contracts + Payments |
| Admin BFF | Admin dashboard | All services + analytics |

**BFF responsibilities:**
- Request aggregation (1 BFF call = N internal calls)
- Response shaping per consumer
- Auth token validation (JWT)
- Rate limiting per consumer type

> **TODO: production-ready** — BFF can be replaced with GraphQL Federation later.

---

## Infrastructure

### API Gateway — Traefik v3

Traefik uses file-based provider for routing configuration (not Docker labels,
due to Docker Desktop for Mac socket compatibility issues).

```yaml
# Routing rules (infra/traefik/dynamic.yml)
routers:
  bff-client:
    rule: "PathPrefix(`/client`)"
    middlewares: [strip-client]
    service: bff-client
  bff-freelancer:
    rule: "PathPrefix(`/freelancer`)"
    middlewares: [strip-freelancer]
    service: bff-freelancer
  bff-admin:
    rule: "PathPrefix(`/admin`)"
    middlewares: [strip-admin]
    service: bff-admin
```

**Features enabled:**
- Rate limiting middleware
- Circuit breaker (per service)
- Dashboard: `http://localhost:8080`
- Health check routing

### Message Broker — NATS JetStream

| Feature | Configuration |
|---------|--------------|
| Streams | `JOBS`, `MATCHES`, `CONTRACTS`, `PAYMENTS` |
| Consumers | Durable, pull-based |
| Retention | WorkQueue policy (ack removes) |
| Replay | File-based storage for durability |

**Why NATS over Kafka for MVP:**
- Single binary, no ZooKeeper/KRaft
- Docker image: 20MB vs Kafka's 500MB+
- JetStream gives persistence, exactly-once, replay
- Migration path to Kafka is clean (same pub/sub mental model)

> **Milestone: Kafka migration** — swap NATS → Kafka + Schema Registry when adding CDC with Debezium.

### CDC (Change Data Capture)

**Phase 1 (MVP):** Polling-based CDC
- Jobs API polls outbox table every 5s
- Publishes to NATS

**Phase 2 (Milestone):** Debezium + Kafka Connect
- PostgreSQL WAL → Debezium → Kafka → consumers
- Real CDC without polling overhead

### Outbox Pattern

Every service that publishes events uses the same pattern:

```sql
CREATE TABLE outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    published_at TIMESTAMP NULL
);

-- Index for polling
CREATE INDEX idx_outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;
```

Outbox publisher (Go goroutine) polls every 5s, publishes to NATS, marks `published_at`.

---

## Frontend (React SPA)

Three separate SPA apps (one per BFF consumer), sharing a common UI kit.

| App | BFF | Purpose |
|-----|-----|---------|
| client-app | Client BFF | Job posting, match review, contract management |
| freelancer-app | Freelancer BFF | Profile, skill management, match notifications, contracts |
| admin-app | Admin BFF | Moderation, analytics, dispute resolution |

### Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Core | React 19 + Vite 6 + TypeScript 5 | SPA-first (BFF handles aggregation, SSR not needed) |
| Routing | TanStack Router | Type-safe, modern, better DX than React Router v7 |
| Server State | TanStack Query v5 | Caching, refetch, optimistic updates for REST BFF |
| Client State | Zustand | Minimal — most state lives in TanStack Query |
| UI Components | shadcn/ui + Tailwind CSS v4 | Copy-paste components, full control, not a dependency |
| Icons | Lucide React | Ships with shadcn/ui |
| Validation | Zod | Type-safe validation for forms + API responses |
| URL State | nuqs | Typed URL params (filters, pagination in URL) |
| Animations | Framer Motion | Smooth transitions |
| Charts | Recharts | Admin dashboard analytics |
| Linting | Biome | Replaces ESLint + Prettier, 25x faster |
| Build Orchestration | Turborepo (optional) | Monorepo build caching |

### Why NOT Next.js

This is an API-first architecture. Three BFF services (Go) already aggregate data
from microservices. Adding Next.js SSR on top would create a redundant layer.
Vite SPA is simpler, faster to develop, and lets BFF be the single data gateway.

### Structure (pnpm workspace)

```
frontend/
├── packages/
│   └── ui/                    # Shared shadcn/ui components + Tailwind config
│       ├── package.json
│       └── src/
│           ├── components/    # Button, Input, Table, Modal, etc.
│           ├── hooks/         # Shared hooks (useAuth, useBFF)
│           └── lib/           # API client, Zod schemas, utils
├── apps/
│   ├── client/                # Client-facing app
│   │   ├── package.json
│   │   ├── vite.config.ts
│   │   └── src/
│   │       ├── routes/        # TanStack Router file-based routes
│   │       ├── features/      # Feature modules (jobs, contracts, payments)
│   │       └── main.tsx
│   ├── freelancer/            # Freelancer app
│   │   └── ... (same structure)
│   └── admin/                 # Admin dashboard
│       └── ... (same structure)
├── package.json               # pnpm workspace root
├── pnpm-workspace.yaml
└── biome.json                 # Shared linting config
```

> **Note:** Frontend is a later milestone (M8+). Backend is priority.
> Frontend apps can be added incrementally — start with admin-app (simplest, tables + forms).

---

## Go Architecture — Idiomatic Approach

**Important:** Go services do NOT use Java/PHP-style hexagonal folder structure
(domain/port/adapter/app). Instead, we follow idiomatic Go patterns:
flat packages, grouped by feature. Interfaces belong to the consumer, not the provider.

References: "100 Go Mistakes and How to Avoid Them" (T. Harsanyi),
Ben Johnson's "Standard Package Layout", Mat Ryer's patterns.

### Principles

1. **Interfaces belong to the consumer, not the provider**
2. **Group by feature, not by layer**
3. **Flat package structure** — avoid deep nesting
4. **Accept interfaces, return structs**
5. **Package names are part of the API** — `job.Service`, not `service.JobService`

### Example: Jobs API Structure

```
services/jobs-api/
├── cmd/
│   └── server/
│       └── main.go              # Wiring, DI, startup
├── job.go                       # Job domain type + JobRepository interface
├── job_store.go                 # PostgreSQL JobRepository implementation
├── job_handler.go               # HTTP handlers (Echo)
├── job_service.go               # Business logic / use cases
├── job_test.go                  # Unit tests
├── profile.go                   # Profile domain type + repository interface
├── profile_store.go             # PostgreSQL implementation
├── profile_handler.go           # HTTP handlers
├── outbox.go                    # Outbox domain + publisher goroutine
├── outbox_store.go              # PostgreSQL outbox implementation
├── skill.go                     # Skill value object / taxonomy
├── migrations/
│   ├── 001_create_jobs.up.sql
│   └── ...
├── go.mod
└── go.sum
```

**Key principles:**
- `job.go` defines BOTH the domain type AND the interface (JobStore)
- `job_store.go` implements JobStore (consumer defines interface)
- No `internal/` unless package is large enough to warrant sub-packages
- No `domain/`, `port/`, `adapter/` folders
- DDD concepts still apply (aggregates, value objects) — just flat files
- Test files next to the code they test

### When to add sub-packages

If a service grows beyond ~15 files, split by subdomain:

```
services/contracts/
├── cmd/server/main.go
├── contract.go
├── contract_store.go
├── contract_handler.go
├── saga/                      # Saga is complex enough for own package
│   ├── orchestrator.go
│   ├── steps.go
│   └── orchestrator_test.go
├── milestone.go
├── milestone_store.go
└── ...
```

### Contrast with PHP/Symfony DDD

| Aspect | PHP/Symfony (ddd_template) | Go (hire-flow) |
|--------|---------------------------|----------------|
| Interfaces | Explicit ports in `Port/` dir | Defined in consumer file |
| Structure | `Domain/Port/Adapter/App/` | Flat, grouped by entity |
| DI | Symfony container, YAML config | Manual wiring in `main.go` |
| Value Objects | Readonly classes | Plain structs or custom types |
| Repository | Interface + Doctrine adapter | Interface in `job.go` + impl in `job_store.go` |

The hexagonal principle (dependencies point inward) is the same.
The folder structure is different because Go's package system works differently.

---

## Repo Structure (Monorepo)

```
hire-flow/
├── CLAUDE.md                      # Claude Code instructions
├── compose.yaml                   # All services + infra
├── Makefile                       # Common commands
├── go.work                        # Go workspace (links all Go services)
├── docs/
│   ├── architecture/
│   │   └── overview.md            # This document (canonical)
│   └── plans/                     # Implementation plans
├── services/
│   ├── jobs-api/                  # Go (Echo) + PostgreSQL — flat structure
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   ├── cmd/server/main.go
│   │   ├── job.go                 # Domain type + store interface
│   │   ├── profile.go
│   │   ├── postgres.go            # Store implementations
│   │   ├── handler.go             # Echo HTTP handlers
│   │   ├── outbox.go
│   │   ├── nats.go
│   │   ├── migrations/
│   │   └── job_test.go
│   ├── ai-matching/               # Python (FastAPI) + Qdrant
│   │   ├── Dockerfile
│   │   ├── pyproject.toml
│   │   ├── src/
│   │   │   ├── models.py          # Pydantic models
│   │   │   ├── embedding.py       # Embedding generation
│   │   │   ├── qdrant_store.py    # Vector DB operations
│   │   │   ├── api.py             # FastAPI routes
│   │   │   ├── consumer.py        # NATS event consumer
│   │   │   └── config.py
│   │   └── tests/
│   ├── contracts/                 # Go (Chi) + MySQL — flat structure
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   ├── cmd/server/main.go
│   │   ├── contract.go            # Domain type + store interface
│   │   ├── mysql.go               # Store implementation
│   │   ├── handler.go             # Chi HTTP handlers
│   │   ├── saga.go                # Saga orchestrator
│   │   ├── migrations/
│   │   └── contract_test.go
│   ├── payments/                  # Go (Gin) + PostgreSQL — flat structure
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   ├── cmd/server/main.go
│   │   ├── wallet.go              # Domain type + store interface
│   │   ├── transaction.go
│   │   ├── postgres.go
│   │   ├── handler.go             # Gin HTTP handlers
│   │   ├── outbox.go
│   │   ├── migrations/
│   │   └── wallet_test.go
│   ├── bff/
│   │   ├── client/                # Go (stdlib) — client-facing BFF
│   │   ├── freelancer/            # Go (stdlib) — freelancer-facing BFF
│   │   └── admin/                 # Go (stdlib) — admin-facing BFF
│   └── frontend/                  # React (Vite + TanStack + shadcn/ui)
│       ├── packages/ui/           # Shared component library
│       ├── apps/
│       │   ├── client/
│       │   ├── freelancer/
│       │   └── admin/
│       ├── package.json           # pnpm workspace
│       └── turbo.json
├── infra/
│   ├── traefik/
│   │   ├── traefik.yml            # Static config
│   │   └── dynamic.yml            # Routing config (routers, middlewares, services)
│   └── nats/
│       └── nats-server.conf
└── scripts/
    ├── migrate.sh                 # Run all migrations
    ├── seed.sh                    # Seed test data
    └── dev-up.sh                  # Start everything
```

---

## Milestones

### M0 — Skeleton ✅

**Goal:** Monorepo structure, Docker Compose, all services boot and respond to health checks.

- [x] Init repo with `CLAUDE.md`, `Makefile`
- [x] `compose.yaml` — Traefik + PostgreSQL x2 + MySQL + NATS + Qdrant
- [x] Skeleton for each Go service (health endpoint only)
- [x] Skeleton for Python service (health endpoint only)
- [x] Traefik routing to BFFs via file provider
- [x] `make up`, `make down`, `make logs`, `make health`
- [x] Traefik dashboard accessible

**Merge criteria:** `make up && make health` — all services green.

### M1 — Jobs API

**Goal:** Full CRUD for jobs and profiles with Outbox pattern.

- [ ] Domain: Job, Profile, Skill entities + value objects
- [ ] PostgreSQL migrations (jobs, profiles, skills, outbox)
- [ ] Store implementations
- [ ] Echo HTTP handlers with OpenAPI docs
- [ ] Outbox publisher goroutine → NATS
- [ ] Unit tests + integration tests
- [ ] Seed data script

**Merge criteria:** API works end-to-end, events appear in NATS.

### M2 — AI Matching Service

**Goal:** Embedding generation + vector search + CDC consumer.

- [ ] FastAPI app structure
- [ ] Embedding generation with sentence-transformers
- [ ] Qdrant collection setup + CRUD
- [ ] NATS consumer: `job.created` → embed → store
- [ ] NATS consumer: `profile.updated` → re-embed (CDC)
- [ ] Match scoring endpoint
- [ ] Tests with mock embeddings

**Merge criteria:** Create job → auto-embeds → search returns relevant profiles.

### M3 — Payments Service

**Goal:** Mock wallet + hold/release/transfer + Outbox.

- [ ] Domain: Wallet, Transaction, Hold entities
- [ ] PostgreSQL migrations (wallets, transactions, holds, outbox)
- [ ] Gin handlers
- [ ] Outbox publisher → NATS
- [ ] Seed wallets with test balance
- [ ] Tests

**Merge criteria:** Can hold, release, transfer funds. Events in NATS.

### M4 — Contracts + Saga

**Goal:** Contract lifecycle with Saga orchestration across services.

- [ ] Domain: Contract, Milestone, Proposal entities
- [ ] MySQL migrations
- [ ] Chi handlers
- [ ] Saga orchestrator: create contract flow
- [ ] Saga compensation: decline/cancel flow
- [ ] Integration tests (saga happy path + compensation)
- [ ] Events consumed from Jobs + Payments
- [ ] Events published to NATS

**Merge criteria:** Full contract flow works: propose → accept → complete → pay. Cancel triggers compensation.

### M5 — BFF Layer

**Goal:** Three BFF services aggregating internal APIs.

- [ ] Client BFF: job posting flow, match review, contract management
- [ ] Freelancer BFF: profile management, match notifications, contract acceptance
- [ ] Admin BFF: overview dashboard data, moderation endpoints
- [ ] JWT auth validation in each BFF
- [ ] Rate limiting per BFF
- [ ] Traefik routing: `/client/*`, `/freelancer/*`, `/admin/*`

**Merge criteria:** All three BFFs route through Traefik, aggregate data from internal services.

### M6 — CDC + Observability

**Goal:** Proper CDC pipeline + logging + metrics + tracing.

- [x] Structured logging with `slog` (Go) / `structlog` (Python)
- [x] OpenTelemetry traces across services
- [x] Prometheus metrics endpoint per service
- [x] Grafana dashboards (basic: request rate, latency, errors)
- [x] Health check aggregation in Traefik
- [ ] Replace polling outbox → Debezium CDC (if Kafka added)

> **TODO: production-ready** — Jaeger/Tempo for trace storage, alerting rules.

**Merge criteria:** Can trace a request across 3+ services. Grafana shows basic metrics.

### M7 — Kafka Migration (optional)

**Goal:** Replace NATS with Kafka for CDC + event streaming experience.

- [ ] Kafka + Schema Registry in Docker Compose
- [ ] Debezium connector for PostgreSQL CDC
- [ ] Migrate publishers from NATS → Kafka
- [ ] Migrate consumers
- [ ] Schema Registry with Avro/Protobuf schemas

**Merge criteria:** All events flow through Kafka. CDC via Debezium works.

### M8–M10 — Frontend

**M8 — Client Frontend:** Full client flow (post job → review matches → propose contract → pay).
**M9 — Freelancer Frontend:** Profile management, match notifications, contract acceptance.
**M10 — Admin Frontend:** Moderation, analytics, dispute resolution.

> Frontend details (component structure, design system) — finalized in Claude Code with frontend-design skill.

---

## Tech Stack Summary

| Component | Technology | Why |
|-----------|-----------|-----|
| API Gateway | Traefik v3 | Auto-discovery, file provider, same tool for K8s later |
| Jobs API | Go + Echo v4 + PostgreSQL 16 | Middleware-rich, clean context API |
| AI Matching | Python + FastAPI + Qdrant | ML ecosystem, async, vector DB |
| Contracts | Go + Chi v5 + MySQL 8 | Idiomatic Go, stdlib-compatible |
| Payments | Go + Gin + PostgreSQL 16 | Most popular Go framework, portfolio value |
| BFF (×3) | Go stdlib (net/http) | Shows pure Go skill, no framework needed |
| Frontend (×3) | React 19 + Vite 6 + TanStack | Modern stack, type-safe routing + data fetching |
| UI | shadcn/ui + Tailwind CSS v4 | Copy-paste components, full control |
| State | Zustand | Minimal, no boilerplate |
| Monorepo (FE) | pnpm workspaces + Turborepo | Shared UI package across 3 apps |
| Broker (MVP) | NATS JetStream | Lightweight, fast start, clean migration path |
| Broker (later) | Apache Kafka + Schema Registry | Industry standard, Debezium CDC |
| CDC (MVP) | Outbox polling | Simple, reliable, no extra infra |
| CDC (later) | Debezium + Kafka Connect | Real WAL-based CDC |
| Observability | OpenTelemetry + Prometheus + Grafana | Industry standard stack |
| Containerization | Docker Compose (dev), K8s (later projects) | Progressive complexity |

---

## Key Patterns Reference

### Saga Pattern (Contract Creation)

```
Orchestrator: Contracts Service

Step 1: CreateContract (local) → status: PENDING
Step 2: HoldPayment (command → Payments) → wait for payment.held
Step 3: NotifyFreelancer (command → Jobs/Notification)
Step 4: WaitForAcceptance (timeout: 48h)

If accepted:
  Step 5: ActivateContract (local) → status: ACTIVE
  Step 6: ConfirmHold (command → Payments)

If declined/timeout:
  Step 5c: ReleasePayment (compensate → Payments)
  Step 6c: DeclineContract (local) → status: DECLINED
  Step 7c: NotifyClient (compensate → Jobs/Notification)
```

### Outbox Pattern

```
[Business logic] → BEGIN TX
  1. Save entity
  2. Insert into outbox table
→ COMMIT

[Background goroutine] → every 5s
  1. SELECT * FROM outbox WHERE published_at IS NULL ORDER BY created_at LIMIT 100
  2. Publish each to NATS
  3. UPDATE outbox SET published_at = NOW() WHERE id IN (...)
```

### CDC Flow (Profile → Re-embed)

```
Freelancer updates profile
→ Jobs API saves to PostgreSQL + outbox event: profile.updated
→ Outbox publisher → NATS stream: JOBS
→ AI Matching consumer picks up event
→ Re-generates embedding for updated profile
→ Upserts vector in Qdrant
→ Next match query returns updated results
```

---

## Notes

- **Auth is JWT-based but simplified for MVP.** No OAuth provider.
  BFF validates JWT, services trust internal calls.
  TODO: production-ready — Keycloak/Auth0 integration.
- **No real ML training.** We use pre-trained sentence-transformers model.
  Fine-tuning is out of scope — this is a backend/infra project.
- **Database per service is strict.** No shared databases, no cross-service joins.
  If service A needs data from service B, it calls B's API or consumes B's events.
