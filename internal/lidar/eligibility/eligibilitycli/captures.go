package eligibilitycli

import (
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/version"
)

// AuthorizeCaptures gates every capture before packet parsing or output creation.
// The entire capture is requested: warm-up and pre-roll also expose samples.
// Identities come from bytes, never from a caller-supplied site or run ID.
func (f *CLIFlags) AuthorizeCaptures(tool string, paths []string) error {
	req := eligibility.Request{Purpose: eligibility.PurposeDevelopmentEvaluation, BuildVersion: version.Version, BuildGitSHA: version.GitSHA}
	hasher := eligibility.NewHasher()
	for _, path := range paths {
		id, err := hasher.Capture(path)
		req.Inputs = append(req.Inputs, eligibility.Input{Identity: id, Err: err})
	}
	_, err := f.Authorize(tool, req)
	return err
}
