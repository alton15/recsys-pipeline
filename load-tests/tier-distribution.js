import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';

// Per-tier counters to track distribution.
const tier1Count = new Counter('tier1_precomputed_count');
const tier2Count = new Counter('tier2_rerank_count');
const tier3Count = new Counter('tier3_inference_count');
const fallbackCount = new Counter('fallback_popular_count');
const unknownTierCount = new Counter('unknown_tier_count');
const totalRequests = new Counter('total_requests');

// Rates for threshold checks.
const tier1Rate = new Rate('tier1_rate');
const tier2Rate = new Rate('tier2_rate');
const tier3Rate = new Rate('tier3_rate');

const BASE_URL = __ENV.BASE_URL || 'http://localhost:10000';

// Moderate sustained load to collect statistically significant tier distribution.
export const options = {
  stages: [
    { duration: '15s', target: 200 },
    { duration: '3m', target: 500 },
    { duration: '15s', target: 0 },
  ],
  thresholds: {
    // Expected distribution: ~85% Tier 1, ~12% Tier 2, ~3% Tier 3.
    // Use relaxed bounds to account for variance:
    //   Tier 1: at least 75%
    //   Tier 2: at most 20%
    //   Tier 3: at most 10%
    tier1_rate: ['rate>0.75'],
    tier2_rate: ['rate<0.20'],
    tier3_rate: ['rate<0.10'],

    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  // Mix of users with and without sessions:
  //   - ~60% have a session_id (may trigger Tier 2 re-ranking)
  //   - ~40% have no session_id (stays Tier 1 or fallback)
  const userId = `u_${Math.floor(Math.random() * 100000).toString().padStart(5, '0')}`;
  const includeSession = Math.random() < 0.6;
  const sessionId = includeSession ? `s_${__VU}_${__ITER}` : '';

  let url = `${BASE_URL}/api/v1/recommend?user_id=${userId}&limit=20`;
  if (sessionId) {
    url += `&session_id=${sessionId}`;
  }

  const res = http.get(url);

  check(res, {
    'status is 200': (r) => r.status === 200,
  });

  if (res.status !== 200) {
    return;
  }

  let tier;
  try {
    const body = JSON.parse(res.body);
    tier = body.tier || 'unknown';
  } catch (_) {
    return;
  }

  totalRequests.add(1);

  const isTier1 = tier === 'tier1_precomputed';
  const isTier2 = tier === 'tier2_rerank';
  const isTier3 = tier === 'tier3_inference';

  // Record rates for threshold evaluation.
  tier1Rate.add(isTier1);
  tier2Rate.add(isTier2);
  tier3Rate.add(isTier3);

  // Record absolute counts for summary output.
  if (isTier1) {
    tier1Count.add(1);
  } else if (isTier2) {
    tier2Count.add(1);
  } else if (isTier3) {
    tier3Count.add(1);
  } else if (tier === 'fallback_popular') {
    fallbackCount.add(1);
  } else {
    unknownTierCount.add(1);
  }
}
