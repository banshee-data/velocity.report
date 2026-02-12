package sweep

// Schema contracts for JSON payloads persisted in lidar_sweeps and related tables.
// These constants document the expected structure of versioned JSON columns,
// enabling backward-compatible evolution and validation.

// SchemaVersion is the current version of the JSON payload contracts.
const SchemaVersion = "1"

// RoundRecordSchemaVersion identifies the schema version of round_results JSON.
// The round_results column in lidar_sweeps stores a JSON array of RLHFRound objects.
// Each RLHFRound follows the schema defined by the RLHFRound struct.
const RoundRecordSchemaVersion = "1"

// ObjectiveComponentSchemaVersion identifies the schema version of score_components_json.
// The score_components_json column stores a ScoreComponents JSON object.
const ObjectiveComponentSchemaVersion = "1"

// RecommendationSchemaVersion identifies the schema version of recommendation_explanation_json.
// The recommendation_explanation_json column stores a ScoreExplanation JSON object.
const RecommendationSchemaVersion = "1"

// LabelProvenanceSchemaVersion identifies the schema version of label_provenance_summary_json.
// The label_provenance_summary_json column stores a LabelProvenanceSummary JSON object.
const LabelProvenanceSchemaVersion = "1"

// LabelProvenanceSummary aggregates label source counts for a sweep's reference runs.
type LabelProvenanceSummary struct {
	SchemaVersion string          `json:"schema_version"`
	TotalTracks   int             `json:"total_tracks"`
	LabelledCount int             `json:"labelled_count"`
	BySource      map[string]int  `json:"by_source"` // human_manual, carried_over, auto_suggested
	ByClass       map[string]int  `json:"by_class"`  // vehicle, pedestrian, etc.
	Confidence    ConfidenceStats `json:"confidence"`
}

// ConfidenceStats summarises label confidence across a sweep.
type ConfidenceStats struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}
