# hire-flow — AI Hiring Platform (Microservices)

## Quick Start
make up          # Start all services
make down        # Stop all
make health      # Check all services respond to /health
make logs s=jobs # Logs for specific service
make test        # Run all tests

## Architecture
Monorepo with 3 Go core services + 1 Python ML service + 3 Go BFFs (7 total).
See docs/architecture/overview.md for full details.

## Conventions
- Go services: flat package structure (group by feature, not layer)
- Interfaces belong to the consumer, not the provider
- Python service: FastAPI with src/ layout
- Each service has own database, NO cross-service DB access
- Events via NATS JetStream (future milestones)
- Outbox pattern for reliable event publishing (future milestones)

## Go Version
Go 1.25 (modules target 1.25.0, Dockerfiles use golang:1.25-alpine).
Use modern features up to 1.24: enhanced ServeMux patterns, t.Context() in tests,
omitzero in JSON tags, SplitSeq/FieldsSeq for iteration, wg.Go().

## Service Ports (local dev)
| Service          | Port | Framework |
|------------------|------|-----------|
| Traefik HTTP     | 80   | —         |
| Traefik Dashboard| 8080 | —         |
| jobs-api         | 8001 | Echo v4   |
| ai-matching      | 8002 | FastAPI   |
| contracts        | 8003 | Chi v5    |
| payments         | 8004 | Gin       |
| bff-client       | 8010 | stdlib    |
| bff-freelancer   | 8011 | stdlib    |
| bff-admin        | 8012 | stdlib    |
| postgres-jobs    | 5432 | —         |
| postgres-payments| 5433 | —         |
| MySQL            | 3306 | —         |
| NATS client      | 4222 | —         |
| NATS monitoring  | 8222 | —         |
| Qdrant HTTP      | 6333 | —         |
| Qdrant gRPC      | 6334 | —         |
| OTel Collector   | 4317 | gRPC      |
| OTel Collector   | 4318 | HTTP      |
| Prometheus       | 9090 | —         |
| Grafana          | 3000 | —         |
| frontend-client  | 5173 | Vite      |
| frontend-freelancer | 5174 | Vite   |

## Go Conventions
- Error handling: wrap with fmt.Errorf("operation: %w", err)
- Naming: domain types are NOT prefixed (Job, not JobEntity)
- Tests: table-driven, testify for assertions
- Each service has own go.mod (Go workspace via go.work at root)
- No internal/ unless service grows beyond ~15 files

## Python Conventions
- Python 3.12+, FastAPI
- pydantic v2 for models
- pytest for tests
- ruff for linting

## Design System
Always read DESIGN.md before making any visual or UI decisions.
All font choices, colors, spacing, and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match DESIGN.md.

## Docker
- compose.yaml (not docker-compose.yml)
- One process per container
- Multi-stage builds for Go services
- Docker healthchecks on all containers
