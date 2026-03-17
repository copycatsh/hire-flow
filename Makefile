.PHONY: up down health health-traefik logs test migrate build

# Start all services
up:
	docker compose up -d --build

# Stop all services
down:
	docker compose down

# Check all services respond to /health
health:
	@echo "Checking service health..."
	@failed=0; \
	for svc in \
		"jobs-api:8001" \
		"ai-matching:8002" \
		"contracts:8003" \
		"payments:8004" \
		"bff-client:8010" \
		"bff-freelancer:8011" \
		"bff-admin:8012"; \
	do \
		name=$${svc%%:*}; \
		port=$${svc##*:}; \
		if curl -sf "http://localhost:$$port/health" > /dev/null 2>&1; then \
			printf "  %-20s ✓ healthy\n" "$$name"; \
		else \
			printf "  %-20s ✗ unhealthy\n" "$$name"; \
			failed=1; \
		fi; \
	done; \
	echo ""; \
	if [ "$$failed" = "1" ]; then \
		echo "FAIL: some services are unhealthy"; \
		exit 1; \
	else \
		echo "OK: all services healthy"; \
	fi

# Traefik routing check (BFF via gateway)
health-traefik:
	@echo "Checking Traefik routing..."
	@failed=0; \
	for path in client freelancer admin; do \
		if curl -sf "http://localhost:80/$$path/health" > /dev/null 2>&1; then \
			printf "  %-20s ✓ routed\n" "/$$path/health"; \
		else \
			printf "  %-20s ✗ not routed\n" "/$$path/health"; \
			failed=1; \
		fi; \
	done; \
	echo ""; \
	if [ "$$failed" = "1" ]; then \
		echo "FAIL: some routes are broken"; \
		exit 1; \
	else \
		echo "OK: all routes healthy"; \
	fi

# Show logs for a specific service (usage: make logs s=jobs-api)
logs:
	docker compose logs -f $(s)

# Run all Go tests (via go workspace)
test:
	go test ./...

# Run migrations (placeholder for M1+)
migrate:
	@echo "No migrations to run (M0 skeleton)"

# Build all images without starting
build:
	docker compose build
