#!/usr/bin/env bash
# Run the same demand under each car-following model and print per-lane counting-line headways.
#
# Usage: compare.sh [model ...]   (default: krauss idm eidm; each must be a vType id in routes.rou.xml)
#
# Writes out/routes.<model>.rou.xml, out/counting-lines.<model>.add.xml and their outputs. Run
# sumo-gui with --output-prefix (see the README) so an open window never writes the same files.
set -euo pipefail
: "${SUMO_HOME:?set SUMO_HOME to the SUMO install}"
cd "$(dirname "$0")"
[ -f colbway.net.xml ] || { echo "colbway.net.xml missing: run build-net.sh first" >&2; exit 1; }
[ $# -gt 0 ] || set -- krauss idm eidm
mkdir -p out
for m in "$@"; do
  sed "s/CFTYPE/$m/g" routes.rou.xml >"out/routes.$m.rou.xml"
  # SUMO resolves a detector's file= relative to the additional file, which lives in out/.
  sed "s|det.out.xml|det.$m.xml|" counting-lines.add.xml >"out/counting-lines.$m.add.xml"
  "$SUMO_HOME/bin/sumo" -c colbway.sumocfg -r "out/routes.$m.rou.xml" \
    -a "out/counting-lines.$m.add.xml" --tripinfo-output "out/trip.$m.xml" 2>&1 |
    grep -E 'Inserted|TimeLoss|Error' | tr -s ' \n' ' '
  echo
  python3 headways.py "out/det.$m.xml"
done
