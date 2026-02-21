# LiDAR Documentation

Status: Active

Documentation for the velocity.report LiDAR subsystem.

## Structure

- `architecture/`: active architecture and runtime design notes
- `operations/`: implemented runtime behavior and operational procedures
- `visualiser/`: implemented visualiser design and integration docs
- `future/`: LiDAR work explicitly planned for future implementation
- `troubleshooting/`: historical investigation and fix notes
- `noise_investigation/`: analysis artifacts from tuning/investigation work
- `refactor/`: refactor plans and design notes
- `ROADMAP.md`: staged forward plan
- `docs/proposals/lidar/architecture/`: proposal architecture docs targeting `docs/lidar/architecture/`
- `docs/proposals/lidar/visualiser/`: proposal visualiser docs targeting `docs/lidar/visualiser/`

## Separation Boundary

- Current runtime foundations (vector-grid baseline and production tracking path) are documented in active `architecture/` and `operations/` docs.
- Velocity-coherent extraction and other non-runtime designs are documented in `future/` and `docs/proposals/lidar/` docs.

Use directory listings for file-level lookup so indexes do not go stale.
