# Nostr Hitchhiking Bot

A Python bot that fetches hitchhiking data from Hitchmap and recent changes from Hitchwiki, then posts them as Nostr notes. These notes appear on https://notes.trustroots.org/ and other Nostr relays.

![nostrhitch](https://github.com/Hitchwiki/nostrhitch/blob/main/nostrhitch.jpg?raw=true)

## Features

- **Hitchmap Integration**: Fetches hitchhiking experiences from hitchmap.com and posts them as Nostr notes
- **Hitchwiki Integration**: Posts recent changes from Hitchwiki as social media-friendly Nostr notes
- **Daemon Mode**: Run continuously as a background service with configurable intervals
- **Duplicate Prevention**: Prevents posting duplicate content across restarts
- **Multiple Event Types**: Supports both kind 1 (social) and kind 30399 (hitchhiking-specific) events

## Quick Start

### 1. Setup

```bash
cp settings.py.example settings.py
```

Edit `settings.py` and add your `nsec` private key:

```python
nsec = "your_nsec_key_here"
post_to_relays = True
relays = ["wss://relay.hitchwiki.org"]
```

### 2. Install Dependencies

On Debian/Ubuntu you might need this:

```bash
apt install python3.XX-venv
```

Set up the virtual environment with Python 3.12:

```bash
python3 -m venv .venv
source venv/bin/activate
pip install -r requirements.txt
```

### 3. Run the Bot

#### Individual Scripts
```bash
# Run Hitchmap bot (posts hitchhiking experiences)
python nostrhitch.py

# Run Hitchwiki bot (posts recent changes)
python hwrecentchanges.py
```

#### Daemon Mode (Recommended)
```bash
# Run as daemon with default intervals
python daemon.py

# Run with custom intervals
python daemon.py --hw-interval 600 --hitchmap-interval 43200

# Debug mode
python daemon.py --debug

# Dry run (test without posting)
python daemon.py --dry-run
```

## Daemon Mode

The daemon combines both functionalities into a single, continuously running service:

- **Hitchwiki Recent Changes**: Posts recent changes from Hitchwiki to Nostr (configurable interval, default: 5 minutes)
- **Hitchmap Data**: Posts new hitchhiking experiences from Hitchmap to Nostr (configurable interval, default: 24 hours)
- **Graceful Shutdown**: Handles SIGINT and SIGTERM signals properly
- **Comprehensive Logging**: Logs to both file and console with configurable levels
- **Dry Run Mode**: Test the daemon without actually posting to relays
- **One-time Run**: Run tasks once and exit (useful for testing)

### Command Line Options

- `--hw-interval SECONDS`: Interval between Hitchwiki checks (default: 300)
- `--hitchmap-interval SECONDS`: Interval between Hitchmap checks (default: 86400)
- `--debug`: Enable debug logging
- `--dry-run`: Don't actually post to relays
- `--run-once`: Run each task once and exit

### System Service Installation

For production use, install as a systemd service:

```bash
sudo ./install_daemon.sh
```

This will:
- Create a `nostr` system user
- Install files to `/opt/nostrhitch`
- Install the systemd service
- Enable the service to start on boot

#### Service Management

```bash
# Start the service
sudo systemctl start nostr-hitch-daemon

# Stop the service
sudo systemctl stop nostr-hitch-daemon

# Restart the service
sudo systemctl restart nostr-hitch-daemon

# Check status
sudo systemctl status nostr-hitch-daemon

# View logs
sudo journalctl -u nostr-hitch-daemon -f

# Enable/disable auto-start
sudo systemctl enable nostr-hitch-daemon
sudo systemctl disable nostr-hitch-daemon
```

## Testing

Run the test suite to verify everything works:

```bash
python test_daemon.py
```

## Logging

Logs are written to:
- **File**: `logs/daemon_YYYYMMDD.log`
- **Console**: Standard output
- **System Journal**: When running as systemd service

Log levels:
- **INFO**: Normal operation messages
- **DEBUG**: Detailed debugging information (use `--debug` flag)
- **WARNING**: Non-critical issues
- **ERROR**: Errors that don't stop the daemon

## Configuration

### Settings

All settings are in `settings.py`:
- `nsec`: Your Nostr private key (nsec format)
- `post_to_relays`: Whether to actually post to relays
- `relays`: List of relay URLs to post to

### Intervals

- **Hitchwiki Interval**: How often to check for recent changes (default: 5 minutes)
- **Hitchmap Interval**: How often to check for new hitchhiking data (default: 24 hours)

## Troubleshooting

### Common Issues

1. **Permission Denied**: Make sure the daemon has write access to the logs and data directories
2. **Connection Errors**: Check your internet connection and relay URLs
3. **Key Errors**: Verify your nsec key is correct and properly formatted

### Debug Mode

Run with `--debug` to get detailed logging:

```bash
python daemon.py --debug
```

### Dry Run Mode

Test without posting to relays:

```bash
python daemon.py --dry-run --debug
```

### One-time Run

Test both tasks once:

```bash
python daemon.py --run-once --debug
```

## Status - 2024-11

The script is posting nightly from `npub12vz4acq3a94qpfc9kwp98wwtces5ej7n0h6d44pwc6dmucyvggyses5uyk`.

There's also hitchwiki notes, coming from `npub1zmd4ydxpmkqg9ztm6tfph0kyhqz36txs8cjtsxd22geqwl2y8k5s7x6qpm`, generated by https://github.com/Hitchwiki/hwpybot

Both of these are sending out kind 30399 events to relay.trustroots.org. See https://github.com/Trustroots/nostroots/blob/main/docs/Events.md
Not sure if there are currently any other apps besides https://github.com/trustroots/nostroots that can do something useful with these notes, besides e.g. https://lightningk0ala.github.io/nostr-wtf/query

We can consider sending out kind 1 notes, to make this hitch stuff visible in the nostr social media sphere.

## Architecture

The daemon runs two separate worker threads:

1. **Hitchwiki Worker**: Fetches recent changes and posts them as Nostr notes
2. **Hitchmap Worker**: Downloads and posts new hitchhiking experiences

Both workers run independently with their own intervals and error handling.

## Security

When installed as a system service:
- Runs as unprivileged `nostr` user
- Limited file system access
- No network binding capabilities
- Resource limits applied

## Monitoring

Monitor the daemon using:
- `systemctl status nostr-hitch-daemon`
- `journalctl -u nostr-hitch-daemon -f`
- Log files in `logs/` directory

## Development

For development and testing:
1. Use `--debug` for detailed logging
2. Use `--dry-run` to test without posting
3. Use `--run-once` to test both tasks quickly
4. Check logs for any issues
    
