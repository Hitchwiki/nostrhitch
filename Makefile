.PHONY: build run test clean install

# Build the daemon
build:
	go build -o nostrhitch-daemon main.go

# Run the daemon
run: build
	./nostrhitch-daemon

# Run in dry-run mode
dry-run: build
	./nostrhitch-daemon -dry-run

# Run once for testing
once: build
	./nostrhitch-daemon -once

# Run with debug
debug: build
	./nostrhitch-daemon -debug

# Test the daemon
test: build
	./nostrhitch-daemon -once -debug

# Clean build artifacts
clean:
	rm -f nostrhitch-daemon

# Install as systemd service (quick)
install: build
	sudo cp nostrhitch-daemon /usr/local/bin/
	sudo cp nostrhitch-daemon.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable nostrhitch-daemon

# Full server setup (recommended)
setup: build
	sudo ./setup.sh

# Quick install (one command)
quick-install:
	sudo ./install.sh

# Uninstall systemd service
uninstall:
	sudo systemctl stop nostrhitch-daemon
	sudo systemctl disable nostrhitch-daemon
	sudo rm -f /usr/local/bin/nostrhitch-daemon
	sudo rm -f /etc/systemd/system/nostrhitch-daemon.service
	sudo systemctl daemon-reload

# Service management
start:
	sudo systemctl start nostrhitch-daemon

stop:
	sudo systemctl stop nostrhitch-daemon

restart:
	sudo systemctl restart nostrhitch-daemon

status:
	sudo systemctl status nostrhitch-daemon

logs:
	sudo journalctl -u nostrhitch-daemon -f

# Show recent logs
log-recent:
	sudo journalctl -u nostrhitch-daemon --since "1 hour ago"

# Show configuration
config:
	@echo "Configuration file: /opt/nostrhitch/config.json"
	@if [ -f "/opt/nostrhitch/config.json" ]; then \
		echo "Current configuration:"; \
		sudo cat /opt/nostrhitch/config.json | jq . 2>/dev/null || sudo cat /opt/nostrhitch/config.json; \
	else \
		echo "Configuration file not found. Run 'make setup' first."; \
	fi

# Edit configuration
edit-config:
	sudo nano /opt/nostrhitch/config.json

# Test configuration
test-config:
	./nostrhitch-daemon -once -debug

