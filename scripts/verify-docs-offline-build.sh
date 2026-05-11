#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "$0")/.." && pwd)
build_root="$repo_root/docs_html/_site"
index_file="$build_root/index.html"

if [[ ! -f "$index_file" ]]; then
	echo "Embedded offline docs build did not produce docs_html/_site/index.html" >&2
	exit 1
fi

echo "✓ Offline docs build successful"
echo "Build output size: $(du -sh "$build_root" | cut -f1)"
echo "Files generated: $(find "$build_root" -type f | wc -l | awk '{print $1}')"
