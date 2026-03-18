.PHONY: up down seed-data health-check simulate-traffic bench-local test

up:
	docker compose up -d

down:
	docker compose down -v

seed-data:
	go run ./services/traffic-simulator/cmd/seed/main.go

health-check:
	@echo "Checking services..."
	@curl -sf http://localhost:8080/health && echo " event-collector: OK" || echo " event-collector: FAIL"
	@curl -sf http://localhost:8090/health && echo " recommendation-api: OK" || echo " recommendation-api: FAIL"
	@docker compose exec dragonfly redis-cli ping 2>/dev/null && echo " dragonfly: OK" || echo " dragonfly: FAIL"

simulate-traffic:
	go run ./services/traffic-simulator/cmd/simulate/main.go

bench-local:
	k6 run ./load-tests/local-benchmark.js

test:
	go test ./shared/go/... -v -count=1
	-go test ./services/event-collector/... -v -count=1
	-go test ./services/recommendation-api/... -v -count=1
	-go test ./services/traffic-simulator/... -v -count=1
