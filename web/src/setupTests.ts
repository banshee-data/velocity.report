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
} catch (e) {
	// Fallback: window.location is not configurable in this jsdom version/environment.
	// Patch the existing location object where possible without triggering navigation.
	// Assign no-op navigation methods and try to define readable properties.
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const loc: any = window.location;
	try {
		loc.assign = jest.fn();
	} catch (e) {
		void e;
	}
	try {
		loc.replace = jest.fn();
	} catch (e) {
		void e;
	}
	try {
		loc.reload = jest.fn();
	} catch (e) {
		void e;
	}

	const safeDefine = (obj: Record<string, unknown>, key: string, val: unknown) => {
		try {
			Object.defineProperty(obj, key, { configurable: true, value: val });
		} catch (e) {
			void e;
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
