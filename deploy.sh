#!/bin/bash
set -euo pipefail

# Deploy and install stratux-pusher on a Raspberry Pi running Stratux.
#
# Usage:
#   ./deploy.sh <pi-host> [awoslog-server] [source-name]
#
# Examples:
#   ./deploy.sh 192.168.0.119
#   ./deploy.sh 192.168.0.119 http://192.168.0.107:8080 stratux-home
#   ./deploy.sh pi@192.168.0.119 https://awoslog.com my-stratux

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PI_HOST="${1:-}"
AWOSLOG_SERVER="${2:-http://awoslog.com}"
SOURCE_NAME="${3:-stratux-home}"

if [ -z "$PI_HOST" ]; then
    echo "Usage: $0 <pi-host> [awoslog-server] [source-name]"
    echo ""
    echo "  pi-host         SSH target (e.g., 192.168.0.119 or pi@192.168.0.119)"
    echo "  awoslog-server   awoslog URL (default: http://awoslog.com)"
    echo "  source-name      source identifier (default: stratux-home)"
    exit 1
fi

# Add default user if not specified.
if [[ "$PI_HOST" != *@* ]]; then
    PI_HOST="pi@${PI_HOST}"
fi

BINARY="$SCRIPT_DIR/stratux-pusher"

if [ ! -f "$BINARY" ]; then
    echo "Binary not found. Building..."
    "$SCRIPT_DIR/build.sh"
fi

echo "=== Deploying Stratux Pusher ==="
echo "  Target:  $PI_HOST"
echo "  Server:  $AWOSLOG_SERVER"
echo "  Source:   $SOURCE_NAME"
echo ""

# 1. Copy binary
echo "[1/3] Copying binary to Pi..."
scp "$BINARY" "${PI_HOST}:/tmp/stratux-pusher"

# 2. Install binary and systemd service
echo "[2/3] Installing on Pi..."
ssh "$PI_HOST" bash -s -- "$AWOSLOG_SERVER" "$SOURCE_NAME" << 'REMOTE_SCRIPT'
set -euo pipefail

AWOSLOG_SERVER="$1"
SOURCE_NAME="$2"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="stratux-pusher"

sudo mv /tmp/stratux-pusher "$INSTALL_DIR/stratux-pusher"
sudo chmod 755 "$INSTALL_DIR/stratux-pusher"

echo "  Binary installed to $INSTALL_DIR/stratux-pusher"

# Install systemd service
sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null <<UNIT
[Unit]
Description=Stratux ADS-B Pusher for awoslog.com
After=network.target stratux.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/stratux-pusher -sbs localhost:30003 -server ${AWOSLOG_SERVER} -source ${SOURCE_NAME} -interval 3s
Restart=always
RestartSec=5s

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=stratux-pusher

[Install]
WantedBy=multi-user.target
UNIT

sudo systemctl daemon-reload
sudo systemctl enable "$SERVICE_NAME"
sudo systemctl restart "$SERVICE_NAME"

echo "  Service installed and started."
REMOTE_SCRIPT

# 3. Verify
echo "[3/3] Verifying..."
ssh "$PI_HOST" "systemctl is-active stratux-pusher && echo '  stratux-pusher is running.'"

echo ""
echo "=== Deployment complete ==="
echo ""
echo "Useful commands (on the Pi):"
echo "  sudo systemctl status stratux-pusher"
echo "  sudo journalctl -u stratux-pusher -f"
echo "  sudo systemctl stop stratux-pusher"
echo "  sudo systemctl restart stratux-pusher"
echo ""
echo "To uninstall:"
echo "  ssh $PI_HOST 'sudo systemctl stop stratux-pusher && sudo systemctl disable stratux-pusher && sudo rm /etc/systemd/system/stratux-pusher.service /usr/local/bin/stratux-pusher && sudo systemctl daemon-reload'"
