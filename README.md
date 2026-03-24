# hire-flow

AI-powered hiring platform connecting clients with freelancers. Microservices architecture with Go backend, Python ML service, and React frontends.

## Tech Stack

**Backend:** Go 1.25 (Echo, Chi, Gin, stdlib), Python 3.12+ (FastAPI)
**Frontend:** React 19, TanStack Router, Vite, pnpm workspaces
**Infrastructure:** Docker, Traefik, NATS JetStream, PostgreSQL, MySQL, Qdrant
**Observability:** OpenTelemetry, Prometheus, Grafana

## Architecture

| Service | Port | Description |
|---------|------|-------------|
| jobs-api | 8001 | Job postings (Echo v4, PostgreSQL) |
| ai-matching | 8002 | ML matching (FastAPI, Qdrant) |
| contracts | 8003 | Contract management (Chi v5, MySQL) |
| payments | 8004 | Payments & escrow (Gin, PostgreSQL) |
| bff-client | 8010 | Client BFF (stdlib) |
| bff-freelancer | 8011 | Freelancer BFF (stdlib) |
| bff-admin | 8012 | Admin BFF (stdlib) |

**Infrastructure:** Traefik (:80, :8080), PostgreSQL (:5432, :5433), MySQL (:3306), NATS (:4222), Qdrant (:6333)

## Quick Start

```bash
# Backend (all services via Docker)
make up            # Build & start all containers
make health        # Verify all services are healthy
make logs s=jobs-api  # Tail logs for a service

# Frontend (from services/frontend/)
cd services/frontend
pnpm install
pnpm --filter @hire-flow/client dev        # Client app :5173
pnpm --filter @hire-flow/freelancer dev    # Freelancer app :5174
pnpm --filter @hire-flow/admin dev         # Admin app :5175

# Tests
make test          # All Go + Python tests
```

## Demo Credentials

All passwords: `password`

| Email | Role | Frontend URL |
|-------|------|-------------|
| client@example.com | Client | http://localhost:5173 |
| freelancer@example.com | Freelancer | http://localhost:5174 |
| admin@example.com | Admin | http://localhost:5175 |

## Key URLs

| Service | URL |
|---------|-----|
| Traefik Dashboard | http://localhost:8080 |
| Grafana | http://localhost:3000 (admin/admin) |
| Prometheus | http://localhost:9090 |
| NATS Monitoring | http://localhost:8222 |
| Qdrant Dashboard | http://localhost:6333/dashboard |
