#!/bin/sh
# Clone one capture's labels from the sf-street-speeds dataset, for labelling:
# its label sets and their views, and the dataset's index and rules, without
# the captures. The full dataset is about 74 GB; this is the capture's
# labels/ folder (kirk0's is 65 MB) and the plain Git files.
#
#   clone-labels.sh <capture stem> [dir]
#
#   <capture stem>  the capture's folder under labels/, which is its raw file's
#                   name without the extension, for example
#                   20251205T1417_80858-75c_kirkham-9th
#   [dir]           where to clone (default: sf-street-speeds)
#
# Environment:
#   LABELS_DATASET_URL  the dataset to clone (default: the Hub's https URL)
#   VELOCITY            the velocity binary to run 'labels git-setup' with
#                       (default: velocity on PATH, if there is one)
#
# The clone skips LFS/Xet content at checkout (GIT_LFS_SKIP_SMUDGE), checks out
# only labels/<stem>/ and the dataset's top-level files, then fetches that
# folder's LFS/Xet content alone. It takes every plain-Git blob, which is
# small, since the captures are LFS/Xet pointers there: the Hub accepts a
# partial clone (--filter=blob:none) and then cannot serve the blobs it left
# out, so none is asked for. 'git sparse-checkout add labels/<stem>' and
# 'git lfs pull --include "labels/<stem>/**"' later add another capture.
set -eu

usage() {
	sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-2}"
}

[ $# -ge 1 ] || usage
case $1 in -h | --help) usage 0 ;; esac
stem=${1%/}
dir=${2:-sf-street-speeds}
url=${LABELS_DATASET_URL:-https://huggingface.co/datasets/banshee-data/sf-street-speeds}

case $stem in
*/* | .* | '') echo "clone-labels: $stem is not a capture folder name" >&2; exit 2 ;;
esac
[ ! -e "$dir" ] || { echo "clone-labels: $dir exists" >&2; exit 1; }

GIT_LFS_SKIP_SMUDGE=1 git clone --quiet --no-checkout "$url" "$dir"
cd "$dir"
git sparse-checkout set --no-cone \
	"/labels/$stem/" /manifest.json /README.md /.gitattributes /.gitignore
GIT_LFS_SKIP_SMUDGE=1 git checkout --quiet
[ -d "labels/$stem" ] || {
	echo "clone-labels: the dataset has no labels/$stem; captures with labels:" >&2
	git ls-tree -d --name-only HEAD labels/ 2>/dev/null | sed 's|^labels/|  |' >&2
	exit 1
}
if git lfs version >/dev/null 2>&1; then
	git lfs pull --include="labels/$stem/**"
else
	echo "clone-labels: git-lfs is not installed: install it (and git-xet), then run" >&2
	echo "  git lfs pull --include=\"labels/$stem/**\"" >&2
fi

velocity=${VELOCITY:-$(command -v velocity || true)}
if [ -n "$velocity" ]; then
	"$velocity" lidar labels git-setup --repo . >/dev/null
	echo "cloned labels/$stem into $dir, with the label set merge drivers and hooks"
else
	echo "cloned labels/$stem into $dir; now run: velocity lidar labels git-setup --repo $dir"
fi
