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

The configuration uses a flat schema that matches the `/api/lidar/params` endpoint, allowing the same JSON structure to be used for both startup configuration and runtime parameter updates:

```json
{
  "noise_relative": 0.04,
  "seed_from_first": true,
  "buffer_timeout": "500ms",
  "min_frame_points": 1000,
  "flush_interval": "60s",
  "flush_disable": false
}
```

### Parameters

#### Background Configuration

- **noise_relative** (float64, default: 0.04): Fraction of range treated as expected measurement noise (e.g., 0.04 = 4%). Must be between 0 and 1.
- **closeness_multiplier** (float64, optional): Sensitivity multiplier for closeness testing.
- **neighbor_confirmation_count** (int, optional): Number of neighboring cells required to confirm foreground.
- **seed_from_first** (bool, default: true): Seed background cells from first observation (useful for PCAP replay and development runs).
- **warmup_duration_nanos** (int64, optional): Warmup duration in nanoseconds.
- **warmup_min_frames** (int, optional): Minimum frames for warmup period.
- **post_settle_update_fraction** (float64, optional): Update fraction after settling period.
- **foreground_min_cluster_points** (int, optional): Minimum points for DBSCAN clustering.
- **foreground_dbscan_eps** (float64, optional): DBSCAN epsilon parameter for clustering.

#### Frame Builder Configuration

- **buffer_timeout** (string, default: "500ms"): Finalise idle frames after this duration. Valid duration strings like "250ms", "500ms", "1s".
- **min_frame_points** (int, default: 1000): Minimum points required for a valid frame before finalising.

#### Flush Configuration

- **flush_interval** (string, default: "60s"): Interval to flush background grid to database. Valid duration strings like "60s", "2m", "1h".
- **flush_disable** (bool, default: false): Disable periodic background grid flushing to database (reduces CPU/IO during development).

#### Tracker Configuration (Optional)

- **gating_distance_squared** (float64, optional): Gating distance squared for data association.
- **process_noise_pos** (float64, optional): Process noise for position.
- **process_noise_vel** (float64, optional): Process noise for velocity.
- **measurement_noise** (float64, optional): Measurement noise parameter.
- **occlusion_cov_inflation** (float64, optional): Covariance inflation during occlusion.
- **hits_to_confirm** (int, optional): Hits required to confirm a track.
- **max_misses** (int, optional): Maximum consecutive misses before track deletion.
- **max_misses_confirmed** (int, optional): Maximum misses for confirmed tracks.

### Creating Custom Configurations

To create a custom configuration:

1. Copy `tuning.defaults.json` to a new file (e.g., `tuning.custom.json`)
2. Modify the values as needed
3. Use the `--config` flag to specify your custom configuration:

```bash
./velocity-report --config config/tuning.custom.json --enable-lidar
```

### Partial Configurations

The configuration system supports partial configs. You can specify only the parameters you want to override, and the rest will use built-in defaults:

```json
{
  "noise_relative": 0.06,
  "flush_interval": "120s"
}
```

### Precedence

When `--config` is specified, the JSON file is loaded on top of the built-in defaults, so fields omitted from the file keep their default values.

When `--config` is **not** specified, built-in defaults are used.

The configuration file is the **only** way to set initial tuning parameters. Runtime updates can be made via the `/api/lidar/params` endpoint.
