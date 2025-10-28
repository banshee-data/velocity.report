import '@testing-library/jest-dom';

// Mock window.location safely. jsdom may make window.location non-configurable,
// so attempt to redefine it and fall back to patching the existing object.
const mockLocation = {
	origin: 'http://localhost:3000',
	href: 'http://localhost:3000',
	pathname: '/',
	search: '',
	hash: '',
	assign: jest.fn(),
	replace: jest.fn(),
	reload: jest.fn()
};

try {
	// Try to replace the whole property (works when configurable)
	Object.defineProperty(window, 'location', {
		configurable: true,
		value: mockLocation as unknown as Location
	});
} catch {
	// Fallback: window.location is not configurable in this jsdom version/environment.
	// Patch the existing location object where possible without triggering navigation.
	// Assign no-op navigation methods and try to define readable properties.
	const loc = window.location as unknown as Record<string, unknown>;
	/* eslint-disable @typescript-eslint/no-explicit-any */
	try {
		// attempt to set no-op navigation methods
		(loc as any).assign = jest.fn();
	} catch {
		/* ignore */
	}
	try {
		(loc as any).replace = jest.fn();
	} catch {
		/* ignore */
	}
	try {
		(loc as any).reload = jest.fn();
	} catch {
		/* ignore */
	}
	/* eslint-enable @typescript-eslint/no-explicit-any */

	const safeDefine = (obj: Record<string, unknown>, key: string, val: unknown) => {
		try {
			Object.defineProperty(obj, key, { configurable: true, value: val });
		} catch {
			/* ignore */
		}
	};

	safeDefine(loc, 'origin', mockLocation.origin);
	safeDefine(loc, 'href', mockLocation.href);
	safeDefine(loc, 'pathname', mockLocation.pathname);
	safeDefine(loc, 'search', mockLocation.search);
	safeDefine(loc, 'hash', mockLocation.hash);
}

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
