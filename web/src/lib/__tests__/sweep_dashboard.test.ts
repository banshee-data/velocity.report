// eslint-disable-next-line @typescript-eslint/no-require-imports
const { parseDuration, formatDuration, comboLabel, formatParamValues, PARAM_SCHEMA } =
	require('@monitor/assets/sweep_dashboard.js') as {
		parseDuration: (s: string | null | undefined) => number;
		formatDuration: (secs: number) => string;
		comboLabel: (r: Record<string, unknown>) => string;
		formatParamValues: (p: Record<string, unknown> | null | undefined) => string;
		PARAM_SCHEMA: Record<
			string,
			{
				label: string;
				desc: string;
				type: string;
				step?: number;
				defaultStart?: number;
				defaultEnd?: number;
			}
		>;
	};

describe('PARAM_SCHEMA', () => {
	it('exports a non-empty object of parameter definitions', () => {
		expect(typeof PARAM_SCHEMA).toBe('object');
		expect(Object.keys(PARAM_SCHEMA).length).toBeGreaterThan(0);
	});

	it('includes expected parameters', () => {
		expect(PARAM_SCHEMA).toHaveProperty('noise_relative');
		expect(PARAM_SCHEMA).toHaveProperty('closeness_multiplier');
		expect(PARAM_SCHEMA).toHaveProperty('neighbor_confirmation_count');
		expect(PARAM_SCHEMA).toHaveProperty('hits_to_confirm');
		expect(PARAM_SCHEMA).toHaveProperty('max_misses');
	});

	it('each entry has type, label, and desc', () => {
		for (const [key, schema] of Object.entries(PARAM_SCHEMA)) {
			expect(schema).toHaveProperty('type');
			expect(schema).toHaveProperty('label');
			expect(schema).toHaveProperty('desc');
			expect(typeof schema.label).toBe('string');
			expect(typeof schema.desc).toBe('string');
			expect(['float64', 'int', 'int64', 'bool', 'string']).toContain(schema.type);
		}
	});

	it('numeric types have step and default range', () => {
		const numericTypes = ['float64', 'int', 'int64'];
		for (const [key, schema] of Object.entries(PARAM_SCHEMA)) {
			if (numericTypes.includes(schema.type)) {
				expect(schema).toHaveProperty('step');
				expect(schema).toHaveProperty('defaultStart');
				expect(schema).toHaveProperty('defaultEnd');
				expect(schema.defaultEnd!).toBeGreaterThanOrEqual(schema.defaultStart!);
			}
		}
	});
});

describe('parseDuration', () => {
	it('returns 0 for empty/null input', () => {
		expect(parseDuration('')).toBe(0);
		expect(parseDuration(null)).toBe(0);
		expect(parseDuration(undefined)).toBe(0);
	});

	it('parses seconds', () => {
		expect(parseDuration('5s')).toBe(5);
		expect(parseDuration('30s')).toBe(30);
		expect(parseDuration('0s')).toBe(0);
	});

	it('parses milliseconds', () => {
		expect(parseDuration('500ms')).toBe(0.5);
		expect(parseDuration('1000ms')).toBe(1);
		expect(parseDuration('100ms')).toBe(0.1);
	});

	it('parses minutes', () => {
		expect(parseDuration('2m')).toBe(120);
		expect(parseDuration('1m')).toBe(60);
	});

	it('parses hours', () => {
		expect(parseDuration('1h')).toBe(3600);
		expect(parseDuration('2h')).toBe(7200);
	});

	it('parses compound durations', () => {
		expect(parseDuration('1m30s')).toBe(90);
		expect(parseDuration('1h30m')).toBe(5400);
		expect(parseDuration('2h15m30s')).toBe(8130);
	});

	it('handles fractional values', () => {
		expect(parseDuration('1.5s')).toBe(1.5);
		expect(parseDuration('2.5m')).toBe(150);
	});
});

describe('formatDuration', () => {
	it('formats seconds (< 60)', () => {
		expect(formatDuration(5)).toBe('5s');
		expect(formatDuration(30)).toBe('30s');
		expect(formatDuration(0)).toBe('0s');
	});

	it('formats minutes (< 3600)', () => {
		expect(formatDuration(60)).toBe('1m');
		expect(formatDuration(90)).toBe('1m 30s');
		expect(formatDuration(120)).toBe('2m');
	});

	it('formats hours (>= 3600)', () => {
		expect(formatDuration(3600)).toBe('1h');
		expect(formatDuration(5400)).toBe('1h 30m');
		expect(formatDuration(7200)).toBe('2h');
	});

	it('rounds fractional seconds', () => {
		expect(formatDuration(5.4)).toBe('5s');
		expect(formatDuration(5.6)).toBe('6s');
	});
});

describe('comboLabel', () => {
	it('formats param_values with short keys', () => {
		const result = {
			param_values: {
				noise_relative: 0.05,
				closeness_multiplier: 5
			}
		};
		const label = comboLabel(result);
		expect(label).toContain('relative=0.050');
		expect(label).toContain('multiplier=5');
	});

	it('formats integer values without decimals', () => {
		const result = {
			param_values: {
				neighbor_confirmation_count: 3
			}
		};
		const label = comboLabel(result);
		expect(label).toContain('count=3');
	});

	it('falls back to legacy format without param_values', () => {
		const result = {
			noise: 0.05,
			closeness: 5.0,
			neighbour: 3
		};
		const label = comboLabel(result);
		expect(label).toContain('n=0.050');
		expect(label).toContain('c=5.0');
		expect(label).toContain('nb=3');
	});
});

describe('formatParamValues', () => {
	it('returns empty string for null/undefined', () => {
		expect(formatParamValues(null)).toBe('');
		expect(formatParamValues(undefined)).toBe('');
	});

	it('formats parameter values with labels from PARAM_SCHEMA', () => {
		const params = {
			noise_relative: 0.05,
			closeness_multiplier: 5
		};
		const formatted = formatParamValues(params);
		expect(formatted).toContain('Noise Relative=0.0500');
		expect(formatted).toContain('Closeness Multiplier=5');
	});

	it('excludes metric keys (score, acceptance_rate, etc.)', () => {
		const params = {
			noise_relative: 0.05,
			score: 0.95,
			acceptance_rate: 0.9,
			misalignment_ratio: 0.01,
			alignment_deg: 1.5,
			nonzero_cells: 100
		};
		const formatted = formatParamValues(params);
		expect(formatted).toContain('Noise Relative');
		expect(formatted).not.toContain('score');
		expect(formatted).not.toContain('acceptance_rate');
		expect(formatted).not.toContain('misalignment_ratio');
		expect(formatted).not.toContain('alignment_deg');
		expect(formatted).not.toContain('nonzero_cells');
	});

	it('uses raw key as label for unknown parameters', () => {
		const params = {
			unknown_param: 42
		};
		const formatted = formatParamValues(params);
		expect(formatted).toContain('unknown_param=42');
	});
});
