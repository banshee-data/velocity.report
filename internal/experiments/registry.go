package experiments

import (
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs"
)

// The closed vocabularies a spec is checked against. Each is small on
// purpose: a new entry is a decision, made here with its description, not a
// string a spec invents.

// Corpus is a named set of cases an experiment fans out over.
type Corpus struct {
	Cases       int
	Description string
}

// Corpora are the case sets experiments run on today.
var Corpora = map[string]Corpus{
	"kirk0":             {1, "kirk0, the in-repository capture (83 s), with the reviewed pack ad8b9438 and annotation ground truth 8e422582"},
	"kirk-labelled":     {2, "kirk0 and kirk1's first 272 s, the two captures with labelled road users (runs dd98c68e, 60a4774c)"},
	"d2-first-segment":  {23, "the first five-minute segment of the 23 tuning and screen sites; embarcadero-folsom held out"},
	"d2-second-segment": {22, "the second segment of the same sites less ashbury-downey, which has one capture; held out from every first-segment screen"},
	"held-out-site":     {1, "embarcadero-folsom, the declared held-out site, read only for a frozen held-out score"},
	"lombard-laguna":    {1, "the 39-minute Lombard–Laguna placement, eight segments in order: the long capture for arrival and scene-change behaviour"},
	"s2-placements":     {23, "the whole placements of the 23 tuning and screen sites, raw captures read for geometry only; embarcadero-folsom held out"},
}

// Capabilities are what a worker can offer a step: data it can read, tools
// or hardware it has. A worker advertises its own; a step runs only on a
// worker that has all of the experiment's.
var Capabilities = map[string]string{
	"pcap:kirk":    "the kirk captures (kirk0 in the repository, kirk1 on the LiDAR volume)",
	"pcap:s2":      "the S2 corpus captures, on the LiDAR volume or the NAS",
	"labels:kirk":  "kirk0 and kirk1 labels: the deployment database's runs and the annotation packs",
	"macos-review": "a person at the macOS annotate-and-grade tool",
	"pi":           "a Raspberry Pi 5 for timing on the deployment target",
	"pcap-build":   "a Go toolchain and libpcap, to build the tools at the job's commit",
}

// ExportOptions are the default-off options an arm's scene export can name
// (`velocity scene export` flags without their dashes), for steps that
// export web scenes. An arm that names one is the same replay as an arm
// that does not, exported differently.
var ExportOptions = map[string]string{
	"smoothed_trail": "--smoothed-trail: the trail as the whole-track RTS smoother's estimate rather than the online posterior",
}

// WorkerClasses are the machines an estimate can be made for.
var WorkerClasses = map[string]string{
	"m1pro":  "the development Mac (M1 Pro, 8 cores, 16 GB)",
	"swan":   "the Unraid blade worker",
	"pi5":    "the deployment Raspberry Pi 5",
	"person": "a person's time, for manual steps",
}

// ManualKinds are steps a person closes. Each is completed by recording the
// artefact named here, which the hub checks before the experiment continues.
var ManualKinds = map[string]string{
	"manual:implement": "a commit on main (the PR that built what the next step needs)",
	"manual:capture":   "a capture registered in the capture index, by SHA-256",
	"manual:label":     "track labels on a reference run, meeting the step's gate",
	"manual:annotate":  "an annotation pack, by digest",
	"manual:review":    "a graded review pack, by digest",
	"manual:freeze":    "a frozen split, by SHA-256 (velocity lidar annotation-split freeze)",
	"manual:decide":    "a decision entry in docs/DECISIONS.md (D-NN)",
	"manual:write":     "the report, merged at the spec's report path",
}

// PlannedKinds are job steps no worker can run yet: the tool exists or is
// designed, but it is not a job kind. An experiment with one is not
// runnable by a worker until the kind lands in internal/lidar/jobs.
var PlannedKinds = map[string]string{
	"bench_sweep":       "a lidar-bench grid scored against kirk0's annotation ground truth",
	"ground_truth_eval": "lidar-ground-truth-eval perframe against a frozen split",
	"corpus_screen":     "label-free cluster and track counts over a corpus case set",
	"scene_variants":    "one window replayed under each tuning config of a set, exported as web scenes",
	"pi_timing":         "frame-time tails on the deployment target",
	"chart":             "a chart rendered from step summaries",
	"box_eval":          "lidar-box-eval: one replay scoring the drawn box, and box-model variants beside it, for containment and turning",
}

// KindClass says how a step kind is executed.
type KindClass int

const (
	KindUnknown KindClass = iota
	// KindJob is a kind in internal/lidar/jobs: a worker can run it now.
	KindJob
	// KindPlanned is designed but has no executor yet.
	KindPlanned
	// KindManual is closed by a person.
	KindManual
)

// ClassifyKind places a step kind in its class.
func ClassifyKind(kind string) KindClass {
	if _, ok := jobs.Kinds[kind]; ok {
		return KindJob
	}
	if _, ok := PlannedKinds[kind]; ok {
		return KindPlanned
	}
	if _, ok := ManualKinds[kind]; ok {
		return KindManual
	}
	return KindUnknown
}

// KnownKinds lists every accepted step kind, sorted.
func KnownKinds() []string {
	var out []string
	for k := range jobs.Kinds {
		out = append(out, k)
	}
	for k := range PlannedKinds {
		out = append(out, k)
	}
	for k := range ManualKinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
