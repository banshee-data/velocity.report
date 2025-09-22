# Lidar UDP Listener

This binary listens for lidar packets on UDP port 2369 (configurable), processes them using embedded Pandar40P configuration, stores statistics, and optionally forwards packets to other applications like LidarView.

## Usage

```bash
# Build the binary
go build ./cmd/lidar

# Run with default settings (UDP port 2369, HTTP on :8081, parsing enabled)
./lidar

# Run with custom UDP port
./lidar -udp-port 3000

# Run with custom UDP bind address
./lidar -udp-addr 192.168.1.100 -udp-port 2369

# Run with custom HTTP listen address
./lidar -listen :9090

# Disable packet parsing
./lidar -no-parse

# Forward packets to LidarView on port 2368
./lidar -forward -forward-port 2368

# Forward packets to remote application
./lidar -forward -forward-addr 192.168.1.100 -forward-port 2370

# Run with custom logging interval
./lidar -log-interval 10

# Run with custom receive buffer size
./lidar -rcvbuf 8388608  # 8MB buffer

# Run with forwarding enabled and custom log interval
./lidar -forward -forward-port 2370 -log-interval 5
```

## Command Line Options

### Network Configuration
- `-udp-port int`: UDP port to listen for lidar packets (default: 2369)
- `-udp-addr string`: UDP bind address (default: listen on all interfaces)
- `-listen string`: HTTP listen address (default: ":8081")
- `-rcvbuf int`: UDP receive buffer size in bytes (default: 4MB)

### Packet Processing
- `-no-parse`: Disable lidar packet parsing (parsing is enabled by default)

### Packet Forwarding
- `-forward`: Forward received UDP packets to another port (default: false)
- `-forward-port int`: Port to forward UDP packets to (default: 2368)
- `-forward-addr string`: Address to forward UDP packets to (default: "localhost")

### Monitoring and Logging
- `-log-interval int`: Statistics logging interval in seconds (default: 2)
- `-db string`: Path to the SQLite database file (default: "lidar_data.db")

## Architecture

The application is organized into separate components in the `./internal/lidar` directory for better maintainability:

### Core Components

- **`forwarder.go`**: Handles asynchronous packet forwarding with drop counting and colored logging
- **`listener.go`**: Manages UDP packet reception, parsing, and statistics collection
- **`webserver.go`**: Provides HTTP interface with embedded status page and health checks
- **`parser.go`**: Pandar40P LiDAR packet parsing with embedded calibration data
- **`stats.go`**: Thread-safe statistics tracking with real-time snapshots
- **`status.html`**: Embedded web template for real-time monitoring interface

### Benefits

- **Modular Design**: Each component has a single responsibility
- **Testable**: Components can be tested in isolation
- **Maintainable**: Changes are isolated to specific functionality
- **Reusable**: Components can be used independently

## Features

- **High-performance UDP packet reception**: Handles ~1800 packets/sec with optimized buffering
- **Built-in packet parsing**: Parse Pandar40P LiDAR packets into 3D points using embedded sensor configuration (enabled by default)
- **Real-time statistics**: Configurable logging intervals with colored output for dropped packets
- **Non-blocking packet forwarding**: Dedicated forwarding goroutine prevents receive loop blocking
- **HTTP monitoring interface**: Web interface showing packet statistics, uptime, and configuration
- **Graceful shutdown**: Clean shutdown on SIGINT/SIGTERM with proper resource cleanup
- **Configurable networking**: Flexible UDP binding, buffer sizes, and forwarding options
- **Zero configuration**: Works out of the box with embedded Pandar40P sensor settings
- **Embedded resources**: All templates and configurations built into the binary

## LidarView Integration

To use with LidarView for real-time visualization:

1. **Option 1: Forward to LidarView's default port (2368)**
   ```bash
   # Start lidar with forwarding to LidarView's default port
   ./lidar -forward

   # LidarView will receive packets on its default port 2368
   ```

2. **Option 2: Forward to custom port (if port 2368 is in use)**
   ```bash
   # Start lidar with forwarding to a custom port
   ./lidar -forward -forward-port 2370

   # Configure LidarView to listen on port 2370 instead of 2368
   ```

**Note**: Port 2368 is LidarView's default listening port. Use a custom port only if there's a conflict or you need multiple LidarView instances.

## Embedded Configuration

The binary includes embedded Pandar40P sensor configuration files (angle and firetime corrections), providing zero-configuration operation:

- **Zero Configuration**: Works out of the box with embedded Pandar40P configurations - no external files needed
- **Built-in Sensor Data**: Angle and firetime correction tables are compiled into the binary
- **Maintenance-free**: No risk of missing or corrupted configuration files
- **Portable**: Single binary contains everything needed for Pandar40P LiDAR processing

```bash
# Ready to use immediately with embedded configuration (parsing enabled by default)
./lidar

# Disable parsing if you only need packet forwarding
./lidar -no-parse -forward
```

## Performance

- **Packet Rate**: ~1800 packets/sec sustained throughput
- **Data Rate**: ~2.17 MB/sec with ~700k points/sec when parsing enabled
- **Point Processing**: ~700,000 3D points/sec from parsed Pandar40P packets
- **Forwarding Latency**: Microsecond-level with dedicated forwarding goroutine
- **Memory Usage**: Optimized buffering with 1000-packet forwarding buffer and configurable UDP receive buffer
- **CPU Usage**: Minimal overhead with direct packet processing (no per-packet goroutines)
- **Statistics Logging**: Configurable intervals (1-60 seconds) with colored output for errors

## Database Schema

The lidar database (`lidar_data.db`) contains:

- **lidar_sessions**: Recording sessions with metadata
- **lidar_points**: Parsed 3D points with coordinates, intensity, timestamps
- Automatic schema initialization and migration support

## HTTP Endpoints

- `GET /`: Status page showing configuration and packet statistics
- `GET /health`: Health check endpoint returning JSON status

## Troubleshooting

### Port Conflicts
- **Error**: "bind: address already in use"
- **Solution**: Check for other processes using the UDP port with `lsof -i UDP:2369`

### LidarView Socket Errors
- **Error**: "Error while opening socket!" in LidarView
- **Solution**: Use different forwarding ports (avoid 2368 if LidarView binds to it)

### No Packets Received
- **Check**: Firewall settings, UDP port configuration, network interface binding
- **Debug**: Use `netstat -un` to verify UDP listener is active
