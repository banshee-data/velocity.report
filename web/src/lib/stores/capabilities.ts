// src/lib/stores/capabilities.ts
// Store for sensor capabilities — fetched from /api/capabilities and
// refreshed periodically so the UI reflects runtime transitions
// (e.g. LiDAR coming online after the radar process starts).

import { writable, derived } from 'svelte/store';
import { getCapabilities, type Capabilities } from '../api';

/** Default capabilities: radar only, LiDAR disabled. */
const DEFAULT_CAPABILITIES: Capabilities = {
	radar: true,
	lidar: { enabled: false, state: 'disabled' },
	lidar_sweep: false
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

/** Derived convenience: true when the LiDAR subsystem is enabled. */
export const lidarEnabled = derived(capabilities, ($caps) => $caps.lidar.enabled);

/** Derived convenience: the LiDAR runtime state string. */
export const lidarState = derived(capabilities, ($caps) => $caps.lidar.state);

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
		console.warn('Could not refresh capabilities:', err);
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
