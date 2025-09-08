# Lidar UDP Listener

This binary listens for lidar packets on UDP port 2369 (configurable), stores them in a dedicated lidar database, and optionally forwards packets to other applications like LidarView.

## Usage

```bash
# Build the binary
go build ./cmd/lidar

# Run with default settings (UDP port 2369, HTTP on :8080)
./lidar

# Run with custom UDP port
./lidar -udp-port 3000

# Run with custom UDP bind address
./lidar -udp-addr 192.168.1.100 -udp-port 2369

# Run with custom HTTP listen address
./lidar -listen :9090

# Enable packet parsing (uses embedded sensor config)
./lidar -parse

# Forward packets to LidarView on port 2368
./lidar -forward -forward-port 2368

# Forward packets to remote application
./lidar -forward -forward-addr 192.168.1.100 -forward-port 2370

# Run with parsing and forwarding enabled
./lidar -parse -forward -forward-port 2370
```

## Command Line Options

### Network Configuration
- `-udp-port int`: UDP port to listen for lidar packets (default: 2369)
- `-udp-addr string`: UDP bind address (default: listen on all interfaces)
- `-listen string`: HTTP listen address (default: ":8080")

### Packet Processing
- `-parse`: Parse lidar packets into points and store in database using embedded Pandar40P configuration (default: false)

### Packet Forwarding
- `-forward`: Forward received UDP packets to another port (default: false)
- `-forward-port int`: Port to forward UDP packets to (default: 2368)
- `-forward-addr string`: Address to forward UDP packets to (default: "localhost")

## Features

- **High-performance UDP packet reception**: Handles ~1430 packets/sec with optimized buffering
- **Built-in packet parsing**: Parse Pandar40P LiDAR packets into 3D points using embedded sensor configuration
- **Database storage**: Store raw packets and parsed points in SQLite database
- **Packet forwarding**: Forward packets to external applications like LidarView without blocking
- **HTTP monitoring interface**: Web interface showing packet statistics and configuration
- **Graceful shutdown**: Clean shutdown on SIGINT/SIGTERM with proper resource cleanup
- **Configurable networking**: Flexible UDP binding and forwarding options
- **Zero configuration**: Works out of the box with embedded Pandar40P sensor settings

## LidarView Integration

To use with LidarView for real-time visualization:

1. **Option 1: Forward to custom port**
   ```bash
   # Start lidar with forwarding to port 2370
   ./lidar -forward -forward-port 2370

   # Configure LidarView to listen on port 2370
   ```

2. **Option 2: Direct monitoring**
   ```bash
   # LidarView listens directly on port 2369 (no forwarding needed)
   # Our lidar binary receives packets and processes them in parallel
   ./lidar
   ```

**Note**: Avoid forwarding to port 2368 if LidarView expects to bind to that port as a server.

## Embedded Configuration

The binary includes embedded Pandar40P sensor configuration files (angle and firetime corrections), providing zero-configuration operation:

- **Zero Configuration**: Works out of the box with embedded Pandar40P configurations - no external files needed
- **Built-in Sensor Data**: Angle and firetime correction tables are compiled into the binary
- **Maintenance-free**: No risk of missing or corrupted configuration files
- **Portable**: Single binary contains everything needed for Pandar40P LiDAR processing

```bash
# Ready to use immediately with embedded configuration
./lidar -parse
```

## Performance

- **Packet Rate**: ~1430 packets/sec sustained throughput
- **Data Rate**: ~1.76 MB/sec (1762 KB/sec)
- **Forwarding Latency**: Microsecond-level with dedicated forwarding goroutine
- **Memory Usage**: Optimized buffering with 1000-packet forwarding buffer
- **CPU Usage**: Minimal overhead with direct packet processing (no per-packet goroutines)

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
