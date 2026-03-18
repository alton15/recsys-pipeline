# recsys-pipeline

## Overview

50M DAU commerce recommendation system template. Cloud-agnostic: runs locally
via `docker-compose`, scales on Kubernetes.

## Architecture

### 4-Plane Model
- **Data Plane**: Redpanda (event streaming), DragonflyDB (feature store), Milvus (ANN), PostgreSQL (metadata)
- **Serving Plane**: Event Collector (Go), Recommendation API (Go), Triton Inference Server
- **Training Plane**: Feature pipeline (Python/Flink), model training (Python), MLflow
- **Control Plane**: A/B experiment config, circuit breakers, traffic shaping

### 3-Tier Recommendation
- **Tier 0 (CDN)**: Pre-computed popular/trending lists, < 5ms
- **Tier 1 (Cache)**: Per-user top-K from DragonflyDB, < 20ms
- **Tier 2 (Compute)**: Real-time ANN retrieval + re-ranking, < 100ms

## Languages & Versions
- **Go 1.23**: Event Collector, Recommendation API, Traffic Simulator
- **Python 3.12**: Feature pipelines, model training, evaluation
- **Java/Kotlin**: Flink jobs (future)

## Key Patterns

### DragonflyDB Key Schema
See `shared/go/keys/keys.go` for all key patterns:
- `rec:<user_id>:top_k` — cached recommendations
- `feat:user:<user_id>:*` — user features (clicks_1h, views_1d, recent_items)
- `feat:item:<item_id>:*` — item features (ctr_7d, popularity)
- `stock:bitmap` / `stock:id_map:<item_id>` — stock availability
- `session:<session_id>:events` — session event buffer
- `experiment:<exp_id>` — A/B experiment config

### Event Schema
See `shared/go/event/event.go`. Types: click, view, purchase, search, add_to_cart, remove_from_cart.

## Commands

```bash
make up                # Start all services via docker-compose
make down              # Stop and remove volumes
make test              # Run all Go tests
make seed-data         # Generate sample items/users
make simulate-traffic  # Run traffic simulator
make health-check      # Check service health
make bench-local       # Run k6 load test
```

## Project Structure

```
services/
  event-collector/     # HTTP → Redpanda event ingestion (Go)
  recommendation-api/  # 3-tier recommendation serving (Go)
  traffic-simulator/   # Load generation & seed data (Go)
shared/
  go/                  # Shared Go types (event, keys)
configs/               # Environment configs
docs/                  # Architecture specs
load-tests/            # k6 benchmark scripts
```

## Development

- Go workspace: `go.work` at project root
- Shared code: import `github.com/recsys-pipeline/shared/...`
- Config: copy `configs/local.env.example` to `.env` for local dev
- All services use `replace` directives to reference shared module locally
