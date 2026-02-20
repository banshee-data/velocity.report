# Internal Project Documentation

This directory contains internal project documentation for development and planning purposes.

## Contents

### LiDAR Documentation

- [`lidar/`](lidar/) - LiDAR subsystem documentation
  - [LiDAR Overview](lidar/README.md) - Index of architecture, operations, and reference docs
  - `architecture/` - Core system design and implementation specs
  - `operations/` - Runtime operations and debugging
  - `reference/` - Protocol specs and data formats
  - `roadmap/` - Development progress and planning
  - `future/` - Deferred features for specialised use cases

### Mathematical Documentation

- [`maths/`](maths/) - Math-focused subsystem notes and assumptions
  - [Maths Overview](maths/README.md) - High-level assumptions and architecture
  - [Background Grid Settling Maths](maths/background-grid-settling-maths.md)
  - [Ground Plane Maths](maths/ground-plane-maths.md)
  - [Clustering Maths](maths/clustering-maths.md)
  - [Tracking Maths](maths/tracking-maths.md)

### Planning Documents

- [`plans/`](plans/) - Migration and refactoring plans
  - [Python venv Consolidation Plan](plans/python-venv-consolidation-plan.md)
  - [Transit Deduplication Plan](plans/transit-deduplication-plan.md)
  - [Distribution and Packaging Plan](plans/DISTRIBUTION_AND_PACKAGING_PLAN.md)
  - [Frontend Background Debug Surfaces Plan](plans/frontend-background-debug-surfaces-plan.md)

### Feature Specifications

- [`features/`](features/) - Feature specifications and requirements

## Public Documentation Site

The public-facing documentation site has been moved to [`public_html/`](../public_html/).
