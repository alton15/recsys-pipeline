import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const tierCounter = new Counter('tier_distribution');
const latencyByTier = new Trend('latency_by_tier');

const BASE_URL = __ENV.BASE_URL || 'http://localhost:10000';

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

  const res = http.get(`${BASE_URL}/api/v1/recommend?user_id=${userId}&limit=20`);

  check(res, {
    'status is 200': (r) => r.status === 200,
    'has items': (r) => {
      try {
        return JSON.parse(r.body).items.length > 0;
      } catch (_) {
        return false;
      }
    },
  });

  if (res.status === 200) {
    try {
      const body = JSON.parse(res.body);
      const tier = body.tier || 'unknown';
      tierCounter.add(1, { tier });
      latencyByTier.add(res.timings.duration, { tier });
    } catch (_) {
      // Response parsing failed; skip metric recording.
    }
  }
}
