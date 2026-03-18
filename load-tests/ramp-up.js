import http from 'k6/http';
import { check } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const latency = new Trend('recommend_latency');
const errorRate = new Rate('recommend_errors');

const BASE_URL = __ENV.BASE_URL || 'http://localhost:10000';

// Step-by-step ramp: 1K -> 10K -> 50K -> 100K concurrent VUs.
// Each plateau holds for 2 minutes to measure steady-state performance.
export const options = {
  stages: [
    // Warm-up: ramp to 1K
    { duration: '30s', target: 1000 },
    { duration: '2m', target: 1000 },

    // Scale to 10K
    { duration: '1m', target: 10000 },
    { duration: '2m', target: 10000 },

    // Scale to 50K
    { duration: '2m', target: 50000 },
    { duration: '2m', target: 50000 },

    // Peak at 100K
    { duration: '2m', target: 100000 },
    { duration: '2m', target: 100000 },

    // Cool-down
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    // 1K stage: aggressive latency targets
    'recommend_latency{stage:1k}': ['p(95)<50', 'p(99)<100'],
    // 10K stage: slightly relaxed
    'recommend_latency{stage:10k}': ['p(95)<100', 'p(99)<200'],
    // 50K stage: moderate
    'recommend_latency{stage:50k}': ['p(95)<200', 'p(99)<500'],
    // 100K stage: peak load tolerance
    'recommend_latency{stage:100k}': ['p(95)<500', 'p(99)<1000'],

    http_req_failed: ['rate<0.05'],
    recommend_errors: ['rate<0.05'],
  },
};

function getStageLabel(vuCount) {
  if (vuCount <= 1000) return '1k';
  if (vuCount <= 10000) return '10k';
  if (vuCount <= 50000) return '50k';
  return '100k';
}

export default function () {
  const userId = `u_${Math.floor(Math.random() * 500000).toString().padStart(6, '0')}`;
  const sessionId = `s_${__VU}_${__ITER}`;

  const res = http.get(
    `${BASE_URL}/api/v1/recommend?user_id=${userId}&session_id=${sessionId}&limit=20`,
    { tags: { name: 'recommend' } },
  );

  const stageLabel = getStageLabel(__VU);

  const passed = check(res, {
    'status is 200': (r) => r.status === 200,
    'body is valid JSON': (r) => {
      try {
        JSON.parse(r.body);
        return true;
      } catch (_) {
        return false;
      }
    },
    'response has items array': (r) => {
      try {
        return Array.isArray(JSON.parse(r.body).items);
      } catch (_) {
        return false;
      }
    },
  });

  latency.add(res.timings.duration, { stage: stageLabel });
  errorRate.add(!passed);
}
