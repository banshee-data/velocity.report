package annotation

import "strings"

// What a copy of a pack carries.
//
// A pack's data is fixed when it is exported. Its reference records are the
// work done on it. Everything else is the working state of whoever has it
// open: locks, the revision archives behind the records, the proposal layers
// the macOS client regenerates, and a segment job's record. A published copy
// carries data and records, and the dataset's Git history is the history of
// its records, as a published label set's is of its membership.

// PhysicalReferencesFile and FeatureProposalsFile are the reference records a
// pack keeps beside its data.
const (
	PhysicalReferencesFile = physicalReferenceFile
	FeatureProposalsFile   = featureFile
)

// proposalLayerDir holds the macOS client's proposal layers, which it writes
// when it opens a pack that has none.
const proposalLayerDir = "proposals"

// PackDataFiles names a pack's data files, the optional background included.
func PackDataFiles() []string {
	return []string{manifestFile, samplesFile, pointsFile, packetsFile, returnsFile, backgroundsFile, backgroundPointsFile}
}

// PackRecordFiles names the reference records a pack keeps.
func PackRecordFiles() []string {
	return []string{PhysicalReferencesFile, FeatureProposalsFile}
}

// IsPackLocalEntry reports whether a pack directory entry is working state
// that no copy of the pack carries.
func IsPackLocalEntry(name string) bool {
	switch name {
	case annotationLock, physicalRevisionDir, featureHistory, proposalLayerDir, segmentRecordFile, ".DS_Store":
		return true
	}
	return strings.HasSuffix(name, ".tmp")
}
