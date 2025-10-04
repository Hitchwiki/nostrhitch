#!/bin/bash

# Nostr Hitchhiking Bot Deployment Script
# This script helps deploy the bot to a server

set -e

echo "🚀 Nostr Hitchhiking Bot Deployment Script"
echo "=========================================="

# Check if we're in the right directory
if [ ! -f "main.go" ]; then
    echo "❌ Error: Please run this script from the project root directory"
    exit 1
fi

# Check if config.json exists
if [ ! -f "config.json" ]; then
    echo "❌ Error: config.json not found. Please create it first."
    echo "   Copy config.json.example and fill in your settings."
    exit 1
fi

# Function to deploy with Docker
deploy_docker() {
    echo "🐳 Deploying with Docker..."
    
    # Build the image
    echo "Building Docker image..."
    docker build -t nostrhitch:latest .
    
    # Stop existing container if running
    docker stop nostrhitch-daemon 2>/dev/null || true
    docker rm nostrhitch-daemon 2>/dev/null || true
    
    # Run the container
    echo "Starting container..."
    docker run -d \
        --name nostrhitch-daemon \
        --restart unless-stopped \
        -v "$(pwd)/config.json:/root/config.json:ro" \
        -v "$(pwd)/logs:/root/logs" \
        -v "$(pwd)/hitchmap-dumps:/root/hitchmap-dumps" \
        nostrhitch:latest
    
    echo "✅ Bot deployed with Docker!"
    echo "📊 View logs: docker logs -f nostrhitch-daemon"
    echo "🛑 Stop bot: docker stop nostrhitch-daemon"
}

# Function to deploy as systemd service
deploy_systemd() {
    echo "🔧 Deploying as systemd service..."
    
    # Build the binary
    echo "Building Go binary..."
    go build -o nostrhitch-daemon .
    
    # Create systemd service file
    sudo tee /etc/systemd/system/nostrhitch.service > /dev/null <<EOF
[Unit]
Description=Nostr Hitchhiking Bot
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$(pwd)
ExecStart=$(pwd)/nostrhitch-daemon
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd and start service
    sudo systemctl daemon-reload
    sudo systemctl enable nostrhitch
    sudo systemctl start nostrhitch
    
    echo "✅ Bot deployed as systemd service!"
    echo "📊 View logs: sudo journalctl -u nostrhitch -f"
    echo "🛑 Stop bot: sudo systemctl stop nostrhitch"
}

# Function to test deployment
test_deployment() {
    echo "🧪 Testing deployment..."
    
    # Build and run once
    go build -o nostrhitch-daemon .
    ./nostrhitch-daemon --once
    
    echo "✅ Test completed!"
}

# Main menu
echo ""
echo "Choose deployment method:"
echo "1) Docker (recommended)"
echo "2) Systemd service"
echo "3) Test only (run once)"
echo "4) Exit"
echo ""

read -p "Enter your choice (1-4): " choice

case $choice in
    1)
        deploy_docker
        ;;
    2)
        deploy_systemd
        ;;
    3)
        test_deployment
        ;;
    4)
        echo "👋 Goodbye!"
        exit 0
        ;;
    *)
        echo "❌ Invalid choice. Please run the script again."
        exit 1
        ;;
esac
