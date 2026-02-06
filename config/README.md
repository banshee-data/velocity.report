# Configuration System

This directory contains configuration files for tuning the velocity.report system.

## LiDAR Tuning Configuration

The `tuning.defaults.json` file contains default tuning parameters for LiDAR components.

### Usage

You can use the configuration file with the radar binary:

```bash
./velocity-report --config config/tuning.defaults.json --enable-lidar
```

### Configuration Structure

```json
{
  "lidar": {
    "background": {
      "noise_relative_fraction": 0.04,
      "flush_interval": "60s",
      "flush_disable": false,
      "seed_from_first": true
    },
    "frame_builder": {
      "buffer_timeout": "500ms",
      "min_frame_points": 1000
    }
  }
}
```

### Parameters

#### Background Configuration

- **noise_relative_fraction** (float64, default: 0.04): Fraction of range treated as expected measurement noise (e.g., 0.04 = 4%). Must be between 0 and 1.
- **flush_interval** (string, default: "60s"): Interval to flush background grid to database. Valid duration strings like "60s", "2m", "1h".
- **flush_disable** (bool, default: false): Disable periodic background grid flushing to database (reduces CPU/IO during development).
- **seed_from_first** (bool, default: true): Seed background cells from first observation (useful for PCAP replay and development runs).

#### Frame Builder Configuration

- **buffer_timeout** (string, default: "500ms"): Finalise idle frames after this duration. Valid duration strings like "250ms", "500ms", "1s".
- **min_frame_points** (int, default: 1000): Minimum points required for a valid frame before finalising.

### Creating Custom Configurations

To create a custom configuration:

1. Copy `tuning.defaults.json` to a new file (e.g., `tuning.custom.json`)
2. Modify the values as needed
3. Use the `--config` flag to specify your custom configuration:

```bash
./velocity-report --config config/tuning.custom.json --enable-lidar
```

### Backward Compatibility

The configuration file system is optional. If you don't specify `--config`, the existing CLI flags will be used:

- `--lidar-bg-noise-relative`
- `--lidar-bg-flush-interval`
- `--lidar-bg-flush-disable`
- `--lidar-seed-from-first`
- `--lidar-frame-buffer-timeout`
- `--lidar-min-frame-points`

**Note:** When `--config` is specified, it takes precedence over individual CLI flags.
