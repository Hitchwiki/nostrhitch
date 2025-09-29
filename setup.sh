#!/bin/bash

# Nostr Hitchhiking Bot - Server Setup Script
# This script sets up the Go daemon on a server

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SERVICE_NAME="nostrhitch-daemon"
INSTALL_DIR="/opt/nostrhitch"
SERVICE_USER="nostr"
SERVICE_GROUP="nostr"
BINARY_NAME="nostrhitch-daemon"

echo -e "${BLUE}Nostr Hitchhiking Bot - Server Setup${NC}"
echo "======================================"
echo ""

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

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Go not found. Installing Go...${NC}"
    
    # Detect architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) GOARCH="amd64" ;;
        arm64|aarch64) GOARCH="arm64" ;;
        armv7l) GOARCH="armv6l" ;;
        *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
    esac
    
    # Download and install Go
    GO_VERSION="1.21.5"
    GO_TAR="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
    
    echo -e "${YELLOW}Downloading Go ${GO_VERSION}...${NC}"
    cd /tmp
    wget -q "https://go.dev/dl/${GO_TAR}"
    
    echo -e "${YELLOW}Installing Go...${NC}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "${GO_TAR}"
    rm "${GO_TAR}"
    
    # Add Go to PATH
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    export PATH=$PATH:/usr/local/go/bin
    
    echo -e "${GREEN}Go installed successfully${NC}"
else
    echo -e "${GREEN}Go found: $(go version)${NC}"
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

# Build the binary
echo -e "${YELLOW}Building Go daemon...${NC}"
if [ -f "main.go" ]; then
    go build -o "$BINARY_NAME" main.go
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build Go daemon${NC}"
        exit 1
    fi
    echo -e "${GREEN}Go daemon built successfully${NC}"
else
    echo -e "${RED}main.go not found. Please run this script from the project directory${NC}"
    exit 1
fi

# Copy files
echo -e "${YELLOW}Installing files...${NC}"
cp "$BINARY_NAME" "$INSTALL_DIR/"
cp "config.json.example" "$INSTALL_DIR/"

# Set permissions
echo -e "${YELLOW}Setting permissions...${NC}"
chown -R "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Create config file if it doesn't exist
if [ ! -f "$INSTALL_DIR/config.json" ]; then
    echo -e "${YELLOW}Creating config.json from example...${NC}"
    cp "$INSTALL_DIR/config.json.example" "$INSTALL_DIR/config.json"
    chown "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR/config.json"
    echo -e "${YELLOW}Please edit $INSTALL_DIR/config.json with your settings${NC}"
fi

# Install systemd service
echo -e "${YELLOW}Installing systemd service...${NC}"
cat > "/etc/systemd/system/$SERVICE_NAME.service" << EOF
[Unit]
Description=Nostr Hitchhiking Bot Daemon
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$BINARY_NAME
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=$INSTALL_DIR
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo -e "${GREEN}Installation completed successfully!${NC}"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo "1. Edit $INSTALL_DIR/config.json with your Nostr private key and settings"
echo "2. Start the service: sudo systemctl start $SERVICE_NAME"
echo "3. Check status: sudo systemctl status $SERVICE_NAME"
echo "4. View logs: sudo journalctl -u $SERVICE_NAME -f"
echo ""
echo -e "${BLUE}Service management commands:${NC}"
echo "  Start:   sudo systemctl start $SERVICE_NAME"
echo "  Stop:    sudo systemctl stop $SERVICE_NAME"
echo "  Restart: sudo systemctl restart $SERVICE_NAME"
echo "  Status:  sudo systemctl status $SERVICE_NAME"
echo "  Logs:    sudo journalctl -u $SERVICE_NAME -f"
echo ""
echo -e "${BLUE}Configuration file:${NC}"
echo "  $INSTALL_DIR/config.json"
echo ""
echo -e "${GREEN}Setup complete! The daemon is ready to run.${NC}"
