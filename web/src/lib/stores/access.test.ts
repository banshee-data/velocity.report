import { get } from 'svelte/store';
import { getCallerAccess, type CallerAccess } from '../api';
import {
	access,
	hasPermission,
	pagePermission,
	refreshAccess,
	startAccessPolling,
	stopAccessPolling,
	ACCESS_POLL_INTERVAL_MS
} from './access';

jest.mock('../api', () => ({ getCallerAccess: jest.fn() }));
const admin: CallerAccess = {
	profile: 'hardened',
	mechanism: 'tailscale-direct',
	authenticated: true,
	permissions: ['aggregate:view', 'reports:read', 'configuration:write']
};
const viewer: CallerAccess = { ...admin, permissions: ['aggregate:view', 'reports:read'] };

beforeEach(() => {
	jest.useFakeTimers();
	stopAccessPolling();
	jest.resetAllMocks();
});
afterEach(() => {
	stopAccessPolling();
	jest.useRealTimers();
});

test('page guards protect deep links and permissions remain absent until resolved', () => {
	expect(hasPermission(get(access), 'configuration:write')).toBe(false);
	expect(pagePermission('/app/settings')).toBe('configuration:read');
	expect(pagePermission('/app/site/42')).toBe('configuration:write');
	expect(pagePermission('/app/lidar/tracks')).toBe('configuration:write');
	expect(pagePermission('/app/reports/')).toBe('reports:read');
	expect(pagePermission('/app/')).toBe('aggregate:view');
});

test('refresh failures discard previously granted permission', async () => {
	(getCallerAccess as jest.Mock)
		.mockResolvedValueOnce(admin)
		.mockRejectedValueOnce(new Error('offline'));
	await refreshAccess();
	expect(hasPermission(get(access), 'configuration:write')).toBe(true);
	await refreshAccess();
	expect(get(access).status).toBe('error');
	expect(get(access).caller).toBeNull();
	expect(hasPermission(get(access), 'configuration:write')).toBe(false);
});

test('an older response cannot restore permission after revocation', async () => {
	let resolveOlder!: (value: CallerAccess) => void;
	(getCallerAccess as jest.Mock)
		.mockImplementationOnce(
			() =>
				new Promise<CallerAccess>((resolve) => {
					resolveOlder = resolve;
				})
		)
		.mockResolvedValueOnce(viewer);
	const older = refreshAccess();
	await refreshAccess();
	resolveOlder(admin);
	await older;
	expect(get(access).caller).toEqual(viewer);
});

test('polling refreshes once per interval and stopping rejects pending results', async () => {
	(getCallerAccess as jest.Mock).mockResolvedValue(admin);
	startAccessPolling();
	startAccessPolling();
	await jest.advanceTimersByTimeAsync(0);
	expect(getCallerAccess).toHaveBeenCalledTimes(1);
	await jest.advanceTimersByTimeAsync(ACCESS_POLL_INTERVAL_MS);
	expect(getCallerAccess).toHaveBeenCalledTimes(2);
	stopAccessPolling();
	await jest.advanceTimersByTimeAsync(ACCESS_POLL_INTERVAL_MS);
	expect(getCallerAccess).toHaveBeenCalledTimes(2);
	expect(hasPermission(get(access), 'configuration:write')).toBe(false);
});

test('stopping while a request is pending cannot repopulate the store', async () => {
	let resolve!: (value: CallerAccess) => void;
	(getCallerAccess as jest.Mock).mockImplementation(
		() =>
			new Promise<CallerAccess>((r) => {
				resolve = r;
			})
	);
	const request = refreshAccess();
	stopAccessPolling();
	resolve(admin);
	await request;
	expect(get(access).status).toBe('loading');
	expect(get(access).caller).toBeNull();
});
