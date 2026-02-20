import { LEGEND_ORDER, PERCENTILE_COLOURS } from './palette';

describe('palette', () => {
	describe('PERCENTILE_COLOURS', () => {
		it('should match canonical DESIGN.md §3.3 hex values', () => {
			expect(PERCENTILE_COLOURS.p50).toBe('#fbd92f');
			expect(PERCENTILE_COLOURS.p85).toBe('#f7b32b');
			expect(PERCENTILE_COLOURS.p98).toBe('#f25f5c');
			expect(PERCENTILE_COLOURS.max).toBe('#2d1e2f');
			expect(PERCENTILE_COLOURS.count_bar).toBe('#2d1e2f');
			expect(PERCENTILE_COLOURS.low_sample).toBe('#f7b32b');
		});

		it('should have exactly six entries', () => {
			expect(Object.keys(PERCENTILE_COLOURS)).toHaveLength(6);
		});
	});

	describe('LEGEND_ORDER', () => {
		it('should list p50, p85, p98, max in that order', () => {
			expect(LEGEND_ORDER).toEqual(['p50', 'p85', 'p98', 'max']);
		});
	});
});
