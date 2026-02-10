/* eslint-disable @typescript-eslint/no-require-imports -- CommonJS module requires require() */
const { escapeHTML, parseDuration, formatDuration, comboLabel, formatParamValues, PARAM_SCHEMA } =
	require('@monitor/assets/sweep_dashboard.js') as {
		escapeHTML: (str: unknown) => string;
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
/* eslint-enable @typescript-eslint/no-require-imports */

describe('re-exported shared utilities', () => {
	it('re-exports escapeHTML from dashboard_common', () => {
		expect(typeof escapeHTML).toBe('function');
		expect(escapeHTML('<b>')).toBe('&lt;b&gt;');
	});

	it('re-exports parseDuration from dashboard_common', () => {
		expect(typeof parseDuration).toBe('function');
		expect(parseDuration('5s')).toBe(5);
	});

	it('re-exports formatDuration from dashboard_common', () => {
		expect(typeof formatDuration).toBe('function');
		expect(formatDuration(60)).toBe('1m');
	});
});

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
		for (const [, schema] of Object.entries(PARAM_SCHEMA)) {
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
		for (const [, schema] of Object.entries(PARAM_SCHEMA)) {
			if (numericTypes.includes(schema.type)) {
				expect(schema).toHaveProperty('step');
				expect(schema).toHaveProperty('defaultStart');
				expect(schema).toHaveProperty('defaultEnd');
				expect(schema.defaultEnd!).toBeGreaterThanOrEqual(schema.defaultStart!);
			}
		}
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
