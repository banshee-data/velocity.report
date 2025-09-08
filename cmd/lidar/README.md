# Lidar UDP Listener

This binary listens for lidar packets on UDP port 2369 (configurable) and logs them to a dedicated lidar database.

## Usage

```bash
# Build the binary
make build-lidar-local

# Run with default settings (UDP port 2369, HTTP on :8080)
./lidar-local

# Run with custom UDP port
./lidar-local -udp-port 3000

# Run with custom UDP bind address
./lidar-local -udp-addr 192.168.1.100 -udp-port 2369

# Run in development mode
./lidar-local -dev

# Run with custom HTTP listen address
./lidar-local -listen :9090
```

## Command Line Options

- `-udp-port int`: UDP port to listen for lidar packets (default: 2369)
- `-udp-addr string`: UDP bind address (default: listen on all interfaces)
- `-listen string`: HTTP listen address (default: ":8080")
- `-dev`: Run in dev mode

## Features

- Listens for UDP packets on the specified port
- Provides HTTP API for data access
- Graceful shutdown on SIGINT/SIGTERM
- Concurrent packet handling to avoid blocking
