// src/lib/stores/timezone.ts
import { writable } from 'svelte/store';
import { getDisplayTimezone, setStoredTimezone, type Timezone } from '../timezone';

// Create a writable store for the current display timezone
export const displayTimezone = writable<Timezone>('UTC');

// Function to initialize the store with config data
export function initializeTimezone(serverDefault: string) {
  const timezone = getDisplayTimezone(serverDefault);
  displayTimezone.set(timezone);
  return timezone;
}

// Function to update timezone and save to localStorage
export function updateTimezone(newTimezone: Timezone) {
  setStoredTimezone(newTimezone);
  displayTimezone.set(newTimezone);
}
