#!/bin/bash

# Nostr Hitchhiking Bot Daemon Installation Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SERVICE_NAME="nostr-hitch-daemon"
INSTALL_DIR="/opt/nostrhitch"
SERVICE_USER="nostr"
SERVICE_GROUP="nostr"

echo -e "${GREEN}Nostr Hitchhiking Bot Daemon Installer${NC}"
echo "=============================================="

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo -e "${RED}This script must be run as root (use sudo)${NC}"
   exit 1
fi

# Check if systemd is available
if ! command -v systemctl &> /dev/null; then
    echo -e "${RED}systemd is not available on this system${NC}"
    exit 1
fi

# Create service user if it doesn't exist
if ! id "$SERVICE_USER" &>/dev/null; then
    echo -e "${YELLOW}Creating service user: $SERVICE_USER${NC}"
    useradd --system --no-create-home --shell /bin/false "$SERVICE_USER"
else
    echo -e "${GREEN}Service user $SERVICE_USER already exists${NC}"
fi

# Create installation directory
echo -e "${YELLOW}Creating installation directory: $INSTALL_DIR${NC}"
mkdir -p "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR/logs"
mkdir -p "$INSTALL_DIR/hitchmap-dumps"

# Copy files
echo -e "${YELLOW}Copying files to $INSTALL_DIR${NC}"
cp daemon.py "$INSTALL_DIR/"
cp hwrecentchanges.py "$INSTALL_DIR/"
cp nostrhitch.py "$INSTALL_DIR/"
cp settings.py "$INSTALL_DIR/"
cp requirements.txt "$INSTALL_DIR/"

# Set permissions
echo -e "${YELLOW}Setting permissions${NC}"
chown -R "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/daemon.py"

# Install Python dependencies
echo -e "${YELLOW}Installing Python dependencies${NC}"
if command -v pip3 &> /dev/null; then
    pip3 install -r "$INSTALL_DIR/requirements.txt"
else
    echo -e "${RED}pip3 not found. Please install Python dependencies manually:${NC}"
    echo "pip3 install -r $INSTALL_DIR/requirements.txt"
fi

# Install systemd service
echo -e "${YELLOW}Installing systemd service${NC}"
cp nostr-hitch-daemon.service /etc/systemd/system/
systemctl daemon-reload

# Enable service
echo -e "${YELLOW}Enabling service${NC}"
systemctl enable "$SERVICE_NAME"

echo -e "${GREEN}Installation completed successfully!${NC}"
echo ""
echo "Next steps:"
echo "1. Edit $INSTALL_DIR/settings.py with your configuration"
echo "2. Start the service: sudo systemctl start $SERVICE_NAME"
echo "3. Check status: sudo systemctl status $SERVICE_NAME"
echo "4. View logs: sudo journalctl -u $SERVICE_NAME -f"
echo ""
echo "Service management commands:"
echo "  Start:   sudo systemctl start $SERVICE_NAME"
echo "  Stop:    sudo systemctl stop $SERVICE_NAME"
echo "  Restart: sudo systemctl restart $SERVICE_NAME"
echo "  Status:  sudo systemctl status $SERVICE_NAME"
echo "  Logs:    sudo journalctl -u $SERVICE_NAME -f"
