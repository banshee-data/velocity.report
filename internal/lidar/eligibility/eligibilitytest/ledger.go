package eligibilitytest

import (
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
)

// MemoryLedger is an in-memory use ledger for tests that have no database.
type MemoryLedger struct {
	mu   sync.Mutex
	Uses []eligibility.Use
}

// Record appends the uses.
func (l *MemoryLedger) Record(uses []eligibility.Use) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Uses = append(l.Uses, uses...)
	return nil
}

// Exposures are the capture's allowed development reads.
func (l *MemoryLedger) Exposures(sha string) ([]eligibility.Use, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []eligibility.Use
	for _, u := range l.Uses {
		dev := u.Purpose == string(eligibility.PurposeTuning) || u.Purpose == string(eligibility.PurposeDeveloperDiagnostic) ||
			(u.Purpose == string(eligibility.PurposeDevelopmentEvaluation) && !u.HeldOutScore)
		if u.CaptureSHA256 == sha && u.Decision == "allowed" && dev {
			out = append(out, u)
		}
	}
	return out, nil
}

// List returns every use, newest last.
func (l *MemoryLedger) List(eligibility.UseQuery) ([]eligibility.Use, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]eligibility.Use{}, l.Uses...), len(l.Uses), nil
}

// Summary is not kept in memory.
func (l *MemoryLedger) Summary() ([]eligibility.ExposureSummary, error) {
	return []eligibility.ExposureSummary{}, nil
}
