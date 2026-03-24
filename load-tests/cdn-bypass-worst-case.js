import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';

// --- Metrics -----------------------------------------------------------

// Latency broken down by degradation-tier response.
const latencyByTier = new Trend('latency_by_tier');
const overallLatency = new Trend('overall_latency');

// Track how the system degrades under full load.
const tier1Rate = new Rate('tier1_rate');
const fallbackRate = new Rate('fallback_rate');
const errorRate = new Rate('error_rate');
const timeoutRate = new Rate('timeout_rate');

// Absolute counts for the summary report.
const tier1Count = new Counter('tier1_count');
const tier2Count = new Counter('tier2_count');
const tier3Count = new Counter('tier3_count');
const fallbackCount = new Counter('fallback_count');
const errorCount = new Counter('error_count');
const timeoutCount = new Counter('timeout_count');
const totalRequests = new Counter('total_requests');

// Track /api/v1/popular separately — this is Tier 0 (CDN-equivalent).
const popularLatency = new Trend('popular_endpoint_latency');
const popularErrorRate = new Rate('popular_error_rate');

// --- Config ------------------------------------------------------------

const BASE_URL = __ENV.BASE_URL || 'http://localhost:10000';

// 50M DAU ≈ ~580 RPS average, but peak hours can be 3-5x.
// CDN normally absorbs 70-85% of this. If CDN is down, backend takes all.
//
// This test simulates:
//   Phase 1: Normal load (CDN handles ~80%) — baseline
//   Phase 2: CDN failure — 100% traffic hits backend, sudden 5x spike
//   Phase 3: Sustained worst-case — full 50M DAU load on backend
//   Phase 4: Recovery probe — traffic drops as CDN comes back
//
// VU counts model requests-per-second at peak:
//   - Phase 1 baseline: ~500 VUs  (CDN absorbing 80%)
//   - Phase 2 spike:    ~2500 VUs (sudden CDN loss)
//   - Phase 3 sustained: ramp to 5000 → 10000 (full load, peak hours)
//   - Phase 4 recovery: drop back to 500

export const options = {
  scenarios: {
    // Scenario 1: Recommendation endpoint under full CDN-bypass load.
    cdn_bypass: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        // Phase 1: Baseline — CDN is healthy, only cache-miss traffic hits backend.
        { duration: '30s', target: 500 },
        { duration: '1m', target: 500 },

        // Phase 2: CDN failure — instant spike, all traffic hits backend.
        { duration: '10s', target: 2500 },
        { duration: '1m', target: 2500 },

        // Phase 3: Sustained worst-case — peak hour traffic, no CDN.
        { duration: '30s', target: 5000 },
        { duration: '2m', target: 5000 },
        { duration: '1m', target: 10000 },
        { duration: '2m', target: 10000 },

        // Phase 4: CDN recovery — traffic drops.
        { duration: '30s', target: 500 },
        { duration: '1m', target: 500 },

        // Cool-down.
        { duration: '30s', target: 0 },
      ],
      exec: 'recommendEndpoint',
    },

    // Scenario 2: Popular endpoint — the Tier 0 fallback that CDN would cache.
    // When CDN is down, clients may call this directly.
    popular_flood: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m30s', target: 0 },    // Wait for baseline phase to complete.
        { duration: '10s', target: 1000 },    // CDN fails — popular requests flood in.
        { duration: '3m', target: 3000 },     // Sustained popular endpoint pressure.
        { duration: '3m', target: 5000 },     // Peak popular requests.
        { duration: '30s', target: 100 },     // CDN recovers.
        { duration: '1m30s', target: 0 },
      ],
      exec: 'popularEndpoint',
    },
  },

  thresholds: {
    // Phase 1 (baseline): should meet normal SLOs.
    'overall_latency{phase:baseline}': ['p(95)<100', 'p(99)<200'],

    // Phase 2 (CDN failure spike): degraded but functional.
    // System should degrade gracefully — higher latency is OK, errors are not.
    'overall_latency{phase:cdn_failure}': ['p(95)<500', 'p(99)<1000'],

    // Phase 3 (sustained worst-case): expect graceful degradation.
    // At this point the degradation manager should kick in.
    'overall_latency{phase:sustained}': ['p(95)<1000', 'p(99)<2000'],

    // Must stay responsive even under worst-case load.
    // The system should fall back to popular items, not crash.
    error_rate: ['rate<0.05'],       // < 5% errors overall
    timeout_rate: ['rate<0.01'],     // < 1% timeouts

    // Degradation behavior: at peak load, system SHOULD use fallback.
    // If fallback rate is too low during peak, the degradation manager isn't working.
    // (This threshold validates graceful degradation is actually happening.)
    'fallback_rate{phase:sustained}': ['rate>0.30'],

    // Popular endpoint must stay fast — it's the last line of defense.
    popular_endpoint_latency: ['p(95)<50', 'p(99)<100'],
    popular_error_rate: ['rate<0.01'],
  },
};

// --- Helpers -----------------------------------------------------------

function getPhaseLabel(vuCount) {
  if (vuCount <= 500) return 'baseline';
  if (vuCount <= 2500) return 'cdn_failure';
  return 'sustained';
}

// Simulate realistic user distribution:
//   - 70% returning users (narrow ID range → likely to have cached recs)
//   - 30% new/long-tail users (wide ID range → cache miss → fallback)
function generateUserId() {
  if (Math.random() < 0.7) {
    // Returning users: narrow range, higher chance of Tier 1 cache hit.
    return `u_${Math.floor(Math.random() * 50000).toString().padStart(6, '0')}`;
  }
  // Long-tail users: wide range, almost guaranteed cache miss.
  return `u_${Math.floor(Math.random() * 50000000).toString().padStart(8, '0')}`;
}

// --- Scenario Functions ------------------------------------------------

// Hit the main recommendation endpoint — the one CDN normally shields.
export function recommendEndpoint() {
  const userId = generateUserId();
  const sessionId = `s_${__VU}_${__ITER}`;
  const phase = getPhaseLabel(__VU);

  const res = http.get(
    `${BASE_URL}/api/v1/recommend?user_id=${userId}&session_id=${sessionId}&limit=20`,
    {
      tags: { name: 'recommend', phase },
      timeout: '5s',
    },
  );

  totalRequests.add(1);

  const isTimeout = res.error && res.error.includes('timeout');
  timeoutRate.add(isTimeout);
  if (isTimeout) {
    timeoutCount.add(1);
    errorRate.add(true);
    errorCount.add(1);
    return;
  }

  const passed = check(res, {
    'status is 200': (r) => r.status === 200,
    'response is valid JSON': (r) => {
      try {
        JSON.parse(r.body);
        return true;
      } catch (_) {
        return false;
      }
    },
    'response has items': (r) => {
      try {
        return Array.isArray(JSON.parse(r.body).items);
      } catch (_) {
        return false;
      }
    },
  });

  errorRate.add(!passed);
  if (!passed) {
    errorCount.add(1);
    return;
  }

  overallLatency.add(res.timings.duration, { phase });

  let tier = 'unknown';
  try {
    tier = JSON.parse(res.body).tier || 'unknown';
  } catch (_) {
    return;
  }

  latencyByTier.add(res.timings.duration, { tier });

  const isTier1 = tier === 'tier1_precomputed';
  const isFallback = tier === 'fallback_popular';

  tier1Rate.add(isTier1);
  fallbackRate.add(isFallback, { phase });

  if (isTier1) {
    tier1Count.add(1);
  } else if (tier === 'tier2_rerank') {
    tier2Count.add(1);
  } else if (tier === 'tier3_inference') {
    tier3Count.add(1);
  } else if (isFallback) {
    fallbackCount.add(1);
  }
}

// Hit the popular endpoint — Tier 0, normally cached by CDN.
// When CDN is down, this becomes the hottest endpoint.
export function popularEndpoint() {
  const res = http.get(
    `${BASE_URL}/api/v1/popular?limit=20`,
    {
      tags: { name: 'popular' },
      timeout: '3s',
    },
  );

  const passed = check(res, {
    'popular: status 200': (r) => r.status === 200,
    'popular: has items': (r) => {
      try {
        return Array.isArray(JSON.parse(r.body).items);
      } catch (_) {
        return false;
      }
    },
    'popular: has cache headers': (r) =>
      r.headers['Cache-Control'] && r.headers['Cache-Control'].includes('public'),
  });

  popularLatency.add(res.timings.duration);
  popularErrorRate.add(!passed);
}
