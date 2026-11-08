package sweep

import (
	"encoding/json"
	"fmt"
)

// autoCheckpoint is what an auto-tune keeps in a checkpoint's results: every
// combination's result so far and the summary of each round that finished.
// The summaries carry each round's top results, so the best result across the
// finished rounds can be rebuilt from them when the tune resumes; without
// them a resumed tune recommended the best of only the rounds it ran itself.
type autoCheckpoint struct {
	Results      []ComboResult  `json:"results"`
	RoundResults []RoundSummary `json:"round_results"`
}

// encodeAutoCheckpoint marshals the results and finished rounds for a
// checkpoint.
func encodeAutoCheckpoint(results []ComboResult, rounds []RoundSummary) (json.RawMessage, error) {
	if results == nil {
		results = []ComboResult{}
	}
	if rounds == nil {
		rounds = []RoundSummary{}
	}
	return json.Marshal(autoCheckpoint{Results: results, RoundResults: rounds})
}

// decodeAutoCheckpoint reads a checkpoint's results. A checkpoint written
// before round summaries were kept holds a bare array of results; it decodes
// with no rounds, so a tune resumed from it starts its best afresh as before.
func decodeAutoCheckpoint(data json.RawMessage) (autoCheckpoint, error) {
	var cp autoCheckpoint
	trimmed := firstNonSpace(data)
	switch trimmed {
	case 0:
		return cp, nil
	case '[':
		if err := json.Unmarshal(data, &cp.Results); err != nil {
			return cp, fmt.Errorf("checkpoint results: %w", err)
		}
		return cp, nil
	default:
		if err := json.Unmarshal(data, &cp); err != nil {
			return cp, fmt.Errorf("checkpoint results: %w", err)
		}
		return cp, nil
	}
}

func firstNonSpace(data []byte) byte {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return b
		}
	}
	return 0
}

// bestOfRounds is the best result across finished rounds: the highest-scoring
// first entry of any round's top results, the same comparison runFromRound
// makes as each round finishes (an earlier round wins a tie). Nil when no
// round has a top result.
func bestOfRounds(rounds []RoundSummary) *ScoredResult {
	var best *ScoredResult
	for i := range rounds {
		if len(rounds[i].TopK) == 0 {
			continue
		}
		candidate := rounds[i].TopK[0]
		if best == nil || candidate.Score > best.Score {
			c := candidate
			best = &c
		}
	}
	return best
}
