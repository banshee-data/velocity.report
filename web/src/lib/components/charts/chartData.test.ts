import {
	buildHeatmap,
	colourByValue,
	formatValue,
	rampColour,
	sortCategories,
	symmetricDomain,
	VIRIDIS
} from './chartData';

describe('rampColour', () => {
	it('maps the domain onto the ramp, clamping outside it', () => {
		expect(rampColour(0, 0, 10)).toBe(VIRIDIS[0]);
		expect(rampColour(5, 0, 10)).toBe(VIRIDIS[5]);
		expect(rampColour(10, 0, 10)).toBe(VIRIDIS[9]);
		expect(rampColour(-3, 0, 10)).toBe(VIRIDIS[0]);
		expect(rampColour(30, 0, 10)).toBe(VIRIDIS[9]);
	});

	it('colours everything as the first stop with no spread or a bad value', () => {
		expect(rampColour(4, 0, 0)).toBe(VIRIDIS[0]);
		expect(rampColour(4, 5, 5)).toBe(VIRIDIS[0]);
		expect(rampColour(Number.NaN, 0, 10)).toBe(VIRIDIS[0]);
	});
});

describe('colourByValue', () => {
	it('scales to the largest value by default', () => {
		const out = colourByValue([
			{ x: 0, y: 0, value: 2 },
			{ x: 1, y: 1, value: 4 },
			{ x: 2, y: 2 }
		]);
		expect(out.map((p) => p.colour)).toEqual([VIRIDIS[5], VIRIDIS[9], VIRIDIS[0]]);
		expect(out[0]).toMatchObject({ x: 0, y: 0, value: 2 });
	});

	it('honours an explicit maximum', () => {
		expect(colourByValue([{ x: 0, y: 0, value: 2 }], 20)[0].colour).toBe(VIRIDIS[1]);
	});
});

describe('symmetricDomain', () => {
	it('pads the largest absolute coordinate across every set', () => {
		const [lo, hi] = symmetricDomain([{ x: 3, y: -1 }], [{ x: 0, y: -10 }]);
		expect(hi).toBeCloseTo(10.5);
		expect(lo).toBeCloseTo(-10.5);
	});

	it('falls back to a unit square when empty', () => {
		expect(symmetricDomain([], [])).toEqual([-1, 1]);
		expect(symmetricDomain([{ x: 0, y: 0 }])).toEqual([-1, 1]);
	});
});

describe('sortCategories', () => {
	it('sorts numbers numerically and de-duplicates', () => {
		expect(sortCategories(['10', '9', '0.5', '9'])).toEqual(['0.5', '9', '10']);
	});

	it('sorts mixed labels lexically', () => {
		expect(sortCategories(['true', 'false', ''])).toEqual(['', 'false', 'true']);
	});
});

describe('buildHeatmap', () => {
	it('scales colour from the smallest positive value to the largest', () => {
		const hm = buildHeatmap([
			{ x: '2', y: 'a', value: 0 },
			{ x: '10', y: 'a', value: 2 },
			{ x: '2', y: 'b', value: 4 }
		]);
		expect(hm.xValues).toEqual(['2', '10']);
		expect(hm.yValues).toEqual(['a', 'b']);
		expect(hm.min).toBe(2);
		expect(hm.max).toBe(4);
		expect(hm.cells.map((c) => c.colour)).toEqual([VIRIDIS[0], VIRIDIS[0], VIRIDIS[9]]);
	});

	it('runs from zero when every value is equal', () => {
		const hm = buildHeatmap([
			{ x: 'a', y: 'a', value: 3 },
			{ x: 'b', y: 'a', value: 3 }
		]);
		expect(hm.min).toBe(0);
		expect(hm.max).toBe(3);
	});

	it('handles no cells', () => {
		expect(buildHeatmap([])).toEqual({ xValues: [], yValues: [], cells: [], min: 0, max: 0 });
	});
});

describe('formatValue', () => {
	it('formats integers, fractions and missing values', () => {
		expect(formatValue(1200)).toBe((1200).toLocaleString());
		expect(formatValue(0.12345)).toBe('0.123');
		expect(formatValue(0.12345, 1)).toBe('0.1');
		expect(formatValue(null)).toBe('—');
		expect(formatValue(undefined)).toBe('—');
		expect(formatValue(Number.POSITIVE_INFINITY)).toBe('—');
	});
});
