// Mock implementations for SvelteKit navigation
export const goto = jest.fn();
export const invalidate = jest.fn();
export const invalidateAll = jest.fn();
export const preloadData = jest.fn();
export const preloadCode = jest.fn();
export const beforeNavigate = jest.fn();
export const afterNavigate = jest.fn();
export const pushState = jest.fn();
export const replaceState = jest.fn();
