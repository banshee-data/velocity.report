#!/bin/bash
# Test script for M1 visualiser features: Record and Replay
#
# This script demonstrates the recorder/replayer functionality:
# 1. Starts visualiser-server in synthetic mode with recording enabled
# 2. Records 60 seconds of synthetic data
# 3. Stops recording
# 4. Replays the recording via replay-server
#
# Usage: ./test-m1-replay.sh

set -e

RECORD_DURATION=60
LOG_PATH="/tmp/vrlog_test_$(date +%s)"

echo "=== M1 Visualiser Features Test: Record and Replay ==="
echo ""
echo "Step 1: Recording ${RECORD_DURATION} seconds of synthetic data..."
echo "Log path: ${LOG_PATH}"
echo ""

# Build the tools if needed
echo "Building tools..."
go build -o /tmp/visualiser-server ./cmd/tools/visualiser-server
go build -o /tmp/gen-vrlog ./cmd/tools/gen-vrlog
echo "✓ Tools built"
echo ""

# Start synthetic server in background
echo "Starting synthetic data generator..."
/tmp/visualiser-server -addr localhost:50051 -mode synthetic -rate 10 -points 5000 -tracks 10 &
SERVER_PID=$!

# Give server time to start
sleep 2

echo "✓ Server started (PID: ${SERVER_PID})"
echo ""

# TODO: Implement recording via StartRecording RPC
# For now, we'll use the recorder programmatically in a future commit
echo "⚠ Recording via gRPC RPC not yet implemented (M1 feature)"
echo "  For testing, you can:"
echo "  1. Use recorder.NewRecorder() programmatically in Go code"
echo "  2. Connect the macOS visualiser and use its recording feature"
echo ""

# Clean up
echo "Stopping server..."
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "=== Test Complete ==="
echo ""
echo "Next steps:"
echo "1. Implement StartRecording/StopRecording RPCs in Server"
echo "2. Add recording UI in macOS visualiser"
echo "3. Test full record/replay workflow"
echo ""
echo "To manually test replay:"
echo "  1. Create a .vrlog directory with recorded frames"
echo "  2. Run: /tmp/visualiser-server -mode replay -log <path-to-vrlog>"
echo "  3. Connect macOS visualiser to localhost:50051"
