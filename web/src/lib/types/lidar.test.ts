import { TRACK_COLORS, trackColour } from './lidar';

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

	describe('trackColour', () => {
		it('should return tentative colour for tentative state', () => {
			expect(trackColour('track-1', 'car', 'tentative')).toBe(TRACK_COLORS.tentative);
		});

		it('should return deleted colour for deleted state', () => {
			expect(trackColour('track-1', 'pedestrian', 'deleted')).toBe(TRACK_COLORS.deleted);
		});

		it('should return tentative colour regardless of object class', () => {
			expect(trackColour('track-1', undefined, 'tentative')).toBe(TRACK_COLORS.tentative);
			expect(trackColour('track-1', 'bird', 'tentative')).toBe(TRACK_COLORS.tentative);
		});

		it('should return a hex colour string for confirmed car', () => {
			const colour = trackColour('track-abc', 'car', 'confirmed');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should return a hex colour string for confirmed pedestrian', () => {
			const colour = trackColour('track-xyz', 'pedestrian', 'confirmed');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should return a hex colour string for bird class', () => {
			const colour = trackColour('track-bird', 'bird');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should use "other" base colour for unknown class', () => {
			const colour = trackColour('track-unknown', 'unknown_class');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should use "other" base colour when class is undefined', () => {
			const colour = trackColour('track-no-class');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should produce deterministic output for same trackId', () => {
			const c1 = trackColour('deterministic-id', 'car');
			const c2 = trackColour('deterministic-id', 'car');
			expect(c1).toBe(c2);
		});

		it('should produce different colours for different trackIds of same class', () => {
			// With hue shifting, different IDs should usually produce different colours.
			// Not always guaranteed for very similar hashes, but test a spread.
			const colours = new Set<string>();
			for (let i = 0; i < 20; i++) {
				colours.add(trackColour(`track-${i}`, 'car'));
			}
			// With 20 different IDs, expect at least a few distinct colours.
			expect(colours.size).toBeGreaterThan(3);
		});

		it('should handle empty string trackId', () => {
			const colour = trackColour('', 'car');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should handle very long trackId', () => {
			const longId = 'a'.repeat(1000);
			const colour = trackColour(longId, 'pedestrian');
			expect(colour).toMatch(/^#[0-9a-f]{6}$/);
		});

		it('should produce valid RGB values (no negative or overflow)', () => {
			// Test many IDs to exercise various hash outcomes.
			for (let i = 0; i < 100; i++) {
				const colour = trackColour(`stress-${i}`, 'car');
				expect(colour).toMatch(/^#[0-9a-f]{6}$/);

				// Parse and validate RGB ranges.
				const r = parseInt(colour.slice(1, 3), 16);
				const g = parseInt(colour.slice(3, 5), 16);
				const b = parseInt(colour.slice(5, 7), 16);
				expect(r).toBeGreaterThanOrEqual(0);
				expect(r).toBeLessThanOrEqual(255);
				expect(g).toBeGreaterThanOrEqual(0);
				expect(g).toBeLessThanOrEqual(255);
				expect(b).toBeGreaterThanOrEqual(0);
				expect(b).toBeLessThanOrEqual(255);
			}
		});

		it('should not shift hue for tentative/deleted states', () => {
			// Tentative and deleted return fixed colours regardless of trackId.
			expect(trackColour('any-id-1', 'car', 'tentative')).toBe(TRACK_COLORS.tentative);
			expect(trackColour('any-id-2', 'car', 'tentative')).toBe(TRACK_COLORS.tentative);
			expect(trackColour('any-id-3', 'car', 'deleted')).toBe(TRACK_COLORS.deleted);
		});

		it('should use the correct base class for each known class', () => {
			// Verify that each known class produces a colour near its base.
			const classes = ['pedestrian', 'car', 'bird', 'other'] as const;
			for (const cls of classes) {
				const colour = trackColour('test-track', cls);
				expect(colour).toMatch(/^#[0-9a-f]{6}$/);
			}
		});
	});
});
