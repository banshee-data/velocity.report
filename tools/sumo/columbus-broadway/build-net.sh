#!/usr/bin/env bash
# Build the Columbus Ave / Broadway SUMO network from an OpenStreetMap extract.
#
# Usage: build-net.sh <extract.osm>
#
# The extract must cover the bounding box below. The edge IDs in routes.rou.xml and
# counting-lines.add.xml come from OSM way IDs, so a different extract may need them updated.
set -euo pipefail
: "${SUMO_HOME:?set SUMO_HOME to the SUMO install (its bin/ and data/ are used)}"
osm=${1:?usage: build-net.sh <extract.osm>}
osm="$(cd "$(dirname "$osm")" && pwd)/$(basename "$osm")"
cd "$(dirname "$0")"

# sf25.typ.xml sets 25 mph for roads OSM gives no limit; the default primary road speed is 62 mph.
# A 25 mph network falls below netconvert's 49 km/h right-before-left threshold, which would turn
# Columbus's mid-block junctions into yield-to-the-right; the threshold flag keeps them priority.
"$SUMO_HOME/bin/netconvert" --osm-files "$osm" \
  --type-files "$SUMO_HOME/data/typemap/osmNetconvert.typ.xml,sf25.typ.xml" \
  --keep-edges.in-geo-boundary -122.4110,37.7955,-122.4020,37.8010 \
  --keep-edges.by-vclass passenger --remove-edges.isolated --geometry.remove \
  --junctions.join --junctions.right-before-left.speed-threshold 5 \
  --tls.guess-signals --tls.discard-simple --tls.join \
  --output.street-names --output.original-names --proj.utm \
  -o colbway.net.xml
