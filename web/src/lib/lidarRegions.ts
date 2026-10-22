/**
 * Helpers for the background regions page: the region palette, mapping a
 * pointer position to a grid cell, and the legend rows. The grid is drawn as
 * a raster, one pixel block per cell, ring 0 at the top, azimuth across.
 */
import type { BackgroundRegion, BackgroundRegions } from './lidarControl.js';

/** Region colours, cycled by region index; the regions dashboard's palette. */
export const REGION_COLOURS = [
	'#e74c3c',
	'#3498db',
	'#2ecc71',
	'#f39c12',
	'#9b59b6',
	'#1abc9c',
	'#e67e22',
	'#34495e',
	'#16a085',
	'#c0392b',
	'#8e44ad',
	'#2980b9',
	'#27ae60',
	'#f1c40f',
	'#d35400',
	'#7f8c8d',
	'#95a5a6',
	'#bdc3c7',
	'#ecf0f1',
	'#e8daef'
] as const;

export function regionColour(regionId: number): string {
	return REGION_COLOURS[
		((regionId % REGION_COLOURS.length) + REGION_COLOURS.length) % REGION_COLOURS.length
	];
}

export interface GridCell {
	ring: number;
	azimuthBin: number;
	index: number;
}

/** The cell under a point on a raster of the given size, or null outside it. */
export function cellAt(
	x: number,
	y: number,
	width: number,
	height: number,
	rings: number,
	azimuthBins: number
): GridCell | null {
	if (width <= 0 || height <= 0 || rings <= 0 || azimuthBins <= 0) return null;
	const azimuthBin = Math.floor((x / width) * azimuthBins);
	const ring = Math.floor((y / height) * rings);
	if (azimuthBin < 0 || azimuthBin >= azimuthBins || ring < 0 || ring >= rings) return null;
	return { ring, azimuthBin, index: ring * azimuthBins + azimuthBin };
}

/** The region a cell belongs to, or null for a cell in none. */
export function regionAt(data: BackgroundRegions, cell: GridCell | null): BackgroundRegion | null {
	if (!cell) return null;
	const id = data.grid_mapping?.[cell.index];
	if (id == null || id < 0) return null;
	return data.regions.find((r) => r.id === id) ?? data.regions[id] ?? null;
}

export interface RegionLegendRow {
	id: number;
	colour: string;
	text: string;
}

/** One legend row per region: its variance, noise fraction and size. */
export function regionLegend(data: BackgroundRegions | null): RegionLegendRow[] {
	return (data?.regions ?? []).map((r) => ({
		id: r.id,
		colour: regionColour(r.id),
		text: `Region ${r.id}: variance ${(r.mean_variance || 0).toFixed(3)}, noise ${(
			r.params?.noise_relative_fraction || 0
		).toFixed(3)}, ${(r.cell_count || 0).toLocaleString()} cells`
	}));
}

/** Azimuth in degrees at the start of a bin. */
export function azimuthDegrees(azimuthBin: number, azimuthBins: number): number {
	return azimuthBins > 0 ? (azimuthBin * 360) / azimuthBins : 0;
}
