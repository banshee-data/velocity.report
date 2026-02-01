import { TRACK_COLORS } from './lidar';

describe('lidar types', () => {
	describe('TRACK_COLORS', () => {
		it('should have correct colour for pedestrian', () => {
			expect(TRACK_COLORS.pedestrian).toBe('#4CAF50');
		});

		it('should have correct colour for car', () => {
			expect(TRACK_COLORS.car).toBe('#2196F3');
		});

		it('should have correct colour for bird', () => {
			expect(TRACK_COLORS.bird).toBe('#FFC107');
		});

		it('should have correct colour for other', () => {
			expect(TRACK_COLORS.other).toBe('#9E9E9E');
		});

		it('should have correct colour for tentative', () => {
			expect(TRACK_COLORS.tentative).toBe('#FF9800');
		});

		it('should have correct colour for deleted', () => {
			expect(TRACK_COLORS.deleted).toBe('#F44336');
		});

		it('should have all 6 track colours defined', () => {
			const keys = Object.keys(TRACK_COLORS);
			expect(keys).toHaveLength(6);
			expect(keys).toContain('pedestrian');
			expect(keys).toContain('car');
			expect(keys).toContain('bird');
			expect(keys).toContain('other');
			expect(keys).toContain('tentative');
			expect(keys).toContain('deleted');
		});
	});
});
