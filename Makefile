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

# Install as systemd service
install: build
	sudo cp nostrhitch-daemon /usr/local/bin/
	sudo cp nostrhitch-daemon.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable nostrhitch-daemon

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

