import '@testing-library/jest-dom';

// Mock window.location
// eslint-disable-next-line @typescript-eslint/no-explicit-any
delete (window as any).location;
window.location = {
	origin: 'http://localhost:3000',
	href: 'http://localhost:3000',
	pathname: '/',
	search: '',
	hash: ''
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
} as any;

// Mock localStorage
const localStorageMock = (() => {
	let store: Record<string, string> = {};

	return {
		getItem: (key: string) => store[key] || null,
		setItem: (key: string, value: string) => {
			store[key] = value.toString();
		},
		removeItem: (key: string) => {
			delete store[key];
		},
		clear: () => {
			store = {};
		}
	};
})();

Object.defineProperty(window, 'localStorage', {
	value: localStorageMock
});

// Reset localStorage before each test
beforeEach(() => {
  window.localStorage.clear();
});
