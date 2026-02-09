const { regionColors, cellAtPixel } = require('@monitor/assets/regions_dashboard.js');

describe('regionColors', () => {
	it('is an array of 20 hex colour strings', () => {
		expect(Array.isArray(regionColors)).toBe(true);
		expect(regionColors).toHaveLength(20);
	});

	it('every entry is a valid hex colour', () => {
		const hexPattern = /^#[0-9a-fA-F]{6}$/;
		regionColors.forEach((color: string) => {
			expect(color).toMatch(hexPattern);
		});
	});
});

describe('cellAtPixel', () => {
	// The grid has 40 rings and 1800 azimuth bins
	const W = 1200;
	const H = 600;

	it('returns ring=0, azBin=0 for top-left corner', () => {
		const cell = cellAtPixel(0, 0, W, H);
		expect(cell).not.toBeNull();
		expect(cell.ring).toBe(0);
		expect(cell.azBin).toBe(0);
		expect(cell.cellIdx).toBe(0);
	});

	it('returns last ring and azBin for near-bottom-right corner', () => {
		// Just inside the last cell
		const cell = cellAtPixel(W - 0.01, H - 0.01, W, H);
		expect(cell).not.toBeNull();
		expect(cell.ring).toBe(39); // 40 rings, 0-indexed
		expect(cell.azBin).toBe(1799); // 1800 bins, 0-indexed
	});

	it('computes correct cellIdx from ring and azBin', () => {
		const cell = cellAtPixel(W / 2, H / 2, W, H);
		expect(cell).not.toBeNull();
		expect(cell.cellIdx).toBe(cell.ring * 1800 + cell.azBin);
	});

	it('returns null for negative coordinates', () => {
		expect(cellAtPixel(-1, 0, W, H)).toBeNull();
		expect(cellAtPixel(0, -1, W, H)).toBeNull();
	});

	it('returns null for coordinates beyond canvas', () => {
		expect(cellAtPixel(W + 1, 0, W, H)).toBeNull();
		expect(cellAtPixel(0, H + 1, W, H)).toBeNull();
	});

	it('maps centre of canvas to expected ring and azBin', () => {
		const cell = cellAtPixel(W / 2, H / 2, W, H);
		expect(cell).not.toBeNull();
		expect(cell.azBin).toBe(900); // middle of 1800 bins
		expect(cell.ring).toBe(20); // middle of 40 rings
	});
});
