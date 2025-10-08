# Nostr Hitchhiking Bot

A Go daemon that publishes Hitchwiki and Hitchmap content to Nostr relays.

## Quick Start

### Local Development
```bash
# Build
go build -o nostrhitch-daemon main.go

# Configure
cp config.json.example config.json
# Edit config.json with your nsec key and relay URLs

# Run
./nostrhitch-daemon -once          # Test once
./nostrhitch-daemon -dry-run       # Test without posting
./nostrhitch-daemon                # Run daemon
```

### Docker
```bash
# Run with Docker
docker-compose up -d

# View logs
docker-compose logs -f

# Reload after config changes
docker-compose restart
```

## Configuration

Edit `config.json`:
```json
{
  "nsec": "your_nsec_key_here",
  "post_to_relays": true,
  "relays": ["wss://relay.hitchwiki.org"],
  "hw_interval": 300,
  "hitch_interval": 86400,
  "secret_hitchwiki_url": "http://hitchwiki-alternative.example.net"
}
```

## Features

- **Multi-language**: Fetches from 17 Hitchwiki language versions
- **Duplicate Prevention**: Two-tier system prevents reposting
- **Geo Tags**: Automatic location detection and plus codes
- **Persistent Storage**: SQLite database survives container restarts
- **NIP-05 Verification**: Automatic profile management

## Duplicate Prevention

The bot uses a robust two-tier system to avoid posting duplicates:

### 1. Session Tracking
- **In-memory map**: Tracks notes posted during current session
- **Immediate prevention**: Skips already processed entries

### 2. Relay-based Tracking  
- **Startup scan**: Queries all relays for existing notes at startup
- **Persistent detection**: Uses RSS entry IDs and database record IDs as unique keys
- **Cross-session**: Prevents duplicates even after bot restarts

### 3. Unique Identifiers
- **Hitchwiki**: Uses RSS entry ID (full diff URL) as unique key
- **Hitchmap**: Uses database record ID with `hitchmap_` prefix
- **Nostr tags**: `r` tag contains reference URL for future duplicate detection

## Data Sources

- **Hitchwiki**: Recent changes from RSS feeds (17 languages)
- **Hitchmap**: Geographic hitchhiking data from SQLite database
- **Tags**: `#hitchhiking`, `#hitchwiki`, `#hitchmap`, geo coordinates

## Commands

```bash
# Local
./nostrhitch-daemon -once          # Run once
./nostrhitch-daemon -debug         # Debug mode
./nostrhitch-daemon -dry-run       # Test mode

# Docker
docker-compose up -d               # Start daemon
docker-compose down                # Stop daemon
docker-compose restart             # Reload config
docker-compose logs -f             # View logs
```

## File Structure

```
nostrhitch/
├── main.go                 # Main daemon
├── config.json             # Configuration
├── docker-compose.yml      # Docker setup
├── hitchmap-dumps/         # SQLite databases (persistent)
└── logs/                   # Log files
```