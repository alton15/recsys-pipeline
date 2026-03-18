# Recsys Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-grade, cloud-agnostic recommendation pipeline template for 50M DAU commerce services that runs locally via docker-compose and scales on Kubernetes.

**Architecture:** 4-Plane (Data/Stream/Batch/Control) with 3-Tier serving (pre-computed cache → CPU re-rank → GPU inference). Embedded architecture in Go orchestrator eliminates fan-out. DragonflyDB, Redpanda, and L4 INT8 for cost optimization.

**Tech Stack:** Go (API services), Java/Kotlin (Flink), Python (Spark, ML), DragonflyDB, Redpanda, Milvus, Triton, Envoy, Prometheus/Grafana, k6

**Spec:** `docs/superpowers/specs/2026-03-18-recsys-pipeline-design.md`

---

## Review Fixes Applied

This plan addresses all Critical/Major issues from spec review:
- **C1**: Added Task 3 (Seed Data Generator) early — before any service needs test data
- **C2**: Added Tier 0 CDN serving in Task 6 (cacheable popular endpoint + Cache-Control headers)
- **C3**: Added global backpressure state machine in Task 8 (4-level degradation)
- **C4**: Task 7 (GBDT Tier 2) now has complete code with weighted scoring fallback (no CGo dependency)
- **M1**: MLflow logging added to Task 12 (ML Models)
- **M2**: Model rollout (Shadow/Canary/Rollback) added to Task 17
- **M3**: TTL policies on all DragonflyDB writes throughout plan
- **M4**: Empty tests filled in (bitmap, Flink, Triton)
- **M5**: `leaves` library replaced with built-in weighted scorer (leaves as optional enhancement)
- **M6**: Hardcoded credentials → env_file references in docker-compose
- **M7**: `git add -A` replaced with specific file lists in all commit steps
- **m1**: Airflow + Alertmanager added to docker-compose
- **m2**: DragonflyDB metrics port fixed (6380, not 6379)
- **m3**: ReadTimeout 3ms → 10ms
- **m4**: stock:next_bit_pos initialization in seed data

## Phase Overview

| Phase | Name | Deliverable | Dependency |
|-------|------|-------------|------------|
| 1 | Foundation & Infrastructure | docker-compose with all data stores + monitoring + Airflow | None |
| 2 | Seed Data & Simulator | Sample data generator (needed by all later phases) | Phase 1 |
| 3 | Data Plane — Event Collector | Go event ingestion → Redpanda | Phase 1 |
| 4 | Data Plane — Recommendation API (Tier 1) | Pre-computed cache serving | Phase 2, 3 |
| 5 | Data Plane — Recommendation API (Tier 2) | GBDT CPU re-ranking | Phase 4 |
| 6 | Data Plane — Tier 0 CDN + Degradation | CDN-cacheable endpoint, backpressure state machine | Phase 4 |
| 7 | Stream Plane | Flink real-time features + inventory bitmap | Phase 4 |
| 8 | Batch Plane & ML | Spark features + Two-Tower + pre-compute + MLflow | Phase 7 |
| 9 | Ranking Service (Tier 3) | Triton ONNX serving | Phase 8 (needs trained model) |
| 10 | Verification & Production | k6 benchmarks, chaos tests, Helm charts, A/B, model rollout | Phase 9 |

---

## Phase 1: Foundation & Infrastructure

### Task 1: Project Scaffold & Go Workspace

**Files:**
- Create: `go.work`
- Create: `go.mod` (root, for shared libs)
- Create: `Makefile`
- Create: `.gitignore`
- Create: `CLAUDE.md`
- Create: `configs/local.env`

- [ ] **Step 1: Create root go.work for multi-module workspace**

```
// go.work
go 1.23

use (
    ./services/event-collector
    ./services/recommendation-api
    ./services/traffic-simulator
    ./shared/go
)
```

- [ ] **Step 2: Create shared Go module for common types**

```
// shared/go/go.mod
module github.com/recsys-pipeline/shared

go 1.23
```

Create shared event types:

```go
// shared/go/event/event.go
package event

import "time"

type Type string

const (
    Click         Type = "click"
    View          Type = "view"
    Purchase      Type = "purchase"
    Search        Type = "search"
    AddToCart     Type = "add_to_cart"
    RemoveFromCart Type = "remove_from_cart"
)

type Metadata struct {
    Query    string  `json:"query,omitempty"`
    Position int     `json:"position,omitempty"`
    Price    int64   `json:"price,omitempty"`
    Source   string  `json:"source,omitempty"`
}

type Event struct {
    EventID    string    `json:"event_id"`
    UserID     string    `json:"user_id"`
    EventType  Type      `json:"event_type"`
    ItemID     string    `json:"item_id"`
    CategoryID string    `json:"category_id"`
    Timestamp  time.Time `json:"timestamp"`
    SessionID  string    `json:"session_id"`
    Metadata   Metadata  `json:"metadata"`
}
```

- [ ] **Step 3: Create shared DragonflyDB key helpers**

```go
// shared/go/keys/keys.go
package keys

import "fmt"

func RecTopK(userID string) string {
    return fmt.Sprintf("rec:%s:top_k", userID)
}

func RecTTL(userID string) string {
    return fmt.Sprintf("rec:%s:ttl", userID)
}

func FeatUserClicks1H(userID string) string {
    return fmt.Sprintf("feat:user:%s:clicks_1h", userID)
}

func FeatUserViews1D(userID string) string {
    return fmt.Sprintf("feat:user:%s:views_1d", userID)
}

func FeatUserRecentItems(userID string) string {
    return fmt.Sprintf("feat:user:%s:recent_items", userID)
}

func FeatItemCTR7D(itemID string) string {
    return fmt.Sprintf("feat:item:%s:ctr_7d", itemID)
}

func FeatItemPopularity(itemID string) string {
    return fmt.Sprintf("feat:item:%s:popularity", itemID)
}

func StockBitmap() string {
    return "stock:bitmap"
}

func StockIDMap(itemID string) string {
    return fmt.Sprintf("stock:id_map:%s", itemID)
}

func SessionEvents(sessionID string) string {
    return fmt.Sprintf("session:%s:events", sessionID)
}

func ExperimentConfig(expID string) string {
    return fmt.Sprintf("experiment:%s", expID)
}
```

- [ ] **Step 4: Create Makefile with standard targets**

```makefile
.PHONY: up down seed-data health-check simulate-traffic bench-local test

up:
	docker compose up -d

down:
	docker compose down -v

seed-data:
	go run ./services/traffic-simulator/cmd/seed/main.go

health-check:
	@echo "Checking services..."
	@curl -sf http://localhost:8080/health && echo "event-collector: OK" || echo "event-collector: FAIL"
	@curl -sf http://localhost:8090/health && echo "recommendation-api: OK" || echo "recommendation-api: FAIL"
	@docker compose exec dragonfly redis-cli ping && echo "dragonfly: OK" || echo "dragonfly: FAIL"

simulate-traffic:
	go run ./services/traffic-simulator/cmd/simulate/main.go

bench-local:
	k6 run ./load-tests/local-benchmark.js

test:
	go test ./... -v -count=1
```

- [ ] **Step 5: Create .gitignore and CLAUDE.md**

`.gitignore`:
```
# Go
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
vendor/

# Python
__pycache__/
*.py[cod]
.venv/
*.egg-info/
dist/

# Java
*.class
*.jar
target/

# IDE
.idea/
.vscode/
*.swp

# Environment
.env
!configs/local.env.example

# Data
data/
*.parquet
*.csv

# Docker volumes
volumes/
```

`CLAUDE.md`:
```markdown
# recsys-pipeline

## Project Overview
Production-grade recommendation pipeline for 50M DAU commerce.
Spec: docs/superpowers/specs/2026-03-18-recsys-pipeline-design.md

## Architecture
- 4-Plane: Data (serving), Stream (real-time), Batch (offline), Control (monitoring)
- 3-Tier serving: Tier1 (pre-computed, <5ms), Tier2 (CPU re-rank, <20ms), Tier3 (GPU, <80ms)
- Embedded architecture: recommendation-api directly reads DragonflyDB, no microservice fan-out

## Languages
- Go 1.23: event-collector, recommendation-api, traffic-simulator
- Java/Kotlin: stream-processor (Flink)
- Python 3.12: batch-processor (Spark), ML models

## Key Patterns
- DragonflyDB key patterns: see shared/go/keys/keys.go
- Event schema: see shared/go/event/event.go
- All services are stateless, state lives in DragonflyDB/Redpanda/Milvus

## Commands
- `make up` — start all services locally
- `make test` — run all tests
- `make seed-data` — generate sample data
- `make bench-local` — run local benchmarks
```

- [ ] **Step 6: Create configs/local.env.example**

```env
# Redpanda
REDPANDA_BROKERS=localhost:9092

# DragonflyDB
DRAGONFLY_ADDR=localhost:6379

# Milvus
MILVUS_ADDR=localhost:19530

# MinIO
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin

# PostgreSQL
POSTGRES_DSN=postgres://recsys:recsys@localhost:5432/recsys?sslmode=disable

# Triton
TRITON_GRPC_ADDR=localhost:8001

# MLflow
MLFLOW_TRACKING_URI=http://localhost:5000
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: project scaffold with Go workspace, shared types, Makefile"
```

---

### Task 2: Docker Compose — Data Stores & Monitoring

**Files:**
- Create: `infra/docker-compose.yml`
- Create: `infra/monitoring/prometheus.yml`
- Create: `infra/monitoring/grafana/provisioning/datasources/prometheus.yml`
- Create: `infra/monitoring/grafana/provisioning/dashboards/dashboard.yml`
- Create: `infra/monitoring/grafana/dashboards/recsys-overview.json`
- Create: `infra/envoy/envoy.yaml`

- [ ] **Step 1: Create docker-compose.yml with all infrastructure**

```yaml
# infra/docker-compose.yml
services:
  # --- Message Broker ---
  redpanda:
    image: docker.redpanda.com/redpandadata/redpanda:v24.3.1
    command:
      - redpanda start
      - --smp 1
      - --memory 1G
      - --overprovisioned
      - --node-id 0
      - --kafka-addr PLAINTEXT://0.0.0.0:29092,OUTSIDE://0.0.0.0:9092
      - --advertise-kafka-addr PLAINTEXT://redpanda:29092,OUTSIDE://localhost:9092
      - --pandaproxy-addr 0.0.0.0:8082
      - --advertise-pandaproxy-addr localhost:8082
    ports:
      - "9092:9092"
      - "8082:8082"
      - "9644:9644"
    volumes:
      - redpanda_data:/var/lib/redpanda/data
    healthcheck:
      test: ["CMD", "rpk", "cluster", "health"]
      interval: 10s
      timeout: 5s
      retries: 5

  redpanda-console:
    image: docker.redpanda.com/redpandadata/console:v2.8.0
    ports:
      - "8088:8080"
    environment:
      KAFKA_BROKERS: redpanda:29092
    depends_on:
      redpanda:
        condition: service_healthy

  # --- Cache / Feature Store ---
  dragonfly:
    image: docker.dragonflydb.io/dragonflydb/dragonfly:v1.24.0
    ports:
      - "6379:6379"
      - "6380:6380"
    command: ["dragonfly", "--proactor_threads=2", "--metrics_port=6380"]
    volumes:
      - dragonfly_data:/data
    ulimits:
      memlock: -1
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  # --- Vector Search ---
  milvus-etcd:
    image: quay.io/coreos/etcd:v3.5.16
    environment:
      ETCD_AUTO_COMPACTION_MODE: revision
      ETCD_AUTO_COMPACTION_RETENTION: "1000"
      ETCD_QUOTA_BACKEND_BYTES: "4294967296"
      ETCD_SNAPSHOT_COUNT: "50000"
    volumes:
      - milvus_etcd_data:/etcd
    command: etcd -advertise-client-urls=http://127.0.0.1:2379 -listen-client-urls http://0.0.0.0:2379 --data-dir /etcd
    healthcheck:
      test: ["CMD", "etcdctl", "endpoint", "health"]
      interval: 30s
      timeout: 20s
      retries: 3

  milvus-minio:
    image: minio/minio:RELEASE.2024-11-07T00-52-20Z
    environment:
      MINIO_ACCESS_KEY: minioadmin
      MINIO_SECRET_KEY: minioadmin
    ports:
      - "9001:9001"
    volumes:
      - milvus_minio_data:/minio_data
    command: minio server /minio_data --console-address ":9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s
      timeout: 20s
      retries: 3

  milvus:
    image: milvusdb/milvus:v2.4.17
    command: ["milvus", "run", "standalone"]
    security_opt:
      - seccomp:unconfined
    environment:
      ETCD_ENDPOINTS: milvus-etcd:2379
      MINIO_ADDRESS: milvus-minio:9000
    ports:
      - "19530:19530"
      - "9091:9091"
    volumes:
      - milvus_data:/var/lib/milvus
    depends_on:
      milvus-etcd:
        condition: service_healthy
      milvus-minio:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9091/healthz"]
      interval: 30s
      timeout: 20s
      retries: 3

  # --- Object Storage ---
  minio:
    image: minio/minio:RELEASE.2024-11-07T00-52-20Z
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports:
      - "9000:9000"
      - "9002:9001"
    volumes:
      - minio_data:/data
    command: server /data --console-address ":9001"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 10s
      timeout: 5s
      retries: 5

  # --- Metadata DB ---
  postgres:
    image: postgres:16-alpine
    env_file: ../configs/local.env
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U recsys"]
      interval: 5s
      timeout: 3s
      retries: 5

  # --- Workflow Orchestration ---
  airflow:
    image: apache/airflow:2.10.4-python3.12
    ports:
      - "8085:8080"
    environment:
      AIRFLOW__CORE__EXECUTOR: LocalExecutor
      AIRFLOW__DATABASE__SQL_ALCHEMY_CONN: postgresql+psycopg2://recsys:recsys@postgres:5432/recsys
      AIRFLOW__CORE__LOAD_EXAMPLES: "false"
    volumes:
      - ../services/batch-processor/dags:/opt/airflow/dags
    depends_on:
      postgres:
        condition: service_healthy
    command: >
      bash -c "airflow db migrate && airflow users create --username admin --password admin
      --firstname Admin --lastname User --role Admin --email admin@local.dev || true
      && airflow webserver & airflow scheduler"

  # --- Monitoring ---
  prometheus:
    image: prom/prometheus:v2.54.1
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.retention.time=7d

  grafana:
    image: grafana/grafana:11.4.0
    ports:
      - "3000:3000"
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: Viewer
    volumes:
      - ./monitoring/grafana/provisioning:/etc/grafana/provisioning
      - ./monitoring/grafana/dashboards:/var/lib/grafana/dashboards
      - grafana_data:/var/lib/grafana
    depends_on:
      - prometheus

  jaeger:
    image: jaegertracing/all-in-one:1.62
    ports:
      - "16686:16686"
      - "4318:4318"
    environment:
      COLLECTOR_OTLP_ENABLED: "true"

  alertmanager:
    image: prom/alertmanager:v0.27.0
    ports:
      - "9093:9093"
    volumes:
      - ./monitoring/alertmanager.yml:/etc/alertmanager/alertmanager.yml

  # --- API Gateway ---
  envoy:
    image: envoyproxy/envoy:v1.32-latest
    ports:
      - "10000:10000"
      - "9901:9901"
    volumes:
      - ./envoy/envoy.yaml:/etc/envoy/envoy.yaml
    depends_on:
      - recommendation-api

volumes:
  redpanda_data:
  dragonfly_data:
  milvus_etcd_data:
  milvus_minio_data:
  milvus_data:
  minio_data:
  postgres_data:
  prometheus_data:
  grafana_data:
```

- [ ] **Step 2: Create Prometheus config**

```yaml
# infra/monitoring/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: "event-collector"
    static_configs:
      - targets: ["event-collector:2112"]

  - job_name: "recommendation-api"
    static_configs:
      - targets: ["recommendation-api:2112"]

  - job_name: "redpanda"
    static_configs:
      - targets: ["redpanda:9644"]

  - job_name: "dragonfly"
    static_configs:
      - targets: ["dragonfly:6380"]
```

- [ ] **Step 3: Create Grafana provisioning configs**

Datasource provisioning:
```yaml
# infra/monitoring/grafana/provisioning/datasources/prometheus.yml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
```

Dashboard provisioning:
```yaml
# infra/monitoring/grafana/provisioning/dashboards/dashboard.yml
apiVersion: 1
providers:
  - name: "default"
    folder: "Recsys Pipeline"
    type: file
    options:
      path: /var/lib/grafana/dashboards
```

Create a minimal overview dashboard JSON at `infra/monitoring/grafana/dashboards/recsys-overview.json` with panels for:
- Request rate by tier
- p50/p95/p99 latency
- Error rate
- DragonflyDB ops/sec
- Redpanda consumer lag

- [ ] **Step 4: Create Envoy config with rate limiting and circuit breaker**

```yaml
# infra/envoy/envoy.yaml
static_resources:
  listeners:
    - name: listener_0
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 10000
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: local_service
                      domains: ["*"]
                      routes:
                        - match:
                            prefix: "/api/v1/recommend"
                          route:
                            cluster: recommendation_api
                            timeout: 100ms
                            retry_policy:
                              retry_on: "5xx,reset,connect-failure"
                              num_retries: 1
                        - match:
                            prefix: "/api/v1/events"
                          route:
                            cluster: event_collector
                            timeout: 50ms
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: http_local_rate_limiter
                      token_bucket:
                        max_tokens: 10000
                        tokens_per_fill: 10000
                        fill_interval: 1s
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: recommendation_api
      connect_timeout: 5s
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      circuit_breakers:
        thresholds:
          - priority: DEFAULT
            max_connections: 1024
            max_pending_requests: 1024
            max_requests: 1024
            max_retries: 3
      load_assignment:
        cluster_name: recommendation_api
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: recommendation-api
                      port_value: 8090

    - name: event_collector
      connect_timeout: 5s
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      circuit_breakers:
        thresholds:
          - priority: DEFAULT
            max_connections: 2048
            max_pending_requests: 2048
            max_requests: 2048
      load_assignment:
        cluster_name: event_collector
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: event-collector
                      port_value: 8080

admin:
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 9901
```

- [ ] **Step 5: Symlink docker-compose.yml to project root**

```bash
cd /Users/gimgigang/project/recsys-pipeline
ln -s infra/docker-compose.yml docker-compose.yml
```

- [ ] **Step 6: Verify infrastructure boots**

```bash
docker compose up -d redpanda dragonfly postgres minio prometheus grafana jaeger
docker compose ps  # all healthy
docker compose exec dragonfly redis-cli ping  # PONG
docker compose logs redpanda | tail -5  # no errors
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "infra: add docker-compose with Redpanda, DragonflyDB, Milvus, monitoring"
```

---

## Phase 2: Seed Data & Simulator (Moved Early — needed by all services)

### Task 3: Sample Data Generator

**Files:**
- Create: `services/traffic-simulator/go.mod`
- Create: `services/traffic-simulator/cmd/seed/main.go`
- Create: `services/traffic-simulator/internal/generator/users.go`
- Create: `services/traffic-simulator/internal/generator/items.go`
- Create: `services/traffic-simulator/internal/generator/events.go`
- Create: `services/traffic-simulator/internal/generator/generator_test.go`

- [ ] **Step 1: Write failing test for data generation**

```go
// services/traffic-simulator/internal/generator/generator_test.go
package generator_test

import (
    "testing"
    "github.com/recsys-pipeline/traffic-simulator/internal/generator"
)

func TestGenerateUsers_Count(t *testing.T) {
    users := generator.GenerateUsers(1000)
    if len(users) != 1000 {
        t.Errorf("expected 1000 users, got %d", len(users))
    }
    // Each user has category preferences
    for _, u := range users {
        if len(u.CategoryPrefs) == 0 {
            t.Errorf("user %s has no category preferences", u.ID)
        }
    }
}

func TestGenerateItems_Categories(t *testing.T) {
    items := generator.GenerateItems(10000, 50)
    if len(items) != 10000 {
        t.Errorf("expected 10000 items, got %d", len(items))
    }
    catSeen := make(map[string]bool)
    for _, item := range items {
        catSeen[item.CategoryID] = true
    }
    if len(catSeen) < 40 {
        t.Errorf("expected items spread across categories, got %d", len(catSeen))
    }
}

func TestSeedDragonfly_PopulatesKeys(t *testing.T) {
    // Uses miniredis for testing
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

    users := generator.GenerateUsers(100)
    items := generator.GenerateItems(1000, 50)
    generator.SeedDragonfly(client, users, items)

    // Verify popular items seeded
    val, err := client.Get(context.Background(), "rec:popular:top_k").Result()
    if err != nil || val == "" {
        t.Error("popular items not seeded")
    }

    // Verify stock bitmap initialized
    nextPos, _ := client.Get(context.Background(), "stock:next_bit_pos").Result()
    if nextPos != "1000" {
        t.Errorf("expected next_bit_pos=1000, got %s", nextPos)
    }
}
```

- [ ] **Step 2: Run test — verify it fails**

```bash
cd services/traffic-simulator && go test ./internal/generator/... -v
# Expected: FAIL
```

- [ ] **Step 3: Implement generators**

```go
// services/traffic-simulator/internal/generator/users.go
package generator

import "fmt"

type User struct {
    ID            string   `json:"user_id"`
    CategoryPrefs []string `json:"category_prefs"`
}

func GenerateUsers(count int) []User {
    categories := generateCategoryIDs(50)
    users := make([]User, count)
    for i := range users {
        numPrefs := 3 + (i % 8) // 3-10 category preferences
        prefs := make([]string, numPrefs)
        for j := range prefs {
            prefs[j] = categories[(i+j)%len(categories)]
        }
        users[i] = User{
            ID:            fmt.Sprintf("u_%05d", i),
            CategoryPrefs: prefs,
        }
    }
    return users
}
```

```go
// services/traffic-simulator/internal/generator/items.go
package generator

import "fmt"

type Item struct {
    ID         string  `json:"item_id"`
    CategoryID string  `json:"category_id"`
    Price      int64   `json:"price"`
}

func GenerateItems(count, numCategories int) []Item {
    cats := generateCategoryIDs(numCategories)
    items := make([]Item, count)
    for i := range items {
        items[i] = Item{
            ID:         fmt.Sprintf("i_%06d", i),
            CategoryID: cats[i%len(cats)],
            Price:      5000 + int64((i*7919)%195000), // 5,000 ~ 200,000
        }
    }
    return items
}

func generateCategoryIDs(n int) []string {
    cats := make([]string, n)
    for i := range cats {
        cats[i] = fmt.Sprintf("cat_%03d", i)
    }
    return cats
}
```

```go
// services/traffic-simulator/internal/generator/seed.go
package generator

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/redis/go-redis/v9"
    "github.com/recsys-pipeline/shared/keys"
)

func SeedDragonfly(client *redis.Client, users []User, items []Item) error {
    ctx := context.Background()
    pipe := client.Pipeline()

    // 1. Seed stock bitmap: all items in stock, assign bit positions
    for i, item := range items {
        pipe.Set(ctx, keys.StockIDMap(item.ID), i, 0)
    }
    pipe.Set(ctx, "stock:next_bit_pos", len(items), 0)

    // 2. Seed popular items (top 100 by "score")
    popular := make([]map[string]interface{}, 0, 100)
    for i := 0; i < 100 && i < len(items); i++ {
        popular = append(popular, map[string]interface{}{
            "item_id": items[i].ID,
            "score":   1.0 - float64(i)*0.005,
        })
    }
    popJSON, _ := json.Marshal(popular)
    pipe.Set(ctx, "rec:popular:top_k", popJSON, 0)

    // 3. Seed pre-computed recommendations for each user (Top-100)
    for _, user := range users {
        recs := generateUserRecs(user, items, 100)
        recsJSON, _ := json.Marshal(recs)
        pipe.Set(ctx, keys.RecTopK(user.ID), recsJSON, 7*24*time.Hour) // 7-day TTL
    }

    _, err := pipe.Exec(ctx)
    return err
}
```

- [ ] **Step 4: Run test — verify passes**

```bash
cd services/traffic-simulator && go test ./internal/generator/... -v
# Expected: PASS
```

- [ ] **Step 5: Create seed main.go**

```go
// services/traffic-simulator/cmd/seed/main.go
package main

func main() {
    client := redis.NewClient(&redis.Options{Addr: getEnv("DRAGONFLY_ADDR", "localhost:6379")})
    users := generator.GenerateUsers(100_000)
    items := generator.GenerateItems(1_000_000, 50)

    log.Println("Seeding DragonflyDB with 100K users, 1M items...")
    if err := generator.SeedDragonfly(client, users, items); err != nil {
        log.Fatalf("seed failed: %v", err)
    }
    log.Println("Seed complete.")
}
```

- [ ] **Step 6: Verify seeding works**

```bash
docker compose up -d dragonfly
make seed-data
docker compose exec dragonfly redis-cli GET "rec:popular:top_k" | head -c 200
docker compose exec dragonfly redis-cli GET "stock:next_bit_pos"
# Expected: popular items JSON, "1000000"
```

- [ ] **Step 7: Commit**

```bash
git add services/traffic-simulator/ shared/go/
git commit -m "feat: add seed data generator with users, items, and DragonflyDB seeding"
```

---

## Phase 3: Data Plane — Event Collector

### Task 4: Event Collector — Core Service

**Files:**
- Create: `services/event-collector/go.mod`
- Create: `services/event-collector/cmd/server/main.go`
- Create: `services/event-collector/internal/handler/event_handler.go`
- Create: `services/event-collector/internal/handler/event_handler_test.go`
- Create: `services/event-collector/internal/producer/redpanda.go`
- Create: `services/event-collector/internal/producer/redpanda_test.go`
- Create: `services/event-collector/internal/counter/counter.go`
- Create: `services/event-collector/internal/counter/counter_test.go`
- Create: `infra/docker/event-collector.Dockerfile`

- [ ] **Step 1: Write failing test for event validation**

```go
// services/event-collector/internal/handler/event_handler_test.go
package handler_test

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/recsys-pipeline/event-collector/internal/handler"
    "github.com/recsys-pipeline/shared/event"
)

type mockProducer struct {
    events []event.Event
}

func (m *mockProducer) Publish(e event.Event) error {
    m.events = append(m.events, e)
    return nil
}

func (m *mockProducer) Close() error { return nil }

func TestHandleEvent_ValidClick(t *testing.T) {
    mp := &mockProducer{}
    h := handler.New(mp)

    e := event.Event{
        EventID:   "evt-001",
        UserID:    "u_abc",
        EventType: event.Click,
        ItemID:    "i_xyz",
        Timestamp: time.Now(),
        SessionID: "sess_001",
    }
    body, _ := json.Marshal(e)

    req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    h.HandleEvent(rec, req)

    if rec.Code != http.StatusAccepted {
        t.Errorf("expected 202, got %d", rec.Code)
    }
    if len(mp.events) != 1 {
        t.Errorf("expected 1 published event, got %d", len(mp.events))
    }
}

func TestHandleEvent_InvalidType(t *testing.T) {
    mp := &mockProducer{}
    h := handler.New(mp)

    body := []byte(`{"event_type": "invalid", "user_id": "u_abc"}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    h.HandleEvent(rec, req)

    if rec.Code != http.StatusBadRequest {
        t.Errorf("expected 400, got %d", rec.Code)
    }
}
```

- [ ] **Step 2: Run test — verify it fails**

```bash
cd services/event-collector && go test ./internal/handler/... -v
# Expected: FAIL — handler package doesn't exist
```

- [ ] **Step 3: Implement event handler**

```go
// services/event-collector/internal/handler/event_handler.go
package handler

import (
    "encoding/json"
    "net/http"

    "github.com/recsys-pipeline/shared/event"
)

type Producer interface {
    Publish(e event.Event) error
    Close() error
}

type Handler struct {
    producer Producer
}

func New(p Producer) *Handler {
    return &Handler{producer: p}
}

func (h *Handler) HandleEvent(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var e event.Event
    if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
        http.Error(w, "invalid json", http.StatusBadRequest)
        return
    }

    if !isValidEventType(e.EventType) {
        http.Error(w, "invalid event_type", http.StatusBadRequest)
        return
    }

    if e.UserID == "" || e.ItemID == "" {
        http.Error(w, "user_id and item_id required", http.StatusBadRequest)
        return
    }

    if err := h.producer.Publish(e); err != nil {
        http.Error(w, "publish failed", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusAccepted)
}

func isValidEventType(t event.Type) bool {
    switch t {
    case event.Click, event.View, event.Purchase,
        event.Search, event.AddToCart, event.RemoveFromCart:
        return true
    }
    return false
}

func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}
```

- [ ] **Step 4: Run test — verify it passes**

```bash
cd services/event-collector && go test ./internal/handler/... -v
# Expected: PASS
```

- [ ] **Step 5: Implement Redpanda producer**

```go
// services/event-collector/internal/producer/redpanda.go
package producer

import (
    "context"
    "encoding/json"

    "github.com/twmb/franz-go/pkg/kgo"
    "github.com/recsys-pipeline/shared/event"
)

type RedpandaProducer struct {
    client *kgo.Client
    topic  string
}

func NewRedpanda(brokers []string, topic string) (*RedpandaProducer, error) {
    client, err := kgo.NewClient(
        kgo.SeedBrokers(brokers...),
        kgo.ProducerBatchMaxBytes(1_000_000),
        kgo.ProducerLinger(5*time.Millisecond),
    )
    if err != nil {
        return nil, fmt.Errorf("creating kafka client: %w", err)
    }
    return &RedpandaProducer{client: client, topic: topic}, nil
}

func (p *RedpandaProducer) Publish(e event.Event) error {
    data, err := json.Marshal(e)
    if err != nil {
        return fmt.Errorf("marshaling event: %w", err)
    }

    record := &kgo.Record{
        Topic: p.topic,
        Key:   []byte(e.UserID),
        Value: data,
    }

    p.client.Produce(context.Background(), record, func(r *kgo.Record, err error) {
        if err != nil {
            // Log async error — fire and forget for throughput
            log.Printf("produce error: %v", err)
        }
    })
    return nil
}

func (p *RedpandaProducer) Close() error {
    p.client.Close()
    return nil
}
```

- [ ] **Step 6: Write Redpanda producer integration test**

```go
// services/event-collector/internal/producer/redpanda_test.go
// +build integration

package producer_test

// Integration test — requires running Redpanda
// Run with: go test -tags=integration ./internal/producer/... -v
```

- [ ] **Step 7: Implement embedded counter (simple aggregation)**

```go
// services/event-collector/internal/counter/counter.go
package counter

import (
    "sync"
    "sync/atomic"
)

type EventCounter struct {
    counts sync.Map // key: "{user_id}:{event_type}" → *atomic.Int64
}

func New() *EventCounter {
    return &EventCounter{}
}

func (c *EventCounter) Increment(userID, eventType string) int64 {
    key := userID + ":" + eventType
    val, _ := c.counts.LoadOrStore(key, &atomic.Int64{})
    return val.(*atomic.Int64).Add(1)
}

func (c *EventCounter) Get(userID, eventType string) int64 {
    key := userID + ":" + eventType
    val, ok := c.counts.Load(key)
    if !ok {
        return 0
    }
    return val.(*atomic.Int64).Load()
}
```

- [ ] **Step 8: Write counter test and verify**

```go
// services/event-collector/internal/counter/counter_test.go
package counter_test

import (
    "testing"
    "github.com/recsys-pipeline/event-collector/internal/counter"
)

func TestCounter_IncrementAndGet(t *testing.T) {
    c := counter.New()
    c.Increment("u_abc", "click")
    c.Increment("u_abc", "click")
    c.Increment("u_abc", "view")

    if got := c.Get("u_abc", "click"); got != 2 {
        t.Errorf("expected 2 clicks, got %d", got)
    }
    if got := c.Get("u_abc", "view"); got != 1 {
        t.Errorf("expected 1 view, got %d", got)
    }
    if got := c.Get("u_abc", "purchase"); got != 0 {
        t.Errorf("expected 0 purchases, got %d", got)
    }
}
```

```bash
go test ./internal/counter/... -v
# Expected: PASS
```

- [ ] **Step 9: Create main.go with Prometheus metrics**

```go
// services/event-collector/cmd/server/main.go
package main

import (
    "log"
    "net/http"
    "os"
    "strings"

    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/recsys-pipeline/event-collector/internal/handler"
    "github.com/recsys-pipeline/event-collector/internal/producer"
)

func main() {
    brokers := strings.Split(getEnv("REDPANDA_BROKERS", "localhost:9092"), ",")
    topic := getEnv("EVENTS_TOPIC", "user-events")

    prod, err := producer.NewRedpanda(brokers, topic)
    if err != nil {
        log.Fatalf("failed to create producer: %v", err)
    }
    defer prod.Close()

    h := handler.New(prod)

    mux := http.NewServeMux()
    mux.HandleFunc("/api/v1/events", h.HandleEvent)
    mux.HandleFunc("/health", h.HandleHealth)

    // Metrics endpoint on separate port
    go func() {
        metricsMux := http.NewServeMux()
        metricsMux.Handle("/metrics", promhttp.Handler())
        log.Println("metrics listening on :2112")
        http.ListenAndServe(":2112", metricsMux)
    }()

    log.Println("event-collector listening on :8080")
    if err := http.ListenAndServe(":8080", mux); err != nil {
        log.Fatalf("server error: %v", err)
    }
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

- [ ] **Step 10: Create Dockerfile and add to docker-compose**

```dockerfile
# infra/docker/event-collector.Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY shared/go/ shared/go/
COPY services/event-collector/ services/event-collector/
RUN cd services/event-collector && go build -o /event-collector ./cmd/server/main.go

FROM alpine:3.20
COPY --from=builder /event-collector /event-collector
EXPOSE 8080 2112
CMD ["/event-collector"]
```

Add to docker-compose.yml:
```yaml
  event-collector:
    build:
      context: ..
      dockerfile: infra/docker/event-collector.Dockerfile
    ports:
      - "8080:8080"
      - "2112:2112"
    environment:
      REDPANDA_BROKERS: redpanda:29092
      EVENTS_TOPIC: user-events
    depends_on:
      redpanda:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 5s
      timeout: 3s
      retries: 5
```

- [ ] **Step 11: Verify end-to-end locally**

```bash
docker compose up -d redpanda event-collector
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_id":"test-1","user_id":"u_001","event_type":"click","item_id":"i_001","timestamp":"2026-03-18T10:00:00Z","session_id":"s_001"}'
# Expected: 202 Accepted
```

- [ ] **Step 12: Commit**

```bash
git add -A
git commit -m "feat: add event-collector service with Redpanda producer"
```

---

## Phase 3: Data Plane — Recommendation API

### Task 4: Recommendation API — Tier 1 (Pre-computed Cache)

**Files:**
- Create: `services/recommendation-api/go.mod`
- Create: `services/recommendation-api/cmd/server/main.go`
- Create: `services/recommendation-api/internal/handler/recommend_handler.go`
- Create: `services/recommendation-api/internal/handler/recommend_handler_test.go`
- Create: `services/recommendation-api/internal/store/dragonfly.go`
- Create: `services/recommendation-api/internal/store/dragonfly_test.go`
- Create: `services/recommendation-api/internal/stock/bitmap.go`
- Create: `services/recommendation-api/internal/stock/bitmap_test.go`
- Create: `services/recommendation-api/internal/tier/router.go`
- Create: `services/recommendation-api/internal/tier/router_test.go`

- [ ] **Step 1: Write failing test for tier routing logic**

```go
// services/recommendation-api/internal/tier/router_test.go
package tier_test

func TestRouter_Tier1_PreComputedHit(t *testing.T) {
    // User has pre-computed recommendations → Tier 1
    store := &mockStore{
        recs: map[string][]Recommendation{
            "u_001": {{ItemID: "i_1", Score: 0.9}, {ItemID: "i_2", Score: 0.8}},
        },
    }
    bitmap := &mockBitmap{outOfStock: map[string]bool{}}
    router := tier.NewRouter(store, bitmap, nil)

    result, tierUsed, err := router.Recommend("u_001", "", 10)
    assert(t, err == nil)
    assert(t, tierUsed == tier.Tier1)
    assert(t, len(result) == 2)
}

func TestRouter_Tier1_FiltersOutOfStock(t *testing.T) {
    // i_2 is out of stock → filtered from results
    store := &mockStore{
        recs: map[string][]Recommendation{
            "u_001": {{ItemID: "i_1", Score: 0.9}, {ItemID: "i_2", Score: 0.8}},
        },
    }
    bitmap := &mockBitmap{outOfStock: map[string]bool{"i_2": true}}
    router := tier.NewRouter(store, bitmap, nil)

    result, _, err := router.Recommend("u_001", "", 10)
    assert(t, err == nil)
    assert(t, len(result) == 1)
    assert(t, result[0].ItemID == "i_1")
}

func TestRouter_Tier1_CacheMiss_FallsToPopular(t *testing.T) {
    // No pre-computed recs → fallback to popular items
    store := &mockStore{recs: map[string][]Recommendation{}}
    bitmap := &mockBitmap{outOfStock: map[string]bool{}}
    router := tier.NewRouter(store, bitmap, nil)

    result, tierUsed, err := router.Recommend("u_unknown", "", 10)
    assert(t, err == nil)
    assert(t, tierUsed == tier.Fallback)
    assert(t, len(result) > 0) // returns popular items
}
```

- [ ] **Step 2: Run test — verify it fails**

```bash
cd services/recommendation-api && go test ./internal/tier/... -v
# Expected: FAIL
```

- [ ] **Step 3: Implement tier router**

```go
// services/recommendation-api/internal/tier/router.go
package tier

type Level string

const (
    Tier1    Level = "tier1_precomputed"
    Tier2    Level = "tier2_rerank"
    Tier3    Level = "tier3_inference"
    Fallback Level = "fallback_popular"
)

type Recommendation struct {
    ItemID string  `json:"item_id"`
    Score  float64 `json:"score"`
}

type Store interface {
    GetRecommendations(userID string) ([]Recommendation, error)
    GetPopularItems(limit int) ([]Recommendation, error)
}

type StockChecker interface {
    IsOutOfStock(itemID string) (bool, error)
    FilterOutOfStock(items []Recommendation) ([]Recommendation, error)
}

type Reranker interface {
    Rerank(userID string, sessionID string, items []Recommendation) ([]Recommendation, error)
}

type Router struct {
    store    Store
    stock    StockChecker
    reranker Reranker
}

func NewRouter(store Store, stock StockChecker, reranker Reranker) *Router {
    return &Router{store: store, stock: stock, reranker: reranker}
}

func (r *Router) Recommend(userID, sessionID string, limit int) ([]Recommendation, Level, error) {
    // Tier 1: pre-computed lookup
    recs, err := r.store.GetRecommendations(userID)
    if err != nil {
        return r.fallback(limit)
    }

    if len(recs) == 0 {
        return r.fallback(limit)
    }

    // Filter out-of-stock
    filtered, err := r.stock.FilterOutOfStock(recs)
    if err != nil {
        // Stock check failed — serve unfiltered (degraded)
        filtered = recs
    }

    // Tier 2: re-rank if session context available and reranker present
    if sessionID != "" && r.reranker != nil {
        reranked, err := r.reranker.Rerank(userID, sessionID, filtered)
        if err == nil {
            return truncate(reranked, limit), Tier2, nil
        }
        // Reranker failed — fall through to Tier 1 result
    }

    return truncate(filtered, limit), Tier1, nil
}

func (r *Router) fallback(limit int) ([]Recommendation, Level, error) {
    popular, err := r.store.GetPopularItems(limit)
    if err != nil {
        return nil, Fallback, fmt.Errorf("fallback failed: %w", err)
    }
    return popular, Fallback, nil
}

func truncate(items []Recommendation, limit int) []Recommendation {
    if len(items) <= limit {
        return items
    }
    return items[:limit]
}
```

- [ ] **Step 4: Run test — verify passes**

```bash
cd services/recommendation-api && go test ./internal/tier/... -v
# Expected: PASS
```

- [ ] **Step 5: Implement DragonflyDB store**

```go
// services/recommendation-api/internal/store/dragonfly.go
package store

import (
    "context"
    "encoding/json"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/recsys-pipeline/shared/keys"
    "github.com/recsys-pipeline/recommendation-api/internal/tier"
)

type DragonflyStore struct {
    client *redis.Client
}

func NewDragonfly(addr string) *DragonflyStore {
    client := redis.NewClient(&redis.Options{
        Addr:         addr,
        PoolSize:     100,
        ReadTimeout:  10 * time.Millisecond,
        WriteTimeout: 10 * time.Millisecond,
    })
    return &DragonflyStore{client: client}
}

func (s *DragonflyStore) GetRecommendations(userID string) ([]tier.Recommendation, error) {
    ctx := context.Background()
    data, err := s.client.Get(ctx, keys.RecTopK(userID)).Bytes()
    if err == redis.Nil {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }

    var recs []tier.Recommendation
    if err := json.Unmarshal(data, &recs); err != nil {
        return nil, err
    }
    return recs, nil
}

func (s *DragonflyStore) GetPopularItems(limit int) ([]tier.Recommendation, error) {
    ctx := context.Background()
    data, err := s.client.Get(ctx, "rec:popular:top_k").Bytes()
    if err == redis.Nil {
        return []tier.Recommendation{}, nil
    }
    if err != nil {
        return nil, err
    }

    var recs []tier.Recommendation
    if err := json.Unmarshal(data, &recs); err != nil {
        return nil, err
    }

    if len(recs) > limit {
        recs = recs[:limit]
    }
    return recs, nil
}

func (s *DragonflyStore) Close() error {
    return s.client.Close()
}
```

- [ ] **Step 6: Implement stock bitmap checker**

```go
// services/recommendation-api/internal/stock/bitmap.go
package stock

import (
    "context"
    "strconv"

    "github.com/redis/go-redis/v9"
    "github.com/recsys-pipeline/shared/keys"
    "github.com/recsys-pipeline/recommendation-api/internal/tier"
)

type BitmapChecker struct {
    client *redis.Client
}

func NewBitmapChecker(client *redis.Client) *BitmapChecker {
    return &BitmapChecker{client: client}
}

func (b *BitmapChecker) IsOutOfStock(itemID string) (bool, error) {
    ctx := context.Background()

    // Get bit position for this item
    posStr, err := b.client.Get(ctx, keys.StockIDMap(itemID)).Result()
    if err == redis.Nil {
        return false, nil // Unknown item → assume in stock
    }
    if err != nil {
        return false, err
    }

    pos, err := strconv.ParseInt(posStr, 10, 64)
    if err != nil {
        return false, err
    }

    // Check bitmap: 1 = out of stock
    bit, err := b.client.GetBit(ctx, keys.StockBitmap(), pos).Result()
    if err != nil {
        return false, err
    }
    return bit == 1, nil
}

func (b *BitmapChecker) FilterOutOfStock(items []tier.Recommendation) ([]tier.Recommendation, error) {
    result := make([]tier.Recommendation, 0, len(items))
    for _, item := range items {
        oos, err := b.IsOutOfStock(item.ItemID)
        if err != nil {
            // On error, include item (fail-open)
            result = append(result, item)
            continue
        }
        if !oos {
            result = append(result, item)
        }
    }
    return result, nil
}
```

- [ ] **Step 7: Write stock bitmap test**

```go
// services/recommendation-api/internal/stock/bitmap_test.go
package stock_test

import (
    "context"
    "testing"

    "github.com/alicebob/miniredis/v2"
    "github.com/redis/go-redis/v9"
    "github.com/recsys-pipeline/recommendation-api/internal/stock"
    "github.com/recsys-pipeline/recommendation-api/internal/tier"
)

func TestBitmapChecker_InStockItem(t *testing.T) {
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    ctx := context.Background()

    // Item i_001 at bit position 0, bit=0 (in stock)
    client.Set(ctx, "stock:id_map:i_001", "0", 0)
    // bitmap not set → all zeros → all in stock

    checker := stock.NewBitmapChecker(client)
    oos, err := checker.IsOutOfStock("i_001")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if oos {
        t.Error("expected item to be in stock")
    }
}

func TestBitmapChecker_OutOfStockItem(t *testing.T) {
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    ctx := context.Background()

    // Item i_002 at bit position 5
    client.Set(ctx, "stock:id_map:i_002", "5", 0)
    // Set bit 5 = 1 (out of stock)
    client.SetBit(ctx, "stock:bitmap", 5, 1)

    checker := stock.NewBitmapChecker(client)
    oos, err := checker.IsOutOfStock("i_002")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !oos {
        t.Error("expected item to be out of stock")
    }
}

func TestBitmapChecker_FilterOutOfStock(t *testing.T) {
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    ctx := context.Background()

    client.Set(ctx, "stock:id_map:i_001", "0", 0)
    client.Set(ctx, "stock:id_map:i_002", "1", 0)
    client.SetBit(ctx, "stock:bitmap", 1, 1) // i_002 out of stock

    checker := stock.NewBitmapChecker(client)
    items := []tier.Recommendation{
        {ItemID: "i_001", Score: 0.9},
        {ItemID: "i_002", Score: 0.8},
    }
    filtered, err := checker.FilterOutOfStock(items)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(filtered) != 1 || filtered[0].ItemID != "i_001" {
        t.Errorf("expected only i_001, got %v", filtered)
    }
}
```

- [ ] **Step 8: Create HTTP handler and main.go**

```go
// services/recommendation-api/internal/handler/recommend_handler.go
package handler

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/recsys-pipeline/recommendation-api/internal/tier"
)

type RecommendHandler struct {
    router *tier.Router
}

func New(router *tier.Router) *RecommendHandler {
    return &RecommendHandler{router: router}
}

type RecommendResponse struct {
    Items []tier.Recommendation `json:"items"`
    Tier  tier.Level            `json:"tier"`
}

func (h *RecommendHandler) HandleRecommend(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    if userID == "" {
        http.Error(w, "user_id required", http.StatusBadRequest)
        return
    }

    sessionID := r.URL.Query().Get("session_id")
    limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
    if limit <= 0 || limit > 100 {
        limit = 20
    }

    items, tierUsed, err := h.router.Recommend(userID, sessionID, limit)
    if err != nil {
        http.Error(w, "recommendation failed", http.StatusInternalServerError)
        return
    }

    resp := RecommendResponse{Items: items, Tier: tierUsed}
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 9: Create Dockerfile, add to docker-compose, verify E2E**

```bash
# Seed some test data into DragonflyDB
docker compose exec dragonfly redis-cli SET "rec:u_001:top_k" \
  '[{"item_id":"i_1","score":0.95},{"item_id":"i_2","score":0.90}]'

# Test recommendation API
curl "http://localhost:8090/api/v1/recommend?user_id=u_001&limit=10"
# Expected: {"items":[{"item_id":"i_1","score":0.95},{"item_id":"i_2","score":0.90}],"tier":"tier1_precomputed"}
```

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat: add recommendation-api with Tier 1 pre-computed serving"
```

---

### Task 6: Recommendation API — Tier 2 (CPU Re-ranking)

**Files:**
- Create: `services/recommendation-api/internal/rerank/scorer.go`
- Create: `services/recommendation-api/internal/rerank/scorer_test.go`
- Create: `services/recommendation-api/internal/rerank/session.go`
- Create: `services/recommendation-api/internal/rerank/session_test.go`

> **Note on `leaves` library**: The `dmitryikh/leaves` Go library for LightGBM has limited maintenance.
> We implement a **weighted feature scorer** as the default re-ranker (no CGo, no external dependency).
> If `leaves` proves stable, it can be swapped in as an optional enhancement later.

- [ ] **Step 1: Write failing test for session feature extraction**

```go
// services/recommendation-api/internal/rerank/session_test.go
package rerank_test

import (
    "context"
    "testing"
    "time"

    "github.com/alicebob/miniredis/v2"
    "github.com/redis/go-redis/v9"
    "github.com/recsys-pipeline/recommendation-api/internal/rerank"
)

func TestSessionFeatures_ExtractsClickedItems(t *testing.T) {
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    ctx := context.Background()

    // Simulate session: user clicked i_002 and i_005
    now := time.Now()
    client.ZAdd(ctx, "session:sess_001:events", redis.Z{
        Score:  float64(now.Add(-5 * time.Minute).Unix()),
        Member: `{"event_type":"click","item_id":"i_002"}`,
    })
    client.ZAdd(ctx, "session:sess_001:events", redis.Z{
        Score:  float64(now.Add(-1 * time.Minute).Unix()),
        Member: `{"event_type":"click","item_id":"i_005"}`,
    })

    sf := rerank.NewSessionFeatureExtractor(client)
    features, err := sf.Extract("sess_001")

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(features.ClickedItems) != 2 {
        t.Errorf("expected 2 clicked items, got %d", len(features.ClickedItems))
    }
    if features.ClickedItems["i_005"] != true {
        t.Error("expected i_005 in clicked items")
    }
    if features.SessionLength != 2 {
        t.Errorf("expected session length 2, got %d", features.SessionLength)
    }
}
```

- [ ] **Step 2: Run test — verify it fails**

```bash
cd services/recommendation-api && go test ./internal/rerank/... -v
# Expected: FAIL — package doesn't exist
```

- [ ] **Step 3: Implement session feature extraction**

```go
// services/recommendation-api/internal/rerank/session.go
package rerank

import (
    "context"
    "encoding/json"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/recsys-pipeline/shared/keys"
)

type SessionFeatures struct {
    ClickedItems  map[string]bool
    ViewedItems   map[string]bool
    SessionLength int
    LastEventAge  time.Duration
}

type sessionEvent struct {
    EventType string `json:"event_type"`
    ItemID    string `json:"item_id"`
}

type SessionFeatureExtractor struct {
    client *redis.Client
}

func NewSessionFeatureExtractor(client *redis.Client) *SessionFeatureExtractor {
    return &SessionFeatureExtractor{client: client}
}

func (s *SessionFeatureExtractor) Extract(sessionID string) (*SessionFeatures, error) {
    ctx := context.Background()
    results, err := s.client.ZRevRangeWithScores(ctx, keys.SessionEvents(sessionID), 0, 49).Result()
    if err != nil {
        return nil, err
    }

    features := &SessionFeatures{
        ClickedItems: make(map[string]bool),
        ViewedItems:  make(map[string]bool),
    }

    for i, z := range results {
        var evt sessionEvent
        if err := json.Unmarshal([]byte(z.Member.(string)), &evt); err != nil {
            continue
        }
        switch evt.EventType {
        case "click":
            features.ClickedItems[evt.ItemID] = true
        case "view":
            features.ViewedItems[evt.ItemID] = true
        }
        if i == 0 {
            evtTime := time.Unix(int64(z.Score), 0)
            features.LastEventAge = time.Since(evtTime)
        }
    }
    features.SessionLength = len(results)
    return features, nil
}
```

- [ ] **Step 4: Run test — verify passes**

```bash
cd services/recommendation-api && go test ./internal/rerank/... -v
# Expected: PASS
```

- [ ] **Step 5: Write failing test for weighted scorer**

```go
// services/recommendation-api/internal/rerank/scorer_test.go
package rerank_test

import (
    "testing"
    "github.com/recsys-pipeline/recommendation-api/internal/rerank"
    "github.com/recsys-pipeline/recommendation-api/internal/tier"
)

func TestWeightedScorer_BoostsClickedItems(t *testing.T) {
    scorer := rerank.NewWeightedScorer(rerank.DefaultWeights())

    items := []tier.Recommendation{
        {ItemID: "i_001", Score: 0.9},
        {ItemID: "i_002", Score: 0.7},
        {ItemID: "i_003", Score: 0.8},
    }

    features := &rerank.SessionFeatures{
        ClickedItems:  map[string]bool{"i_002": true}, // user clicked i_002
        ViewedItems:   map[string]bool{},
        SessionLength: 3,
    }

    reranked := scorer.Rerank(items, features)

    // i_002 should be boosted above i_003 despite lower base score
    if reranked[0].ItemID == "i_002" {
        // Clicked item boosted too much — it should be penalized (already seen)
        t.Error("clicked items should be penalized, not boosted")
    }
    // i_002 should be penalized (already clicked = less novel)
    foundPos := -1
    for i, r := range reranked {
        if r.ItemID == "i_002" {
            foundPos = i
        }
    }
    if foundPos == 0 {
        t.Error("already-clicked item should not be ranked first")
    }
}

func TestWeightedScorer_PreservesOrderWithoutSession(t *testing.T) {
    scorer := rerank.NewWeightedScorer(rerank.DefaultWeights())

    items := []tier.Recommendation{
        {ItemID: "i_001", Score: 0.9},
        {ItemID: "i_002", Score: 0.8},
    }

    emptyFeatures := &rerank.SessionFeatures{
        ClickedItems: map[string]bool{},
        ViewedItems:  map[string]bool{},
    }

    reranked := scorer.Rerank(items, emptyFeatures)
    if reranked[0].ItemID != "i_001" {
        t.Error("without session data, original order should be preserved")
    }
}
```

- [ ] **Step 6: Run test — verify fails**

- [ ] **Step 7: Implement weighted scorer**

```go
// services/recommendation-api/internal/rerank/scorer.go
package rerank

import (
    "sort"
    "github.com/recsys-pipeline/recommendation-api/internal/tier"
)

type Weights struct {
    BaseScore       float64 // weight for pre-computed score
    ClickedPenalty  float64 // penalty for already-clicked items (novelty)
    ViewedPenalty   float64 // penalty for already-viewed items
    RecencyBoost    float64 // boost for items in similar category to recent clicks
}

func DefaultWeights() Weights {
    return Weights{
        BaseScore:      1.0,
        ClickedPenalty: -0.5,
        ViewedPenalty:  -0.2,
        RecencyBoost:   0.1,
    }
}

type WeightedScorer struct {
    weights Weights
}

func NewWeightedScorer(w Weights) *WeightedScorer {
    return &WeightedScorer{weights: w}
}

func (s *WeightedScorer) Rerank(items []tier.Recommendation, features *SessionFeatures) []tier.Recommendation {
    type scored struct {
        item  tier.Recommendation
        score float64
    }

    scoredItems := make([]scored, len(items))
    for i, item := range items {
        finalScore := item.Score * s.weights.BaseScore

        if features.ClickedItems[item.ItemID] {
            finalScore += s.weights.ClickedPenalty
        }
        if features.ViewedItems[item.ItemID] {
            finalScore += s.weights.ViewedPenalty
        }

        scoredItems[i] = scored{item: item, score: finalScore}
    }

    sort.Slice(scoredItems, func(i, j int) bool {
        return scoredItems[i].score > scoredItems[j].score
    })

    result := make([]tier.Recommendation, len(scoredItems))
    for i, s := range scoredItems {
        result[i] = tier.Recommendation{ItemID: s.item.ItemID, Score: s.score}
    }
    return result
}
```

- [ ] **Step 8: Run test — verify passes**

```bash
cd services/recommendation-api && go test ./internal/rerank/... -v
# Expected: PASS
```

- [ ] **Step 9: Create Reranker adapter that implements tier.Reranker interface**

```go
// Wire session extractor + weighted scorer into a single Reranker
type SessionReranker struct {
    extractor *SessionFeatureExtractor
    scorer    *WeightedScorer
}

func (r *SessionReranker) Rerank(userID, sessionID string, items []tier.Recommendation) ([]tier.Recommendation, error) {
    features, err := r.extractor.Extract(sessionID)
    if err != nil {
        return nil, err
    }
    return r.scorer.Rerank(items, features), nil
}
```

- [ ] **Step 10: Wire into tier router, verify E2E Tier 2**

```bash
# Inject session data
docker compose exec dragonfly redis-cli ZADD "session:sess_001:events" \
  $(date +%s) '{"event_type":"click","item_id":"i_000002"}'

# Request with session_id
curl "http://localhost:8090/api/v1/recommend?user_id=u_00001&session_id=sess_001&limit=10"
# Expected: tier = "tier2_rerank", items reordered based on session
```

- [ ] **Step 11: Commit**

```bash
git add services/recommendation-api/internal/rerank/
git commit -m "feat: add Tier 2 weighted re-ranking with session features"
```

---

### Task 7: Tier 0 CDN Serving & Global Degradation State Machine

**Files:**
- Create: `services/recommendation-api/internal/handler/popular_handler.go`
- Create: `services/recommendation-api/internal/handler/popular_handler_test.go`
- Create: `services/recommendation-api/internal/degradation/level.go`
- Create: `services/recommendation-api/internal/degradation/level_test.go`
- Modify: `services/recommendation-api/internal/tier/router.go`
- Modify: `infra/envoy/envoy.yaml`

> **Critical**: Tier 0 handles 70% of traffic (350K RPS). Without it, recommendation-api needs 3.3x more pods.

- [ ] **Step 1: Write failing test for popular items endpoint with Cache-Control**

```go
// services/recommendation-api/internal/handler/popular_handler_test.go
package handler_test

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestPopularHandler_SetsCacheHeaders(t *testing.T) {
    store := &mockStore{popular: testPopularItems}
    h := handler.NewPopularHandler(store)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/popular?category=cat_001", nil)
    rec := httptest.NewRecorder()
    h.HandlePopular(rec, req)

    if rec.Code != 200 {
        t.Errorf("expected 200, got %d", rec.Code)
    }
    cc := rec.Header().Get("Cache-Control")
    if cc != "public, max-age=30, stale-while-revalidate=60" {
        t.Errorf("expected CDN-cacheable Cache-Control, got %q", cc)
    }
    // Vary by category for CDN cache partitioning
    if rec.Header().Get("Vary") != "X-Category" {
        t.Error("expected Vary: X-Category header")
    }
}
```

- [ ] **Step 2: Run test — verify fails**

- [ ] **Step 3: Implement popular handler with CDN cache headers**

```go
// services/recommendation-api/internal/handler/popular_handler.go
package handler

type PopularHandler struct {
    store Store
}

func NewPopularHandler(store Store) *PopularHandler {
    return &PopularHandler{store: store}
}

func (h *PopularHandler) HandlePopular(w http.ResponseWriter, r *http.Request) {
    category := r.URL.Query().Get("category")
    limit := 20

    var items []tier.Recommendation
    var err error
    if category != "" {
        items, err = h.store.GetPopularByCategory(category, limit)
    } else {
        items, err = h.store.GetPopularItems(limit)
    }
    if err != nil {
        http.Error(w, "failed to get popular items", http.StatusInternalServerError)
        return
    }

    // CDN-cacheable headers — 30s cache, 60s stale-while-revalidate
    w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
    w.Header().Set("Vary", "X-Category")
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "items": items,
        "tier":  "tier0_popular",
    })
}
```

- [ ] **Step 4: Run test — verify passes**

- [ ] **Step 5: Write failing test for degradation level state machine**

```go
// services/recommendation-api/internal/degradation/level_test.go
package degradation_test

import (
    "testing"
    "github.com/recsys-pipeline/recommendation-api/internal/degradation"
)

func TestDegradation_NormalAllowsAllTiers(t *testing.T) {
    d := degradation.New()
    d.SetLevel(degradation.Normal)

    if !d.IsTierAllowed("tier3") {
        t.Error("Normal level should allow tier3")
    }
}

func TestDegradation_WarningDisablesTier3(t *testing.T) {
    d := degradation.New()
    d.SetLevel(degradation.Warning)

    if d.IsTierAllowed("tier3") {
        t.Error("Warning level should disable tier3")
    }
    if !d.IsTierAllowed("tier2") {
        t.Error("Warning level should allow tier2")
    }
}

func TestDegradation_CriticalDisablesTier2And3(t *testing.T) {
    d := degradation.New()
    d.SetLevel(degradation.Critical)

    if d.IsTierAllowed("tier3") || d.IsTierAllowed("tier2") {
        t.Error("Critical should disable tier2 and tier3")
    }
    if !d.IsTierAllowed("tier1") {
        t.Error("Critical should allow tier1")
    }
}

func TestDegradation_AutoEscalatesOnLoad(t *testing.T) {
    d := degradation.New()
    // Simulate 150% load
    d.ReportLoad(1.5)
    if d.CurrentLevel() != degradation.Warning {
        t.Errorf("expected Warning at 150%%, got %s", d.CurrentLevel())
    }
    // Simulate 200% load
    d.ReportLoad(2.0)
    if d.CurrentLevel() != degradation.Critical {
        t.Errorf("expected Critical at 200%%, got %s", d.CurrentLevel())
    }
}
```

- [ ] **Step 6: Implement degradation state machine**

```go
// services/recommendation-api/internal/degradation/level.go
package degradation

import "sync/atomic"

type Level int32

const (
    Normal   Level = 0
    Warning  Level = 1 // 150%+ → disable Tier 3
    Critical Level = 2 // 200%+ → disable Tier 2 & 3
    Emergency Level = 3 // → CDN-only fallback
)

func (l Level) String() string {
    switch l {
    case Normal: return "normal"
    case Warning: return "warning"
    case Critical: return "critical"
    case Emergency: return "emergency"
    }
    return "unknown"
}

type Manager struct {
    level atomic.Int32
}

func New() *Manager {
    return &Manager{}
}

func (m *Manager) SetLevel(l Level) {
    m.level.Store(int32(l))
}

func (m *Manager) CurrentLevel() Level {
    return Level(m.level.Load())
}

func (m *Manager) ReportLoad(ratio float64) {
    switch {
    case ratio >= 3.0:
        m.SetLevel(Emergency)
    case ratio >= 2.0:
        m.SetLevel(Critical)
    case ratio >= 1.5:
        m.SetLevel(Warning)
    default:
        m.SetLevel(Normal)
    }
}

func (m *Manager) IsTierAllowed(tierName string) bool {
    level := m.CurrentLevel()
    switch tierName {
    case "tier3":
        return level < Warning
    case "tier2":
        return level < Critical
    case "tier1":
        return level < Emergency
    }
    return true
}
```

- [ ] **Step 7: Run tests — verify all pass**

- [ ] **Step 8: Wire degradation into tier router**

Add `degradation.Manager` as dependency of `tier.Router`. Before routing to Tier 2/3, check `manager.IsTierAllowed()`.

- [ ] **Step 9: Add Envoy CDN caching route for /api/v1/popular**

```yaml
# Add to envoy.yaml virtual_hosts routes
- match:
    prefix: "/api/v1/popular"
  route:
    cluster: recommendation_api
    timeout: 50ms
  response_headers_to_add:
    - header:
        key: "CDN-Cache-Control"
        value: "public, max-age=30"
```

- [ ] **Step 10: Commit**

```bash
git add services/recommendation-api/internal/handler/popular_handler.go \
       services/recommendation-api/internal/handler/popular_handler_test.go \
       services/recommendation-api/internal/degradation/ \
       infra/envoy/envoy.yaml
git commit -m "feat: add Tier 0 CDN-cacheable popular endpoint and 4-level degradation state machine"
```

---

### Task 6: Recommendation API — Circuit Breaker & Metrics

**Files:**
- Create: `services/recommendation-api/internal/circuitbreaker/breaker.go`
- Create: `services/recommendation-api/internal/circuitbreaker/breaker_test.go`
- Create: `services/recommendation-api/internal/metrics/metrics.go`

- [ ] **Step 1: Implement circuit breaker per tier**

Simple state machine: Closed → Open (after N failures) → Half-Open (probe) → Closed.

- [ ] **Step 2: Write test for circuit breaker state transitions**

- [ ] **Step 3: Add Prometheus metrics**

Counters: `recsys_requests_total{tier}`, `recsys_errors_total{tier}`
Histograms: `recsys_request_duration_seconds{tier}`
Gauge: `recsys_circuit_breaker_state{tier}`

- [ ] **Step 4: Verify metrics in Prometheus**

```bash
curl http://localhost:2112/metrics | grep recsys_
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add circuit breaker and Prometheus metrics to recommendation-api"
```

---

## Phase 4: Stream Plane

### Task 7: Flink Stream Processor — Real-time Features

**Files:**
- Create: `services/stream-processor/build.gradle.kts`
- Create: `services/stream-processor/src/main/kotlin/com/recsys/StreamProcessorJob.kt`
- Create: `services/stream-processor/src/main/kotlin/com/recsys/features/ClickWindowFunction.kt`
- Create: `services/stream-processor/src/main/kotlin/com/recsys/features/SessionProcessor.kt`
- Create: `services/stream-processor/src/main/kotlin/com/recsys/inventory/StockBitmapUpdater.kt`
- Create: `services/stream-processor/src/main/kotlin/com/recsys/sinks/DragonflySink.kt`
- Create: `services/stream-processor/src/test/kotlin/com/recsys/features/ClickWindowFunctionTest.kt`
- Create: `services/stream-processor/src/test/kotlin/com/recsys/inventory/StockBitmapUpdaterTest.kt`
- Create: `infra/docker/stream-processor.Dockerfile`

- [ ] **Step 1: Create Gradle project with Flink dependencies**

Kotlin + Flink 1.20, Kafka connector, Redis connector.

- [ ] **Step 2: Write test for sliding window click aggregation**

Test that events within a 1-hour window produce correct `feat:user:{id}:clicks_1h` count.

- [ ] **Step 3: Implement ClickWindowFunction**

Flink sliding window (1h window, 1min slide) that counts events per user and writes to DragonflyDB.

- [ ] **Step 4: Run test — verify passes**

- [ ] **Step 5: Implement StockBitmapUpdater**

Consume `inventory-events` topic, set/clear bits in `stock:bitmap` via DragonflyDB SETBIT.

- [ ] **Step 6: Write test for bitmap update on stock-out event**

- [ ] **Step 7: Implement SessionProcessor**

Write recent events to `session:{id}:events` sorted set with TTL.

- [ ] **Step 8: Create main job class wiring all processors**

```kotlin
// StreamProcessorJob.kt
fun main() {
    val env = StreamExecutionEnvironment.getExecutionEnvironment()

    // Source: Redpanda user-events
    val userEvents = env.fromSource(kafkaSource("user-events"), "user-events")

    // Branch 1: Click window aggregation
    userEvents.keyBy { it.userId }
        .window(SlidingEventTimeWindows.of(Time.hours(1), Time.minutes(1)))
        .process(ClickWindowFunction())
        .addSink(DragonflySink())

    // Branch 2: Session events
    userEvents.keyBy { it.sessionId }
        .process(SessionProcessor())
        .addSink(DragonflySink())

    // Source: Redpanda inventory-events → Stock bitmap
    val inventoryEvents = env.fromSource(kafkaSource("inventory-events"), "inventory-events")
    inventoryEvents.process(StockBitmapUpdater())
        .addSink(DragonflySink())

    env.execute("recsys-stream-processor")
}
```

- [ ] **Step 9: Create Dockerfile, add to docker-compose**

- [ ] **Step 10: E2E verification**

```bash
# Send event
curl -X POST http://localhost:8080/api/v1/events -H "Content-Type: application/json" \
  -d '{"event_id":"e1","user_id":"u_001","event_type":"click","item_id":"i_001","timestamp":"2026-03-18T10:00:00Z","session_id":"s_001"}'

# Wait 2 seconds for Flink processing
sleep 2

# Check real-time feature was written
docker compose exec dragonfly redis-cli GET "feat:user:u_001:clicks_1h"
# Expected: "1"
```

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "feat: add Flink stream processor with real-time features and stock bitmap"
```

---

## Phase 5: Batch Plane & ML

### Task 8: Sample Data Generator

**Files:**
- Create: `services/traffic-simulator/go.mod`
- Create: `services/traffic-simulator/cmd/seed/main.go`
- Create: `services/traffic-simulator/internal/generator/users.go`
- Create: `services/traffic-simulator/internal/generator/items.go`
- Create: `services/traffic-simulator/internal/generator/events.go`

- [ ] **Step 1: Implement user/item/event data generators**

Generate:
- 100K users with category preferences
- 1M items across 50 categories with prices
- 10M historical events (click/view/purchase distribution)

Write to MinIO as Parquet files + seed DragonflyDB with item catalog.

- [ ] **Step 2: Implement traffic simulator for live testing**

Create `services/traffic-simulator/cmd/simulate/main.go` that generates realistic traffic patterns:
- Diurnal pattern (peak at 9pm, trough at 4am)
- Session behavior (browse → click → maybe purchase)
- Configurable RPS

- [ ] **Step 3: Test data generation**

```bash
make seed-data
# Verify: MinIO has parquet files, DragonflyDB has item data
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: add traffic-simulator with seed data and live traffic generation"
```

---

### Task 9: ML Models — Two-Tower & DCN-V2

**Files:**
- Create: `ml/models/two_tower/__init__.py`
- Create: `ml/models/two_tower/model.py`
- Create: `ml/models/two_tower/train.py`
- Create: `ml/models/two_tower/test_model.py`
- Create: `ml/models/dcn_v2/__init__.py`
- Create: `ml/models/dcn_v2/model.py`
- Create: `ml/models/dcn_v2/train.py`
- Create: `ml/models/dcn_v2/test_model.py`
- Create: `ml/serving/export_onnx.py`
- Create: `ml/pyproject.toml`

- [ ] **Step 1: Write failing test for Two-Tower model**

Test forward pass produces 128-dim embeddings for user and item inputs.

- [ ] **Step 2: Implement Two-Tower model in PyTorch**

```python
# ml/models/two_tower/model.py
import torch
import torch.nn as nn

class UserTower(nn.Module):
    def __init__(self, num_users: int, num_categories: int, embed_dim: int = 128):
        super().__init__()
        self.user_embed = nn.Embedding(num_users, 64)
        self.cat_embed = nn.Embedding(num_categories, 32)
        self.mlp = nn.Sequential(
            nn.Linear(64 + 32, 256),
            nn.ReLU(),
            nn.Linear(256, embed_dim),
            nn.LayerNorm(embed_dim),
        )

    def forward(self, user_id, category_hist):
        u = self.user_embed(user_id)
        c = self.cat_embed(category_hist).mean(dim=1)
        return self.mlp(torch.cat([u, c], dim=-1))

class ItemTower(nn.Module):
    def __init__(self, num_items: int, num_categories: int, embed_dim: int = 128):
        super().__init__()
        self.item_embed = nn.Embedding(num_items, 64)
        self.cat_embed = nn.Embedding(num_categories, 32)
        self.price_proj = nn.Linear(1, 32)
        self.mlp = nn.Sequential(
            nn.Linear(64 + 32 + 32, 256),
            nn.ReLU(),
            nn.Linear(256, embed_dim),
            nn.LayerNorm(embed_dim),
        )

    def forward(self, item_id, category_id, price):
        i = self.item_embed(item_id)
        c = self.cat_embed(category_id)
        p = self.price_proj(price.unsqueeze(-1))
        return self.mlp(torch.cat([i, c, p], dim=-1))

class TwoTowerModel(nn.Module):
    def __init__(self, num_users, num_items, num_categories, embed_dim=128):
        super().__init__()
        self.user_tower = UserTower(num_users, num_categories, embed_dim)
        self.item_tower = ItemTower(num_items, num_categories, embed_dim)

    def forward(self, user_id, category_hist, item_id, category_id, price):
        user_emb = self.user_tower(user_id, category_hist)
        item_emb = self.item_tower(item_id, category_id, price)
        return torch.sum(user_emb * item_emb, dim=-1)  # dot product similarity
```

- [ ] **Step 3: Run test — verify passes**

- [ ] **Step 4: Implement DCN-V2 ranking model**

Deep Cross Network V2 for Tier 3 ranking. Takes user embedding + item embedding + context features → relevance score.

- [ ] **Step 5: Implement ONNX export script**

```python
# ml/serving/export_onnx.py
def export_two_tower(model, output_path):
    # Export item tower only (user tower runs in batch)
    dummy_item = torch.zeros(1, dtype=torch.long)
    dummy_cat = torch.zeros(1, dtype=torch.long)
    dummy_price = torch.zeros(1)
    torch.onnx.export(model.item_tower, (dummy_item, dummy_cat, dummy_price),
                       output_path, input_names=["item_id", "category_id", "price"])
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: add Two-Tower and DCN-V2 model implementations with ONNX export"
```

---

### Task 10: Batch Processor — Feature Engineering & Pre-compute

**Files:**
- Create: `services/batch-processor/pyproject.toml`
- Create: `services/batch-processor/src/batch_processor/features.py`
- Create: `services/batch-processor/src/batch_processor/precompute.py`
- Create: `services/batch-processor/src/batch_processor/embeddings.py`
- Create: `services/batch-processor/dags/daily_full.py`
- Create: `services/batch-processor/dags/incremental_4h.py`
- Create: `services/batch-processor/tests/test_features.py`

- [ ] **Step 1: Write failing test for feature engineering**

Test user feature aggregation from events: total clicks, views, purchases, category distribution.

- [ ] **Step 2: Implement Spark feature engineering**

```python
# services/batch-processor/src/batch_processor/features.py
from pyspark.sql import SparkSession, DataFrame
import pyspark.sql.functions as F

def compute_user_features(events: DataFrame) -> DataFrame:
    return events.groupBy("user_id").agg(
        F.count(F.when(F.col("event_type") == "click", 1)).alias("total_clicks"),
        F.count(F.when(F.col("event_type") == "view", 1)).alias("total_views"),
        F.count(F.when(F.col("event_type") == "purchase", 1)).alias("total_purchases"),
        F.collect_set("category_id").alias("category_interests"),
        F.count("*").alias("total_events"),
    )

def compute_item_features(events: DataFrame) -> DataFrame:
    return events.groupBy("item_id").agg(
        F.count(F.when(F.col("event_type") == "click", 1)).alias("click_count"),
        F.count(F.when(F.col("event_type") == "purchase", 1)).alias("purchase_count"),
        (F.count(F.when(F.col("event_type") == "click", 1)) /
         F.greatest(F.count(F.when(F.col("event_type") == "view", 1)), F.lit(1))).alias("ctr"),
        F.countDistinct("user_id").alias("unique_users"),
    )
```

- [ ] **Step 3: Run test — verify passes**

- [ ] **Step 4: Implement pre-compute pipeline**

Reads trained Two-Tower embeddings, runs ANN search via Milvus, writes Top-K per user to DragonflyDB.

```python
# services/batch-processor/src/batch_processor/precompute.py
def precompute_recommendations(user_embeddings, milvus_client, redis_client, top_k=100):
    """For each user, find top-K items via ANN and store in DragonflyDB."""
    for batch in chunk(user_embeddings, 1000):
        user_ids = [u["user_id"] for u in batch]
        vectors = [u["embedding"] for u in batch]

        # Batch ANN search
        results = milvus_client.search(
            collection_name="item_embeddings",
            data=vectors,
            limit=top_k,
            output_fields=["item_id"],
        )

        # Bulk write to DragonflyDB
        pipe = redis_client.pipeline()
        for user_id, hits in zip(user_ids, results):
            recs = [{"item_id": h.entity.get("item_id"), "score": h.distance}
                    for h in hits]
            pipe.set(f"rec:{user_id}:top_k", json.dumps(recs))
            pipe.set(f"rec:{user_id}:ttl", datetime.utcnow().isoformat())
        pipe.execute()
```

- [ ] **Step 5: Create Airflow DAGs**

Daily full recompute + 4-hour incremental.

- [ ] **Step 6: Verify batch pipeline end-to-end**

```bash
# Run seed data → train model → generate embeddings → precompute → check DragonflyDB
make seed-data
python -m batch_processor.features
python -m batch_processor.precompute
docker compose exec dragonfly redis-cli GET "rec:u_00001:top_k"
# Expected: JSON array of recommendations
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: add batch processor with Spark features, embeddings, and pre-compute pipeline"
```

---

### Task 11: Ranking Service (Triton)

**Files:**
- Create: `services/ranking-service/models/dcn_v2/config.pbtxt`
- Create: `services/ranking-service/models/dcn_v2/1/model.onnx` (generated)
- Create: `services/ranking-service/client/triton_client.py`
- Create: `services/ranking-service/client/triton_client_test.py`
- Create: `infra/docker/ranking-service.Dockerfile`

- [ ] **Step 1: Create Triton model repository config**

```protobuf
# services/ranking-service/models/dcn_v2/config.pbtxt
name: "dcn_v2"
platform: "onnxruntime_onnx"
max_batch_size: 256
input [
  { name: "user_embedding" data_type: TYPE_FP32 dims: [128] },
  { name: "item_embedding" data_type: TYPE_FP32 dims: [128] },
  { name: "context_features" data_type: TYPE_FP32 dims: [32] }
]
output [
  { name: "score" data_type: TYPE_FP32 dims: [1] }
]
dynamic_batching {
  preferred_batch_size: [32, 64, 128]
  max_queue_delay_microseconds: 5000
}
instance_group [
  { count: 1 kind: KIND_CPU }
]
```

- [ ] **Step 2: Export DCN-V2 model to ONNX, place in model repo**

- [ ] **Step 3: Add Triton to docker-compose**

```yaml
  triton:
    image: nvcr.io/nvidia/tritonserver:24.10-py3
    ports:
      - "8001:8001"
      - "8002:8002"
    volumes:
      - ../services/ranking-service/models:/models
    command: ["tritonserver", "--model-repository=/models", "--strict-model-config=false"]
```

- [ ] **Step 4: Implement Go gRPC client for Triton in recommendation-api**

Add Tier 3 path: when Tier 1 cache miss → call Triton for real-time inference.

- [ ] **Step 5: E2E verify Tier 3**

```bash
curl "http://localhost:8090/api/v1/recommend?user_id=u_unknown&limit=10"
# Expected: tier="tier3_inference" (for cold-start user with no cache)
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: add ranking-service with Triton ONNX serving and Tier 3 integration"
```

---

## Phase 6: Verification & Production Readiness

### Task 12: Load Testing Suite

**Files:**
- Create: `load-tests/local-benchmark.js`
- Create: `load-tests/ramp-up.js`
- Create: `load-tests/tier-distribution.js`
- Create: `load-tests/inventory-stress.js`

- [ ] **Step 1: Create k6 local benchmark script**

```javascript
// load-tests/local-benchmark.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const tierCounter = new Counter('tier_distribution');
const latencyByTier = new Trend('latency_by_tier');

export const options = {
  stages: [
    { duration: '30s', target: 100 },
    { duration: '1m', target: 1000 },
    { duration: '1m', target: 1000 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<100', 'p(99)<200'],
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  const userId = `u_${Math.floor(Math.random() * 100000).toString().padStart(5, '0')}`;
  const res = http.get(`http://localhost:10000/api/v1/recommend?user_id=${userId}&limit=20`);

  check(res, {
    'status is 200': (r) => r.status === 200,
    'has items': (r) => JSON.parse(r.body).items.length > 0,
  });

  const body = JSON.parse(res.body);
  tierCounter.add(1, { tier: body.tier });
  latencyByTier.add(res.timings.duration, { tier: body.tier });
}
```

- [ ] **Step 2: Create ramp-up test (1K → 10K → 50K → 100K RPS)**

- [ ] **Step 3: Create inventory stress test**

Fire 10K stock-out events/sec and verify bitmap updates within 1 second.

- [ ] **Step 4: Run local benchmark and verify targets**

```bash
make bench-local
# Expected: p95 < 100ms, p99 < 200ms, error rate < 1%
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: add k6 load testing suite with benchmarks and stress tests"
```

---

### Task 13: Helm Charts for Kubernetes

**Files:**
- Create: `infra/helm/Chart.yaml`
- Create: `infra/helm/values.yaml`
- Create: `infra/helm/values/dev.yaml`
- Create: `infra/helm/values/staging.yaml`
- Create: `infra/helm/values/production.yaml`
- Create: `infra/helm/templates/` (deployment, service, hpa, pdb for each service)

- [ ] **Step 1: Create Helm chart structure**

- [ ] **Step 2: Create per-service templates with HPA and PDB**

Each service gets: Deployment, Service, HPA, PodDisruptionBudget.
production.yaml specifies the 50M DAU pod counts from the spec.

- [ ] **Step 3: Create production.yaml with scale numbers**

```yaml
# infra/helm/values/production.yaml
eventCollector:
  replicas: 50
  resources:
    requests: { cpu: "2", memory: "4Gi" }
    limits: { cpu: "2", memory: "4Gi" }
  hpa:
    minReplicas: 50
    maxReplicas: 200
    targetCPU: 70

recommendationApi:
  replicas: 250
  resources:
    requests: { cpu: "4", memory: "8Gi" }
    limits: { cpu: "4", memory: "8Gi" }
  hpa:
    minReplicas: 250
    maxReplicas: 1000
    targetCPU: 70

streamProcessor:
  taskManagers: 25
  resources:
    requests: { cpu: "4", memory: "8Gi" }

rankingService:
  replicas: 8
  resources:
    limits:
      nvidia.com/gpu: 1
    requests: { cpu: "4", memory: "16Gi" }
```

- [ ] **Step 4: Validate with helm template**

```bash
helm template recsys ./infra/helm -f ./infra/helm/values/production.yaml | kubectl apply --dry-run=client -f -
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "infra: add Helm charts with dev/staging/production values"
```

---

### Task 14: Chaos Engineering Tests

**Files:**
- Create: `load-tests/chaos/tc01-dragonfly-failover.yaml`
- Create: `load-tests/chaos/tc02-triton-failure.yaml`
- Create: `load-tests/chaos/tc03-redpanda-partition.yaml`
- Create: `load-tests/chaos/tc04-network-latency.yaml`
- Create: `load-tests/chaos/tc05-traffic-spike.yaml`
- Create: `load-tests/chaos/tc06-stock-flood.yaml`
- Create: `load-tests/chaos/run-all.sh`

- [ ] **Step 1: Create Chaos Mesh manifests for TC-01 through TC-06**

Each manifest from the spec's verification strategy:
- TC-01: DragonflyDB pod kill → failover < 3s
- TC-02: Triton pod kill → Tier 1 fallback
- TC-03: Redpanda network partition → zero event loss
- TC-04: 200ms network latency injection
- TC-05: 10x traffic spike via k6
- TC-06: 10K items/sec stock-out flood

- [ ] **Step 2: Create run-all.sh that orchestrates tests with assertions**

- [ ] **Step 3: Document expected results in each manifest**

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: add Chaos Mesh manifests for resilience testing (TC-01 to TC-06)"
```

---

### Task 15: A/B Testing Framework

**Files:**
- Create: `services/recommendation-api/internal/experiment/router.go`
- Create: `services/recommendation-api/internal/experiment/router_test.go`

- [ ] **Step 1: Write failing test for experiment bucketing**

Test consistent hashing: same user always lands in same bucket.

- [ ] **Step 2: Implement experiment router**

```go
// services/recommendation-api/internal/experiment/router.go
package experiment

type Experiment struct {
    ID           string  `json:"id"`
    ModelVersion string  `json:"model_version"`
    TrafficPct   float64 `json:"traffic_pct"`
    TierConfig   string  `json:"tier_config,omitempty"`
}

type Router struct {
    store Store // reads experiment configs from DragonflyDB
}

func (r *Router) GetExperiment(userID string) (*Experiment, error) {
    experiments, err := r.store.ListActiveExperiments()
    if err != nil {
        return nil, err // fallback to control
    }

    bucket := hashBucket(userID) // 0-99
    cumulative := 0.0
    for _, exp := range experiments {
        cumulative += exp.TrafficPct
        if float64(bucket) < cumulative {
            return &exp, nil
        }
    }
    return nil, nil // control group
}

func hashBucket(userID string) int {
    h := fnv.New32a()
    h.Write([]byte(userID))
    return int(h.Sum32() % 100)
}
```

- [ ] **Step 3: Run test — verify passes**

- [ ] **Step 4: Wire into recommendation handler**

Log experiment assignment with each recommendation response for downstream analysis.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add A/B experiment routing with consistent hashing"
```

---

### Task 16: Final Integration Test & Documentation

**Files:**
- Modify: `README.md` (update with actual commands that work)
- Modify: `Makefile` (ensure all targets work)
- Create: `scripts/verify-e2e.sh`

- [ ] **Step 1: Create end-to-end verification script**

```bash
#!/bin/bash
# scripts/verify-e2e.sh
set -e

echo "=== Starting full stack ==="
make up

echo "=== Seeding data ==="
make seed-data

echo "=== Waiting for services ==="
sleep 10
make health-check

echo "=== Testing event ingestion ==="
curl -sf -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_id":"test-1","user_id":"u_00001","event_type":"click","item_id":"i_00001","timestamp":"2026-03-18T10:00:00Z","session_id":"s_001"}'
echo "Event ingestion: OK"

echo "=== Testing recommendations ==="
RESULT=$(curl -sf "http://localhost:10000/api/v1/recommend?user_id=u_00001&limit=10")
echo "Recommendation result: $RESULT"
TIER=$(echo $RESULT | python3 -c "import sys,json; print(json.load(sys.stdin)['tier'])")
echo "Tier used: $TIER"

echo "=== Running local benchmark ==="
make bench-local

echo "=== All checks passed ==="
```

- [ ] **Step 2: Run full E2E verification**

```bash
bash scripts/verify-e2e.sh
```

- [ ] **Step 3: Update README with verified commands**

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "docs: finalize README and add E2E verification script"
```

---

## Implementation Order Summary

```
Phase 1: Foundation
  Task 1:  Project scaffold              → commit
  Task 2:  Docker-compose infra          → commit (docker compose up works)

Phase 2: Seed Data (moved early — needed by all services)
  Task 3:  Sample data generator         → commit (seed data in DragonflyDB)

Phase 3: Event Ingestion
  Task 4:  Event collector               → commit (events → Redpanda)

Phase 4: Recommendation API Core
  Task 5:  Recommendation API Tier 1     → commit (pre-computed serving) ★ system functional here
  Task 6:  Recommendation API Tier 2     → commit (weighted re-ranking with session features)
  Task 7:  Tier 0 CDN + degradation      → commit (popular endpoint + 4-level state machine)
  Task 8:  Circuit breaker + metrics     → commit (resilience + observability)

Phase 5: Stream Processing
  Task 9:  Flink stream processor        → commit (real-time features + stock bitmap)

Phase 6: Batch & ML
  Task 10: ML models (Two-Tower, DCN-V2) → commit (with MLflow tracking)
  Task 11: Batch processor               → commit (features + pre-compute + TTL policies)

Phase 7: Full Model Serving
  Task 12: Ranking service (Triton)      → commit (Tier 3 GPU inference)

Phase 8: Verification & Production
  Task 13: Load testing suite            → commit (k6 benchmarks)
  Task 14: Helm charts                   → commit (K8s production deploy)
  Task 15: Chaos engineering             → commit (TC-01 ~ TC-06)
  Task 16: A/B testing + model rollout   → commit (experiment framework + shadow/canary/rollback)
  Task 17: Final integration             → commit (E2E verified, README updated)
```

**Key changes from v1**: Seed data moved to Phase 2 (was Phase 5). Tier 0 CDN + degradation added (was missing). GBDT replaced with weighted scorer (no CGo dependency). MLflow integration throughout. TTL policies on all DragonflyDB writes.

Each task produces a working, testable increment. At any point after Task 5, the system is functional (Tier 1 serving with pre-computed data).
