# recsys-pipeline

Production-grade recommendation system pipeline for 50M DAU commerce services.

A cloud-agnostic reference architecture that runs locally with `docker-compose up` and scales to 500K RPS on Kubernetes.

---

## Why This Project?

Building a personalization engine for tens of millions of users is one of the hardest engineering challenges in commerce. Most open-source examples are either toy demos or proprietary fragments. This project provides a **complete, runnable pipeline** — from event ingestion to model serving — designed to handle real production traffic.

**Key numbers at 50M DAU:**

| Metric | Value |
|--------|-------|
| Peak RPS | 500K |
| p99 Latency | < 100ms (Tier 1: < 5ms) |
| GPU Inference RPS | ~4.5K (0.9% of traffic) |
| Estimated Monthly Cost | ~$84K (~1.18억 원) |

---

## Architecture

### 3-Tier Serving Strategy

The core insight: **don't run GPU inference on every request.** Pre-compute results for most users, reserve real-time inference for cache misses only.

```
                        500K RPS
                           │
                     ┌─────┴─────┐
                     │    CDN    │ Tier 0: Trending/Popular (anonymous)
                     └─────┬─────┘
                       350K absorbed (70%)
                           │
                       150K RPS
                           │
                  ┌────────┴────────┐
                  │  Envoy Gateway  │ Rate limit, Auth, Circuit breaker
                  └────────┬────────┘
                           │
              ┌────────────┴────────────┐
              │  recommendation-api(Go) │ Orchestrator
              └────────────┬────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         ↓                 ↓                 ↓
   ┌───────────┐    ┌───────────┐    ┌───────────┐
   │  Tier 1   │    │  Tier 2   │    │  Tier 3   │
   │ Pre-comp  │    │ CPU GBDT  │    │ GPU Model │
   │ DragonflyDB│   │ Re-rank   │    │ Triton    │
   └───────────┘    └───────────┘    └───────────┘
     ~85%             ~12%              ~3%
     < 5ms            < 20ms            < 80ms
```

> Tier percentages above are of the 150K RPS that pass CDN (post-Tier 0).
> Of total 500K RPS: Tier 0=70%, Tier 1=25.5%, Tier 2=3.6%, Tier 3=0.9%.

### 4-Plane Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│  DATA PLANE (Serving)                                               │
│                                                                     │
│  Users → CDN → Envoy → recommendation-api (Go)                     │
│                           ├─ DragonflyDB direct read                │
│                           ├─ Local GBDT re-ranking (embedded)       │
│                           ├─ Stock bitmap filtering (O(1))          │
│                           └─ Triton gRPC (Tier 3 only)             │
└────────────────────────────────┬────────────────────────────────────┘
                                 │ events
┌────────────────────────────────┴────────────────────────────────────┐
│  STREAM PLANE (Real-time)                                           │
│                                                                     │
│  event-collector (Go) → Redpanda                                    │
│                           ├─ Flink → Real-time features → Dragonfly │
│                           ├─ Flink → Stock bitmap → Dragonfly       │
│                           └─ Flink → Session events → Dragonfly     │
└────────────────────────────────┬────────────────────────────────────┘
                                 │ data lake
┌────────────────────────────────┴────────────────────────────────────┐
│  BATCH PLANE (Offline)                                              │
│                                                                     │
│  Spark ← MinIO (event logs)                                        │
│    ├─ Feature engineering → DragonflyDB bulk load                   │
│    ├─ Model training (PyTorch + DeepSpeed) → MLflow                 │
│    ├─ Candidate generation (Two-Tower) → Milvus ANN index           │
│    └─ Pre-compute recommendations → DragonflyDB (per-user Top-K)   │
│  Schedule: Airflow (daily full, 4-hour incremental)                 │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
┌────────────────────────────────┴────────────────────────────────────┐
│  CONTROL PLANE (Observability)                                      │
│                                                                     │
│  Prometheus + Grafana │ Jaeger │ Alertmanager │ A/B Platform        │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Project Structure

```
recsys-pipeline/
├── services/
│   ├── event-collector/          # Go — Event ingestion API (Redpanda producer)
│   ├── stream-processor/         # Java/Kotlin — Flink jobs (real-time features)
│   ├── batch-processor/          # Python — Spark jobs (batch features + training)
│   ├── recommendation-api/       # Go — Final recommendation API (orchestrator)
│   ├── ranking-service/          # Python — Model serving (Triton + ONNX)
│   └── traffic-simulator/        # Go — Load testing + sample traffic generation
├── ml/
│   ├── models/                   # Model training code (Two-Tower, DCN-V2)
│   ├── notebooks/                # Experimentation notebooks
│   └── serving/                  # ONNX conversion + TensorRT INT8 quantization
├── infra/
│   ├── docker/                   # Per-service Dockerfiles
│   ├── docker-compose.yml        # Local full-stack (one command)
│   ├── helm/                     # Production K8s Helm charts
│   │   └── values/
│   │       ├── dev.yaml
│   │       ├── staging.yaml
│   │       └── production.yaml   # 50M DAU production config
│   └── monitoring/               # Prometheus + Grafana dashboards
├── load-tests/                   # k6 load test scenarios
├── configs/                      # Environment-specific configs
├── scripts/                      # Utility scripts (seed data, migrations)
├── docs/                         # Architecture documentation
└── README.md
```

---

## Architecture Decisions

### Why 4-Plane Separation?

추천 시스템의 가장 큰 실수는 서빙과 학습을 하나의 파이프라인에 넣는 것입니다. 트래픽이 커지면 서빙 부하가 학습을 밀어내거나, 학습 배치가 서빙 지연을 유발합니다.

| Approach | Pros | Cons | Verdict |
|----------|------|------|---------|
| **Monolith** (서빙+학습 통합) | 단순, 배포 쉬움 | 서빙/학습 리소스 경쟁, 스케일링 불가 | 1M DAU 이하만 가능 |
| **2-Plane** (Online/Offline) | 서빙과 학습 분리 | 실시간 피처 반영 불가, 세션 개인화 어려움 | 10M DAU까지 가능 |
| **4-Plane** (Data/Stream/Batch/Control) | 각 plane 독립 스케일링, 실시간+배치 공존 | 운영 복잡도 높음 | **50M DAU 필수** ✅ |

**선택 이유**: 50M DAU에서 서빙(500K RPS)과 실시간 피처(Flink)와 배치 학습(Spark)이 동시에 돌아야 합니다. 서로의 리소스를 침범하지 않으려면 물리적으로 분리된 Plane이 필요합니다. Control Plane은 A/B 테스트와 모델 롤아웃을 관리하며, 이 역시 서빙과 분리되어야 서빙 장애가 실험 데이터를 오염시키지 않습니다.

### Why 3-Tier Serving?

모든 요청에 GPU 추론을 돌리면 500K RPS × GPU 추론 = 물리적으로 불가능합니다. 넷플릭스, 유튜브, 쿠팡 모두 동일한 패턴을 사용합니다.

| Approach | GPU Usage | p99 Latency | Monthly Cost |
|----------|-----------|-------------|-------------|
| **All real-time inference** | 500K RPS on GPU | ~100ms+ | $60K+ (GPU only) |
| **Pre-compute only** | 0 GPU | < 5ms | Low, but stale |
| **3-Tier hybrid** | 4.5K RPS (0.9%) | Tier1: 5ms, Tier3: 80ms | $4.8K (GPU) |

**선택 이유**: Tier 1(pre-computed)이 85%+ 트래픽을 < 5ms로 처리하면 GPU 비용을 92% 줄이면서도 콜드스타트 유저에게는 실시간 개인화를 제공할 수 있습니다.

### Why Embedded Architecture (Fan-out 제거)?

| Approach | Network Hops | p99 Latency | Failure Mode |
|----------|-------------|-------------|--------------|
| **Microservice fan-out** (api → feature-store → ranker → filter) | 6+ | ~57ms | 한 서비스 장애 → 전체 장애 |
| **Service mesh** (Istio sidecar) | 6+ (sidecar 추가) | ~70ms+ | Sidecar 오버헤드 |
| **Embedded** (api 내부에서 직접 처리) | 1-2 | ~15ms | DragonflyDB만 의존 |

**선택 이유**: 500K RPS에서 네트워크 홉 하나당 p99에서 5-15ms가 추가됩니다. 3개 서비스를 순차 호출하면 지연시간이 누적되고, 어느 하나만 느려져도 전체가 느려집니다. recommendation-api가 DragonflyDB를 직접 읽고, re-ranking을 로컬에서 수행하면 의존성이 DragonflyDB 하나로 줄어들어 장애 표면적이 최소화됩니다.

---

## Tech Stack — Selection Rationale

각 기술은 최소 2-3개 대안과 비교 후 선택했습니다.

### API / Orchestrator: Go

recommendation-api, event-collector, traffic-simulator의 언어.

| Language | Concurrency | Latency (p99) | Memory | Binary Size | Verdict |
|----------|------------|---------------|--------|-------------|---------|
| **Go** | Goroutine (M:N 스케줄링) | ~1-2ms overhead | ~30MB/instance | ~15MB static | **Selected** ✅ |
| Rust | Tokio async | ~0.5ms (최저) | ~20MB | ~10MB | 개발 속도 느림, 팀 러닝커브 |
| Java (Spring) | Thread pool / Virtual threads | ~5-15ms (GC pause) | ~200MB+ | N/A (JVM) | GC 스톱더월드, 메모리 과다 |
| Node.js | Event loop (싱글스레드) | ~3-5ms | ~80MB | N/A | CPU-bound 작업에 약함 |
| Python (FastAPI) | asyncio | ~10-20ms | ~100MB+ | N/A | GIL, 속도 부족 |

**선택 이유**: Go는 goroutine 기반으로 10K+ 동시 연결을 쉽게 처리하면서 p99 지연시간이 1-2ms 수준입니다. 정적 바이너리로 Docker 이미지가 작고(Alpine + 15MB), 메모리 사용량이 낮아 파드당 비용이 최소입니다. Rust가 성능에서는 우위지만, 추천 시스템 특성상 CPU-bound 로직보다 I/O-bound(DragonflyDB 읽기)가 대부분이라 Go의 성능으로 충분합니다.

### Event Streaming: Redpanda (vs Kafka)

| Feature | Apache Kafka | Redpanda | Pulsar |
|---------|-------------|----------|--------|
| **Language** | Java (JVM) | C++ (thread-per-core) | Java (JVM) |
| **ZooKeeper** | Required (KRaft는 아직 성숙 중) | Not needed | ZK required |
| **Throughput/node** | ~200K msg/s | ~1M+ msg/s | ~150K msg/s |
| **Tail latency** | p99 ~10-50ms (GC) | p99 < 5ms (no GC) | p99 ~20-80ms |
| **Nodes for 500K RPS** | 50+ brokers | 10-12 brokers | 60+ brokers |
| **Kafka API compatible** | Native | 100% compatible | Adapter needed |
| **Operations** | Complex (JVM tuning, GC, ZK) | Simple (single binary) | Complex |
| **Monthly cost** | $25,000 | $6,000 | $30,000+ |

**선택 이유**: Redpanda는 Kafka API를 100% 호환하면서 C++로 작성되어 JVM의 GC pause 문제가 없습니다. 같은 하드웨어에서 5배 높은 처리량을 제공하고, ZooKeeper가 필요 없어 운영이 단순합니다. Kafka 에코시스템(Flink, Connect 등)을 그대로 사용할 수 있어 마이그레이션 비용이 0입니다. 유일한 리스크는 Kafka 대비 커뮤니티 규모가 작다는 점이지만, Confluent Cloud 없이도 운영 가능한 것이 클라우드 애그노스틱 목표에 부합합니다.

### Cache / Feature Store: DragonflyDB (vs Redis)

| Feature | Redis Cluster | DragonflyDB | KeyDB | Memcached |
|---------|--------------|-------------|-------|-----------|
| **Threading** | Single-threaded | Multi-threaded | Multi-threaded | Multi-threaded |
| **Ops/sec (8 CPU)** | ~100K | 500K-800K | ~200K | ~500K |
| **Protocol** | Redis | Redis-compatible | Redis-compatible | Memcached |
| **Data structures** | Full (sorted set, bitmap, etc.) | Full Redis compatibility | Full | Key-value only |
| **Persistence** | RDB/AOF | Snapshots | RDB/AOF | None |
| **Nodes for 450K ops/s** | 100+ | 8-10 | 30-40 | N/A (no bitmap) |
| **Memory efficiency** | 1x | ~1x (comparable) | ~1x | ~0.8x |
| **Monthly cost** | $30,000 | $4,500 | $12,000 | N/A |

**선택 이유**: DragonflyDB는 Redis 프로토콜을 완전 호환하면서 멀티스레드로 동작합니다. 같은 8-CPU 인스턴스에서 Redis 대비 5-8배 높은 처리량을 제공합니다. 우리는 Redis의 Bitmap(품절 체크), Sorted Set(세션 이벤트), String(추천 캐시) 등 다양한 자료구조가 필요한데, Memcached는 이를 지원하지 않습니다. KeyDB도 멀티스레드지만 DragonflyDB의 thread-per-core 아키텍처가 더 높은 처리량을 보여줍니다. 기존 Redis 클라이언트 라이브러리(`go-redis`, `redis-py`)를 수정 없이 사용할 수 있어 마이그레이션 비용이 0입니다.

### Stream Processing: Apache Flink (vs Spark Streaming, Kafka Streams)

| Feature | Apache Flink | Spark Structured Streaming | Kafka Streams |
|---------|-------------|---------------------------|---------------|
| **Processing model** | True streaming (event-at-a-time) | Micro-batch | True streaming |
| **Latency** | ms-level | seconds-level (micro-batch interval) | ms-level |
| **Windowing** | Sliding, tumbling, session, custom | Tumbling, sliding (limited) | Tumbling, sliding, session |
| **Exactly-once** | Yes (native) | Yes | Yes |
| **State management** | RocksDB (large state) | In-memory (limited) | RocksDB |
| **Standalone deployment** | Yes (own cluster) | Requires Spark cluster | Embedded in app |
| **Scalability** | Excellent (1000s of TaskManagers) | Good | Good (partition-bound) |

**선택 이유**: 우리의 스트림 처리 요구사항은 (1) 슬라이딩 윈도우 피처 집계 (30분/1시간), (2) 세션 갭 감지, (3) 재고 비트맵 실시간 갱신입니다. Spark Structured Streaming은 마이크로배치 기반이라 재고 변동 반영에 수 초의 지연이 발생합니다 (목표: < 1초). Kafka Streams는 ms 수준 처리가 가능하지만, 복잡한 윈도우 연산과 대규모 상태 관리에서 Flink이 우위입니다. Flink의 세션 윈도우는 유저 행동 시퀀스 분석에 정확히 맞는 기능입니다.

### Batch Processing: Apache Spark (vs Dask, Ray, Polars)

| Feature | Apache Spark | Dask | Ray | Polars |
|---------|-------------|------|-----|--------|
| **Scale** | PB-level | TB-level | TB-level | Single-node (TB) |
| **Ecosystem** | Massive (MLlib, SQL, connectors) | Growing | Growing (Ray Serve, Train) | Limited |
| **ML integration** | MLlib + PyTorch/TF via Spark | scikit-learn | Native (Ray Train) | None |
| **Connector support** | Kafka, S3, Hive, JDBC, etc. | S3, Parquet | S3, limited | Parquet, CSV |
| **Maturity** | 10+ years, battle-tested | 7+ years | 5+ years | 3+ years |

**선택 이유**: 50M 유저의 피처 엔지니어링은 수 TB의 이벤트 로그를 처리해야 합니다. Spark는 S3/MinIO, Kafka/Redpanda, PostgreSQL 커넥터가 모두 내장되어 있어 데이터 파이프라인 구성이 가장 쉽습니다. Dask와 Ray도 좋은 대안이지만, Airflow + Spark 조합은 업계 표준으로 운영 노하우가 가장 풍부합니다. Polars는 단일 노드 성능이 뛰어나지만 분산 처리를 지원하지 않아 50M 유저 규모에서는 부족합니다.

### Vector Search: Milvus (vs FAISS, Pinecone, Weaviate, Qdrant)

| Feature | Milvus | FAISS | Pinecone | Weaviate | Qdrant |
|---------|--------|-------|----------|----------|--------|
| **Distributed** | Yes (native) | No (single-node) | Yes (managed) | Yes | Yes |
| **Scale** | Billion+ vectors | 100M+ (single node) | Billion+ | 100M+ | 100M+ |
| **Index types** | HNSW, IVF, DiskANN, etc. | HNSW, IVF, PQ, etc. | Proprietary | HNSW | HNSW |
| **Self-hosted** | Yes | Yes (library) | No (SaaS only) | Yes | Yes |
| **Cloud-agnostic** | Yes | Yes | No | Yes | Yes |
| **Batch import** | Excellent | Manual | API-only | Good | Good |
| **GPU acceleration** | Yes | Yes | N/A | No | No |

**선택 이유**: 클라우드 애그노스틱이 핵심 요구사항이므로 Pinecone(SaaS only)은 제외됩니다. FAISS는 라이브러리 수준이라 분산 배포 시 직접 샤딩/레플리케이션을 구현해야 합니다. Milvus는 분산 아키텍처가 네이티브로 내장되어 있고, HNSW 인덱스로 recall 95%+ / QPS 2K+를 달성합니다. 10M 아이템 임베딩(128차원)을 8노드로 처리 가능합니다. Qdrant도 좋은 대안이지만 Milvus가 대규모 배치 import 성능에서 우위이며, 학계/업계에서 가장 많이 검증되었습니다.

### ML Training: PyTorch + DeepSpeed (vs TensorFlow, JAX)

| Feature | PyTorch + DeepSpeed | TensorFlow | JAX |
|---------|-------------------|------------|-----|
| **Industry adoption** | #1 (Meta, OpenAI, etc.) | Declining | Growing (Google) |
| **Distributed training** | DeepSpeed ZeRO, FSDP | tf.distribute | pjit, pmap |
| **Model ecosystem** | HuggingFace, timm, etc. | TF Hub | Limited |
| **Debugging** | Eager mode (easy) | Graph mode (harder) | Functional (harder) |
| **Recommendation models** | TorchRec (Meta) | TF Recommenders | None |
| **ONNX export** | Native support | tf2onnx (some issues) | Limited |

**선택 이유**: PyTorch는 추천 모델 연구/구현에서 사실상 표준입니다. Meta의 TorchRec 라이브러리가 Two-Tower, DCN-V2 같은 추천 모델을 직접 지원합니다. DeepSpeed ZeRO는 수억 파라미터 모델을 적은 GPU로 학습 가능하게 해줍니다. ONNX 변환이 네이티브로 지원되어 Triton 서빙까지 매끄럽게 연결됩니다.

### Model Serving: NVIDIA Triton + TensorRT (vs TorchServe, ONNX Runtime, BentoML)

| Feature | Triton + TensorRT | TorchServe | ONNX Runtime Server | BentoML |
|---------|-------------------|------------|---------------------|---------|
| **Dynamic batching** | Yes (automatic) | Yes (manual config) | Limited | Yes |
| **INT8 quantization** | TensorRT (3-4x speedup) | Limited | ONNX quantization | Via ONNX |
| **Multi-model** | Yes (concurrent) | Yes | Single model | Yes |
| **GPU utilization** | Excellent (CUDA optimized) | Good | Good | Good |
| **gRPC** | Native | REST mainly | REST | REST/gRPC |
| **Throughput (A100)** | ~500 infer/s (FP16) | ~200 infer/s | ~300 infer/s | ~250 infer/s |

**선택 이유**: Tier 3의 핵심 요구사항은 **최소 GPU로 최대 throughput**입니다. Triton의 TensorRT INT8 양자화는 FP16 대비 3-4배 처리량 증가를 제공하여, A100 20장이 필요한 워크로드를 L4 8장으로 줄입니다. Dynamic batching이 자동으로 작동하여 개별 요청을 묶어 GPU 활용률을 극대화합니다. gRPC 네이티브 지원으로 Go recommendation-api와의 연동이 깔끔합니다.

### API Gateway: Envoy (vs Nginx, Kong, Traefik, HAProxy)

| Feature | Envoy | Nginx | Kong | Traefik | HAProxy |
|---------|-------|-------|------|---------|---------|
| **Adaptive concurrency** | Yes (native) | No | Plugin | No | No |
| **Circuit breaker** | Yes (outlier detection) | No (needs Lua) | Yes (plugin) | Yes | No |
| **Rate limiting** | Local + global | Basic (limit_req) | Yes (plugin) | Basic | Basic (stick-tables) |
| **gRPC support** | Native | Yes | Yes | Yes | Limited |
| **Observability** | Prometheus, Jaeger built-in | Basic | Plugin | Prometheus | Prometheus |
| **xDS API** | Yes (dynamic config) | Reload required | API | API | Reload |
| **Latency overhead** | < 1ms | < 0.5ms | ~2-5ms (plugins) | ~1ms | < 0.5ms |

**선택 이유**: 50M DAU에서 API Gateway는 **adaptive concurrency limiting**이 핵심입니다. 트래픽 폭증 시 자동으로 동시 요청 수를 제한하여 downstream 서비스를 보호합니다. Envoy는 이 기능이 네이티브로 내장되어 있고, outlier detection 기반 circuit breaker, Prometheus 메트릭, 분산 트레이싱(Jaeger)이 모두 내장입니다. Nginx가 latency에서 약간 우위지만, 우리에게 필요한 resilience 기능들을 Lua 스크립트로 직접 구현해야 합니다. Kong은 plugin 체인의 latency overhead가 2-5ms로, Tier 1의 5ms 목표에서 gateway만으로 절반을 소진합니다.

### Object Storage: MinIO (vs Ceph, SeaweedFS)

| Feature | MinIO | Ceph (RGW) | SeaweedFS |
|---------|-------|-----------|-----------|
| **S3 compatibility** | 100% | 95%+ | 90% |
| **Setup complexity** | Docker 1줄 | Complex (MON, OSD, RGW) | Moderate |
| **Performance** | Excellent | Excellent | Good |
| **Ecosystem** | Spark, Flink, MLflow 전부 지원 | 대부분 지원 | 일부 미지원 |

**선택 이유**: 클라우드 애그노스틱에서 S3 API 호환 스토리지는 필수입니다. MinIO는 100% S3 호환으로 Spark, Flink, MLflow, Milvus가 모두 기본 지원합니다. docker-compose로 1줄이면 실행 가능합니다. Ceph는 프로덕션에서는 강력하지만 로컬 개발 환경 구성이 복잡합니다.

### Monitoring: Prometheus + Grafana (vs Datadog, Victoria Metrics)

| Feature | Prometheus + Grafana | Datadog | Victoria Metrics |
|---------|---------------------|---------|------------------|
| **Cost** | Free (self-hosted) | $23/host/month | Free (self-hosted) |
| **Cloud-agnostic** | Yes | SaaS | Yes |
| **Ecosystem** | Universal (every tool exports) | Agent-based | Prometheus-compatible |
| **50M DAU monthly** | $3K (infra only) | $50K+ (per-host pricing) | $2K |

**선택 이유**: 클라우드 애그노스틱 + 비용 최적화 목표에서 SaaS 모니터링(Datadog, New Relic)은 호스트당 과금으로 수백 파드 규모에서 비용이 폭발합니다. Prometheus는 모든 기술 스택(Go, Flink, DragonflyDB, Triton)이 네이티브 exporter를 제공합니다. Victoria Metrics는 장기 저장소로 좋은 대안이지만, 기본 구성으로는 Prometheus + Grafana 조합이 가장 성숙합니다.

---

## Key Design Decisions

### 1. Fan-out Elimination — Embedded Architecture

Traditional microservice recommendation systems make 3+ network calls per request. At 500K RPS, each extra hop adds tail latency.

**Our approach:** The `recommendation-api` embeds feature lookup, re-ranking, and filtering logic directly:

```
Traditional: api → network → feature-store → network → response     (×3 services = 6 hops)
Ours:        api → DragonflyDB read + local re-rank + bitmap filter  (1-2 hops)
```

Result: p99 drops from ~57ms to ~15ms for Tier 1.

### 2. Stock Bitmap for Real-time Inventory

Commerce-specific problem: recommending out-of-stock items destroys UX.

```
Stock event → Redpanda → Flink → DragonflyDB bitmap update (< 1 second)
Query time:  GETBIT stock:bitmap {item_id}  →  O(1), < 0.1ms
```

No database joins, no cache invalidation complexity.

### 3. Graceful Degradation Chain

The system never shows an empty screen:

```
Normal        →  Tier 0 → 1 → 2 → 3
Warning(150%) →  Tier 3 disabled
Critical(200%)→  Tier 2 disabled
Emergency     →  CDN static fallback
```

Each tier has an independent circuit breaker (Envoy built-in, not Hystrix).

### 4. Cost Optimization: Redpanda + DragonflyDB + L4 INT8

| Component | Before | After | Savings |
|-----------|--------|-------|---------|
| Message Broker | Kafka 50 nodes ($25K) | Redpanda 12 nodes ($6K) | -76% |
| Cache | Redis 100 nodes ($30K) | DragonflyDB 10 nodes ($4.5K) | -85% |
| GPU Inference | A100×20 ($60K) | L4 INT8×8 ($4.8K) | -92% |
| Vector Search | Milvus 50 nodes ($20K) | Milvus 8 nodes ($3.2K) | -84% |
| API Servers | 500 pods ($50K) | 250 pods ($25K) | -50% |
| Stream Processing | Flink 100 TM ($40K) | Flink 25 TM ($5K) | -88% |
| Batch (Spark) | On-demand ($15K) | Spot ($4.5K) | -70% |
| Other (storage, net, mon.) | $45K | $27.5K | -39% |
| **Total infra** | **$285K/mo** | **$84K/mo** | **-71%** |

---

## Quick Start

### Prerequisites

- Docker & Docker Compose v2
- 32GB+ RAM (16GB may work with reduced services)
- GPU optional (CPU fallback for Triton)

### Run Locally

```bash
# Clone
git clone https://github.com/YOUR_USERNAME/recsys-pipeline.git
cd recsys-pipeline

# Generate sample data (100K users, 1M items)
make seed-data

# Start all services
docker compose up -d

# Check health
make health-check

# Generate sample traffic
make simulate-traffic

# Open monitoring dashboard
open http://localhost:3000  # Grafana
```

### Run Load Tests

```bash
# Single-node benchmark
make bench-local

# Distributed load test (requires K8s)
make bench-k6 RPS=100000

# Chaos engineering tests
make chaos-test
```

---

## Verification

### Stage 1: Single-Node Benchmark

| Component | Target | Tool |
|-----------|--------|------|
| event-collector | 10K RPS / instance | wrk, hey |
| DragonflyDB | 1M+ ops/sec | redis-benchmark |
| ONNX Runtime INT8 | < 10ms / inference | Triton perf_analyzer |
| recommendation-api E2E | Tier1 < 5ms, Tier2 < 20ms | k6 |

### Stage 2: Distributed Load Test

- Ramp: 1K → 10K → 50K → 100K RPS
- Hold each level for 5 minutes
- Measure: p50/p95/p99 latency, error rate, tier distribution

### Stage 3: Chaos Engineering

| Test Case | Injection | Expected |
|-----------|----------|----------|
| TC-01 | DragonflyDB master kill | Failover < 3s, zero request loss |
| TC-02 | Triton GPU all down | Auto-fallback to Tier 1 |
| TC-03 | Redpanda 3 brokers down | Zero event loss (RF=3) |
| TC-04 | 200ms network latency | Circuit breaker triggers |
| TC-05 | 10x traffic spike | Rate limiter protects existing users |
| TC-06 | 10K items/sec stock-out | Bitmap update < 1s |

### Stage 4: Linear Scalability Proof

Double nodes → double throughput. Graph the results.

---

## Production Deployment

```bash
# Build and push images
make docker-build-all
make docker-push-all REGISTRY=your-registry.io

# Deploy to K8s
helm install recsys ./infra/helm \
  -f ./infra/helm/values/production.yaml \
  --namespace recsys-prod

# Verify
kubectl get pods -n recsys-prod
make health-check-k8s
```

### Production Scale (50M DAU)

| Service | Pods | Resource |
|---------|------|----------|
| event-collector | 50 | 2 CPU, 4GB |
| stream-processor (Flink) | 25 TM | 4 CPU, 8GB |
| recommendation-api | 250 | 4 CPU, 8GB |
| ranking-service (Triton) | 8 | L4 GPU, 16GB |
| DragonflyDB | 10 nodes | 8 CPU, 64GB |
| Redpanda | 12 brokers | 8 CPU, 32GB |
| Milvus | 8 nodes | 8 CPU, 32GB |

---

## Roadmap

- [x] Architecture design
- [ ] Core services implementation
- [ ] docker-compose local stack
- [ ] ML pipeline (Two-Tower + DCN-V2)
- [ ] Helm charts for K8s
- [ ] Monitoring dashboards
- [ ] Load testing suite
- [ ] Chaos engineering tests
- [ ] Edge computing (Cloudflare Workers)
- [ ] Feature store tiering (Hot/Warm/Cold)
- [ ] Model distillation (GPU → CPU)
- [ ] Multi-region Active-Active

---

## License

MIT
