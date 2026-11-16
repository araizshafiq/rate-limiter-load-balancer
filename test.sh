#!/bin/bash

# Test script for Rate Limiting & Load Balancing System

set -e

BASE_URL="http://localhost"
REQUEST_COUNT=15

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Testing Rate Limiting & Load Balancing System"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Test 1: Check if services are running
echo -e "\n[TEST 1] Checking service health..."
echo "Rate Limiter health:"
curl -s "$BASE_URL:8080/health" | jq '.' || echo "Rate Limiter not responding"

echo -e "\nLoad Balancer health:"
curl -s "$BASE_URL:8081/health" | jq '.' || echo "Load Balancer not responding"

# Test 2: First request (no cookie)
echo -e "\n[TEST 2] First request (no cookie)..."
echo "Making request to test assignment..."
RESPONSE=$(curl -s -i "$BASE_URL/api/test?id=1" 2>&1)
echo "$RESPONSE" | head -20
COOKIE=$(echo "$RESPONSE" | grep -i "set-cookie" | grep "backend_assignment" | sed 's/.*backend_assignment=\([A-C]\).*/\1/')
echo "Assigned backend: $COOKIE"

# Test 3: Multiple requests with cookie (within rate limit)
echo -e "\n[TEST 3] Testing requests within rate limit..."
echo "Making 8 requests with rate limit of 10 req/sec..."
for i in {1..8}; do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/test?id=$i" -b "backend_assignment=$COOKIE")
  echo "Request $i: HTTP $STATUS"
done

# Test 4: Exceed rate limit
echo -e "\n[TEST 4] Exceeding rate limit..."
echo "Making requests that should trigger rate limit (total > 10)..."
for i in {1..12}; do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/test?id=$i" -b "backend_assignment=$COOKIE")
  if [ $STATUS == "429" ]; then
    echo "Request $i: HTTP $STATUS ✗ (Rate limited)"
  else
    echo "Request $i: HTTP $STATUS ✓"
  fi
done

# Test 5: Check logs
echo -e "\n[TEST 5] Checking log files..."
if [ -f "logs/cookie-$COOKIE.log" ]; then
  echo "Log file found: logs/cookie-$COOKIE.log"
  echo "First 5 lines:"
  head -5 "logs/cookie-$COOKIE.log"
  echo "..."
  echo "Last 5 lines:"
  tail -5 "logs/cookie-$COOKIE.log"
else
  echo "No log file found for cookie-$COOKIE.log"
fi

# Test 6: Check backend load distribution
echo -e "\n[TEST 6] Backend load distribution..."
curl -s "$BASE_URL:8081/health" | jq '.loads'

# Test 7: Test multiple cookies (different IPs simulate different clients)
echo -e "\n[TEST 7] Testing multiple cookies (load distribution)..."
for BACKEND in A B C; do
  # Force assign by using different header
  RESPONSE=$(curl -s -c /tmp/cookie-$BACKEND.txt "$BASE_URL/api/test?backend=$BACKEND")
  echo "Testing backend $BACKEND..."
done

echo -e "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Testing complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "\nLog files location: ./logs/"
echo "Check logs with: tail -f logs/cookie-*.log"
echo "Check Redis with: docker exec research-redis redis-cli"
