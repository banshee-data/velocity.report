package config

import "fmt"

// The engine each layer runs on this branch. The registry in tuning.go lists
// every engine the schema can describe, so a config naming any of them parses
// and validates; these are the ones with code behind them.
//
// Nothing below the config dispatches on the selector. The background model,
// the clusterer and the tracker read the selected block's common fields
// through ActiveCommon, and every L3 engine shares L3Common, so a config
// naming another engine would run these algorithms under that engine's name.
const (
	ImplementedL3Engine = "ema_baseline_v1"
	ImplementedL4Engine = "dbscan_xy_v1"
	ImplementedL5Engine = "cv_kf_v1"
)

// ValidateImplementedEngines refuses a config whose engine selector names an
// algorithm this branch parses but does not run.
//
// Every path that builds the pipeline from a tuning config calls it: the
// server at startup, and replay, lidar-bench, pcap-split and settling-eval
// once they have loaded or resolved their config. Without it an offline run
// or sweep arm under `l3.engine: "ema_track_assist_v2"` builds the EMA
// baseline from the shared L3 block and reports results that look like a
// different algorithm's. ema_track_assist_v2 is refused rather than treated
// as an alias: its promotion keys have no consumer, so running it as the
// baseline would drop them without a word.
//
// "none" at L4 or L5 is accepted. It is a depth rather than an engine, it is
// how the reduced profiles in config/profiles/ are written, and Validate
// already keeps the disabled layers a suffix.
func (c *TuningConfig) ValidateImplementedEngines() error {
	if c.L3.Engine != ImplementedL3Engine {
		return UnsupportedEngineError("l3", c.L3.Engine, ImplementedL3Engine)
	}
	if c.L4.Engine != ImplementedL4Engine && c.L4.Engine != EngineNone {
		return UnsupportedEngineError("l4", c.L4.Engine, ImplementedL4Engine)
	}
	if c.L5.Engine != ImplementedL5Engine && c.L5.Engine != EngineNone {
		return UnsupportedEngineError("l5", c.L5.Engine, ImplementedL5Engine)
	}
	return nil
}

// UnsupportedEngineError is the one wording for a refused engine selector, so
// the live server and the offline tools fail with the same text.
func UnsupportedEngineError(layer, engine, implemented string) error {
	return fmt.Errorf("unsupported %s.engine %q: this branch only implements %s at runtime", layer, engine, implemented)
}
