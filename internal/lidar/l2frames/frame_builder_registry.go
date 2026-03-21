package l2frames

import (
	"sync"
)

// Global registry for FrameBuilder instances keyed by SensorID.
var (
	fbRegistry   = map[string]*FrameBuilder{}
	fbRegistryMu = &sync.RWMutex{}
)

// RegisterFrameBuilder registers a FrameBuilder for a sensor ID.
func RegisterFrameBuilder(sensorID string, fb *FrameBuilder) {
	if sensorID == "" || fb == nil {
		return
	}
	fbRegistryMu.Lock()
	defer fbRegistryMu.Unlock()
	fbRegistry[sensorID] = fb
}

// GetFrameBuilder returns a registered FrameBuilder or nil
func GetFrameBuilder(sensorID string) *FrameBuilder {
	fbRegistryMu.RLock()
	defer fbRegistryMu.RUnlock()
	return fbRegistry[sensorID]
}

// UnregisterFrameBuilder removes a FrameBuilder from the global registry.
func UnregisterFrameBuilder(sensorID string) {
	fbRegistryMu.Lock()
	defer fbRegistryMu.Unlock()
	delete(fbRegistry, sensorID)
}
