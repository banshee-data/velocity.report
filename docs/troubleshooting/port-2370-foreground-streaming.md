# Port 2370 Foreground Streaming Troubleshooting Guide

> **Note:** For the current investigation status and known issues regarding data quality and corruption, please see [Foreground Corruption Investigation Status](foreground-corruption-investigation-status.md).

## Overview

Port 2370 should stream foreground-only LIDAR points extracted via background subtraction during real-time PCAP replay. This document outlines what's implemented and what to check if no data appears on port 2370.

## Implementation Summary

### What's Implemented

#### 1. ForegroundForwarder (`internal/lidar/network/foreground_forwarder.go`)

- **Purpose**: Encodes and forwards foreground points as Pandar40P UDP packets
- **Port**: 2370 (configurable)
- **Buffer**: 100-frame buffer for non-blocking operation
- **Encoding**: Pandar40P format (1262 bytes per packet, up to 400 points per packet)

**Key Functions:**

- `NewForegroundForwarder(addr, port, config)`: Creates forwarder instance
- `Start(ctx)`: Starts forwarding goroutine
- `ForwardForeground(points)`: Queues foreground points for transmission
- `encodePointsAsPackets(points)`: Converts polar points to Pandar40P packets

#### 2. Real-Time PCAP Replay with Foreground Extraction (`internal/lidar/network/pcap_realtime.go`)

- **Purpose**: Replays PCAP files with timing, extracts foreground, forwards to port 2370
- **Integration**: Uses BackgroundManager for foreground/background classification

**Key Components:**

- `RealtimeReplayConfig`:
  - `SpeedMultiplier`: Replay speed control (1.0 = real-time)
  - `PacketForwarder`: Forwards original packets to port 2368
  - `ForegroundForwarder`: Forwards foreground to port 2370
  - `BackgroundManager`: **Required** for foreground extraction
- `ReadPCAPFileRealtime()`: Main replay function with integrated foreground extraction

**Foreground Extraction Flow:**

```go
// In ReadPCAPFileRealtime(), for each packet:
1. Parse packet → points (PointPolar[])
2. BackgroundManager.ProcessFramePolarWithMask(points) → foregroundMask (bool[])
3. ExtractForegroundPoints(points, foregroundMask) → foregroundPoints (PointPolar[])
4. ForegroundForwarder.ForwardForeground(foregroundPoints) → UDP port 2370
```

#### 3. Background Subtraction (`internal/lidar/foreground.go`)

- `ProcessFramePolarWithMask()`: Classifies each point as foreground/background
- `ExtractForegroundPoints()`: Filters points using mask

## What to Check

### 1. **Is BackgroundManager Initialized?**

The BackgroundManager is **required** for foreground extraction. Without it, no foreground points will be extracted.

**Check:**

```go
// BackgroundManager must be created and passed to RealtimeReplayConfig
bgManager := lidar.NewBackgroundManager(bgConfig)
if bgManager == nil {
    log.Fatal("BackgroundManager initialization failed")
}

replayConfig := network.RealtimeReplayConfig{
    BackgroundManager: bgManager,  // MUST be set
    ForegroundForwarder: fgForwarder,
    // ...
}
```

**Common Issues:**

- BackgroundManager is nil
- BackgroundManager not passed to config
- Background model not settled (no background learned yet)

**Debug Logging:**
In `pcap_realtime.go` around line 172-190, check for:

```
"Foreground extraction: X/Y points (Z%)"
```

If this log doesn't appear, BackgroundManager integration is not working.

### 2. **Is ForegroundForwarder Created and Started?**

**Check Creation:**

```go
fgForwarder, err := network.NewForegroundForwarder("localhost", 2370, nil)
if err != nil {
    log.Fatalf("Failed to create foreground forwarder: %v", err)
}
```

**Check Started:**

```go
// Must call Start() before replay begins
fgForwarder.Start(ctx)
```

**Debug Logging:**
Look for in logs:

```
"Foreground forwarding started to localhost:2370 (port 2370)"
```

If this doesn't appear, the forwarder wasn't started.

### 3. **Is Foreground Extraction Working?**

**Check Background Model Status:**

```go
// Background model must have learned the static environment
// This requires ~30-60 seconds of data during replay
if !bgManager.HasSettled {
    log.Printf("Warning: Background model not settled yet")
}
```

**Check Extraction Statistics:**
Look for periodic logging (every 1000 packets):

```
"Foreground extraction: 150/400 points (37.5%)"
```

**What this tells you:**

- **0/X points (0%)**: Background model classifying everything as background
  - Model may be too aggressive
  - Scene may truly be static (no moving objects)
- **X/X points (100%)**: Everything classified as foreground
  - Background model not initialized
  - Model parameters too lenient
- **Reasonable ratio (5-50%)**: Extraction working correctly

### 4. **Is Data Being Forwarded?**

**Check Network Connectivity:**

```bash
# Listen on port 2370 to verify packets are being sent
sudo tcpdump -i lo -n udp port 2370 -c 10

# Or use netcat
nc -u -l 2370 | xxd | head -50
```

**Check Forwarder Statistics:**

```go
// In ForegroundForwarder.Start(), check for:
log.Printf("Foreground forwarder stopping (sent %d packets)", f.packetCount)
```

If `packetCount` is 0, no packets were sent.

**Common Issues:**

- Buffer full (non-blocking send drops packets)
  - Look for: "Warning: Foreground forwarding buffer full, dropping X points"
- Network write errors
- Firewall blocking UDP port 2370

### 5. **Is PCAP Replay Actually Running?**

**Check Replay Logs:**

```
"PCAP real-time replay: BPF filter set: udp port 2368 (speed: 1.0x)"
"PCAP real-time replay progress: X packets in Y (Z pkt/s, speed: 1.0x)"
```

**Check Parser is Working:**

```
"PCAP parsed points: packet=1000, points_this_packet=400, total_parsed_points=400000"
```

If no points are parsed, foreground extraction won't have any input.

### 6. **Are Build Tags Set?**

Real-time replay requires the `pcap` build tag:

**Check Build:**

```bash
# Must build with pcap tag
go build -tags pcap -o radar ./cmd/radar

# Without pcap tag, stub returns error:
# "PCAP real-time replay support not compiled in (requires pcap build tag)"
```

## Complete Initialization Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/banshee-data/velocity.report/internal/lidar"
    "github.com/banshee-data/velocity.report/internal/lidar/network"
)

func main() {
    ctx := context.Background()

    // 1. Create BackgroundManager (REQUIRED)
    bgConfig := &lidar.BackgroundGridParams{
        Rings:                   40,
        AzimuthBins:            360,
        BackgroundUpdateFraction: 0.02,
        SafetyMarginMeters:      0.3,
        // ... other params
    }
    bgManager := lidar.NewBackgroundManager(bgConfig)
    if bgManager == nil {
        log.Fatal("Failed to create BackgroundManager")
    }

    // 2. Create ForegroundForwarder
    fgForwarder, err := network.NewForegroundForwarder("localhost", 2370, nil)
    if err != nil {
        log.Fatalf("Failed to create foreground forwarder: %v", err)
    }
    fgForwarder.Start(ctx) // MUST CALL Start()
    defer fgForwarder.Close()

    // 3. Create PacketForwarder (optional, for original packets on 2368)
    packetForwarder, err := network.NewPacketForwarder("localhost", 2368, stats, time.Minute)
    if err != nil {
        log.Fatalf("Failed to create packet forwarder: %v", err)
    }
    packetForwarder.Start(ctx)
    defer packetForwarder.Close()

    // 4. Create Parser
    parser := lidar.NewPandar40PParser()

    // 5. Create FrameBuilder
    frameBuilder := lidar.NewFrameBuilder(/* config */)

    // 6. Create Stats
    stats := lidar.NewPacketStats()

    // 7. Configure Real-Time Replay
    replayConfig := network.RealtimeReplayConfig{
        SpeedMultiplier: 1.0,                     // Real-time
        PacketForwarder: packetForwarder,         // Port 2368 (optional)
        ForegroundForwarder: fgForwarder,         // Port 2370 (REQUIRED)
        BackgroundManager: bgManager,              // REQUIRED for extraction
    }

    // 8. Run Replay
    err = network.ReadPCAPFileRealtime(
        ctx,
        "data.pcap",
        2368,           // UDP port filter
        parser,
        frameBuilder,
        stats,
        replayConfig,
    )
    if err != nil {
        log.Fatalf("PCAP replay failed: %v", err)
    }
}
```

## Common Error Patterns

### Error: No packets on port 2370, logs show "0/X points (0%)"

**Cause**: Background model classifying everything as background

**Solutions:**

1. Verify background model parameters are reasonable
2. Check if scene has any moving objects
3. Increase `ClosenessSensitivityMultiplier` (default 3.0, try 2.0)
4. Decrease `SafetyMarginMeters` (default 0.3m, try 0.2m)

### Error: "BackgroundManager is nil" or segfault

**Cause**: BackgroundManager not created or not passed to config

**Solution**: Always create and pass BackgroundManager:

```go
bgManager := lidar.NewBackgroundManager(config)
replayConfig.BackgroundManager = bgManager  // MUST SET
```

### Error: "Foreground forwarding buffer full, dropping X points"

**Cause**: Foreground forwarder can't keep up with incoming data

**Solutions:**

1. Increase buffer size in `NewForegroundForwarder()` (currently 100 frames)
2. Reduce replay speed multiplier
3. Check network I/O performance

### Error: No logs about foreground extraction

**Cause**: Integration not working or config.ForegroundForwarder is nil

**Solution**: Verify both ForegroundForwarder AND BackgroundManager are set in config

## Testing Without Full System

### Test 1: Verify ForegroundForwarder in Isolation

```go
func TestForegroundForwarder() {
    ctx := context.Background()

    // Create forwarder
    fgForwarder, _ := network.NewForegroundForwarder("localhost", 2370, nil)
    fgForwarder.Start(ctx)
    defer fgForwarder.Close()

    // Create test points
    points := []lidar.PointPolar{
        {Channel: 0, Azimuth: 0.0, Distance: 10.0, Intensity: 100},
        {Channel: 1, Azimuth: 1.0, Distance: 11.0, Intensity: 110},
        // ... more points
    }

    // Forward points
    fgForwarder.ForwardForeground(points)

    // Check on port 2370 with tcpdump or netcat
    time.Sleep(2 * time.Second)
}
```

### Test 2: Verify BackgroundManager Extraction

```go
func TestBackgroundExtraction() {
    // Create background manager
    bgManager := lidar.NewBackgroundManager(config)

    // Feed static background data
    for i := 0; i < 100; i++ {
        staticPoints := createStaticTestPoints()
        bgManager.ProcessFramePolarWithMask(staticPoints)
    }

    // Now add foreground point
    testPoints := append(staticPoints, lidar.PointPolar{
        Channel: 5, Azimuth: 45.0, Distance: 5.0, // Closer than background
    })

    mask, _ := bgManager.ProcessFramePolarWithMask(testPoints)
    fgPoints := lidar.ExtractForegroundPoints(testPoints, mask)

    log.Printf("Extracted %d foreground points from %d total", len(fgPoints), len(testPoints))
}
```

## Debugging Commands

```bash
# 1. Check if binary has pcap support
strings radar | grep -i pcap

# 2. Monitor UDP traffic on port 2370
sudo tcpdump -i any -n udp port 2370 -vv

# 3. Count packets on port 2370
sudo tcpdump -i any -n udp port 2370 -c 1000 | wc -l

# 4. Capture packets to verify format
sudo tcpdump -i any -n udp port 2370 -w foreground_capture.pcap

# 5. Check if port is listening
sudo netstat -ulpn | grep 2370

# 6. Test UDP connectivity
echo "test" | nc -u localhost 2370
```

## Next Steps for VSCode Agent

1. **Verify Build**: Ensure binary compiled with `-tags pcap`
2. **Check Initialization**: Confirm BackgroundManager and ForegroundForwarder both created and started
3. **Add Debug Logging**: Insert additional log statements in extraction path
4. **Monitor Network**: Use tcpdump to verify packets are actually being sent
5. **Test Components**: Run isolated tests for ForegroundForwarder and BackgroundManager
6. **Check Parameters**: Verify background model parameters allow foreground detection

## Key Files to Review

- `internal/lidar/network/foreground_forwarder.go`: UDP forwarding implementation
- `internal/lidar/network/pcap_realtime.go`: Lines 172-190 (foreground extraction integration)
- `internal/lidar/foreground.go`: Background subtraction logic
- Command line tool invoking replay (check how it initializes components)

## Success Indicators

When working correctly, you should see:

1. ✅ Log: "Foreground forwarding started to localhost:2370"
2. ✅ Log: "PCAP real-time replay: BPF filter set: udp port 2368"
3. ✅ Log: "Foreground extraction: X/Y points (Z%)" every 1000 packets
4. ✅ Log: "Foreground forwarder stopping (sent N packets)" where N > 0
5. ✅ tcpdump shows UDP packets on port 2370
6. ✅ Packets are 1262 bytes (Pandar40P format)
