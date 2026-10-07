import { writable } from 'svelte/store';
import { getCallerAccess, type AccessPermission, type CallerAccess } from '../api';

export interface AccessState {
	status: 'loading' | 'ready' | 'error';
	caller: CallerAccess | null;
	error: string;
}

export const access = writable<AccessState>({ status: 'loading', caller: null, error: '' });
export const ACCESS_POLL_INTERVAL_MS = 30_000;
const LOOKUP_TIMEOUT_MS = 5_000;
let timer: ReturnType<typeof setInterval> | null = null;
let controller: AbortController | null = null;
let generation = 0;

export function hasPermission(state: AccessState, permission: AccessPermission): boolean {
	return state.status === 'ready' && state.caller?.permissions.includes(permission) === true;
}

/** Gate page mounting as well as navigation: denied pages must not start protected fetches. */
export function pagePermission(pathname: string): AccessPermission {
	const path = pathname.replace(/^\/app(?=\/|$)/, '').replace(/\/$/, '') || '/';
	if (path === '/settings') return 'configuration:read';
	if (path.startsWith('/site/')) return 'configuration:write';
	if (path === '/lidar' || path.startsWith('/lidar/')) return 'configuration:write';
	if (path === '/reports') return 'reports:read';
	return 'aggregate:view';
}

/** Failed refreshes discard old grants; late responses cannot restore revoked access. */
export async function refreshAccess(): Promise<void> {
	const requestGeneration = ++generation;
	controller?.abort();
	const requestController = new AbortController();
	controller = requestController;
	const timeout = setTimeout(() => requestController.abort(), LOOKUP_TIMEOUT_MS);
	try {
		const caller = await getCallerAccess(requestController.signal);
		if (requestGeneration === generation) {
			access.set({ status: 'ready', caller, error: '' });
		}
	} catch {
		if (requestGeneration === generation) {
			access.set({
				status: 'error',
				caller: null,
				error: 'Your access could not be checked. Retry when the server is available.'
			});
		}
	} finally {
		clearTimeout(timeout);
		if (requestGeneration === generation) controller = null;
	}
}

export function startAccessPolling(): void {
	if (timer !== null) return;
	access.set({ status: 'loading', caller: null, error: '' });
	void refreshAccess();
	timer = setInterval(() => void refreshAccess(), ACCESS_POLL_INTERVAL_MS);
}

export function stopAccessPolling(): void {
	if (timer !== null) clearInterval(timer);
	timer = null;
	++generation;
	controller?.abort();
	controller = null;
	access.set({ status: 'loading', caller: null, error: '' });
}
