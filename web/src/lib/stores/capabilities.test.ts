import { get } from 'svelte/store';
import {
	capabilities,
	capabilitiesLoaded,
	isPolling,
	lidarEnabled,
	lidarState,
	POLL_INTERVAL_MS,
	startCapabilitiesPolling,
	stopCapabilitiesPolling
} from './capabilities';
import type { Capabilities } from '../api';

// Mock the api module
jest.mock('../api', () => ({
	getCapabilities: jest.fn()
}));

import { getCapabilities } from '../api';

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

/** Radar-only capabilities — no LiDAR hardware. */
const RADAR_ONLY: Capabilities = {
	radar: { default: { enabled: true, status: 'receiving' } },
	lidar: {}
};

/** Radar + LiDAR ready. */
const LIDAR_READY: Capabilities = {
	radar: { default: { enabled: true, status: 'receiving' } },
	lidar: { default: { enabled: true, status: 'ready', sweep: true } }
};

/** Radar + LiDAR starting (sweep not yet available). */
const LIDAR_STARTING: Capabilities = {
	radar: { default: { enabled: true, status: 'receiving' } },
	lidar: { default: { enabled: true, status: 'starting', sweep: false } }
};

/** Dev/test: no sensors at all. */
const EMPTY_BOTH: Capabilities = {
	radar: {},
	lidar: {}
};

/** LiDAR only — no radar connected (lidar-only dev rig). */
const LIDAR_ONLY: Capabilities = {
	radar: {},
	lidar: { hesai: { enabled: true, status: 'receiving', sweep: false } }
};

/** Multiple LiDAR sensors, one disabled. */
const MULTI_LIDAR: Capabilities = {
	radar: { default: { enabled: true, status: 'receiving' } },
	lidar: {
		hesai_front: { enabled: true, status: 'ready', sweep: true },
		hesai_rear: { enabled: false, status: 'disabled', sweep: false }
	}
};

describe('capabilities store', () => {
	beforeEach(() => {
		jest.clearAllMocks();
		jest.useFakeTimers();

		// Reset store to defaults
		capabilities.set({
			radar: { default: { enabled: true, status: 'receiving' } },
			lidar: {}
		});
		capabilitiesLoaded.set(false);

		// Ensure monitoring is stopped between tests
		stopCapabilitiesPolling();
	});

	afterEach(() => {
		stopCapabilitiesPolling();
		jest.useRealTimers();
	});

	// -----------------------------------------------------------------------
	// Default state
	// -----------------------------------------------------------------------
	describe('default state', () => {
		it('should have radar enabled and LiDAR map empty by default', () => {
			const caps = get(capabilities);
			expect(caps.radar['default']?.enabled).toBe(true);
			expect(caps.radar['default']?.status).toBe('receiving');
			expect(Object.keys(caps.lidar)).toHaveLength(0);
		});

		it('should not be loaded initially', () => {
			expect(get(capabilitiesLoaded)).toBe(false);
		});

		it('should not be polling initially', () => {
			expect(isPolling()).toBe(false);
		});
	});

	// -----------------------------------------------------------------------
	// Derived stores
	// -----------------------------------------------------------------------
	describe('derived stores', () => {
		it('lidarEnabled should be false when lidar map is empty', () => {
			expect(get(lidarEnabled)).toBe(false);
		});

		it('lidarEnabled should be true when any lidar sensor is enabled', () => {
			capabilities.set(LIDAR_READY);
			expect(get(lidarEnabled)).toBe(true);
		});

		it('lidarEnabled should be true when at least one of several sensors is enabled', () => {
			capabilities.set(MULTI_LIDAR);
			expect(get(lidarEnabled)).toBe(true);
		});

		it('lidarEnabled should be false when all lidar sensors are disabled', () => {
			capabilities.set({
				radar: { default: { enabled: true, status: 'receiving' } },
				lidar: {
					hesai: { enabled: false, status: 'disabled', sweep: false }
				}
			});
			expect(get(lidarEnabled)).toBe(false);
		});

		it('lidarState should be disabled when lidar map is empty', () => {
			expect(get(lidarState)).toBe('disabled');
		});

		it('lidarState should reflect default lidar sensor status', () => {
			capabilities.set(LIDAR_STARTING);
			expect(get(lidarState)).toBe('starting');
		});

		it('lidarState should be disabled when no "default" lidar sensor exists', () => {
			capabilities.set(LIDAR_ONLY); // has "hesai", not "default"
			expect(get(lidarState)).toBe('disabled');
		});

		it('lidarState should return "default" sensor status even with multiple sensors', () => {
			capabilities.set({
				radar: { default: { enabled: true, status: 'receiving' } },
				lidar: {
					default: { enabled: true, status: 'ready', sweep: true },
					hesai_kerb: { enabled: true, status: 'error', sweep: false }
				}
			});
			expect(get(lidarState)).toBe('ready');
		});
	});

	// -----------------------------------------------------------------------
	// Polling lifecycle — radar only (no timer)
	// -----------------------------------------------------------------------
	describe('radar-only (no LiDAR) — static fetch, no polling', () => {
		it('should fetch once and NOT start a poll timer', async () => {
			(getCapabilities as jest.Mock).mockResolvedValueOnce(RADAR_ONLY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			expect(getCapabilities).toHaveBeenCalledTimes(1);
			expect(get(capabilities)).toEqual(RADAR_ONLY);
			expect(get(capabilitiesLoaded)).toBe(true);
			expect(isPolling()).toBe(false);
		});

		it('should not poll even after advancing timers', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(RADAR_ONLY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);

			// Only the initial fetch — no timer ticks.
			expect(getCapabilities).toHaveBeenCalledTimes(1);
		});
	});

	// -----------------------------------------------------------------------
	// Polling lifecycle — LiDAR present (timer starts)
	// -----------------------------------------------------------------------
	describe('LiDAR present — poll timer starts', () => {
		it('should start the poll timer after fetching LiDAR capabilities', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(LIDAR_READY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			expect(isPolling()).toBe(true);
		});

		it('should fire subsequent fetches on each interval tick', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(LIDAR_READY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0); // initial fetch
			expect(getCapabilities).toHaveBeenCalledTimes(1);

			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
			expect(getCapabilities).toHaveBeenCalledTimes(2);

			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
			expect(getCapabilities).toHaveBeenCalledTimes(3);
		});

		it('should stop the timer when LiDAR disappears from the response', async () => {
			// First fetch: LiDAR present → timer starts.
			(getCapabilities as jest.Mock).mockResolvedValueOnce(LIDAR_STARTING);
			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);
			expect(isPolling()).toBe(true);

			// Second fetch: radar only → timer stops.
			(getCapabilities as jest.Mock).mockResolvedValueOnce(RADAR_ONLY);
			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
			expect(isPolling()).toBe(false);

			// No further fetches after timer stopped.
			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
			expect(getCapabilities).toHaveBeenCalledTimes(2);
		});
	});

	// -----------------------------------------------------------------------
	// Empty maps — dev/edge scenarios
	// -----------------------------------------------------------------------
	describe('empty maps', () => {
		it('should handle both maps empty gracefully', async () => {
			(getCapabilities as jest.Mock).mockResolvedValueOnce(EMPTY_BOTH);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			expect(get(capabilities)).toEqual(EMPTY_BOTH);
			expect(get(lidarEnabled)).toBe(false);
			expect(get(lidarState)).toBe('disabled');
			expect(isPolling()).toBe(false);
		});

		it('should handle lidar-only (no radar) with polling', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(LIDAR_ONLY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			expect(get(capabilities)).toEqual(LIDAR_ONLY);
			expect(get(lidarEnabled)).toBe(true);
			// "default" not present → falls back to 'disabled'
			expect(get(lidarState)).toBe('disabled');
			expect(isPolling()).toBe(true);
		});
	});

	// -----------------------------------------------------------------------
	// Error handling
	// -----------------------------------------------------------------------
	describe('error handling', () => {
		it('should keep existing state when fetch fails', async () => {
			const initial = get(capabilities);
			(getCapabilities as jest.Mock).mockRejectedValueOnce(new Error('Network error'));
			const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			expect(get(capabilities)).toEqual(initial);
			expect(get(capabilitiesLoaded)).toBe(true);
			expect(warnSpy).toHaveBeenCalledWith('Failed to refresh capabilities:', expect.any(Error));
			warnSpy.mockRestore();
		});

		it('should NOT start timer when initial fetch fails', async () => {
			(getCapabilities as jest.Mock).mockRejectedValueOnce(new Error('down'));
			const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);

			expect(isPolling()).toBe(false);
			warnSpy.mockRestore();
		});

		it('should preserve running timer when a subsequent fetch fails', async () => {
			const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});

			// Initial fetch succeeds with LiDAR → timer starts.
			(getCapabilities as jest.Mock).mockResolvedValueOnce(LIDAR_READY);
			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);
			expect(isPolling()).toBe(true);

			// Second fetch fails → timer still running.
			(getCapabilities as jest.Mock).mockRejectedValueOnce(new Error('blip'));
			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
			expect(isPolling()).toBe(true);
			expect(get(capabilities)).toEqual(LIDAR_READY); // unchanged

			// Third fetch succeeds again.
			(getCapabilities as jest.Mock).mockResolvedValueOnce(LIDAR_READY);
			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
			expect(getCapabilities).toHaveBeenCalledTimes(3);

			warnSpy.mockRestore();
		});
	});

	// -----------------------------------------------------------------------
	// Idempotency / lifecycle
	// -----------------------------------------------------------------------
	describe('startCapabilitiesPolling', () => {
		it('should not start a second monitor if already active', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(LIDAR_READY);

			startCapabilitiesPolling();
			startCapabilitiesPolling(); // second call is a no-op

			await jest.advanceTimersByTimeAsync(0);
			expect(getCapabilities).toHaveBeenCalledTimes(1);
		});
	});

	describe('stopCapabilitiesPolling', () => {
		it('should be safe to call when not monitoring', () => {
			stopCapabilitiesPolling();
			expect(isPolling()).toBe(false);
		});

		it('should stop an active timer', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(LIDAR_READY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);
			expect(isPolling()).toBe(true);

			stopCapabilitiesPolling();
			expect(isPolling()).toBe(false);

			// No more fetches after stop.
			await jest.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
			expect(getCapabilities).toHaveBeenCalledTimes(1);
		});

		it('should allow restarting after stop', async () => {
			(getCapabilities as jest.Mock).mockResolvedValue(LIDAR_READY);

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);
			expect(getCapabilities).toHaveBeenCalledTimes(1);

			stopCapabilitiesPolling();

			startCapabilitiesPolling();
			await jest.advanceTimersByTimeAsync(0);
			expect(getCapabilities).toHaveBeenCalledTimes(2);
			expect(isPolling()).toBe(true);
		});
	});
});
