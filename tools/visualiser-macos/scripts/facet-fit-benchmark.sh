#!/bin/sh
# Standalone scalar fit timing; no sensor, tracker, decoding or UI cost.
set -eu
task_repo=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
task_build=$(mktemp -d)
trap 'rm -rf "$task_build"' EXIT HUP INT TERM
swiftc -O \
    "$task_repo/tools/visualiser-macos/VelocityVisualiser/Annotation/FacetGeometry.swift" \
    "$task_repo/tools/visualiser-macos/benchmarks/facet-fit.swift" \
    -o "$task_build/facet-fit"
"$task_build/facet-fit" "$@"
