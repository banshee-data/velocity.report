package l4perception

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Function re-exports for voxel-based downsampling.

// VoxelGrid performs voxel-based downsampling on world points.
var VoxelGrid = lidar.VoxelGrid
