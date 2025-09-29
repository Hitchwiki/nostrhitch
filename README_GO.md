# Nostr Hitchhiking Bot - Go Version

A minimal, efficient Go daemon that combines Hitchwiki and Hitchmap functionality into a single binary.

## Features

- **Single Binary**: No dependencies, no package management issues
- **Concurrent Processing**: Both tasks run simultaneously using goroutines
- **Minimal Code**: ~300 lines total, DRY principles throughout
- **Easy Deployment**: Just copy one file
- **Built-in Logging**: Simple, effective logging
- **Configuration**: JSON config file
- **Systemd Ready**: Works perfectly with systemd

## Quick Start

### 1. Build

```bash
make build
```

### 2. Configure

Edit `config.json`:

```json
{
  "nsec": "your_nsec_key_here",
  "post_to_relays": true,
  "relays": ["wss://relay.hitchwiki.org"],
  "hw_interval": 300,
  "hitch_interval": 86400
}
```

### 3. Run

```bash
# Run daemon
make run

# Test once
make once

# Debug mode
make debug

# Dry run
make dry-run
```

## Installation

### System Service

```bash
# Install as systemd service
make install

# Start service
make start

# Check status
make status

# View logs
make logs
```

### Manual Installation

```bash
# Build
make build

# Copy to system
sudo cp nostrhitch-daemon /usr/local/bin/
sudo cp config.json /opt/nostrhitch/
sudo chown -R nostr:nostr /opt/nostrhitch

# Create service user
sudo useradd --system --no-create-home --shell /bin/false nostr

# Install systemd service
sudo cp nostrhitch-daemon.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable nostrhitch-daemon
sudo systemctl start nostrhitch-daemon
```

## Configuration

All settings in `config.json`:

- `nsec`: Your Nostr private key
- `post_to_relays`: Whether to actually post
- `relays`: List of relay URLs
- `hw_interval`: Hitchwiki check interval (seconds)
- `hitch_interval`: Hitchmap check interval (seconds)
- `debug`: Enable debug logging
- `dry_run`: Test mode

## Command Line Options

- `-config`: Configuration file (default: config.json)
- `-debug`: Enable debug logging
- `-dry-run`: Don't post to relays
- `-once`: Run once and exit

## Architecture

The daemon uses a simple, efficient design:

1. **Main Goroutine**: Handles signals and coordinates tasks
2. **Hitchwiki Goroutine**: Fetches and posts recent changes
3. **Hitchmap Goroutine**: Fetches and posts hitchhiking data
4. **Shared State**: Thread-safe duplicate checking

## Code Structure

- **main.go**: Single file with all functionality
- **Config**: JSON-based configuration
- **NostrClient**: Handles all Nostr operations
- **Daemon**: Manages both tasks concurrently
- **Helper Functions**: DRY utility functions

## Advantages Over Python

1. **No Dependencies**: Single binary, no package management
2. **Better Performance**: Lower memory usage, faster execution
3. **Easier Deployment**: Just copy one file
4. **Concurrent by Design**: Natural goroutine usage
5. **System Integration**: Better systemd compatibility
6. **Cross Platform**: Easy to build for different architectures

## Monitoring

```bash
# Check status
make status

# View logs
make logs

# Restart if needed
make restart
```

## Development

```bash
# Test changes
make test

# Debug issues
make debug

# Clean build
make clean && make build
```

## File Sizes

- **Go Binary**: ~8MB (statically linked)
- **Python Version**: ~50MB+ (with dependencies)
- **Deployment**: Single file vs multiple files + dependencies

The Go version is significantly simpler, more reliable, and easier to deploy than the Python version.

