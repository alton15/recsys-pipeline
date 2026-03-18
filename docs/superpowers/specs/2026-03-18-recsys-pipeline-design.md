# Commerce Recommendation Pipeline — Design Spec

## Overview

50M DAU 커머스 서비스를 위한 개인 추천 시스템 참조 구현 (Reference Architecture Template).
클라우드 애그노스틱, docker-compose로 로컬 실행 가능, K8s Helm 차트로 프로덕션 배포 가능.

## Goals

- **500K RPS** 피크 트래픽 처리
- **p99 < 100ms** 응답 지연
- **월 ~1.5억 원** 인프라 비용 (50M DAU 기준)
- `docker-compose up` 한 번으로 전체 파이프라인 로컬 실행
- GitHub 공개 템플릿 — clone → run → 학습 가능

## Non-Goals

- 특정 클라우드 vendor lock-in (AWS/GCP 매니지드 서비스 미사용)
- 프론트엔드 UI
- 실제 상품 데이터 수집/크롤링

---

## Architecture

### 3-Tier Serving Strategy

핵심 설계 원칙: **실시간 GPU 추론을 전체 트래픽의 3% 이하로 제한**

| Tier | 대상 | 처리 방식 | 지연시간 | 트래픽 비중 |
|------|------|----------|---------|------------|
| Tier 0 | 비로그인, 신규 유저 | CDN 캐시 (인기/트렌딩) | < 1ms | 70% |
| Tier 1 | 로그인 유저 | DragonflyDB pre-computed 조회 + 품절 필터 | < 5ms | ~25.5% |
| Tier 2 | 세션 행동 있는 유저 | CPU GBDT 경량 re-ranking | < 20ms | ~3.6% |
| Tier 3 | 캐시 미스, 콜드스타트 | GPU 추론 (Two-Tower + DCN-V2, INT8) | < 80ms | ~0.9% |

### 4-Plane Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     DATA PLANE (서빙)                            │
│                                                                  │
│  [Users] → [CDN] → [Envoy Gateway] → [recommendation-api(Go)]  │
│                      rate limit         ├─ DragonflyDB 직접 조회 │
│                      circuit breaker    ├─ 로컬 GBDT re-rank    │
│                      adaptive conc.     ├─ Bitmap 품절 필터링    │
│                                         └─ Triton (Tier3 only)  │
└──────────────────────────┬──────────────────────────────────────┘
                           │ events
┌──────────────────────────┴──────────────────────────────────────┐
│                   STREAM PLANE (실시간)                           │
│                                                                  │
│  [event-collector(Go)] → [Redpanda]                             │
│                              ├─ [Flink] → 실시간 피처 → Dragonfly│
│                              ├─ [Flink] → 재고 비트맵 → Dragonfly│
│                              └─ [Flink] → 세션 이벤트 → Dragonfly│
└─────────────────────────────────────────────────────────────────┘
                           │ data lake
┌──────────────────────────┴──────────────────────────────────────┐
│                   BATCH PLANE (배치)                              │
│                                                                  │
│  [Spark] ← MinIO (이벤트 로그)                                   │
│     ├─ 피처 엔지니어링 → DragonflyDB bulk load                   │
│     ├─ 모델 학습 (PyTorch + DeepSpeed) → MLflow                 │
│     ├─ 후보 생성 (Two-Tower 임베딩) → Milvus ANN 인덱스          │
│     └─ Pre-compute 추천 → DragonflyDB (user별 Top-K)            │
│  스케줄: Airflow (일 1회 전체, 4시간 증분)                       │
└─────────────────────────────────────────────────────────────────┘
                           │
┌──────────────────────────┴──────────────────────────────────────┐
│                  CONTROL PLANE (관제)                             │
│                                                                  │
│  Prometheus + Grafana │ Jaeger │ Alertmanager │ A/B Platform     │
└─────────────────────────────────────────────────────────────────┘
```

### Fan-out 제거: Embedded Architecture

recommendation-api(Go) 내부에서 직접 처리:

- DragonflyDB 직접 조회 (feature + candidates + stock bitmap) → 1 네트워크 홉
- 로컬 GBDT re-ranking (CGo embedded) → 0 네트워크 홉
- Tier3일 때만 Triton gRPC 호출 → 1 네트워크 홉

결과: 네트워크 홉 6+ → 1~2, p99 57ms+ → 15ms (Tier1)

---

## Services

### 1. event-collector (Go)

이벤트 수집 API. 사용자 행동(클릭, 조회, 구매, 검색)을 수신하여 Redpanda에 발행.

- **언어**: Go
- **역할**: HTTP/gRPC 이벤트 수신, 스키마 검증, Redpanda 비동기 발행
- **단순 집계 내장**: 이벤트 카운팅, 최근 N개 버퍼링 (Flink 부하 경감)
- **성능 목표**: 인스턴스당 10K RPS
- **프로덕션 규모**: 50+ 파드

### 2. stream-processor (Java/Kotlin — Flink)

복잡한 윈도우 기반 실시간 피처 생성.

- **처리 내용**:
  - 슬라이딩 윈도우 피처 (30분/1시간/1일 집계)
  - 세션 분석 (세션 갭 감지, 세션 내 행동 시퀀스)
  - 재고 이벤트 → 품절 비트맵 갱신
  - 실시간 트렌드 스코어 계산
- **출력**: DragonflyDB에 피처 저장
- **프로덕션 규모**: 25 TaskManager

### 3. batch-processor (Python — Spark)

배치 피처 엔지니어링 + 모델 학습 + pre-compute.

- **피처 엔지니어링**: 유저/아이템 통계 피처, 교차 피처
- **모델 학습**: PyTorch + DeepSpeed (Two-Tower, DCN-V2)
- **후보 생성**: Two-Tower 임베딩 → Milvus ANN 인덱스 빌드
- **Pre-compute**: 전체 유저 × Top-K 후보 → DragonflyDB 적재
- **ONNX 변환**: 학습된 모델 → ONNX → TensorRT INT8 양자화
- **스케줄**: Airflow DAG (일 1회 전체, 4시간마다 증분)
- **인스턴스**: Spot/Preemptible

### 4. recommendation-api (Go)

최종 추천 API. 오케스트레이터 역할.

- **언어**: Go (성능 + 동시성)
- **Tier 라우팅 로직**:
  1. 유저 인증 확인 → 비로그인이면 Tier0 (CDN에서 이미 처리)
  2. DragonflyDB에서 pre-computed 결과 조회 (Tier1)
  3. 품절 비트맵으로 필터링 (O(1))
  4. 세션 행동 있으면 로컬 GBDT re-ranking (Tier2)
  5. 캐시 미스면 Triton gRPC 호출 (Tier3)
- **Graceful Degradation**: 각 Tier별 독립 circuit breaker
- **내부 통신**: gRPC (Tier3 Triton만), 나머지 임베디드
- **프로덕션 규모**: 250+ 파드 (HPA)

### 5. ranking-service (Python — Triton)

GPU 기반 딥러닝 모델 추론. Tier3 전용.

- **서빙**: NVIDIA Triton Inference Server
- **모델**: ONNX (TensorRT INT8 양자화)
- **Dynamic Batching**: 지연시간 vs throughput 최적화
- **프로덕션 규모**: L4 GPU 8장

### 6. traffic-simulator (Go)

부하 테스트 + 샘플 데이터 생성.

- 현실적 트래픽 패턴 시뮬레이션 (시간대별 분포, 세션 행동)
- k6 시나리오 + 커스텀 시뮬레이터
- Ramp-up: 1K → 10K → 50K → 100K → 500K RPS
- 샘플 유저/아이템 데이터 생성 스크립트

---

## Data Stores

| 저장소 | 용도 | 로컬 | 프로덕션 |
|--------|------|------|---------|
| **Redpanda** | 이벤트 스트리밍 | 1 브로커 | 12 브로커 |
| **DragonflyDB** | 피처 서빙, 추천 캐시, 품절 비트맵 | 1 노드 | 15 노드 |
| **Milvus** | ANN 벡터 검색 (후보 생성) | standalone | 30 노드 |
| **MinIO** | 오브젝트 스토리지 (이벤트 로그, 모델) | 1 노드 | 클러스터 |
| **PostgreSQL** | 메타데이터 (아이템 카탈로그, 유저 프로필) | 1 노드 | HA 클러스터 |
| **MLflow** | 모델 레지스트리, 실험 추적 | 로컬 | K8s 배포 |

---

## Real-time Inventory Pipeline

품절/가격 변동을 실시간으로 추천에 반영:

```
[재고 이벤트] → Redpanda(inventory-events)
                    → Flink
                        → DragonflyDB 품절 비트맵 갱신
                        → CDN Invalidation (해당 카테고리)

recommendation-api 조회 시:
  candidates = dragonfly.get(user:{id}:recommendations)
  stock_bitmap = dragonfly.getbit(stock:bitmap, item_ids)  // O(1)
  result = filter(candidates, stock_bitmap)
```

목표: 재고 변동 → 추천 반영까지 < 1초

---

## Backpressure & Graceful Degradation

```
트래픽 레벨    동작
──────────────────────────────────────────
Normal         Tier 0→1→2→3 정상 라우팅
Warning(150%)  Tier 3 비활성화, Tier 2 제한
Critical(200%) Tier 2 비활성화, Tier 1만
Emergency      전체 CDN 캐시 서빙 (static fallback)
```

- Envoy Adaptive Concurrency Limit
- 각 Tier 독립 Circuit Breaker (Hystrix 패턴)
- Rate Limiting: 유저당 + 글로벌

---

## Verification Strategy

### Stage 1: 단일 노드 벤치마크 (로컬)

- event-collector: 10K RPS/인스턴스 (wrk, hey)
- DragonflyDB: 1M+ ops/sec (redis-benchmark)
- ONNX Runtime INT8: < 10ms/request (perf_analyzer)
- recommendation-api E2E: Tier1 < 5ms, Tier2 < 20ms (k6)

### Stage 2: 분산 부하 테스트 (K8s)

- Ramp-up: 1K → 10K → 50K → 100K RPS
- p50/p95/p99 latency, error rate < 0.01%
- Tier별 트래픽 분배 비율 검증
- Kafka consumer lag 모니터링

### Stage 3: Chaos Engineering

- Chaos Mesh로 장애 주입
- TC-01: DragonflyDB 마스터 kill → failover < 3초
- TC-02: Triton 전체 down → Tier1 fallback
- TC-03: Redpanda 브로커 3대 down → 이벤트 유실 0
- TC-04: 네트워크 지연 200ms → circuit breaker 발동
- TC-05: Thundering Herd 10배 → rate limiter 작동
- TC-06: 품절 폭주 10K/sec → bitmap 갱신 < 1초

### Stage 4: 선형 확장성 증명

- 노드 2배 → 처리량 2배 검증
- 결과 그래프 시각화
- production.yaml 파드 수 산출 공식 도출

---

## Cost Summary

| 단계 | 월 비용 | 비고 |
|------|--------|------|
| 최적화 전 | ~4억 원 | Kafka + Redis + A100 |
| 1차 최적화 | ~1.5억 원 | Redpanda + DragonflyDB + L4 INT8 |
| 2차 최적화 | ~1.1억 원 | Edge + 피처 계층화 |
| 극한 최적화 | ~0.9억 원 | Student 모델 CPU 서빙 |

---

## Future Improvements

1. **Edge Computing**: Cloudflare Workers에 Tier1 로직 배포
2. **Feature Store 계층화**: Hot(Dragonfly)/Warm(ScyllaDB)/Cold(S3)
3. **Model Distillation**: Teacher(DCN-V2) → Student(MLP), GPU 제거
4. **Predictive Pre-warming**: 접속 패턴 학습, 사전 캐시 준비
5. **Multi-Region Active-Active**: 글로벌 확장

---

## Tech Stack Summary

| Category | Technology | Rationale |
|----------|-----------|-----------|
| API / Orchestrator | Go | 높은 동시성, 낮은 지연, 쉬운 배포 |
| Event Streaming | Redpanda | Kafka 호환, C++ 네이티브, 노드 70% 감소 |
| Stream Processing | Apache Flink | Exactly-once, 윈도우 기반 집계 |
| Batch Processing | Apache Spark | 대규모 피처 엔지니어링 |
| Cache / Feature Store | DragonflyDB | 멀티스레드, Redis 호환, 노드 85% 감소 |
| Vector Search | Milvus | 오픈소스 ANN, 분산 지원 |
| ML Training | PyTorch + DeepSpeed | 분산 학습, 커뮤니티 |
| Model Serving | NVIDIA Triton + TensorRT | Dynamic batching, INT8 양자화 |
| Workflow | Apache Airflow | 배치 파이프라인 오케스트레이션 |
| Model Registry | MLflow | 실험 추적, 모델 버저닝 |
| Object Storage | MinIO | S3 호환, 클라우드 애그노스틱 |
| Metadata DB | PostgreSQL | 안정성, 확장성 |
| API Gateway | Envoy | Adaptive concurrency, circuit breaker |
| Orchestration | Kubernetes + Helm | 수평 확장, 선언적 배포 |
| Monitoring | Prometheus + Grafana | 메트릭 수집 + 시각화 |
| Tracing | Jaeger | 분산 트레이싱 |
| Chaos Testing | Chaos Mesh | K8s 네이티브 장애 주입 |
| Load Testing | k6 | 스크립팅 가능, 분산 부하 |
