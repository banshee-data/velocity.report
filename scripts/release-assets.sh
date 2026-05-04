#!/usr/bin/env bash
# Release asset helpers for GitHub Actions and local release dry-runs.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE_DIR="$REPO_ROOT/image"

ref_name_looks_like_tag() {
    [[ -n "${GITHUB_REF_NAME:-}" && "${GITHUB_REF_NAME}" =~ ^v[0-9] ]]
}

release_tag() {
    if [[ -n "${RELEASE_TAG:-}" ]]; then
        printf '%s\n' "$RELEASE_TAG"
    elif [[ "${GITHUB_REF_TYPE:-}" == "tag" ]] || ref_name_looks_like_tag; then
        printf '%s\n' "$GITHUB_REF_NAME"
    else
        printf '%s\n' ""
    fi
}

release_version() {
    local raw="${VERSION:-}"
    if [[ -z "$raw" ]]; then
        raw="$(release_tag)"
    fi
    if [[ -z "$raw" ]]; then
        raw="dev"
    fi
    printf '%s\n' "${raw#v}"
}

git_sha() {
    git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || printf '%s\n' "unknown"
}

build_time() {
    if [[ -n "${BUILD_TIME:-}" ]]; then
        printf '%s\n' "$BUILD_TIME"
    else
        date -u +"%Y-%m-%dT%H:%M:%SZ"
    fi
}

ensure_github_release() {
    local tag
    tag="$(release_tag)"
    if [[ -z "$tag" ]]; then
        echo "release tag is required; set GITHUB_REF_NAME or RELEASE_TAG" >&2
        exit 2
    fi
    if [[ -z "${GH_TOKEN:-}" || -z "${GH_REPO:-}" ]]; then
        echo "GH_TOKEN and GH_REPO are required to ensure a GitHub release" >&2
        exit 2
    fi

    if gh release view "$tag" >/dev/null 2>&1; then
        echo "Release $tag already exists."
        return 0
    fi

    if ! gh api "repos/$GH_REPO/git/refs/tags/$tag" >/dev/null 2>&1; then
        echo "::error::Expected pushed tag $tag to exist before publishing the release."
        exit 1
    fi

    local create_args=(
        "$tag"
        --verify-tag
        --title "$tag"
        --notes ""
    )

    if [[ "$tag" == *-* ]]; then
        create_args+=(--prerelease)
    fi

    if gh release create "${create_args[@]}"; then
        return 0
    fi

    if gh release view "$tag" >/dev/null 2>&1; then
        echo "Release $tag appeared during create; continuing."
        return 0
    fi

    echo "::error::Failed to create or reuse release $tag."
    exit 1
}

build_linux_binaries() {
    local version
    local sha
    local stamp
    version="$(release_version)"
    sha="$(git_sha)"
    stamp="$(build_time)"

    VERSION="$version" GIT_SHA="$sha" BUILD_TIME="$stamp" "$IMAGE_DIR/scripts/build-image.sh" --binaries-only
}

build_darwin_radar() {
    local version
    local sha
    local stamp
    version="$(release_version)"
    sha="$(git_sha)"
    stamp="$(build_time)"

    make -C "$REPO_ROOT" VERSION="$version" BUILD_TIME="$stamp" build-embedded-assets
    "$REPO_ROOT/scripts/ensure-web-stub.sh"
    "$REPO_ROOT/scripts/ensure-docs-stub.sh"

    (
        cd "$REPO_ROOT"
        CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
            go build -tags=pcap \
            -ldflags "-X 'github.com/banshee-data/velocity.report/internal/version.Version=${version}' -X 'github.com/banshee-data/velocity.report/internal/version.GitSHA=${sha}' -X 'github.com/banshee-data/velocity.report/internal/version.BuildTime=${stamp}'" \
            -o "velocity-report-${version}-darwin-arm64" \
            ./cmd/radar
    )

    write_github_env RADAR_PATH "velocity-report-${version}-darwin-arm64"
}

package_linux_radar() {
    local version
    version="$(release_version)"
    mkdir -p "$REPO_ROOT/dist/linux"
    mv "$REPO_ROOT/dist/linux/velocity-report" "$REPO_ROOT/dist/linux/velocity-report-${version}-linux-arm64"
    write_github_env RADAR_PATH "dist/linux/velocity-report-${version}-linux-arm64"
}

build_image_from_staged_binaries() {
    local version
    local stamp
    version="$(release_version)"
    stamp="$(build_time)"
    VERSION="$version" BUILD_TIME="$stamp" "$IMAGE_DIR/scripts/build-image.sh" --skip-binaries
}

normalize_image_artifact() {
    local image_path
    image_path="$(find "$IMAGE_DIR/.pi-gen/deploy" -name '*.img.xz' -type f -exec ls -t {} + 2>/dev/null | head -n1)"
    if [[ -z "$image_path" ]]; then
        echo "::error::No .img.xz file found in image/.pi-gen/deploy"
        exit 1
    fi

    local final_image_path="$image_path"
    local tag
    tag="$(release_tag)"
    if [[ -n "$tag" ]]; then
        local version
        version="${tag#v}"
        final_image_path="$(dirname "$image_path")/velocity-report-${version}.img.xz"
        mv "$image_path" "$final_image_path"
    fi

    write_github_env IMAGE_PATH "$final_image_path"
    echo "Image size: $(du -h "$final_image_path" | cut -f1)"
}

write_github_env() {
    local name="$1"
    local value="$2"
    if [[ -n "${GITHUB_ENV:-}" ]]; then
        printf '%s=%s\n' "$name" "$value" >> "$GITHUB_ENV"
    else
        printf '%s=%s\n' "$name" "$value"
    fi
}

usage() {
    cat <<'EOF'
Usage: scripts/release-assets.sh <command>

Commands:
  ensure-github-release
  build-linux-binaries
  build-darwin-radar
  package-linux-radar
  build-image-from-staged-binaries
  normalize-image-artifact
EOF
}

case "${1:-}" in
    ensure-github-release)
        ensure_github_release
        ;;
    build-linux-binaries)
        build_linux_binaries
        ;;
    build-darwin-radar)
        build_darwin_radar
        ;;
    package-linux-radar)
        package_linux_radar
        ;;
    build-image-from-staged-binaries)
        build_image_from_staged_binaries
        ;;
    normalize-image-artifact)
        normalize_image_artifact
        ;;
    -h|--help|help|"")
        usage
        ;;
    *)
        usage >&2
        exit 2
        ;;
esac
