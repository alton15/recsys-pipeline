.PHONY: up down logs seed-data simulate-traffic health-check verify-e2e test bench-local docker-build-all docker-push-all

COMPOSE_FILE := infra/docker-compose.yml

# ── Stack lifecycle ──────────────────────────────
up:
	docker compose -f $(COMPOSE_FILE) up -d

down:
	docker compose -f $(COMPOSE_FILE) down -v

logs:
	docker compose -f $(COMPOSE_FILE) logs -f

# ── Data & traffic ───────────────────────────────
seed-data:
	go run ./services/traffic-simulator/cmd/seed/main.go

simulate-traffic:
	go run ./services/traffic-simulator/cmd/simulate/main.go

# ── Health & verification ────────────────────────
health-check:
	@echo "Checking services..."
	@curl -sf http://localhost:8080/health && echo " event-collector: OK" || echo " event-collector: FAIL"
	@curl -sf http://localhost:8090/health && echo " recommendation-api: OK" || echo " recommendation-api: FAIL"
	@docker compose -f $(COMPOSE_FILE) exec dragonfly redis-cli ping 2>/dev/null && echo " dragonfly: OK" || echo " dragonfly: FAIL"

verify-e2e:
	@bash scripts/verify-e2e.sh

# ── Testing ──────────────────────────────────────
test:
	go test ./shared/go/... -v -count=1
	-go test ./services/event-collector/... -v -count=1
	-go test ./services/recommendation-api/... -v -count=1
	-go test ./services/traffic-simulator/... -v -count=1

bench-local:
	k6 run ./load-tests/local-benchmark.js

# ── Docker build ─────────────────────────────────
docker-build-all:
	docker compose -f $(COMPOSE_FILE) build

docker-push-all:
	@test -n "$(REGISTRY)" || (echo "Usage: make docker-push-all REGISTRY=your-registry.io" && exit 1)
	@echo "Pushing images to $(REGISTRY)..."
