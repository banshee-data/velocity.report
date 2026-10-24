// Mock implementations for SvelteKit paths: the app is served under /app.
export const base = '/app';
export const assets = '';
export function resolve(path: string): string {
	return `${base}${path}`;
}
