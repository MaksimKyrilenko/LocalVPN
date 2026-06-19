#!/bin/bash
# Test MeshVPN connection

SERVER_URL="${1:-http://localhost:8080}"

echo "Testing MeshVPN Server at $SERVER_URL"
echo ""

# Test health endpoint
echo "1. Testing health endpoint..."
if curl -s "$SERVER_URL/health" | grep -q "healthy"; then
    echo "   ✓ Health check passed"
else
    echo "   ✗ Health check failed"
    exit 1
fi

# Test info endpoint
echo "2. Testing info endpoint..."
INFO=$(curl -s "$SERVER_URL/info")
if [ ! -z "$INFO" ]; then
    echo "   ✓ Server info received"
    echo "   STUN: $(echo $INFO | grep -o '"stun":"[^"]*"' | cut -d'"' -f4)"
else
    echo "   ✗ Failed to get server info"
fi

# Test WebSocket
echo "3. Testing WebSocket..."
if timeout 2 bash -c "(echo '{\"type\":\"ping\"}') | nc -w 2 $(echo $SERVER_URL | sed 's|http://||' | cut -d: -f1) $(echo $SERVER_URL | grep -o ':[0-9]*' | tr -d ':' || echo 80)" 2>/dev/null; then
    echo "   ✓ WebSocket port accessible"
else
    echo "   ⚠ WebSocket test inconclusive"
fi

# Test STUN
echo "4. Testing STUN..."
STUN_SERVER=$(echo $SERVER_URL | sed 's|http://||' | cut -d: -f1):3478
if command -v stun-client &> /dev/null; then
    if stun-client "$STUN_SERVER" &> /dev/null; then
        echo "   ✓ STUN server responding"
    else
        echo "   ✗ STUN server not responding"
    fi
else
    echo "   ⚠ stun-client not installed, skipping"
fi

echo ""
echo "Test complete!"
