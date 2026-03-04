#!/bin/bash
# Test iOS Shortcuts integration with OpenClaw hooks
# Usage: ./test-ios-hooks.sh [server] [token]

SERVER="${1:-localhost:18789}"
TOKEN="${2:-test-token}"

echo "Testing OpenClaw hooks endpoint..."
echo "Server: $SERVER"
echo ""

# Test 1: Wake endpoint
echo "=== Test 1: Wake endpoint ==="
curl -s -X POST "http://$SERVER/hooks/wake" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"Hello from iOS test","mode":"now"}' | jq . 2>/dev/null || echo "(jq not available)"

echo ""
echo ""

# Test 2: Agent endpoint (async)
echo "=== Test 2: Agent endpoint ==="
curl -s -X POST "http://$SERVER/hooks/agent" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Say hello from iOS test","agentId":"task-manager","wakeMode":"now"}' | jq . 2>/dev/null || echo "(jq not available)"

echo ""
echo ""

# Test 3: Invalid token
echo "=== Test 3: Invalid token (should fail) ==="
curl -s -X POST "http://$SERVER/hooks/agent" \
  -H "Authorization: Bearer invalid-token" \
  -H "Content-Type: application/json" \
  -d '{"message":"test"}'

echo ""
echo ""

# Test 4: Missing agentId
echo "=== Test 4: Default agent (no agentId) ==="
curl -s -X POST "http://$SERVER/hooks/agent" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Hello with default agent","wakeMode":"now"}' | jq . 2>/dev/null || echo "(jq not available)"

echo ""
