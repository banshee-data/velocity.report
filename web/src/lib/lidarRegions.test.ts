import type { BackgroundRegions } from './lidarControl';
import {
	azimuthDegrees,
	cellAt,
	REGION_COLOURS,
	regionAt,
	regionColour,
	regionLegend
} from './lidarRegions';

const data: BackgroundRegions = {
	sensor_id: 's',
	timestamp: '',
	identification_complete: true,
	frames_sampled: 10,
	region_count: 2,
	rings: 2,
	azimuth_bins: 4,
	grid_mapping: [0, 0, 1, -1, 1, 1, 0, 9],
	regions: [
		{
			id: 0,
			cell_count: 3,
			mean_variance: 0.12345,
			params: {
				noise_relative_fraction: 0.02,
				neighbor_confirmation_count: 3,
				settle_update_fraction: 0.1
			}
		},
		{
			id: 1,
			cell_count: 3,
			mean_variance: 0,
			params: undefined as unknown as BackgroundRegions['regions'][0]['params']
		}
	]
};

describe('regionColour', () => {
	it('cycles the palette, including for negative ids', () => {
		expect(regionColour(0)).toBe(REGION_COLOURS[0]);
		expect(regionColour(REGION_COLOURS.length + 1)).toBe(REGION_COLOURS[1]);
		expect(regionColour(-1)).toBe(REGION_COLOURS[REGION_COLOURS.length - 1]);
	});
});

describe('cellAt', () => {
	it('maps a point to its ring-major cell', () => {
		expect(cellAt(0, 0, 400, 100, 2, 4)).toEqual({ ring: 0, azimuthBin: 0, index: 0 });
		expect(cellAt(399, 99, 400, 100, 2, 4)).toEqual({ ring: 1, azimuthBin: 3, index: 7 });
		expect(cellAt(150, 60, 400, 100, 2, 4)).toEqual({ ring: 1, azimuthBin: 1, index: 5 });
	});

	it('returns null outside the raster or for an empty grid', () => {
		expect(cellAt(-1, 0, 400, 100, 2, 4)).toBeNull();
		expect(cellAt(0, 100, 400, 100, 2, 4)).toBeNull();
		expect(cellAt(0, 0, 0, 100, 2, 4)).toBeNull();
		expect(cellAt(0, 0, 400, 100, 0, 4)).toBeNull();
	});
});

describe('regionAt', () => {
	it('finds the region a cell maps to', () => {
		expect(regionAt(data, { ring: 0, azimuthBin: 2, index: 2 })?.id).toBe(1);
		expect(regionAt(data, { ring: 0, azimuthBin: 3, index: 3 })).toBeNull();
		expect(regionAt(data, { ring: 1, azimuthBin: 3, index: 7 })).toBeNull();
		expect(regionAt(data, null)).toBeNull();
		expect(
			regionAt({ ...data, grid_mapping: [] }, { ring: 0, azimuthBin: 0, index: 0 })
		).toBeNull();
	});
});

describe('regionLegend', () => {
	it('describes each region and tolerates missing params', () => {
		expect(regionLegend(data)).toEqual([
			{ id: 0, colour: REGION_COLOURS[0], text: 'Region 0: variance 0.123, noise 0.020, 3 cells' },
			{ id: 1, colour: REGION_COLOURS[1], text: 'Region 1: variance 0.000, noise 0.000, 3 cells' }
		]);
		expect(regionLegend(null)).toEqual([]);
	});
});

describe('azimuthDegrees', () => {
	it('scales a bin to degrees', () => {
		expect(azimuthDegrees(900, 1800)).toBe(180);
		expect(azimuthDegrees(3, 0)).toBe(0);
	});
});
