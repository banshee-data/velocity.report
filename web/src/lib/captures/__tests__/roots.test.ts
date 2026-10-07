import type { CaptureRoot } from '$lib/types/captures';
import { pickActiveRoot, rootLabel } from '../roots';

function root(id: string, path: string, enabled: boolean): CaptureRoot {
	return {
		root_id: id,
		path,
		enabled,
		last_scan_state: enabled ? 'ok' : 'error',
		last_scan_error: enabled ? undefined : 'context canceled',
		created_at_ns: 0,
		updated_at_ns: 0
	};
}

const dropped = root('root-s2', '/Volumes/lidar/lidar/s2', false);
const configured = root('root-lidar', '/Volumes/lidar/lidar', true);

describe('pickActiveRoot', () => {
	it('opens on the configured root when a dropped one is listed first', () => {
		expect(pickActiveRoot([dropped, configured], null)).toBe(configured);
	});

	it('keeps the root the operator chose, even a dropped one', () => {
		expect(pickActiveRoot([dropped, configured], 'root-s2')).toBe(dropped);
	});

	it('falls back to a configured root when the chosen one has gone', () => {
		expect(pickActiveRoot([dropped, configured], 'root-gone')).toBe(configured);
	});

	it('shows a dropped root when it is the only one', () => {
		expect(pickActiveRoot([dropped], null)).toBe(dropped);
	});

	it('returns null with no roots', () => {
		expect(pickActiveRoot([], null)).toBeNull();
	});
});

describe('rootLabel', () => {
	it('names a configured root by its path', () => {
		expect(rootLabel(configured)).toBe('/Volumes/lidar/lidar');
	});

	it('marks a dropped root', () => {
		expect(rootLabel(dropped)).toBe('/Volumes/lidar/lidar/s2 (no longer configured)');
	});
});
