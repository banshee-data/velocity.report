package annotation

import "fmt"

// ReaderLevel is the reader level this build has. A record file says the
// lowest level that reads it correctly in min_reader: a pack's manifest.json,
// a label set's labels.json and physical-references.json. schema_version
// changes when a format breaks; min_reader rises when a writer uses something
// an older reader would misread or drop, so an older build refuses the file
// and says which build reads it, rather than reading it wrongly. A file with
// no min_reader is level 1.
//
// Raise ReaderLevel with the first writer that needs it, and name the level
// and what it adds in the label set README. Level 2 adds reviews pinned to
// the masks they rest on (MembershipPin.Masks).
const ReaderLevel = 2

// CheckReader refuses a file whose min_reader is above this build's level.
func CheckReader(what string, minReader int) error {
	if minReader > ReaderLevel {
		return fmt.Errorf("%s needs reader level %d, and this build reads level %d: update velocity.report to read it", what, minReader, ReaderLevel)
	}
	return nil
}
