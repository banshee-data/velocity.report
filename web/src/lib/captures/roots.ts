/**
 * Which capture volume the Captures page shows.
 *
 * The server keeps a volume it was once configured with as a disabled row, with
 * the scan state it last had, so the captures indexed under it keep their
 * provenance. Such a volume cannot be scanned: the scan route refuses it. A page
 * that opened on it showed an error nobody could clear, and none of the sessions
 * on the volume that is configured.
 */

import type { CaptureRoot } from '$lib/types/captures';

/**
 * pickActiveRoot returns the root the operator chose, else the first configured
 * one, else whatever there is.
 */
export function pickActiveRoot(
	roots: CaptureRoot[],
	selectedRootId: string | null
): CaptureRoot | null {
	return (
		roots.find((r) => r.root_id === selectedRootId) ??
		roots.find((r) => r.enabled) ??
		roots[0] ??
		null
	);
}

/** rootLabel names a root in the volume picker, marking one no longer configured. */
export function rootLabel(root: CaptureRoot): string {
	return root.enabled ? root.path : `${root.path} (no longer configured)`;
}
