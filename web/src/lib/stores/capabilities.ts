// src/lib/stores/capabilities.ts
// Store for sensor capabilities — fetched from /api/capabilities and
// refreshed periodically so the UI reflects runtime transitions
// (e.g. LiDAR coming online after the radar process starts).
//
// Polling strategy:
//   • An initial fetch always fires on startCapabilitiesPolling().
//   • A 30 s interval timer is only started when the response contains
//     at least one LiDAR sensor — LiDAR state transitions at runtime
//     (starting → ready → error) and need periodic updates.
//   • Radar-only deployments get a single static fetch; no timer.
//   • On fetch error the timer state is preserved so transient network
//     blips don't disrupt an already-running poll cycle.

import { writable, derived } from 'svelte/store';
import { getCapabilities, type Capabilities } from '../api';

/** Default capabilities: radar only, LiDAR disabled. */
const DEFAULT_CAPABILITIES: Capabilities = {
	radar: { default: { enabled: true, status: 'receiving' } },
	lidar: {}
};

/**
 * Polling interval in milliseconds (30 seconds).
 * Chosen to balance responsiveness (detect LiDAR state transitions
 * within ~30 s) against load on the Pi 4.
 */
export const POLL_INTERVAL_MS = 30_000;

/** Writable store holding the latest capabilities snapshot. */
export const capabilities = writable<Capabilities>(DEFAULT_CAPABILITIES);

/** Whether the initial fetch has completed (success or failure). */
export const capabilitiesLoaded = writable<boolean>(false);

/** Derived convenience: true when at least one LiDAR sensor is enabled. */
export const lidarEnabled = derived(capabilities, ($caps) =>
	Object.values($caps.lidar).some((s) => s.enabled)
);

/** Derived convenience: the "default" LiDAR sensor's runtime status, or
 *  'disabled' when no sensor named "default" exists. Single-sensor
 *  deployments always use "default"; multi-sensor setups may need a
 *  sensor selector in future. */
export const lidarState = derived(
	capabilities,
	($caps) => $caps.lidar['default']?.status ?? 'disabled'
);

let pollTimer: ReturnType<typeof setInterval> | null = null;
let pollingActive = false;

/** True when a periodic interval timer is running. */
export function isPolling(): boolean {
	return pollTimer !== null;
}

/** Start the interval timer if not already running and polling is active. */
function ensurePollTimer(): void {
	if (pollTimer !== null || !pollingActive) return;
	pollTimer = setInterval(refresh, POLL_INTERVAL_MS);
}

/** Clear the interval timer if one is running. */
function clearPollTimer(): void {
	if (pollTimer !== null) {
		clearInterval(pollTimer);
		pollTimer = null;
	}
}

/** Fetch capabilities once and update the store.
 *  After a successful fetch, starts or stops the poll timer based on
 *  whether LiDAR hardware is present. On error the timer state is
 *  left unchanged so transient failures don't break an active cycle. */
async function refresh(): Promise<void> {
	try {
		const caps = await getCapabilities();
		capabilities.set(caps);

		// LiDAR state can change at runtime; radar is static after startup.
		if (Object.keys(caps.lidar).length > 0) {
			ensurePollTimer();
		} else {
			clearPollTimer();
		}
	} catch (err) {
		// Endpoint unreachable — keep existing state and timer so
		// radar-only navigation remains stable and an active LiDAR
		// poll survives transient blips.
		console.warn('Failed to refresh capabilities:', err);
	} finally {
		capabilitiesLoaded.set(true);
	}
}

/**
 * Start capabilities monitoring. Safe to call multiple times —
 * subsequent calls are no-ops while monitoring is active.
 *
 * Performs an immediate fetch; a periodic poll timer is only started
 * if the response contains LiDAR hardware.
 */
export function startCapabilitiesPolling(): void {
	if (pollingActive) return;
	pollingActive = true;
	refresh();
}

/** Stop monitoring. Idempotent. */
export function stopCapabilitiesPolling(): void {
	pollingActive = false;
	clearPollTimer();
}
