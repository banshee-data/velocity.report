import { OSM_CONSENT_KEY, osmConsentRemembered, rememberOsmConsent } from './osmConsent';

describe('OpenStreetMap consent', () => {
	beforeEach(() => localStorage.clear());

	it('is not given until remembered, and can be forgotten', () => {
		expect(osmConsentRemembered()).toBe(false);
		rememberOsmConsent(true);
		expect(localStorage.getItem(OSM_CONSENT_KEY)).toBe('allowed');
		expect(osmConsentRemembered()).toBe(true);
		rememberOsmConsent(false);
		expect(osmConsentRemembered()).toBe(false);
	});

	it('treats unusable storage as no consent', () => {
		const broken = {
			getItem: () => {
				throw new Error('blocked');
			},
			setItem: () => {
				throw new Error('blocked');
			},
			removeItem: () => {
				throw new Error('blocked');
			}
		} as unknown as Storage;
		expect(osmConsentRemembered(broken)).toBe(false);
		expect(() => rememberOsmConsent(true, broken)).not.toThrow();
		expect(() => rememberOsmConsent(false, broken)).not.toThrow();
		expect(osmConsentRemembered(undefined)).toBe(false);
	});
});
