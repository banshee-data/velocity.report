// src/lib/stores/capabilities.ts
// Store for sensor capabilities — fetched from /api/capabilities and
// refreshed periodically so the UI reflects runtime transitions
// (e.g. LiDAR coming online after the radar process starts).

import { writable, derived } from 'svelte/store';
import { getCapabilities, type Capabilities } from '../api';

/** Default capabilities: radar only, LiDAR disabled. */
const DEFAULT_CAPABILITIES: Capabilities = {
	radar: { default: { enabled: true, status: 'receiving' } },
	lidar: {}
};

/**
 * Polling interval in milliseconds (30 seconds).
 * Chosen to balance responsiveness (detect LiDAR coming online within
 * ~30 s) against load on the Pi 4. LiDAR state transitions are rare
 * (typically only at startup or manual enable/disable).
 */
const POLL_INTERVAL_MS = 30_000;

/** Writable store holding the latest capabilities snapshot. */
export const capabilities = writable<Capabilities>(DEFAULT_CAPABILITIES);

/** Whether the initial fetch has completed (success or failure). */
export const capabilitiesLoaded = writable<boolean>(false);

/** Derived convenience: true when any LiDAR sensor is enabled. */
export const lidarEnabled = derived(capabilities, ($caps) =>
	Object.values($caps.lidar).some((s) => s.enabled)
);

/** Derived convenience: the default LiDAR runtime status string, or 'disabled'. */
export const lidarState = derived(
	capabilities,
	($caps) => $caps.lidar['default']?.status ?? 'disabled'
);

let pollTimer: ReturnType<typeof setInterval> | null = null;

/** Fetch capabilities once and update the store. Logs errors but
 *  keeps the previous state so the UI degrades gracefully. */
async function refresh(): Promise<void> {
	try {
		const caps = await getCapabilities();
		capabilities.set(caps);
	} catch (err) {
		// Endpoint unreachable — keep existing state so radar-only
		// navigation remains stable.
		console.warn('Failed to refresh capabilities:', err);
	} finally {
		capabilitiesLoaded.set(true);
	}
}

/**
 * Start polling for capabilities. Safe to call multiple times —
 * subsequent calls are no-ops while a timer is active.
 */
export function startCapabilitiesPolling(): void {
	if (pollTimer !== null) return;

	// Fetch immediately, then poll.
	refresh();
	pollTimer = setInterval(refresh, POLL_INTERVAL_MS);
}

/** Stop polling. Idempotent. */
export function stopCapabilitiesPolling(): void {
	if (pollTimer !== null) {
		clearInterval(pollTimer);
		pollTimer = null;
	}
}
