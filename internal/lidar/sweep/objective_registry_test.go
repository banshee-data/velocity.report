package sweep

import (
	"testing"
)

func TestObjectiveRegistry_Register(t *testing.T) {
	reg := NewObjectiveRegistry()

	def := &ObjectiveDefinition{
		Name:          "test",
		Version:       "v1",
		Description:   "Test objective",
		InputFeatures: []string{"acceptance"},
		Score: func(result ComboResult, weights ObjectiveWeights) float64 {
			return 1.0
		},
	}

	reg.Register(def)

	retrieved, ok := reg.Get("test")
	if !ok {
		t.Fatal("Expected to retrieve registered objective")
	}

	if retrieved.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", retrieved.Name)
	}
}

func TestObjectiveRegistry_GetNonExistent(t *testing.T) {
	reg := NewObjectiveRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("Expected Get to return false for non-existent objective")
	}
}

func TestObjectiveRegistry_List(t *testing.T) {
	reg := NewObjectiveRegistry()

	// Register multiple objectives
	reg.Register(&ObjectiveDefinition{
		Name:          "zebra",
		Version:       "v1",
		Description:   "Last alphabetically",
		InputFeatures: []string{"acceptance"},
		Score:         func(result ComboResult, weights ObjectiveWeights) float64 { return 0 },
	})

	reg.Register(&ObjectiveDefinition{
		Name:          "apple",
		Version:       "v1",
		Description:   "First alphabetically",
		InputFeatures: []string{"acceptance"},
		Score:         func(result ComboResult, weights ObjectiveWeights) float64 { return 0 },
	})

	reg.Register(&ObjectiveDefinition{
		Name:          "middle",
		Version:       "v1",
		Description:   "Middle alphabetically",
		InputFeatures: []string{"acceptance"},
		Score:         func(result ComboResult, weights ObjectiveWeights) float64 { return 0 },
	})

	infos := reg.List()

	if len(infos) != 3 {
		t.Fatalf("Expected 3 objectives, got %d", len(infos))
	}

	// Check alphabetical ordering
	if infos[0].Name != "apple" {
		t.Errorf("Expected first to be 'apple', got '%s'", infos[0].Name)
	}
	if infos[1].Name != "middle" {
		t.Errorf("Expected second to be 'middle', got '%s'", infos[1].Name)
	}
	if infos[2].Name != "zebra" {
		t.Errorf("Expected third to be 'zebra', got '%s'", infos[2].Name)
	}
}

func TestDefaultObjectiveRegistry(t *testing.T) {
	reg := DefaultObjectiveRegistry()

	// Check that all three built-in objectives are registered
	objectives := []string{"weighted", "acceptance", "ground_truth"}

	for _, name := range objectives {
		def, ok := reg.Get(name)
		if !ok {
			t.Errorf("Expected built-in objective '%s' to be registered", name)
			continue
		}

		if def.Name != name {
			t.Errorf("Expected name '%s', got '%s'", name, def.Name)
		}

		if def.Version != "v1" {
			t.Errorf("Expected version 'v1' for '%s', got '%s'", name, def.Version)
		}

		if def.Description == "" {
			t.Errorf("Expected non-empty description for '%s'", name)
		}

		if len(def.InputFeatures) == 0 {
			t.Errorf("Expected non-empty input features for '%s'", name)
		}

		if def.Score == nil {
			t.Errorf("Expected non-nil Score function for '%s'", name)
		}
	}
}

func TestBuiltInObjective_Weighted(t *testing.T) {
	reg := DefaultObjectiveRegistry()
	def, ok := reg.Get("weighted")
	if !ok {
		t.Fatal("Expected 'weighted' objective to be registered")
	}

	// Create a sample result
	result := ComboResult{
		OverallAcceptMean:     0.8,
		MisalignmentRatioMean: 0.1,
		AlignmentDegMean:      5.0,
		NonzeroCellsMean:      100.0,
		ActiveTracksMean:      10.0,
	}

	// Use default weights
	weights := DefaultObjectiveWeights()

	score := def.Score(result, weights)

	// Verify the score matches what ScoreResult would return
	expectedScore := ScoreResult(result, weights)
	if score != expectedScore {
		t.Errorf("Expected score %.4f to match ScoreResult output %.4f", score, expectedScore)
	}

	// Verify score is non-zero and reasonable (acceptance alone should contribute 0.8)
	if score < 0.5 {
		t.Errorf("Expected score to be at least 0.5 with acceptance=0.8, got %.4f", score)
	}
}

func TestBuiltInObjective_Acceptance(t *testing.T) {
	reg := DefaultObjectiveRegistry()
	def, ok := reg.Get("acceptance")
	if !ok {
		t.Fatal("Expected 'acceptance' objective to be registered")
	}

	result := ComboResult{
		OverallAcceptMean:     0.9,
		MisalignmentRatioMean: 0.5,  // Should be ignored
		AlignmentDegMean:      10.0, // Should be ignored
	}

	// Weights should be overridden to acceptance-only
	weights := DefaultObjectiveWeights()

	score := def.Score(result, weights)

	// The score should only consider acceptance rate
	// With acceptance=0.9 and weight=1.0, score should be 0.9
	expectedScore := 0.9
	if score != expectedScore {
		t.Errorf("Expected score %.2f, got %.2f", expectedScore, score)
	}
}

func TestBuiltInObjective_GroundTruth(t *testing.T) {
	reg := DefaultObjectiveRegistry()
	def, ok := reg.Get("ground_truth")
	if !ok {
		t.Fatal("Expected 'ground_truth' objective to be registered")
	}

	result := ComboResult{
		OverallAcceptMean: 0.7,
	}

	weights := DefaultObjectiveWeights()

	// Ground truth scoring uses a fallback to ScoreResult
	score := def.Score(result, weights)

	// Just verify it doesn't panic and returns a value
	_ = score
}

func TestObjectiveRegistry_ThreadSafety(t *testing.T) {
	reg := NewObjectiveRegistry()

	// Launch multiple goroutines that register and read objectives
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			def := &ObjectiveDefinition{
				Name:          "test",
				Version:       "v1",
				Description:   "Concurrent test",
				InputFeatures: []string{"acceptance"},
				Score:         func(result ComboResult, weights ObjectiveWeights) float64 { return 0 },
			}
			reg.Register(def)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		go func() {
			reg.Get("test")
			reg.List()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final state
	def, ok := reg.Get("test")
	if !ok {
		t.Error("Expected 'test' objective to be registered")
	}
	if def != nil && def.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", def.Name)
	}
}
