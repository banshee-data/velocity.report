# Feature Documentation Index

This directory contains detailed design documents, specifications, and integration guides for velocity.report features.

## LIDAR System

### Tracking & Classification
- **[LIDAR Tracking Integration](lidar-tracking-integration.md)** - Integration status, pipeline architecture, implementation details
- **[LIDAR Tracking Enhancements](lidar-tracking-enhancements.md)** - Comprehensive 4-phase enhancement plan for point cloud extraction, ML classification, introspection, and track quality analysis
- **[LIDAR Background Grid Standards](lidar-background-grid-standards.md)** - Comparison with external standards (SLAM, OctoMap, VTK), export recommendations

### Data Collection & Analysis
- **[PCAP Analysis Mode](pcap-analysis-mode.md)** - Offline PCAP replay with background grid preservation, analysis workflows
- **[PCAP Split Tool](pcap-split-tool.md)** - Split PCAP files by motion/static state for background characterization

## Configuration & Setup
- **[CLI Comprehensive Guide](cli-comprehensive-guide.md)** - Complete command-line interface reference
- **[Serial Configuration UI](serial-configuration-ui.md)** - Web UI for radar sensor configuration
- **[Serial Config Quick Reference](SERIAL-CONFIG-QUICKREF.md)** - Quick lookup for serial port settings

## Data Management
- **[Time-Partitioned Data Tables](time-partitioned-data-tables.md)** - Database partitioning strategy for long-term data retention
- **[Speed Limit Schedules](speed-limit-schedules.md)** - Dynamic speed limit configuration by time of day

## Related Documentation
- [Main README](../../README.md) - Project overview, quick start
- [Architecture Guide](../../ARCHITECTURE.md) - System design and component interaction
- [Troubleshooting Guide](../../TROUBLESHOOTING.md) - Common issues and solutions

## Feature Status Legend
- ✅ **Integrated** - Fully implemented and merged
- 🔨 **In Progress** - Under active development
- 📋 **Planned** - Design documented, implementation pending
- ⏸️ **Deferred** - Planned but deprioritized

## LIDAR Feature Status

| Feature | Status | Documentation |
|---------|--------|---------------|
| Foreground Extraction | ✅ Integrated | [Tracking Integration](lidar-tracking-integration.md) |
| DBSCAN Clustering | ✅ Integrated | [Tracking Integration](lidar-tracking-integration.md) |
| Kalman Filter Tracking | ✅ Integrated | [Tracking Integration](lidar-tracking-integration.md) |
| Rule-Based Classification | ✅ Integrated | [Tracking Enhancements](lidar-tracking-enhancements.md#2-classification-system-rule-based-v10) |
| Track Quality Metrics | 📋 Planned | [Tracking Enhancements](lidar-tracking-enhancements.md#phase-1-enhanced-point-cloud-extraction--track-quality-metrics) |
| ML Classification | 📋 Planned | [Tracking Enhancements](lidar-tracking-enhancements.md#phase-2-ml-ready-classification-infrastructure) |
| Parameter Tuning Tools | 📋 Planned | [Tracking Enhancements](lidar-tracking-enhancements.md#phase-3-advanced-introspection--charting) |
| Split/Merge Detection | 📋 Planned | [Tracking Enhancements](lidar-tracking-enhancements.md#phase-4-track-splitmerge-detection--correction) |
| Analysis Run Management | ⏸️ Deferred | [Tracking Integration](lidar-tracking-integration.md#phase-4-analysis-run-management-deferred) |

## Quick Links by Use Case

### Setting Up LIDAR Tracking
1. [PCAP Analysis Mode](pcap-analysis-mode.md) - Start here for offline analysis
2. [Tracking Integration](lidar-tracking-integration.md) - Understand the pipeline
3. [CLI Guide](cli-comprehensive-guide.md) - Command-line operations

### Improving Track Quality
1. [Tracking Enhancements](lidar-tracking-enhancements.md#phase-1-enhanced-point-cloud-extraction--track-quality-metrics) - Quality metrics plan
2. [Background Grid Standards](lidar-background-grid-standards.md) - Background tuning guidance
3. [PCAP Split Tool](pcap-split-tool.md) - Isolate motion vs static for better background models

### Building ML Classifiers
1. [Tracking Enhancements](lidar-tracking-enhancements.md#phase-2-ml-ready-classification-infrastructure) - Feature extraction plan
2. [Tracking Integration](lidar-tracking-integration.md#phase-3-tracker-integration) - Current classification implementation
3. [Analysis Runs Schema](lidar-tracking-integration.md#phase-4-analysis-run-management-deferred) - Training data storage

### Parameter Tuning
1. [Tracking Enhancements](lidar-tracking-enhancements.md#32-parameter-sensitivity-analysis) - Sensitivity analysis plan
2. [Background Grid Standards](lidar-background-grid-standards.md) - Grid parameter guidance
3. [PCAP Analysis Mode](pcap-analysis-mode.md) - Reproducible testing workflow

## Contributing

When adding new feature documentation:
1. Use descriptive titles and clear headings
2. Include code examples where relevant
3. Link to source code (e.g., `internal/lidar/tracking.go`)
4. Update this README with navigation links
5. Mark status (✅ Integrated, 📋 Planned, etc.)
