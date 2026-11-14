# Experiment corpora

`first-segment-corpus.json` and `first-segment-site-index.json` select the first indexed
capture from each of the 23 tuning and screening sites in
`tools/s2-archive/site-index.json`. Embarcadero–Folsom is held out and omitted.
Capture order and compass bearings come from that index. These are authored input
manifests, not experiment output; source bytes are hashed again before each replay.

`kirk-labelled-corpus.json` and `kirk-labelled-site-index.json` select the
existing kirk0 capture for development scoring. The name identifies the specs'
corpus contract; labels and frozen scoring references must be supplied separately.
Host-specific mount paths belong in the harness host profile's per-corpus
`capture_root`, never in these portable indexes.
