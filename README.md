# Nostr Hitchhiking Bot

A minimal, efficient Go daemon that publishes Hitchwiki and Hitchmap content to Nostr relays.

## Features

- **Single Binary**: No dependencies, ~8MB statically linked
- **Multi-language Support**: Fetches from all 17 Hitchwiki language versions
- **Concurrent Processing**: Both tasks run simultaneously using goroutines
- **Duplicate Prevention**: Relay-based duplicate checking
- **NIP-05 Verification**: Automatic profile management
- **Easy Deployment**: Just copy one file
- **Systemd Ready**: Works perfectly with systemd

## Quick Start

### 1. Build
```bash
make build
```

### 2. Configure
```bash
cp config.json.example config.json
```

Edit `config.json`:
```json
{
  "nsec": "your_nsec_key_here",
  "post_to_relays": true,
  "relays": ["wss://relay.hitchwiki.org"],
  "hw_interval": 300,
  "hitch_interval": 86400,
  "secret_hitchwiki_url": "http://hitchwiki-other-access.example.net"
}
```

**Note**: The `secret_hitchwiki_url` option allows you to use an alternative domain to bypass Cloudflare rate limiting. This should be kept private and not committed to the repository. All links in posted notes will still use the public hitchwiki.org domain.

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

### Quick Setup (Recommended)
```bash
# One-command setup
sudo ./setup.sh

# Or use make
make setup
```

### Manual Installation
```bash
make build
make install
make start
make status
```

## Configuration

All settings in `config.json`:

| Setting | Description | Default |
|---------|-------------|---------|
| `nsec` | Your Nostr private key | Required |
| `post_to_relays` | Whether to actually post | `true` |
| `relays` | List of relay URLs | `["wss://relay.hitchwiki.org"]` |
| `hw_interval` | Hitchwiki check interval (seconds) | `300` |
| `hitch_interval` | Hitchmap check interval (seconds) | `86400` |
| `secret_hitchwiki_url` | Alternative Hitchwiki domain (bypass Cloudflare) | `""` |
| `debug` | Enable debug logging | `false` |
| `dry_run` | Test mode | `false` |

## Command Line Options

| Flag | Description |
|------|-------------|
| `-config` | Configuration file (default: config.json) |
| `-debug` | Enable debug logging |
| `-dry-run` | Don't post to relays |
| `-once` | Run once and exit |
| `-force-post` | Force post 5 Hitchwiki and 5 Hitchmap notes |
| `-disable-duplicate-check` | Disable duplicate checking |

## Data Sources

### Hitchwiki Integration
- **Languages**: 17 language versions (en, de, es, fr, ru, etc.)
- **Format**: RSS/Atom feeds
- **Content**: Recent changes, filtered for bot edits
- **Tags**: `#hitchhiking`, `#hitchwiki`, geo tags when available

### Hitchmap Integration
- **Source**: SQLite database with geographic data
- **Content**: Hitchhiking spot information
- **Tags**: `#hitchhiking`, `#hitchmap`, `#map-notes`, geo tags

## Nostr Integration

### Event Structure
- **Kind**: 1 (text note)
- **Content**: Markdown with hashtags
- **Tags**: 
  - `r` tag: Reference URL for duplicate checking
  - `summary` tag: Full content summary
  - `#hitchhiking`: Primary hashtag
  - Geo tags: `g`, `L`, `l` for coordinates and plus codes

### Duplicate Prevention
- Query existing notes from relays at startup
- Maintain in-memory set of posted content IDs
- Check against existing content before posting
- Use RSS item IDs and database record IDs as unique identifiers

## Service Management

```bash
# Start/stop/restart service
make start
make stop
make restart

# Check status and logs
make status
make logs

# Configuration management
make config          # Show current config
make edit-config     # Edit config file
make test-config     # Test configuration
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

## Architecture

The daemon uses a simple, efficient design:

1. **Main Goroutine**: Handles signals and coordinates tasks
2. **Hitchwiki Goroutine**: Fetches and posts recent changes from all languages
3. **Hitchmap Goroutine**: Fetches and posts hitchhiking data from SQLite
4. **Shared State**: Thread-safe duplicate checking

## Dependencies

### Go Modules
- `github.com/nbd-wtf/go-nostr` - Nostr protocol implementation
- `github.com/mattn/go-sqlite3` - SQLite database driver
- `github.com/btcsuite/btcd/btcutil/bech32` - Bech32 encoding
- `github.com/google/open-location-code/go` - Plus codes
- `github.com/mmcloughlin/geohash` - Geohash encoding

### External Services
- **Hitchwiki**: RSS feeds from multiple language versions
- **Hitchmap**: SQLite database updates
- **Nostr Relays**: WebSocket connections for publishing

## System Requirements

- **OS**: Linux (systemd support)
- **Go**: 1.21+ (for building)
- **Memory**: 50MB+ available
- **Storage**: 100MB+ for databases and logs
- **Network**: Internet access for data sources and relays

## File Structure

```
nostrhitch/
├── main.go                 # Main daemon
├── config.json.example     # Configuration template
├── Makefile               # Build and deployment commands
├── setup.sh              # Automated installation
├── nostrhitch-daemon.service # systemd service file
├── hitchmap-dumps/        # SQLite database files
└── logs/                  # Log files
```

## Performance

- **Memory**: ~10-20MB typical usage
- **CPU**: Low, event-driven processing
- **Network**: Minimal, only during data fetching and posting
- **Storage**: ~50MB for SQLite databases

## Security

- **Private Key**: Stored in configuration file (nsec format)
- **Access**: File system permissions required
- **Data Validation**: Input sanitization and output validation
- **Error Handling**: Graceful failure modes

## Monitoring

- **Log Levels**: Info, Debug, Error
- **Metrics**: Posts published, errors encountered, data source status
- **Relay Connectivity**: Connection status tracking