#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
build_root="$repo_root/docs_html/_site"
index_file="$build_root/index.html"

if [[ ! -f "$index_file" ]]; then
	echo "Embedded offline docs build did not produce docs_html/_site/index.html" >&2
	exit 1
fi

# These routes have previously been served as a raw directory listing or a
# 404.  Keep them as shell pages in every embedded docs build.
for required_page in \
	"$build_root/docs/ui/design/index.html" \
	"$build_root/docs/ui/design/20260511-Velocity_Report_Butterfly_Net/index.html" \
	"$build_root/data/experiments/try/index.html"; do
	if [[ ! -f "$required_page" ]] || ! grep -q 'class="shell"' "$required_page"; then
		echo "Embedded offline docs route is missing its application shell: $required_page" >&2
		exit 1
	fi
done

# The reference graph page and its 3.6 MB graph belong to the public site
# (public_html/src/docs/reference-graph/). These docs are embedded in the
# binary, so fail if any of it is copied in, for instance by a reference-graph
# file landing back under data/ where the src/**/*.json passthrough finds it.
stray=$(find "$build_root" \( -iname '*reference-graph*' -o -path '*/reference-graph/*' \) -print | head -5)
if [[ -n "$stray" ]]; then
	echo "Embedded offline docs build contains reference graph files (public site only):" >&2
	echo "$stray" >&2
	exit 1
fi

echo "✓ Offline docs build successful"
echo "Build output size: $(du -sh "$build_root" | cut -f1)"
echo "Files generated: $(find "$build_root" -type f | wc -l | awk '{print $1}')"
