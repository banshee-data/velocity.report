package sweep

import "sync"

// ObjectiveDefinition describes a registered objective module.
type ObjectiveDefinition struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Description   string   `json:"description"`
	InputFeatures []string `json:"input_features"`
	// Score computes a scalar score for the given result and weights.
	Score func(result ComboResult, weights ObjectiveWeights) float64
}

// ObjectiveRegistry holds registered objective definitions.
type ObjectiveRegistry struct {
	mu         sync.RWMutex
	objectives map[string]*ObjectiveDefinition
}

// ObjectiveInfo is a summary of a registered objective.
type ObjectiveInfo struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Description   string   `json:"description"`
	InputFeatures []string `json:"input_features"`
}

// NewObjectiveRegistry creates a new empty objective registry.
func NewObjectiveRegistry() *ObjectiveRegistry {
	return &ObjectiveRegistry{
		objectives: make(map[string]*ObjectiveDefinition),
	}
}

// Register adds an objective definition to the registry.
// If an objective with the same name already exists, it is replaced.
func (r *ObjectiveRegistry) Register(def *ObjectiveDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objectives[def.Name] = def
}

// Get retrieves an objective definition by name.
// Returns nil and false if the objective is not found.
func (r *ObjectiveRegistry) Get(name string) (*ObjectiveDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.objectives[name]
	return def, ok
}

// List returns a slice of all registered objectives' summary information.
// The slice is sorted alphabetically by name for deterministic output.
func (r *ObjectiveRegistry) List() []ObjectiveInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ObjectiveInfo, 0, len(r.objectives))
	for _, def := range r.objectives {
		infos = append(infos, ObjectiveInfo{
			Name:          def.Name,
			Version:       def.Version,
			Description:   def.Description,
			InputFeatures: def.InputFeatures,
		})
	}

	// Sort by name for deterministic output
	for i := 0; i < len(infos)-1; i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[i].Name > infos[j].Name {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}

	return infos
}

// DefaultObjectiveRegistry returns a registry pre-loaded with built-in objectives.
func DefaultObjectiveRegistry() *ObjectiveRegistry {
	reg := NewObjectiveRegistry()

	// Register the "weighted" objective (default)
	reg.Register(&ObjectiveDefinition{
		Name:    "weighted",
		Version: "v1",
		Description: "Multi-objective scoring using weighted combination of acceptance, " +
			"misalignment, alignment, nonzero cells, active tracks, foreground capture, " +
			"empty boxes, fragmentation, heading jitter, and speed jitter metrics.",
		InputFeatures: []string{
			"acceptance",
			"misalignment",
			"alignment",
			"nonzero_cells",
			"active_tracks",
			"foreground_capture",
			"empty_boxes",
			"fragmentation",
			"heading_jitter",
			"speed_jitter",
		},
		Score: func(result ComboResult, weights ObjectiveWeights) float64 {
			return ScoreResult(result, weights)
		},
	})

	// Register the "acceptance" objective
	reg.Register(&ObjectiveDefinition{
		Name:        "acceptance",
		Version:     "v1",
		Description: "Optimises for acceptance rate only, ignoring all other metrics.",
		InputFeatures: []string{
			"acceptance",
		},
		Score: func(result ComboResult, weights ObjectiveWeights) float64 {
			// Override weights to acceptance-only
			acceptanceWeights := ObjectiveWeights{
				Acceptance: 1.0,
			}
			return ScoreResult(result, acceptanceWeights)
		},
	})

	// Register the "ground_truth" objective
	reg.Register(&ObjectiveDefinition{
		Name:    "ground_truth",
		Version: "v1",
		Description: "Ground truth evaluation mode. Uses labelled data to compute " +
			"detection rate, fragmentation, false positives, velocity coverage, " +
			"quality premium, truncation rate, velocity noise rate, and stopped recovery metrics.",
		InputFeatures: []string{
			"detection_rate",
			"fragmentation",
			"false_positives",
			"velocity_coverage",
			"quality_premium",
			"truncation_rate",
			"velocity_noise_rate",
			"stopped_recovery",
		},
		Score: func(result ComboResult, weights ObjectiveWeights) float64 {
			// Ground truth scoring is handled externally via the analysis pipeline.
			// This fallback uses the standard weighted score for compatibility.
			return ScoreResult(result, weights)
		},
	})

	return reg
}
