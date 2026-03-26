#!/bin/bash
set -euo pipefail

# Build the stratux-pusher binary for ARM64 (Raspberry Pi).
# No CGO, no dependencies — just a static binary.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Building Stratux Pusher (ARM64) ==="

GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o stratux-pusher .
echo "Binary built: $(pwd)/stratux-pusher"
file stratux-pusher

echo ""
echo "=== Build complete ==="
echo ""
echo "Deploy to Stratux Pi:"
echo "  ./deploy.sh <pi-host> [source-name]"
echo ""
echo "Example:"
echo "  ./deploy.sh 192.168.0.119"
