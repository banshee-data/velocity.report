// src/lib/stores/units.ts
import { writable } from 'svelte/store';
import { getDisplayUnits, setStoredUnits, type Unit } from '../units';

// Create a writable store for the current display units
export const displayUnits = writable<Unit>('mph');

// Function to initialize the store with config data
export function initializeUnits(serverDefault: string) {
	const units = getDisplayUnits(serverDefault);
	displayUnits.set(units);
	return units;
}

// Function to update units and save to localStorage
export function updateUnits(newUnits: Unit) {
	setStoredUnits(newUnits);
	displayUnits.set(newUnits);
}
