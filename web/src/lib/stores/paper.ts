// src/lib/stores/paper.ts
import { writable } from 'svelte/store';
import { getDisplayPaperSize, setStoredPaperSize, type PaperSize } from '../paper';

export const paperSize = writable<PaperSize>('a4');

export function initializePaperSize(serverDefault?: string) {
	const fallback: PaperSize = serverDefault === 'letter' ? 'letter' : 'a4';
	const size = getDisplayPaperSize(fallback);
	paperSize.set(size);
	return size;
}

export function updatePaperSize(newSize: PaperSize) {
	setStoredPaperSize(newSize);
	paperSize.set(newSize);
}
