// Package toolargs holds the fixed argument contracts shared by the job worker
// and the offline experiment harness. It accepts typed fields, never commands.
package toolargs

import "strconv"

// Baseline names the inputs and outputs of the state-estimation tool.
type Baseline struct {
	Corpus, Index, CaptureRoot, Case, Out, Tuning string
	Duration, Warmup                              float64
	EvidenceDir, SourceManifest                   string
}

// Args returns the baseline tool's common argument sequence.
func (p Baseline) Args() []string {
	return []string{"-corpus", p.Corpus, "-index", p.Index, "-pcap-root", p.CaptureRoot,
		"-case", p.Case, "-out", p.Out, "-tuning", p.Tuning,
		"-duration", strconv.FormatFloat(p.Duration, 'f', -1, 64),
		"-warmup", strconv.FormatFloat(p.Warmup, 'f', -1, 64),
		"-evidence-dir", p.EvidenceDir, "-source-manifest", p.SourceManifest}
}
