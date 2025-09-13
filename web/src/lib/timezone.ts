// src/lib/timezone.ts
// Utility functions for managing timezones in localStorage

export type Timezone =
  | 'UTC'
  | 'US/Eastern'
  | 'US/Central'
  | 'US/Mountain'
  | 'US/Pacific'
  | 'US/Alaska'
  | 'US/Hawaii'
  | 'America/New_York'
  | 'America/Chicago'
  | 'America/Denver'
  | 'America/Los_Angeles'
  | 'America/Anchorage'
  | 'America/Honolulu'
  | 'Europe/London'
  | 'Europe/Paris'
  | 'Europe/Berlin'
  | 'Europe/Rome'
  | 'Europe/Madrid'
  | 'Europe/Amsterdam'
  | 'Asia/Tokyo'
  | 'Asia/Shanghai'
  | 'Asia/Kolkata'
  | 'Asia/Dubai'
  | 'Australia/Sydney'
  | 'Australia/Melbourne'
  | 'Australia/Perth'
  | 'Pacific/Auckland'
  | 'America/Toronto'
  | 'America/Vancouver'
  | 'America/Mexico_City'
  | 'America/Sao_Paulo'
  | 'Africa/Cairo'
  | 'Africa/Johannesburg';

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
  return stored || (defaultTimezone as Timezone);
}

export function getTimezoneLabel(timezone: Timezone): string {
  switch (timezone) {
    case 'UTC':
      return 'UTC';
    case 'US/Eastern':
      return 'US Eastern (EST/EDT)';
    case 'US/Central':
      return 'US Central (CST/CDT)';
    case 'US/Mountain':
      return 'US Mountain (MST/MDT)';
    case 'US/Pacific':
      return 'US Pacific (PST/PDT)';
    case 'US/Alaska':
      return 'US Alaska (AKST/AKDT)';
    case 'US/Hawaii':
      return 'US Hawaii (HST)';
    case 'America/New_York':
      return 'New York (EST/EDT)';
    case 'America/Chicago':
      return 'Chicago (CST/CDT)';
    case 'America/Denver':
      return 'Denver (MST/MDT)';
    case 'America/Los_Angeles':
      return 'Los Angeles (PST/PDT)';
    case 'America/Anchorage':
      return 'Anchorage (AKST/AKDT)';
    case 'America/Honolulu':
      return 'Honolulu (HST)';
    case 'Europe/London':
      return 'London (GMT/BST)';
    case 'Europe/Paris':
      return 'Paris (CET/CEST)';
    case 'Europe/Berlin':
      return 'Berlin (CET/CEST)';
    case 'Europe/Rome':
      return 'Rome (CET/CEST)';
    case 'Europe/Madrid':
      return 'Madrid (CET/CEST)';
    case 'Europe/Amsterdam':
      return 'Amsterdam (CET/CEST)';
    case 'Asia/Tokyo':
      return 'Tokyo (JST)';
    case 'Asia/Shanghai':
      return 'Shanghai (CST)';
    case 'Asia/Kolkata':
      return 'Kolkata (IST)';
    case 'Asia/Dubai':
      return 'Dubai (GST)';
    case 'Australia/Sydney':
      return 'Sydney (AEST/AEDT)';
    case 'Australia/Melbourne':
      return 'Melbourne (AEST/AEDT)';
    case 'Australia/Perth':
      return 'Perth (AWST)';
    case 'Pacific/Auckland':
      return 'Auckland (NZST/NZDT)';
    case 'America/Toronto':
      return 'Toronto (EST/EDT)';
    case 'America/Vancouver':
      return 'Vancouver (PST/PDT)';
    case 'America/Mexico_City':
      return 'Mexico City (CST/CDT)';
    case 'America/Sao_Paulo':
      return 'São Paulo (BRT)';
    case 'Africa/Cairo':
      return 'Cairo (EET)';
    case 'Africa/Johannesburg':
      return 'Johannesburg (SAST)';
    default:
      return timezone;
  }
}

export const AVAILABLE_TIMEZONES: { value: Timezone; label: string }[] = [
  { value: 'UTC', label: 'UTC' },
  { value: 'US/Eastern', label: 'US Eastern (EST/EDT)' },
  { value: 'US/Central', label: 'US Central (CST/CDT)' },
  { value: 'US/Mountain', label: 'US Mountain (MST/MDT)' },
  { value: 'US/Pacific', label: 'US Pacific (PST/PDT)' },
  { value: 'US/Alaska', label: 'US Alaska (AKST/AKDT)' },
  { value: 'US/Hawaii', label: 'US Hawaii (HST)' },
  { value: 'America/New_York', label: 'New York (EST/EDT)' },
  { value: 'America/Chicago', label: 'Chicago (CST/CDT)' },
  { value: 'America/Denver', label: 'Denver (MST/MDT)' },
  { value: 'America/Los_Angeles', label: 'Los Angeles (PST/PDT)' },
  { value: 'America/Anchorage', label: 'Anchorage (AKST/AKDT)' },
  { value: 'America/Honolulu', label: 'Honolulu (HST)' },
  { value: 'Europe/London', label: 'London (GMT/BST)' },
  { value: 'Europe/Paris', label: 'Paris (CET/CEST)' },
  { value: 'Europe/Berlin', label: 'Berlin (CET/CEST)' },
  { value: 'Europe/Rome', label: 'Rome (CET/CEST)' },
  { value: 'Europe/Madrid', label: 'Madrid (CET/CEST)' },
  { value: 'Europe/Amsterdam', label: 'Amsterdam (CET/CEST)' },
  { value: 'Asia/Tokyo', label: 'Tokyo (JST)' },
  { value: 'Asia/Shanghai', label: 'Shanghai (CST)' },
  { value: 'Asia/Kolkata', label: 'Kolkata (IST)' },
  { value: 'Asia/Dubai', label: 'Dubai (GST)' },
  { value: 'Australia/Sydney', label: 'Sydney (AEST/AEDT)' },
  { value: 'Australia/Melbourne', label: 'Melbourne (AEST/AEDT)' },
  { value: 'Australia/Perth', label: 'Perth (AWST)' },
  { value: 'Pacific/Auckland', label: 'Auckland (NZST/NZDT)' },
  { value: 'America/Toronto', label: 'Toronto (EST/EDT)' },
  { value: 'America/Vancouver', label: 'Vancouver (PST/PDT)' },
  { value: 'America/Mexico_City', label: 'Mexico City (CST/CDT)' },
  { value: 'America/Sao_Paulo', label: 'São Paulo (BRT)' },
  { value: 'Africa/Cairo', label: 'Cairo (EET)' },
  { value: 'Africa/Johannesburg', label: 'Johannesburg (SAST)' }
];
