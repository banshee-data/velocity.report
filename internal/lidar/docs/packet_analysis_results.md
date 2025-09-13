# Hesai Pandar40P LiDAR Packet Structure Analysis

## Overview

This document provides a comprehensive analysis of the Hesai Pandar40P LiDAR UDP packet format based on live packet captures and parsing implementation. The sensor transmits 3D point cloud data via UDP packets at high frequency, with each packet containing measurements from all 40 laser channels across 10 azimuth positions.

## Packet Structure

### Protocol Layer Breakdown

The LiDAR data traverses multiple protocol layers with distinct headers and trailers:

```
Complete Ethernet Frame (as transmitted on wire)
├── Ethernet Header: 14 bytes (MAC addresses + EtherType)
├── IP Header: 20 bytes (IPv4 standard)
├── UDP Header: 8 bytes (ports + length + checksum)
├── UDP Payload: 1262-1266 bytes (LiDAR data)
└── Ethernet FCS: 4 bytes (frame check sequence - not shown in Wireshark)
Total Wire Frame: 1308-1312 bytes
```

```
Wireshark Capture View (FCS not displayed)
├── Ethernet + IP + UDP Headers: 42 bytes
├── UDP Payload: 1262-1266 bytes (LiDAR data)
└── Visible packet: 1304-1308 bytes
```

### LiDAR UDP Payload Structure

The actual LiDAR data within the UDP payload follows this format:

**Without UDP Sequencing (1262 bytes UDP payload = 1304 bytes visible in Wireshark):**
```
LiDAR Data Packet (1262 bytes)
├── Data Blocks: 1240 bytes (10 blocks × 124 bytes each)
│   └── Each block: Preamble (2) + Azimuth (2) + Channels (120)
└── Data Tail: 22 bytes (sensor status and timing)
```

**With UDP Sequencing (1266 bytes UDP payload = 1308 bytes visible in Wireshark):**
```
LiDAR Data Packet (1266 bytes)
├── Data Blocks: 1240 bytes (10 blocks × 124 bytes each)
├── Data Tail: 22 bytes (sensor status and timing)
└── UDP Sequence: 4 bytes (packet sequence number)
```

### Layer Separation Clarification

- **Ethernet FCS (4 bytes)**: Network layer frame check sequence (hidden by Wireshark)
- **LiDAR Data Tail (22 bytes)**: Application layer sensor status and timing
- **UDP Sequence (4 bytes)**: Optional LiDAR protocol extension for packet ordering
- **Wireshark Display**: Shows 1304 bytes (without sequence) or 1308 bytes (with sequence)

## Data Block Structure

Each packet contains **10 data blocks**, with each block representing measurements from all 40 laser channels at a specific azimuth angle.

### Block Format (124 bytes)
```
Block Structure
├── Preamble: 2 bytes (0xFFEE) - Block identifier
├── Azimuth: 2 bytes (little-endian) - Horizontal angle
└── Channel Data: 120 bytes (40 channels × 3 bytes each)
    └── Per channel: Distance (2 bytes) + Reflectivity (1 byte)
```

### Azimuth Encoding
- **Raw format**: 16-bit little-endian integer
- **Resolution**: 0.01 degrees per LSB
- **Range**: 0 to 36,000 (representing 0.00° to 360.00°)
- **Sample values**: 0x0C9B = 3227 = 32.27°

### Channel Measurements
Each of the 40 laser channels provides:

#### Distance Data
- **Format**: 16-bit little-endian integer
- **Resolution**: 4mm per LSB (0.004 meters)
- **Range**: 0 to 262,140mm (0 to 262.14 meters)
- **Invalid measurement**: 0x0000 (no return detected)
- **Sample values**: 0x006F = 111 LSBs = 0.444 meters

#### Reflectivity Data
- **Format**: 8-bit unsigned integer
- **Range**: 0 to 255
- **Unit**: Relative intensity/reflectivity value
- **Sample values**: 0x0A = 10, 0x70 = 112

## LiDAR Data Tail Structure

The 22-byte data tail contains sensor status, timing, and protocol information.

### Data Tail Field Mapping
Based on packet analysis and LiDAR protocol specification:

| Byte Range | Field | Format | Description | Example Value |
|------------|-------|--------|-------------|---------------|
| 0-4 | Reserved1 | 5 bytes | Reserved fields | `0007e90004` |
| 5 | HighTempFlag | uint8 | Temperature status | `0x00` (normal) |
| 6-7 | Reserved2 | 2 bytes | Reserved fields | `7000` |
| 8-9 | MotorSpeed | uint16 LE | RPM × 60 / frame_rate | `0x2e33` = 13363 |
| 10-13 | Timestamp | uint32 LE | Microseconds (UTC) | `0x4209` = 16905 μs |
| 14 | ReturnMode | uint8 | Return mode setting | `0x38` (Last return) |
| 15 | FactoryInfo | uint8 | Manufacturer code | `0x42` (Hesai) |
| 16-21 | DateTime | 6 bytes | UTC date/time | `000057020a80` |
| 22-25 | UDPSequence | 4 bytes | (Optional) Packet sequence number | `9763747` |

### Key Field Values
- **HighTempFlag**: `0x00` = Normal operation, `0x01` = High temperature
- **ReturnMode**: `0x37` = Strongest, `0x38` = Last, `0x39` = Last+Strongest
- **FactoryInfo**: `0x42` = Hesai manufacturer identifier
- **Timestamp**: Microsecond component of UTC time (0-999,999 μs range)

## Data Ranges and Calibration

### Measured Value Ranges
From live packet analysis:

#### Distance Measurements
- **Typical range**: 0.1m to 200m (outdoor scenes)
- **Raw values**: 25 to 50,000 LSBs
- **Resolution**: 4mm precision
- **Zero values**: Indicate no valid return

#### Intensity Values
- **Typical range**: 5 to 255
- **Low intensity**: 5-50 (distant/dark objects)
- **High intensity**: 100-255 (nearby/reflective objects)
- **Weather sensitivity**: Values decrease in rain/fog

#### Azimuth Progression
- **Rotation direction**: Clockwise when viewed from top
- **Angular step**: ~0.1° to 0.4° between blocks
- **Frame completion**: 360° rotation generates ~1000-3600 blocks
- **Frame rate**: Typically 10-20 Hz depending on motor speed

### Calibration Parameters
The parser applies per-channel corrections:

#### Elevation Angles
- **Channel 1-40**: Fixed elevation angles from -16° to +15°
- **Vertical spacing**: ~0.67° between adjacent channels
- **Correction range**: ±0.1° manufacturing tolerance

#### Azimuth Corrections
- **Per-channel offset**: ±0.5° typical range
- **Purpose**: Compensate for mechanical assembly tolerances
- **Application**: Added to raw block azimuth value

#### Timing Corrections
- **Fire time offsets**: 0-50 μs per channel
- **Purpose**: Account for sequential laser firing within block
- **Usage**: Precise timestamp calculation for each point

## Protocol Characteristics

### Packet Transmission
- **Frequency**: ~1800-2000 packets/second (typical)
- **Network load**: ~2.3-2.5 MB/s UDP traffic
- **Sequence**: Incrementing 32-bit counter in UDP suffix
- **Ordering**: Sequential transmission, occasional drops acceptable

### Quality Indicators
- **Preamble validation**: All blocks must start with 0xFFEE
- **Sequence gaps**: Indicate network packet loss
- **Static detection**: Repeated timestamps suggest sensor issues
- **Range validation**: Distance values beyond 200m flagged as suspect

