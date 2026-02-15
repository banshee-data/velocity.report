package sweep

import (
	"math"
	"sync"
)

// Transform applies a metric transformation before scoring.
type Transform interface {
	// Name returns a human-readable name for this transform.
	Name() string
	// Apply transforms the metrics and returns a new map, leaving the original unchanged.
	Apply(metrics map[string]float64) map[string]float64
}

// TransformPipeline applies a sequence of transforms to metrics before scoring.
type TransformPipeline struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	transforms []Transform
}

// NewTransformPipeline creates a new transform pipeline with the given name, version, and transforms.
func NewTransformPipeline(name, version string, transforms ...Transform) *TransformPipeline {
	return &TransformPipeline{
		Name:       name,
		Version:    version,
		transforms: transforms,
	}
}

// Apply applies all transforms in sequence to the given metrics.
// Each transform receives the output of the previous transform.
func (p *TransformPipeline) Apply(metrics map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result[k] = v
	}

	for _, t := range p.transforms {
		result = t.Apply(result)
	}

	return result
}

// TransformNames returns the names of all transforms in the pipeline.
func (p *TransformPipeline) TransformNames() []string {
	names := make([]string, len(p.transforms))
	for i, t := range p.transforms {
		names[i] = t.Name()
	}
	return names
}

// NormaliseTransform scales a metric to [0,1] range given known min/max bounds.
type NormaliseTransform struct {
	Metric string
	Min    float64
	Max    float64
}

// Name returns the transform name.
func (t *NormaliseTransform) Name() string {
	return "normalise:" + t.Metric
}

// Apply normalises the specified metric to [0,1] range.
// If the metric doesn't exist or Max == Min, the value is unchanged.
func (t *NormaliseTransform) Apply(metrics map[string]float64) map[string]float64 {
	val, ok := metrics[t.Metric]
	if !ok || t.Max == t.Min {
		return metrics
	}

	// Create a copy to maintain immutability
	result := make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result[k] = v
	}

	normalised := (val - t.Min) / (t.Max - t.Min)
	result[t.Metric] = normalised
	return result
}

// ClipTransform clamps a metric value to [min, max].
type ClipTransform struct {
	Metric string
	Min    float64
	Max    float64
}

// Name returns the transform name.
func (t *ClipTransform) Name() string {
	return "clip:" + t.Metric
}

// Apply clips the specified metric to [Min, Max] range.
// If the metric doesn't exist, it is unchanged.
func (t *ClipTransform) Apply(metrics map[string]float64) map[string]float64 {
	val, ok := metrics[t.Metric]
	if !ok {
		return metrics
	}

	// Create a copy to maintain immutability
	result := make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result[k] = v
	}

	if val < t.Min {
		result[t.Metric] = t.Min
	} else if val > t.Max {
		result[t.Metric] = t.Max
	}

	return result
}

// LogScaleTransform applies log(1 + value) to a metric.
type LogScaleTransform struct {
	Metric string
}

// Name returns the transform name.
func (t *LogScaleTransform) Name() string {
	return "log:" + t.Metric
}

// Apply applies log(1 + value) transformation to the specified metric.
// If the metric doesn't exist, it is unchanged.
func (t *LogScaleTransform) Apply(metrics map[string]float64) map[string]float64 {
	val, ok := metrics[t.Metric]
	if !ok {
		return metrics
	}

	// Create a copy to maintain immutability
	result := make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result[k] = v
	}

	result[t.Metric] = math.Log(1 + val)
	return result
}

// ClassWeightTransform multiplies a metric by a weight factor.
type ClassWeightTransform struct {
	Metric string
	Weight float64
}

// Name returns the transform name.
func (t *ClassWeightTransform) Name() string {
	return "weight:" + t.Metric
}

// Apply multiplies the specified metric by the weight factor.
// If the metric doesn't exist, it is unchanged.
func (t *ClassWeightTransform) Apply(metrics map[string]float64) map[string]float64 {
	val, ok := metrics[t.Metric]
	if !ok {
		return metrics
	}

	// Create a copy to maintain immutability
	result := make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result[k] = v
	}

	result[t.Metric] = val * t.Weight
	return result
}

// RoundModifierTransform applies a round-dependent multiplier (for HINT).
type RoundModifierTransform struct {
	Metric       string
	Multiplier   float64
	Round        int  // only apply when current round matches
	CurrentRound *int // pointer to current round (mutable)
}

// Name returns the transform name.
func (t *RoundModifierTransform) Name() string {
	return "round_modifier:" + t.Metric
}

// Apply applies the multiplier to the specified metric only when the current round matches.
// If CurrentRound is nil or doesn't match Round, the metric is unchanged.
// If the metric doesn't exist, it is unchanged.
func (t *RoundModifierTransform) Apply(metrics map[string]float64) map[string]float64 {
	if t.CurrentRound == nil || *t.CurrentRound != t.Round {
		return metrics
	}

	val, ok := metrics[t.Metric]
	if !ok {
		return metrics
	}

	// Create a copy to maintain immutability
	result := make(map[string]float64, len(metrics))
	for k, v := range metrics {
		result[k] = v
	}

	result[t.Metric] = val * t.Multiplier
	return result
}

// DefaultTransformPipeline returns the default (identity) pipeline.
// It performs no transformations on the input metrics.
func DefaultTransformPipeline() *TransformPipeline {
	return NewTransformPipeline("default", "1.0")
}

// GroundTruthTransformPipeline returns a pipeline suited for ground truth scoring.
// It clips rates to [0,1] and applies log scaling to count-like metrics.
func GroundTruthTransformPipeline() *TransformPipeline {
	return NewTransformPipeline("ground_truth", "1.0",
		// Clip acceptance and ratio metrics to valid [0,1] range
		&ClipTransform{Metric: "acceptance_rate", Min: 0.0, Max: 1.0},
		&ClipTransform{Metric: "misalignment_ratio", Min: 0.0, Max: 1.0},
		&ClipTransform{Metric: "foreground_capture", Min: 0.0, Max: 1.0},
		&ClipTransform{Metric: "empty_box_ratio", Min: 0.0, Max: 1.0},
		&ClipTransform{Metric: "fragmentation_ratio", Min: 0.0, Max: 1.0},

		// Apply log scaling to count-like metrics (cells, tracks)
		&LogScaleTransform{Metric: "nonzero_cells"},
		&LogScaleTransform{Metric: "active_tracks"},
	)
}

// TransformPipelineInfo is a summary of a registered pipeline.
type TransformPipelineInfo struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Transforms []string `json:"transforms"`
}

// TransformRegistry holds named pipeline presets.
type TransformRegistry struct {
	mu      sync.RWMutex
	presets map[string]*TransformPipeline
}

// NewTransformRegistry creates a new empty transform registry.
func NewTransformRegistry() *TransformRegistry {
	return &TransformRegistry{
		presets: make(map[string]*TransformPipeline),
	}
}

// Register adds a pipeline to the registry.
// If a pipeline with the same name already exists, it is replaced.
func (r *TransformRegistry) Register(pipeline *TransformPipeline) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.presets[pipeline.Name] = pipeline
}

// Get retrieves a pipeline by name.
// Returns (pipeline, true) if found, (nil, false) otherwise.
func (r *TransformRegistry) Get(name string) (*TransformPipeline, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pipeline, ok := r.presets[name]
	return pipeline, ok
}

// List returns information about all registered pipelines.
func (r *TransformRegistry) List() []TransformPipelineInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]TransformPipelineInfo, 0, len(r.presets))
	for _, pipeline := range r.presets {
		infos = append(infos, TransformPipelineInfo{
			Name:       pipeline.Name,
			Version:    pipeline.Version,
			Transforms: pipeline.TransformNames(),
		})
	}
	return infos
}

// DefaultTransformRegistry returns a registry with built-in presets.
func DefaultTransformRegistry() *TransformRegistry {
	registry := NewTransformRegistry()
	registry.Register(DefaultTransformPipeline())
	registry.Register(GroundTruthTransformPipeline())
	return registry
}
