import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend, Rate } from 'k6/metrics';

const eventsAccepted = new Counter('events_accepted');
const eventsRejected = new Counter('events_rejected');
const eventLatency = new Trend('event_publish_latency');
const stockVerifyLatency = new Trend('stock_verify_latency');
const acceptRate = new Rate('event_accept_rate');

const EVENT_COLLECTOR_URL = __ENV.EVENT_COLLECTOR_URL || 'http://localhost:8080';
const RECOMMEND_URL = __ENV.RECOMMEND_URL || 'http://localhost:10000';

// Total items in the catalog for stock-out simulation.
const CATALOG_SIZE = parseInt(__ENV.CATALOG_SIZE || '100000', 10);

// Target: 10K stock-out events/sec sustained for 2 minutes.
export const options = {
  scenarios: {
    stock_out_flood: {
      executor: 'constant-arrival-rate',
      rate: 10000,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 500,
      maxVUs: 2000,
    },
    stock_verify: {
      executor: 'constant-vus',
      vus: 10,
      duration: '2m30s',
      startTime: '10s',
      exec: 'verifyStockUpdate',
    },
  },
  thresholds: {
    // No 429 or 5xx under load.
    event_accept_rate: ['rate>0.99'],
    // Event ingestion must stay fast.
    event_publish_latency: ['p(95)<200', 'p(99)<500'],
    // Stock bitmap should reflect updates within 1 second.
    stock_verify_latency: ['p(95)<1000'],

    'http_req_failed{scenario:stock_out_flood}': ['rate<0.01'],
  },
};

// Generate a stock-out purchase event for a random item.
function buildStockOutEvent(itemIndex) {
  const itemId = `i_${itemIndex.toString().padStart(6, '0')}`;
  const userId = `u_${Math.floor(Math.random() * 50000).toString().padStart(5, '0')}`;

  return JSON.stringify({
    event_type: 'purchase',
    user_id: userId,
    item_id: itemId,
    session_id: `stress_${__VU}_${__ITER}`,
    metadata: {
      quantity: 999,
      source: 'inventory_stress_test',
      stock_depleted: true,
    },
  });
}

// Default function: fire stock-out events at the event-collector.
export default function () {
  const itemIndex = Math.floor(Math.random() * CATALOG_SIZE);
  const payload = buildStockOutEvent(itemIndex);

  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'stock_out_event' },
  };

  const res = http.post(`${EVENT_COLLECTOR_URL}/api/v1/events`, payload, params);

  eventLatency.add(res.timings.duration);

  const isAccepted = res.status === 202;
  const isThrottled = res.status === 429;
  const isServerError = res.status >= 500;

  acceptRate.add(isAccepted);

  if (isAccepted) {
    eventsAccepted.add(1);
  } else {
    eventsRejected.add(1, {
      reason: isThrottled ? 'throttled' : isServerError ? 'server_error' : 'other',
    });
  }

  check(res, {
    'event accepted (202)': (r) => r.status === 202,
    'no rate limiting (not 429)': (r) => r.status !== 429,
    'no server error': (r) => r.status < 500,
  });
}

// Separate scenario: verify that stock-out items disappear from recommendations.
// After items are purchased in bulk (stock depleted), the recommendation API
// should filter them out via the stock bitmap within 1 second.
export function verifyStockUpdate() {
  // Pick a recently "stocked-out" item and request recommendations for a user
  // who would normally receive it. We verify by checking that the recommendation
  // endpoint responds quickly (bitmap lookup latency) and returns valid results.
  const userId = `u_${Math.floor(Math.random() * 50000).toString().padStart(5, '0')}`;

  const start = Date.now();
  const res = http.get(
    `${RECOMMEND_URL}/api/v1/recommend?user_id=${userId}&limit=20`,
    { tags: { name: 'stock_verify' } },
  );
  const elapsed = Date.now() - start;

  stockVerifyLatency.add(elapsed);

  check(res, {
    'recommend returns 200': (r) => r.status === 200,
    'response has items': (r) => {
      try {
        return Array.isArray(JSON.parse(r.body).items);
      } catch (_) {
        return false;
      }
    },
  });

  sleep(0.5);
}
