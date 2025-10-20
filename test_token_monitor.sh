#!/bin/bash

# Test script to verify token monitor starts correctly

echo "=========================================="
echo "Token Monitor Start Test"
echo "=========================================="
echo ""

# Check if geth binary exists
if [ ! -f "./build/bin/geth" ]; then
    echo "ERROR: geth binary not found at ./build/bin/geth"
    exit 1
fi

echo "Starting geth with token monitor..."
echo "Looking for these log messages:"
echo "  1. Token monitor configuration"
echo "  2. Starting token monitor..."
echo "  3. ZMQ publisher started"
echo "  4. Token monitor started successfully"
echo ""

# Start geth in dev mode with verbose logging
./build/bin/geth --dev --http --http.api eth,web3,net --verbosity 4 2>&1 | grep -i "token\|zmq" &

GETH_PID=$!

echo "Geth PID: $GETH_PID"
echo "Waiting 10 seconds for startup..."
sleep 10

echo ""
echo "Checking if port 5555 is listening..."
if lsof -i :5555 | grep -q LISTEN; then
    echo "✓ SUCCESS: Port 5555 is listening!"
    lsof -i :5555 | grep LISTEN
else
    echo "✗ FAILED: Port 5555 is NOT listening"
    echo ""
    echo "All listening ports:"
    lsof -i -P | grep LISTEN | grep geth
fi

echo ""
echo "Stopping geth..."
kill $GETH_PID 2>/dev/null
sleep 2
kill -9 $GETH_PID 2>/dev/null

echo "Test completed."
