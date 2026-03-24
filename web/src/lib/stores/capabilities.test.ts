import { get } from 'svelte/store';
import {
	capabilities,
	capabilitiesLoaded,
	lidarEnabled,
	lidarState,
	startCapabilitiesPolling,
	stopCapabilitiesPolling
} from './capabilities';
import type { Capabilities } from '../api';

// Mock the api module
jest.mock('../api', () => ({
	getCapabilities: jest.fn()
}));

import { getCapabilities } from '../api';

// Helper to flush pending microtasks (Promise callbacks).
function flushPromises(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('capabilities store', () => {
	beforeEach(() => {
		jest.clearAllMocks();

		// Reset store to defaults
		capabilities.set({
			radar: { default: { enabled: true, status: 'receiving' } },
			lidar: {}
		});
		capabilitiesLoaded.set(false);

		// Ensure polling is stopped between tests
		stopCapabilitiesPolling();
	});

	afterEach(() => {
		stopCapabilitiesPolling();
	});

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
	});

	describe('derived stores', () => {
		it('lidarEnabled should be false when lidar map is empty', () => {
			expect(get(lidarEnabled)).toBe(false);
		});

		it('lidarEnabled should be true when any lidar sensor is enabled', () => {
			capabilities.set({
				radar: { default: { enabled: true, status: 'receiving' } },
				lidar: {
					default: { enabled: true, status: 'ready', sweep: true }
				}
			});

			expect(get(lidarEnabled)).toBe(true);
		});

		it('lidarState should be disabled when lidar map is empty', () => {
			expect(get(lidarState)).toBe('disabled');
		});

		it('lidarState should reflect default lidar sensor status', () => {
			capabilities.set({
				radar: { default: { enabled: true, status: 'receiving' } },
				lidar: {
					default: { enabled: true, status: 'starting', sweep: false }
				}
			});

			expect(get(lidarState)).toBe('starting');
		});
	});

	describe('startCapabilitiesPolling', () => {
		it('should fetch capabilities immediately on start', async () => {
			const mockCaps: Capabilities = {
				radar: { default: { enabled: true, status: 'receiving' } },
				lidar: {
					default: { enabled: true, status: 'ready', sweep: true }
				}
			};

			(getCapabilities as jest.Mock).mockResolvedValueOnce(mockCaps);

			startCapabilitiesPolling();

			// Flush the immediate async fetch
			await flushPromises();

			expect(getCapabilities).toHaveBeenCalledTimes(1);
			expect(get(capabilities)).toEqual(mockCaps);
			expect(get(capabilitiesLoaded)).toBe(true);
		});

		it('should not start a second timer if already polling', () => {
			(getCapabilities as jest.Mock).mockResolvedValue({
				radar: { default: { enabled: true, status: 'receiving' } },
				lidar: {}
			});

			startCapabilitiesPolling();
			startCapabilitiesPolling(); // second call is a no-op

			// Only one immediate fetch should have been triggered
			expect(getCapabilities).toHaveBeenCalledTimes(1);
		});

		it('should keep existing state when fetch fails', async () => {
			const initial = get(capabilities);
			(getCapabilities as jest.Mock).mockRejectedValueOnce(new Error('Network error'));
			const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});

			startCapabilitiesPolling();
			await flushPromises();

			// State unchanged, but loaded is set
			expect(get(capabilities)).toEqual(initial);
			expect(get(capabilitiesLoaded)).toBe(true);
			expect(warnSpy).toHaveBeenCalledWith('Failed to refresh capabilities:', expect.any(Error));
			warnSpy.mockRestore();
		});
	});

	describe('stopCapabilitiesPolling', () => {
		it('should be safe to call when not polling', () => {
			// Should not throw
			stopCapabilitiesPolling();
		});
	});
});
