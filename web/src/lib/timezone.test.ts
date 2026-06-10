import {
	AVAILABLE_TIMEZONES,
	getDisplayTimezone,
	getStoredTimezone,
	getTimezoneLabel,
	setStoredTimezone,
	type Timezone
} from './timezone';

describe('timezone', () => {
	describe('AVAILABLE_TIMEZONES', () => {
		it('should contain 56 timezone options', () => {
			expect(AVAILABLE_TIMEZONES).toHaveLength(56);
		});

		it('should have correct structure for each timezone', () => {
			AVAILABLE_TIMEZONES.forEach((tz) => {
				expect(tz).toHaveProperty('value');
				expect(tz).toHaveProperty('label');
				expect(typeof tz.value).toBe('string');
				expect(typeof tz.label).toBe('string');
			});
		});

		it('should include key timezones', () => {
			const values = AVAILABLE_TIMEZONES.map((tz) => tz.value);
			expect(values).toContain('America/New_York');
			expect(values).toContain('Europe/Dublin');
			expect(values).toContain('Asia/Singapore');
			expect(values).toContain('Pacific/Auckland');
		});

		it('should include flag emojis in labels', () => {
			const singaporeLabel = AVAILABLE_TIMEZONES.find((tz) => tz.value === 'Asia/Singapore')?.label;
			expect(singaporeLabel).toContain('🇸🇬');
		});
	});

	describe('getStoredTimezone', () => {
		it('should return stored timezone from localStorage', () => {
			window.localStorage.setItem('velocity-report-timezone', 'Europe/Dublin');
			expect(getStoredTimezone()).toBe('Europe/Dublin');
		});

		it('should return null when no timezone is stored', () => {
			expect(getStoredTimezone()).toBeNull();
		});

		it('should return null when window is undefined (SSR)', () => {
			const originalWindow = global.window;
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			delete (global as any).window;
			expect(getStoredTimezone()).toBeNull();
			global.window = originalWindow;
		});
	});

	describe('setStoredTimezone', () => {
		it('should store timezone in localStorage', () => {
			setStoredTimezone('America/New_York');
			expect(window.localStorage.getItem('velocity-report-timezone')).toBe('America/New_York');
		});

		it('should update existing stored timezone', () => {
			setStoredTimezone('Asia/Singapore');
			expect(window.localStorage.getItem('velocity-report-timezone')).toBe('Asia/Singapore');
			setStoredTimezone('Pacific/Auckland');
			expect(window.localStorage.getItem('velocity-report-timezone')).toBe('Pacific/Auckland');
		});

		it('should handle SSR gracefully when window is undefined', () => {
			const originalWindow = global.window;
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			delete (global as any).window;
			expect(() => setStoredTimezone('UTC')).not.toThrow();
			global.window = originalWindow;
		});
	});

	describe('getDisplayTimezone', () => {
		const realIntl = global.Intl;

		function mockBrowserTimezone(tz: string | null) {
			global.Intl = {
				...realIntl,
				DateTimeFormat: function () {
					return {
						resolvedOptions: () => ({ timeZone: tz })
					};
				}
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
			} as any;
		}

		afterEach(() => {
			global.Intl = realIntl;
		});

		it('returns stored timezone when available, regardless of server default', () => {
			window.localStorage.setItem('velocity-report-timezone', 'Asia/Seoul');
			mockBrowserTimezone('America/Los_Angeles');
			expect(getDisplayTimezone('UTC')).toBe('Asia/Seoul');
			expect(getDisplayTimezone('America/New_York')).toBe('Asia/Seoul');
		});

		it('prefers a non-UTC server default over the browser zone', () => {
			mockBrowserTimezone('America/Los_Angeles');
			expect(getDisplayTimezone('America/New_York')).toBe('America/New_York');
		});

		it('falls back to the browser zone when server default is UTC and browser is in allowlist', () => {
			mockBrowserTimezone('America/Los_Angeles');
			expect(getDisplayTimezone('UTC')).toBe('America/Los_Angeles');
		});

		it('returns UTC when server default is UTC and the browser zone is not in our allowlist', () => {
			mockBrowserTimezone('America/Boise');
			expect(getDisplayTimezone('UTC')).toBe('UTC');
		});

		it('returns UTC when resolving the browser zone throws', () => {
			// Intl.DateTimeFormat().resolvedOptions() blowing up must not crash
			// getDisplayTimezone — it falls back to the server default.
			global.Intl = {
				...realIntl,
				DateTimeFormat: function () {
					return {
						resolvedOptions: () => {
							throw new Error('Intl unavailable');
						}
					};
				}
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
			} as any;
			expect(getDisplayTimezone('UTC')).toBe('UTC');
		});

		it('returns UTC when server default is UTC and Intl is unavailable', () => {
			mockBrowserTimezone(null);
			expect(getDisplayTimezone('UTC')).toBe('UTC');
		});

		it('falls back to the default timezone when window is undefined (SSR)', () => {
			// In SSR, getStoredTimezone() returns null (no localStorage) and the
			// browser-zone probe is skipped (no globalThis.window). The function
			// must then return the server default.
			const originalWindow = global.window;
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			delete (global as any).window;
			mockBrowserTimezone(null);
			try {
				expect(getDisplayTimezone('UTC')).toBe('UTC');
				expect(getDisplayTimezone('America/New_York')).toBe('America/New_York');
			} finally {
				global.window = originalWindow;
			}
		});
	});

	describe('getTimezoneLabel', () => {
		it('should return correct label for UTC', () => {
			expect(getTimezoneLabel('UTC')).toBe('🇺🇳 UTC (+00:00)');
		});

		it('should return correct label for America/New_York', () => {
			expect(getTimezoneLabel('America/New_York')).toBe('🇺🇸 New York (-05:00/-04:00)');
		});

		it('should return correct label for Asia/Singapore', () => {
			expect(getTimezoneLabel('Asia/Singapore')).toBe('🇸🇬 Singapore (+08:00)');
		});

		it('should return correct label for Asia/Seoul', () => {
			expect(getTimezoneLabel('Asia/Seoul')).toBe('🇰🇷 Seoul (+09:00)');
		});

		it('should return correct label for Pacific/Bougainville', () => {
			expect(getTimezoneLabel('Pacific/Bougainville')).toBe('🇵🇬 Bougainville (+11:00)');
		});

		it('should return correct label for Europe/Athens', () => {
			expect(getTimezoneLabel('Europe/Athens')).toBe('🇬🇷 Athens (+02:00/+03:00)');
		});

		it('should return correct label for Africa/Lagos', () => {
			expect(getTimezoneLabel('Africa/Lagos')).toBe('🇳🇬 Lagos (+01:00)');
		});

		it('should return correct label for Africa/Abidjan', () => {
			expect(getTimezoneLabel('Africa/Abidjan')).toBe('🇨🇮 Abidjan (+00:00)');
		});

		it('should return correct label for America/Santiago', () => {
			expect(getTimezoneLabel('America/Santiago')).toBe('🇨🇱 Santiago (-04:00/-03:00)');
		});

		it('should return correct label for Atlantic/South_Georgia', () => {
			expect(getTimezoneLabel('Atlantic/South_Georgia')).toBe('🇬🇸 South Georgia (-02:00)');
		});

		it('should handle all timezones in AVAILABLE_TIMEZONES', () => {
			AVAILABLE_TIMEZONES.forEach((tz) => {
				const label = getTimezoneLabel(tz.value as Timezone);
				expect(label).toBe(tz.label);
				expect(label.length).toBeGreaterThan(0);
			});
		});

		it('should return the timezone as-is for unknown timezones', () => {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			expect(getTimezoneLabel('Unknown/Timezone' as any)).toBe('Unknown/Timezone');
		});
	});
});
