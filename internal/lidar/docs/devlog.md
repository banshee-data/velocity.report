# LiDAR Development Log

## September 18, 2025 - Polar-first refactor: parser & frame builder

- Centralized spherical→Cartesian math into a small `transform.go` helper and introduced `PointPolar`.
- Parser now emits `PointPolar` (polar-first) and the UDP listener forwards polar points directly when possible.
- Added `FrameBuilder.AddPointsPolar([]PointPolar)` and removed the legacy `AddPoints([]Point)` API; tests and integration updated.
- Result: `internal/lidar` tests (unit + PCAP integration) pass after the migration.

## September 17, 2025 - Background model & transform refactor

- Sensor-frame background model (ring × azimuth) for foreground masking.
- Two-level settling per cell (fast noise settling, slow parked-object settling).
- Persist BackgroundGrid snapshots and warm-start on load.
- Refactor spherical→Cartesian into a small transform helper and split polar/cartesian point types.
- World-grid (height-map / ground estimate) on masked Cartesian points for semantic ops.
- Next: add transform helpers, update point types, implement sensor-frame ProcessFrame and snapshot tests.

## September 13, 2025 - Test Code Maintainability & Optimization

### Parse Test Improvements
- **Eliminated implementation dependencies**: Replaced external constants (CHANNELS_PER_BLOCK, PACKET_SIZE_*) with local test constants
- **Enhanced maintainability**: Tests now self-contained and independent of implementation changes
- **Fixed boundary conditions**: Corrected loop bounds in PCAP extraction to include valid edge cases
- **Removed redundant checks**: Eliminated unnecessary bounds checking in packet extraction logic
- **Performance optimization**: Streamlined extractUDPPayloads function by removing redundant conditional checks

### Technical Changes
- **Local test constants**: Added testChannelsPerBlock, testPacketSizeStandard, etc. for test isolation
- **Boundary fix**: Changed `i < len(data)-testPacketSizeStandard` to `i <= len(data)-testPacketSizeStandard`
- **Logic optimization**: Removed redundant `if i+testPacketSizeStandard <= len(data)` check
- **Code clarity**: Added explanatory comments for optimization decisions

## September 12, 2025 - Frame Builder Test Suite Fixes & Validation

### Test Suite Completion
- **All frame builder tests passing**: Fixed 3 previously failing tests using realistic production data patterns
- **Integration test relocation**: Moved PCAP integration test from `cmd/pcap-test/` to `internal/lidar/integration_test.go`
- **Test data organization**: Created `internal/lidar/testdata/` directory following Go conventions
- **Data volume upgrade**: Increased test point counts from ~10,680 to 60,000 points (matching successful PCAP integration test)
- **Production-level validation**: Tests now use MinFramePointsForCompletion = 10,000 threshold with realistic coverage
- **Time-based detection validation**: Confirmed hybrid detection with motor speed adaptation and azimuth wrap fallback
- **Configuration completeness**: Added BufferTimeout and CleanupInterval settings for proper async frame processing

### Fixed Test Cases
- **TestFrameBuilder_TraditionalAzimuthOnly**: Traditional azimuth-only detection (350° → 10°) with 60,000 points
- **TestFrameBuilder_HybridDetection**: Time-based detection with azimuth validation and realistic timing
- **TestFrameBuilder_AzimuthWrapWithTimeBased**: Azimuth wrap in time-based mode with proper configuration

### Test Pattern Analysis
- **Successful data patterns**: 0°-356° azimuth coverage with wrap at 356°→5° triggers completion
- **Timing validation**: ~60ms frame duration matches production expectations (600 RPM motor speed)
- **Point distribution**: Even azimuth distribution across 60,000 points provides adequate coverage
- **Configuration requirements**: BufferTimeout=100ms, CleanupInterval=50ms essential for async processing

## September 12, 2025 - Time-Based Frame Detection & Documentation

### Time-Based Frame Detection Implementation
- **Hybrid frame detection**: Time-based primary trigger with azimuth validation for anomaly prevention
- **Motor speed integration**: Real-time motor speed extraction from packet tail (bytes 8-9)
- **Frame timing adaptation**: Dynamic frame duration based on actual RPM (50ms at 1200 RPM, 100ms at 600 RPM)
- **Azimuth safety checks**: Requires 270° coverage before time-based completion to prevent timing glitches
- **Azimuth wrap secondary**: Respects azimuth wraps (340° → 20°) with minimum half-duration timing constraint
- **Traditional fallback**: Pure azimuth-based detection (350° → 10°) when time-based disabled
- **Motor speed caching**: Parser stores last motor speed for frame builder integration
- **Testing validation**: Confirmed proper frame duration changes during RPM transitions (600→1200→600)

### Code Documentation Enhancement
- **Comment verbosity upgrade**: Comprehensive documentation updates in extract.go
- **Packet structure details**: Complete 22-byte tail parsing documentation with all fields
- **Timestamp mode documentation**: Added detailed explanations for all 5 supported modes
- **Calibration explanations**: Enhanced comments for coordinate transformations and corrections
- **Performance optimization notes**: Documented trigonometric optimizations and memory allocations

### Technical Improvements
- **CLI configurability**: Added --sensor-name flag for flexible deployment scenarios
- **Real-time adaptation**: Frame builder now responds immediately to motor speed changes
- **Accurate timing**: Eliminated hardcoded 600 RPM assumption, uses actual motor speed throughout
- **UDP sequence validation**: Confirmed proper handling of optional 4-byte UDP sequence suffix

## September 11, 2025 - Memory Optimization & Frame Rate Fixes

### Packet Structure Analysis
- **Wireshark investigation**: Analyzed Hesai Pandar40P UDP packet structure
- **Discovered Ethernet tail issue**: Extra 4 bytes appended to UDP packets
- **Tail composition**: 2-byte sequence + 2-byte unknown data (0x00 0x00)
- **Parser fix**: Updated tail offset from last 6 bytes to last 10 bytes
- **Validation**: Confirmed correct UDP sequence extraction and point parsing

### Performance Validation
- **Proper frame characteristics**: ~69,000 points per frame, ~100ms duration
- **Correct LiDAR operation**: Full 360° rotations with expected Hesai Pandar40P output
- **Debug logging**: Added temporary logging to diagnose, then removed for production

### Technical Discoveries
- **Ethernet vs UDP parsing**: Raw UDP data includes Ethernet layer artifacts
- **Tail offset critical**: Incorrect offset leads to malformed sequence numbers
- Frame builder processes points individually, not in packets
- Individual UDP packets contain only 2-3 points with small azimuth ranges
- Azimuth wrap detection must account for accumulated vs instantaneous coverage
- Point-level frame detection requires stricter criteria than packet-level detection

