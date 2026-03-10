#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "${SCRIPT_DIR}/.." && pwd)
WEB_DIR="${REPO_ROOT}/web"
ACTION="${1:-ensure}"
QUIET="${VELOCITY_WEB_CACHE_QUIET:-0}"

log() {
	if [ "${QUIET}" != "1" ]; then
		printf '%s\n' "$*"
	fi
}

COMMON_WORKTREE_ROOT=$(
	git -C "${REPO_ROOT}" worktree list --porcelain | awk '
		/^worktree / {
			sub("^worktree ", "", $0)
			print $0
			exit
		}
	'
)
COMMON_WEB_DIR="${VELOCITY_SHARED_WEB_DIR:-${COMMON_WORKTREE_ROOT}/web}"
SHARED_NODE_MODULES="${COMMON_WEB_DIR}/node_modules"
WORKTREE_NODE_MODULES="${WEB_DIR}/node_modules"

shared_cache_ready() {
	[ -x "${SHARED_NODE_MODULES}/.bin/prettier" ]
}

has_usable_local_node_modules_dir() {
	[ -d "${WORKTREE_NODE_MODULES}" ] && [ ! -L "${WORKTREE_NODE_MODULES}" ] && [ -x "${WORKTREE_NODE_MODULES}/.bin/prettier" ]
}

has_replaceable_local_node_modules_dir() {
	[ -d "${WORKTREE_NODE_MODULES}" ] && [ ! -L "${WORKTREE_NODE_MODULES}" ] && [ ! -x "${WORKTREE_NODE_MODULES}/.bin/prettier" ]
}

ensure_symlink() {
	if [ "${WEB_DIR}" = "${COMMON_WEB_DIR}" ]; then
		return 0
	fi

	if has_usable_local_node_modules_dir; then
		log "Using existing ${WORKTREE_NODE_MODULES}; shared cache activation skipped."
		return 0
	fi

	if has_replaceable_local_node_modules_dir; then
		rm -rf "${WORKTREE_NODE_MODULES}"
	fi

	if [ -L "${WORKTREE_NODE_MODULES}" ]; then
		local current_target
		current_target=$(readlink "${WORKTREE_NODE_MODULES}" || true)
		if [ "${current_target}" = "${SHARED_NODE_MODULES}" ]; then
			return 0
		fi
		rm "${WORKTREE_NODE_MODULES}"
	fi

	ln -s "${SHARED_NODE_MODULES}" "${WORKTREE_NODE_MODULES}"
	log "Linked ${WORKTREE_NODE_MODULES} -> ${SHARED_NODE_MODULES}"
}

install_shared_cache() {
	if shared_cache_ready; then
		return 0
	fi

	if has_usable_local_node_modules_dir; then
		log "Using existing ${WORKTREE_NODE_MODULES}; shared cache bootstrap skipped."
		return 0
	fi

	if ! command -v pnpm >/dev/null 2>&1; then
		if [ "${ACTION}" = "install" ]; then
			printf 'pnpm is required to install the shared web cache.\n' >&2
			exit 1
		fi
		log "pnpm not found; shared web cache bootstrap skipped."
		return 0
	fi

	log "Installing shared web dependencies in ${COMMON_WEB_DIR}..."
	pnpm --dir "${COMMON_WEB_DIR}" install --frozen-lockfile
	log "Shared web dependencies ready."
}

case "${ACTION}" in
	print-root)
		printf '%s\n' "${COMMON_WEB_DIR}"
		;;
	print-node-modules)
		printf '%s\n' "${SHARED_NODE_MODULES}"
		;;
	ensure|install)
		install_shared_cache
		if shared_cache_ready; then
			ensure_symlink
		fi
		;;
	*)
		printf 'Unknown action: %s\n' "${ACTION}" >&2
		exit 1
		;;
esac
