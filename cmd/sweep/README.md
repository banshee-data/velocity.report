# Unified LiDAR Parameter Sweep Tool

A comprehensive parameter sweep tool that combines the functionality of `bg-sweep`, `bg-multisweep`, and `pcap-sweep` into a single command with flexible sweep modes.

## Build

```bash
go build -o app-sweep ./cmd/sweep
# or
make tools-local
```

## Usage Modes

### 1. Multi-Parameter Sweep (Default)

Test all combinations of noise, closeness, and neighbor parameters:

```bash
./app-sweep \
  -mode=multi \
  -noise=0.005,0.01,0.02 \
  -closeness=1.5,2.0,2.5 \
  -neighbors=0,1,2 \
  -iterations=30
```

### 2. Single-Variable Sweeps

#### Noise Sweep (fix closeness and neighbor)

```bash
./app-sweep \
  -mode=noise \
  -noise-start=0.005 \
  -noise-end=0.03 \
  -noise-step=0.005 \
  -fixed-closeness=2.0 \
  -fixed-neighbor=1
```

#### Closeness Sweep (fix noise and neighbor)

```bash
./app-sweep \
  -mode=closeness \
  -closeness-start=1.5 \
  -closeness-end=3.0 \
  -closeness-step=0.5 \
  -fixed-noise=0.01 \
  -fixed-neighbor=1
```

#### Neighbor Sweep (fix noise and closeness)

```bash
./app-sweep \
  -mode=neighbor \
  -neighbor-start=0 \
  -neighbor-end=3 \
  -neighbor-step=1 \
  -fixed-noise=0.01 \
  -fixed-closeness=2.0
```

### 3. PCAP Mode

Run sweeps using PCAP file replay instead of live data:

```bash
./app-sweep \
  -mode=multi \
  -pcap=/path/to/lidar-data.pcap \
  -pcap-settle=20s \
  -noise=0.005,0.01,0.02 \
  -closeness=1.5,2.0,2.5 \
  -neighbors=0,1,2
```

## Key Parameters

### Common Options

- `-monitor`: Server URL (default: `http://localhost:8081`)
- `-sensor`: Sensor ID (default: `hesai-pandar40p`)
- `-output`: Output CSV filename (default: `sweep-<mode>-<timestamp>.csv`)
- `-iterations`: Number of samples per parameter combo (default: 30)
- `-interval`: Time between samples (default: 2s)

### Sweep Mode

- `-mode`: One of `multi`, `noise`, `closeness`, `neighbor`

### Parameter Specification

#### For Multi-Mode

- `-noise`: Comma-separated values (e.g., `0.005,0.01,0.02`)
- `-closeness`: Comma-separated values (e.g., `1.5,2.0,2.5`)
- `-neighbors`: Comma-separated values (e.g., `0,1,2`)

#### For Single-Variable Modes

- Range parameters: `-noise-start`, `-noise-end`, `-noise-step`
- Range parameters: `-closeness-start`, `-closeness-end`, `-closeness-step`
- Range parameters: `-neighbor-start`, `-neighbor-end`, `-neighbor-step`
- Fixed parameters: `-fixed-noise`, `-fixed-closeness`, `-fixed-neighbor`

### PCAP Options

- `-pcap`: Path to PCAP file (enables PCAP mode)
- `-pcap-settle`: Wait time after PCAP replay (default: 20s)

### Other Options

- `-seed`: Seed behavior - `true`, `false`, or `toggle` (default: `true`)
- `-settle-time`: Wait for grid to settle in live mode (default: 5s)

## Output Files

The tool generates two CSV files:

1. **Summary file** (`sweep-<mode>-<timestamp>.csv`):

   - One row per parameter combination
   - Mean and stddev for each acceptance bucket
   - Mean and stddev for nonzero cells
   - Mean and stddev for overall acceptance rate

2. **Raw file** (`sweep-<mode>-<timestamp>-raw.csv`):
   - One row per sample iteration
   - All raw acceptance counts, reject counts, totals, and rates per bucket
   - Nonzero cell count and overall acceptance for each sample
   - Timestamps for time-series analysis

## Examples

### Quick multi-parameter sweep (live data)

```bash
./app-sweep -mode=multi -noise=0.01,0.02 -closeness=2.0,2.5 -neighbors=1,2 -iterations=10
```

### Detailed noise sweep with PCAP

```bash
./app-sweep \
  -mode=noise \
  -pcap=/Users/david/code/sensor_data/lidar/break-80k.pcapng \
  -noise-start=0.005 \
  -noise-end=0.02 \
  -noise-step=0.0025 \
  -fixed-closeness=2.5 \
  -fixed-neighbor=1 \
  -iterations=20 \
  -output=noise-sweep-detailed.csv
```

### Full parameter space exploration

```bash
./app-sweep \
  -mode=multi \
  -noise=0.005,0.0075,0.01,0.015,0.02 \
  -closeness=1.5,2.0,2.5,3.0 \
  -neighbors=0,1,2,3 \
  -iterations=50 \
  -interval=3s
```

## Migration from Old Tools

### From `bg-sweep`

```bash
# Old:
./app-bg-sweep -start=0.01 -end=0.3 -step=0.01

# New:
./app-sweep -mode=noise -noise-start=0.01 -noise-end=0.3 -noise-step=0.01 \
  -fixed-closeness=2.0 -fixed-neighbor=1
```

### From `bg-multisweep`

```bash
# Old:
./app-bg-multisweep -start=0.01 -end=0.02 -step=0.005 \
  -closeness=2.0,3.0 -neighbors=1,2

# New:
./app-sweep -mode=multi \
  -noise=0.01,0.015,0.02 -closeness=2.0,3.0 -neighbors=1,2
```

### From `pcap-sweep`

```bash
# Old:
./app-pcap-sweep -pcap=/path/to/file.pcap -samples=5 -settle=5s

# New:
./app-sweep -mode=multi -pcap=/path/to/file.pcap \
  -iterations=5 -pcap-settle=5s \
  -noise=0.005,0.01,0.02 -closeness=1.5,2.0,2.5 -neighbors=0,1,2
```
