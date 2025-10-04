# Deployment Guide

This guide covers different ways to deploy the Nostr Hitchhiking Bot to a server.

## Prerequisites

- Go 1.21+ installed
- Docker (for containerized deployment)
- A server with internet access
- `config.json` configured with your settings

## Quick Start

1. **Clone the repository**:
   ```bash
   git clone https://github.com/Hitchwiki/nostrhitch.git
   cd nostrhitch
   ```

2. **Configure the bot**:
   ```bash
   cp config.json.example config.json
   # Edit config.json with your settings
   ```

3. **Run the deployment script**:
   ```bash
   ./deploy.sh
   ```

## Deployment Options

### Option 1: Docker (Recommended)

**Best for**: Easy deployment, consistent environment, easy updates

```bash
# Build and run with Docker
docker build -t nostrhitch:latest .
docker run -d \
  --name nostrhitch-daemon \
  --restart unless-stopped \
  -v "$(pwd)/config.json:/root/config.json:ro" \
  -v "$(pwd)/logs:/root/logs" \
  -v "$(pwd)/hitchmap-dumps:/root/hitchmap-dumps" \
  nostrhitch:latest
```

**Or use docker-compose**:
```bash
docker-compose up -d
```

### Option 2: Systemd Service

**Best for**: Native Linux deployment, full control

```bash
# Build the binary
go build -o nostrhitch-daemon .

# Create systemd service
sudo cp nostrhitch-daemon.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable nostrhitch
sudo systemctl start nostrhitch
```

### Option 3: Manual Run

**Best for**: Testing, development

```bash
# Build and run
go build -o nostrhitch-daemon .
./nostrhitch-daemon
```

## Server Requirements

### Minimum Requirements
- **CPU**: 1 core
- **RAM**: 512MB
- **Storage**: 1GB
- **OS**: Linux (Ubuntu 20.04+ recommended)

### Recommended Requirements
- **CPU**: 2 cores
- **RAM**: 1GB
- **Storage**: 5GB
- **OS**: Ubuntu 22.04 LTS

## Cloud Provider Recommendations

### Budget-Friendly Options
- **DigitalOcean**: $6/month droplet
- **Linode**: $5/month nanode
- **Vultr**: $6/month cloud compute
- **Hetzner**: €4.15/month cloud server

### Enterprise Options
- **AWS EC2**: t3.micro (free tier eligible)
- **Google Cloud**: e2-micro
- **Azure**: B1s

## Monitoring and Maintenance

### View Logs
```bash
# Docker
docker logs -f nostrhitch-daemon

# Systemd
sudo journalctl -u nostrhitch -f

# Manual
tail -f logs/daemon.log
```

### Update the Bot
```bash
# Pull latest changes
git pull origin main

# Rebuild and restart
docker-compose down
docker-compose up -d --build

# Or for systemd
go build -o nostrhitch-daemon .
sudo systemctl restart nostrhitch
```

### Health Checks
```bash
# Check if bot is running
docker ps | grep nostrhitch
# or
sudo systemctl status nostrhitch
```

## Troubleshooting

### Common Issues

1. **Permission denied on config.json**
   ```bash
   chmod 644 config.json
   ```

2. **Port already in use**
   - Check what's using the port: `lsof -i :8080`
   - Kill the process or change the port

3. **Out of disk space**
   ```bash
   # Clean up old logs
   find logs/ -name "*.log" -mtime +30 -delete
   
   # Clean up old hitchmap dumps
   find hitchmap-dumps/ -name "*.sqlite" -mtime +7 -delete
   ```

4. **Bot not posting to relays**
   - Check network connectivity
   - Verify relay URLs in config.json
   - Check logs for errors

### Log Analysis
```bash
# Search for errors
grep -i error logs/daemon.log

# Check recent activity
tail -n 100 logs/daemon.log

# Monitor in real-time
tail -f logs/daemon.log | grep -E "(ERROR|WARN|Posted|Skipping)"
```

## Security Considerations

1. **File Permissions**: Ensure config.json is not world-readable
2. **Firewall**: Only open necessary ports
3. **Updates**: Keep the system and dependencies updated
4. **Monitoring**: Set up log monitoring and alerts
5. **Backups**: Regular backups of configuration and logs

## Performance Tuning

1. **Memory Usage**: Monitor with `htop` or `docker stats`
2. **Disk Usage**: Set up log rotation
3. **Network**: Ensure stable internet connection
4. **CPU**: Monitor during peak times

## Support

- **Issues**: GitHub Issues
- **Documentation**: This README and code comments
- **Community**: Hitchwiki Discord/Forum
