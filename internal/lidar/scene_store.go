package lidar

import (
"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Scene represents a LiDAR evaluation scene tying a PCAP to a sensor and parameters.
// This is now an alias for the implementation in storage/sqlite.
type Scene = sqlite.Scene

// SceneStore provides persistence for LiDAR evaluation scenes.
// This is now an alias for the implementation in storage/sqlite.
type SceneStore = sqlite.SceneStore

// NewSceneStore creates a new SceneStore.
// This is now an alias for the implementation in storage/sqlite.
var NewSceneStore = sqlite.NewSceneStore
