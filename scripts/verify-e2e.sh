#!/bin/bash
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
echo " Event ingestion: OK"

echo "=== Testing recommendations ==="
RESULT=$(curl -sf "http://localhost:8090/api/v1/recommend?user_id=u_00001&limit=10")
echo "Recommendation result: $RESULT"

echo "=== Testing popular endpoint ==="
POPULAR=$(curl -sf "http://localhost:8090/api/v1/popular?limit=10")
echo "Popular result: $POPULAR"

echo "=== All checks passed ==="
