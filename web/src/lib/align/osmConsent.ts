/**
 * The operator's choice to allow OpenStreetMap requests (tiles, address
 * search, outlines) on the Locate page, remembered in this browser so the
 * question is asked once rather than on every visit. It stays an explicit
 * opt-in (TENETS.md §1): nothing is requested until the operator has said yes
 * here, and forgetting it puts the question back.
 */
export const OSM_CONSENT_KEY = 'velocity.report.osmConsent';

/** Whether this browser has allowed OpenStreetMap requests. */
export function osmConsentRemembered(
	storage: Storage | undefined = globalThis.localStorage
): boolean {
	try {
		return storage?.getItem(OSM_CONSENT_KEY) === 'allowed';
	} catch {
		// Storage can be unavailable (private windows, blocked site data).
		return false;
	}
}

/** Records or forgets the choice. Failure to store it only means asking again. */
export function rememberOsmConsent(
	allowed: boolean,
	storage: Storage | undefined = globalThis.localStorage
): void {
	try {
		if (allowed) storage?.setItem(OSM_CONSENT_KEY, 'allowed');
		else storage?.removeItem(OSM_CONSENT_KEY);
	} catch {
		// Nothing to do: the page asks again next time.
	}
}
