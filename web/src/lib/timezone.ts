// src/lib/timezone.ts
// Utility functions for managing timezones in localStorage
// Ordered from west to east: -11:00 (Niue) to +14:00 (Kiritimati)

export type Timezone =
	| 'Pacific/Niue'
	| 'America/Adak'
	| 'Pacific/Honolulu'
	| 'Pacific/Marquesas'
	| 'America/Anchorage'
	| 'Pacific/Gambier'
	| 'America/Los_Angeles'
	| 'Pacific/Pitcairn'
	| 'America/Denver'
	| 'America/Phoenix'
	| 'America/Chicago'
	| 'America/Mexico_City'
	| 'America/New_York'
	| 'America/Lima'
	| 'America/Barbados'
	| 'America/Santiago'
	| 'America/St_Johns'
	| 'America/Miquelon'
	| 'America/Sao_Paulo'
	| 'America/Godthab'
	| 'Atlantic/South_Georgia'
	| 'Atlantic/Azores'
	| 'Atlantic/Cape_Verde'
	| 'UTC'
	| 'Africa/Abidjan'
	| 'Europe/Dublin'
	| 'Antarctica/Troll'
	| 'Africa/Lagos'
	| 'Europe/Berlin'
	| 'Africa/Johannesburg'
	| 'Europe/Athens'
	| 'Africa/Nairobi'
	| 'Asia/Tehran'
	| 'Asia/Dubai'
	| 'Asia/Kabul'
	| 'Asia/Karachi'
	| 'Asia/Kolkata'
	| 'Asia/Kathmandu'
	| 'Asia/Dhaka'
	| 'Asia/Yangon'
	| 'Asia/Bangkok'
	| 'Asia/Singapore'
	| 'Australia/Eucla'
	| 'Asia/Seoul'
	| 'Australia/Darwin'
	| 'Australia/Adelaide'
	| 'Australia/Brisbane'
	| 'Australia/Sydney'
	| 'Australia/Lord_Howe'
	| 'Pacific/Bougainville'
	| 'Pacific/Norfolk'
	| 'Pacific/Fiji'
	| 'Pacific/Auckland'
	| 'Pacific/Chatham'
	| 'Pacific/Apia'
	| 'Pacific/Kiritimati';

const TIMEZONE_STORAGE_KEY = 'velocity-report-timezone';

export function getStoredTimezone(): Timezone | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem(TIMEZONE_STORAGE_KEY) as Timezone | null;
}

export function setStoredTimezone(timezone: Timezone): void {
	if (typeof window === 'undefined') return;
	localStorage.setItem(TIMEZONE_STORAGE_KEY, timezone);
}

export function getDisplayTimezone(defaultTimezone: string): Timezone {
	const stored = getStoredTimezone();
	if (stored) return stored;
	if (defaultTimezone && defaultTimezone !== 'UTC') return defaultTimezone as Timezone;

	// Server default is UTC and the user has no stored preference: prefer the
	// browser's IANA zone when it is in our allowlist. This makes the date
	// picker, Vehicle Count card, and chart agree on "today" for first-time
	// visitors whose data crosses the UTC day boundary in their local evening.
	const browser = browserTimezone();
	if (browser && AVAILABLE_TIMEZONES.some((t) => t.value === browser)) {
		return browser as Timezone;
	}
	return (defaultTimezone || 'UTC') as Timezone;
}

function browserTimezone(): string | null {
	// Skip the Intl probe in SSR / non-browser environments. Otherwise Node's
	// Intl resolves to the server's process TZ, which is almost never what
	// the eventual browser client will see. We probe `globalThis.window`
	// instead of `typeof window` because jsdom-based tests can drop the
	// window global without the `typeof` keyword reporting it as undefined.
	const g = typeof globalThis !== 'undefined' ? (globalThis as { window?: unknown }) : null;
	if (!g || g.window === undefined) return null;
	if (typeof Intl === 'undefined' || typeof Intl.DateTimeFormat !== 'function') return null;
	try {
		return Intl.DateTimeFormat().resolvedOptions().timeZone || null;
	} catch {
		return null;
	}
}

export function getTimezoneLabel(timezone: Timezone): string {
	switch (timezone) {
		case 'Pacific/Niue':
			return '🇳🇺 Niue (-11:00)';
		case 'America/Adak':
			return '🇺🇸 Adak (-10:00/-09:00)';
		case 'Pacific/Honolulu':
			return '🇺🇸 Honolulu (-10:00)';
		case 'Pacific/Marquesas':
			return '🇵🇫 Marquesas (-09:30)';
		case 'America/Anchorage':
			return '🇺🇸 Anchorage (-09:00/-08:00)';
		case 'Pacific/Gambier':
			return '🇵🇫 Gambier (-09:00)';
		case 'America/Los_Angeles':
			return '🇺🇸 Los Angeles (-08:00/-07:00)';
		case 'Pacific/Pitcairn':
			return '🇵🇳 Pitcairn (-08:00)';
		case 'America/Denver':
			return '🇺🇸 Denver (-07:00/-06:00)';
		case 'America/Phoenix':
			return '🇺🇸 Phoenix (-07:00)';
		case 'America/Chicago':
			return '🇺🇸 Chicago (-06:00/-05:00)';
		case 'America/Mexico_City':
			return '🇲🇽 Mexico City (-06:00)';
		case 'America/New_York':
			return '🇺🇸 New York (-05:00/-04:00)';
		case 'America/Lima':
			return '🇵🇪 Lima (-05:00)';
		case 'America/Barbados':
			return '🇧🇧 Barbados (-04:00)';
		case 'America/Santiago':
			return '🇨🇱 Santiago (-04:00/-03:00)';
		case 'America/St_Johns':
			return "🇨🇦 St. John's (-03:30/-02:30)";
		case 'America/Miquelon':
			return '🇵🇲 Miquelon (-03:00/-02:00)';
		case 'America/Sao_Paulo':
			return '🇧🇷 São Paulo (-03:00)';
		case 'America/Godthab':
			return '🇬🇱 Godthab/Nuuk (-02:00/-01:00)';
		case 'Atlantic/South_Georgia':
			return '🇬🇸 South Georgia (-02:00)';
		case 'Atlantic/Azores':
			return '🇵🇹 Azores (-01:00/+00:00)';
		case 'Atlantic/Cape_Verde':
			return '🇨🇻 Cape Verde (-01:00)';
		case 'UTC':
			return '🇺🇳 UTC (+00:00)';
		case 'Africa/Abidjan':
			return '🇨🇮 Abidjan (+00:00)';
		case 'Europe/Dublin':
			return '🇮🇪 Dublin (+00:00/+01:00)';
		case 'Antarctica/Troll':
			return '🇦🇶 Troll (+00:00/+02:00)';
		case 'Africa/Lagos':
			return '🇳🇬 Lagos (+01:00)';
		case 'Europe/Berlin':
			return '🇩🇪 Berlin (+01:00/+02:00)';
		case 'Africa/Johannesburg':
			return '🇿🇦 Johannesburg (+02:00)';
		case 'Europe/Athens':
			return '🇬🇷 Athens (+02:00/+03:00)';
		case 'Africa/Nairobi':
			return '🇰🇪 Nairobi (+03:00)';
		case 'Asia/Tehran':
			return '🇮🇷 Tehran (+03:30)';
		case 'Asia/Dubai':
			return '🇦🇪 Dubai (+04:00)';
		case 'Asia/Kabul':
			return '🇦🇫 Kabul (+04:30)';
		case 'Asia/Karachi':
			return '🇵🇰 Karachi (+05:00)';
		case 'Asia/Kolkata':
			return '🇮🇳 Mumbai (+05:30)';
		case 'Asia/Kathmandu':
			return '🇳🇵 Kathmandu (+05:45)';
		case 'Asia/Dhaka':
			return '🇧🇩 Dhaka (+06:00)';
		case 'Asia/Yangon':
			return '🇲🇲 Yangon (+06:30)';
		case 'Asia/Bangkok':
			return '🇹🇭 Bangkok (+07:00)';
		case 'Asia/Singapore':
			return '🇸🇬 Singapore (+08:00)';
		case 'Australia/Eucla':
			return '🇦🇺 Eucla (+08:45)';
		case 'Asia/Seoul':
			return '🇰🇷 Seoul (+09:00)';
		case 'Australia/Darwin':
			return '🇦🇺 Darwin (+09:30)';
		case 'Australia/Adelaide':
			return '🇦🇺 Adelaide (+09:30/+10:30)';
		case 'Australia/Brisbane':
			return '🇦🇺 Brisbane (+10:00)';
		case 'Australia/Sydney':
			return '🇦🇺 Sydney (+10:00/+11:00)';
		case 'Australia/Lord_Howe':
			return '🇦🇺 Lord Howe (+10:30/+11:00)';
		case 'Pacific/Bougainville':
			return '🇵🇬 Bougainville (+11:00)';
		case 'Pacific/Norfolk':
			return '🇦🇺 Norfolk (+11:00/+12:00)';
		case 'Pacific/Fiji':
			return '🇫🇯 Fiji (+12:00)';
		case 'Pacific/Auckland':
			return '🇳🇿 Auckland (+12:00/+13:00)';
		case 'Pacific/Chatham':
			return '🇳🇿 Chatham (+12:45/+13:45)';
		case 'Pacific/Apia':
			return '🇼🇸 Apia (+13:00)';
		case 'Pacific/Kiritimati':
			return '🇰🇮 Kiritimati (+14:00)';
		default:
			return timezone;
	}
}

export const AVAILABLE_TIMEZONES: { value: Timezone; label: string }[] = [
	{ value: 'Pacific/Niue', label: '🇳🇺 Niue (-11:00)' },
	{ value: 'Pacific/Honolulu', label: '🇺🇸 Honolulu (-10:00)' },
	{ value: 'America/Adak', label: '🇺🇸 Adak (-10:00/-09:00)' },
	{ value: 'Pacific/Marquesas', label: '🇵🇫 Marquesas (-09:30)' },
	{ value: 'Pacific/Gambier', label: '🇵🇫 Gambier (-09:00)' },
	{ value: 'America/Anchorage', label: '🇺🇸 Anchorage (-09:00/-08:00)' },
	{ value: 'Pacific/Pitcairn', label: '🇵🇳 Pitcairn (-08:00)' },
	{ value: 'America/Los_Angeles', label: '🇺🇸 Los Angeles (-08:00/-07:00)' },
	{ value: 'America/Phoenix', label: '🇺🇸 Phoenix (-07:00)' },
	{ value: 'America/Denver', label: '🇺🇸 Denver (-07:00/-06:00)' },
	{ value: 'America/Mexico_City', label: '🇲🇽 Mexico City (-06:00)' },
	{ value: 'America/Chicago', label: '🇺🇸 Chicago (-06:00/-05:00)' },
	{ value: 'America/Lima', label: '🇵🇪 Lima (-05:00)' },
	{ value: 'America/New_York', label: '🇺🇸 New York (-05:00/-04:00)' },
	{ value: 'America/Barbados', label: '🇧🇧 Barbados (-04:00)' },
	{ value: 'America/Santiago', label: '🇨🇱 Santiago (-04:00/-03:00)' },
	{ value: 'America/St_Johns', label: "🇨🇦 St. John's (-03:30/-02:30)" },
	{ value: 'America/Miquelon', label: '🇵🇲 Miquelon (-03:00/-02:00)' },
	{ value: 'America/Sao_Paulo', label: '🇧🇷 São Paulo (-03:00)' },
	{ value: 'Atlantic/South_Georgia', label: '🇬🇸 South Georgia (-02:00)' },
	{ value: 'America/Godthab', label: '🇬🇱 Godthab/Nuuk (-02:00/-01:00)' },
	{ value: 'Atlantic/Cape_Verde', label: '🇨🇻 Cape Verde (-01:00)' },
	{ value: 'Atlantic/Azores', label: '🇵🇹 Azores (-01:00/+00:00)' },
	{ value: 'UTC', label: '🇺🇳 UTC (+00:00)' },
	{ value: 'Africa/Abidjan', label: '🇨🇮 Abidjan (+00:00)' },
	{ value: 'Europe/Dublin', label: '🇮🇪 Dublin (+00:00/+01:00)' },
	{ value: 'Antarctica/Troll', label: '🇦🇶 Troll (+00:00/+02:00)' },
	{ value: 'Africa/Lagos', label: '🇳🇬 Lagos (+01:00)' },
	{ value: 'Europe/Berlin', label: '🇩🇪 Berlin (+01:00/+02:00)' },
	{ value: 'Africa/Johannesburg', label: '🇿🇦 Johannesburg (+02:00)' },
	{ value: 'Europe/Athens', label: '🇬🇷 Athens (+02:00/+03:00)' },
	{ value: 'Africa/Nairobi', label: '🇰🇪 Nairobi (+03:00)' },
	{ value: 'Asia/Tehran', label: '🇮🇷 Tehran (+03:30)' },
	{ value: 'Asia/Dubai', label: '🇦🇪 Dubai (+04:00)' },
	{ value: 'Asia/Kabul', label: '🇦🇫 Kabul (+04:30)' },
	{ value: 'Asia/Karachi', label: '🇵🇰 Karachi (+05:00)' },
	{ value: 'Asia/Kolkata', label: '🇮🇳 Mumbai (+05:30)' },
	{ value: 'Asia/Kathmandu', label: '🇳🇵 Kathmandu (+05:45)' },
	{ value: 'Asia/Dhaka', label: '🇧🇩 Dhaka (+06:00)' },
	{ value: 'Asia/Yangon', label: '🇲🇲 Yangon (+06:30)' },
	{ value: 'Asia/Bangkok', label: '🇹🇭 Bangkok (+07:00)' },
	{ value: 'Asia/Singapore', label: '🇸🇬 Singapore (+08:00)' },
	{ value: 'Australia/Eucla', label: '🇦🇺 Eucla (+08:45)' },
	{ value: 'Asia/Seoul', label: '🇰🇷 Seoul (+09:00)' },
	{ value: 'Australia/Darwin', label: '🇦🇺 Darwin (+09:30)' },
	{ value: 'Australia/Adelaide', label: '🇦🇺 Adelaide (+09:30/+10:30)' },
	{ value: 'Australia/Brisbane', label: '🇦🇺 Brisbane (+10:00)' },
	{ value: 'Australia/Sydney', label: '🇦🇺 Sydney (+10:00/+11:00)' },
	{ value: 'Australia/Lord_Howe', label: '🇦🇺 Lord Howe (+10:30/+11:00)' },
	{ value: 'Pacific/Bougainville', label: '🇵🇬 Bougainville (+11:00)' },
	{ value: 'Pacific/Norfolk', label: '🇦🇺 Norfolk (+11:00/+12:00)' },
	{ value: 'Pacific/Fiji', label: '🇫🇯 Fiji (+12:00)' },
	{ value: 'Pacific/Auckland', label: '🇳🇿 Auckland (+12:00/+13:00)' },
	{ value: 'Pacific/Chatham', label: '🇳🇿 Chatham (+12:45/+13:45)' },
	{ value: 'Pacific/Apia', label: '🇼🇸 Apia (+13:00)' },
	{ value: 'Pacific/Kiritimati', label: '🇰🇮 Kiritimati (+14:00)' }
];
