// src/lib/units.ts
// Utility functions for managing units in localStorage

export type Unit = 'mps' | 'mph' | 'kmph' | 'kph';

const UNITS_STORAGE_KEY = 'velocity-report-units';

export function getStoredUnits(): Unit | null {
	if (typeof window === 'undefined') return null;
	return localStorage.getItem(UNITS_STORAGE_KEY) as Unit | null;
}

export function setStoredUnits(units: Unit): void {
	if (typeof window === 'undefined') return;
	localStorage.setItem(UNITS_STORAGE_KEY, units);
}

export function getDisplayUnits(defaultUnits: string): Unit {
	const stored = getStoredUnits();
	return stored || (defaultUnits as Unit);
}

export function getUnitLabel(unit: Unit): string {
	switch (unit) {
		case 'mps':
			return 'm/s';
		case 'mph':
			return 'mph';
		case 'kmph':
		case 'kph':
			return 'km/h';
		default:
			return unit;
	}
}

export const AVAILABLE_UNITS: { value: Unit; label: string }[] = [
	{ value: 'mph', label: 'Miles per hour (mph)' },
	{ value: 'kmph', label: 'Kilometers per hour (km/h)' },
	{ value: 'mps', label: 'Meters per second (m/s)' }
];
