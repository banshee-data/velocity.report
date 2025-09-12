# LiDAR Development Log

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
