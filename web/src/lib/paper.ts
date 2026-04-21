// src/lib/paper.ts
// Paper size preference persisted in localStorage.

export type PaperSize = 'a4' | 'letter';

const PAPER_STORAGE_KEY = 'velocity-report-paper-size';

export function getStoredPaperSize(): PaperSize | null {
	if (typeof window === 'undefined') return null;
	const v = localStorage.getItem(PAPER_STORAGE_KEY);
	return v === 'a4' || v === 'letter' ? v : null;
}

export function setStoredPaperSize(size: PaperSize): void {
	if (typeof window === 'undefined') return;
	localStorage.setItem(PAPER_STORAGE_KEY, size);
}

export function getDisplayPaperSize(defaultSize: PaperSize = 'a4'): PaperSize {
	return getStoredPaperSize() ?? defaultSize;
}

export function getPaperLabel(size: PaperSize): string {
	return size === 'letter' ? 'US Letter (8.5 x 11 in)' : 'A4 (210 x 297 mm)';
}

export const AVAILABLE_PAPER_SIZES: { value: PaperSize; label: string }[] = [
	{ value: 'a4', label: 'A4 (210 x 297 mm)' },
	{ value: 'letter', label: 'US Letter (8.5 x 11 in)' }
];
