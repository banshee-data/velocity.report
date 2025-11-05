#!/usr/bin/env bash
#
# setup-radar-host.sh - Set up velocity.report server on a Raspberry Pi
#
# This script configures a Raspberry Pi (or similar ARM64 Linux system) to run
# the velocity.report radar server as a systemd service.
#
# Usage (run from within the cloned repository on the Pi):
#   1. Clone the repo: git clone https://github.com/banshee-data/velocity.report.git
#   2. cd velocity.report
#   3. Build the binary: make build-radar-linux
#   4. Run this script: sudo ./scripts/setup-radar-host.sh
#
# The script will:
#   - Copy the binary to /usr/local/bin/velocity-server
#   - Create a dedicated service user and working directory
#   - Install and enable the systemd service
#   - Optionally migrate existing database

set -euo pipefail

# Check if running with sudo
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run with sudo"
    echo "Usage: sudo ./scripts/setup-radar-host.sh"
    exit 1
fi

# Configuration
BINARY="app-radar-linux-arm64"
SERVICE_NAME="velocity-server"
INSTALL_PATH="/usr/local/bin/${SERVICE_NAME}"
DATA_DIR="/var/lib/velocity.report"
SERVICE_FILE="velocity-report.service"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Check prerequisites
if [ ! -f "${BINARY}" ]; then
    echo -e "${RED}[ERROR]${NC} Binary '${BINARY}' not found!"
    echo -e "${GREEN}[INFO]${NC} Run 'make build-radar-linux' first to build the binary."
    exit 1
fi

if [ ! -f "${SERVICE_FILE}" ]; then
    echo -e "${RED}[ERROR]${NC} Service file '${SERVICE_FILE}' not found!"
    echo -e "${GREEN}[INFO]${NC} Make sure you're running this from the repository root."
    exit 1
fi

log_info "Setting up velocity.report server on this host"
echo

# Step 1: Install binary
log_info "Step 1/4: Installing binary to ${INSTALL_PATH}..."
cp "${BINARY}" "${INSTALL_PATH}"
chown root:root "${INSTALL_PATH}"
chmod 0755 "${INSTALL_PATH}"

# Step 2: Create service user and working directory
log_info "Step 2/4: Creating service user and working directory..."
useradd --system --no-create-home --shell /usr/sbin/nologin velocity 2>/dev/null || true
mkdir -p "${DATA_DIR}"
chown velocity:velocity "${DATA_DIR}"

# Step 3: Install systemd service
log_info "Step 3/4: Installing systemd service..."
cp "${SERVICE_FILE}" "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}.service"

# Step 4: Ask about database migration
echo
log_info "Step 4/4: Database migration (optional)"
read -p "Do you have an existing sensor_data.db to migrate? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Enter path to sensor_data.db: " db_path
    if [ -f "${db_path}" ]; then
        log_info "Copying database to ${DATA_DIR}..."
        cp "${db_path}" "${DATA_DIR}/sensor_data.db"
        chown velocity:velocity "${DATA_DIR}/sensor_data.db"
        log_info "Database migrated successfully"
    else
        log_error "Database file not found: ${db_path}"
    fi
fi

# Start the service
echo
log_info "Starting ${SERVICE_NAME} service..."
systemctl restart "${SERVICE_NAME}.service"

# Show status
echo
log_info "Setup complete! Service status:"
systemctl status "${SERVICE_NAME}.service" --no-pager -l

echo
log_info "Useful commands:"
echo "  View logs:    sudo journalctl -u ${SERVICE_NAME}.service -f"
echo "  Check status: sudo systemctl status ${SERVICE_NAME}.service"
echo "  Restart:      sudo systemctl restart ${SERVICE_NAME}.service"
