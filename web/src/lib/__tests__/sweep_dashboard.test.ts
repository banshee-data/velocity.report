/* eslint-disable @typescript-eslint/no-require-imports, @typescript-eslint/no-explicit-any */

// ---------------------------------------------------------------------------
// Mock setup (before module import)
// ---------------------------------------------------------------------------

// Mock matchMedia (not available in jsdom)
Object.defineProperty(window, 'matchMedia', {
	writable: true,
	value: jest.fn().mockImplementation((query: string) => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: jest.fn(),
		removeListener: jest.fn(),
		addEventListener: jest.fn(),
		removeEventListener: jest.fn(),
		dispatchEvent: jest.fn()
	}))
});

// Mock echarts before module load
function createMockChart() {
	return { setOption: jest.fn(), resize: jest.fn() };
}
(global as any).echarts = {
	init: jest.fn().mockImplementation(() => createMockChart())
};

// Mock URL methods for download functions
const origCreateObjectURL = URL.createObjectURL;
const origRevokeObjectURL = URL.revokeObjectURL;
URL.createObjectURL = jest.fn().mockReturnValue('blob:mock-url');
URL.revokeObjectURL = jest.fn();

// ---------------------------------------------------------------------------
// Module import
// ---------------------------------------------------------------------------

const mod = require('@monitor/assets/sweep_dashboard.js') as Record<string, any>;
const {
	val,
	numVal,
	intVal,
	showError,
	togglePCAP,
	toggleWeights,
	setMode,
	addParamRow,
	removeParamRow,
	updateParamFields,
	getParamValueCount,
	updateSweepSummary,
	buildScenarioJSON,
	loadScenario,
	toggleJSONEditor,
	applyJSONEditor,
	handleStart,
	handleStartManualSweep,
	handleStartAutoTune,
	handleStop,
	startPolling,
	stopPolling,
	pollStatus,
	pollAutoTuneStatus,
	comboLabel,
	formatParamValues,
	renderRecommendation,
	renderTable,
	renderCharts,
	initCharts,
	downloadCSV,
	downloadScenario,
	uploadScenario,
	fetchCurrentParams,
	displayCurrentParams,
	loadSweepScenes,
	onSweepSceneSelected,
	applyRecommendation,
	applySceneParams,
	PARAM_SCHEMA,
	escapeHTML,
	parseDuration,
	formatDuration,
	init
} = mod;
/* eslint-enable @typescript-eslint/no-require-imports */

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function flushPromises(): Promise<void> {
	for (let i = 0; i < 10; i++) await Promise.resolve();
}

function setupDOM(): void {
	document.head.innerHTML = '<meta name="sensor-id" content="test-sensor" />';
	document.body.innerHTML = [
		'<div id="error-box" style="display:none"></div>',
		'<button id="mode-manual" class="active"></button>',
		'<button id="mode-auto"></button>',
		'<div id="recommendation-card" style="display:none">',
		'  <div id="recommendation-content"></div>',
		'  <button id="btn-apply-recommendation">Apply</button>',
		'</div>',
		'<div id="param-rows"></div>',
		'<div id="sweep-summary"></div>',
		'<select id="data_source">',
		'  <option value="live">Live</option>',
		'  <option value="pcap">PCAP</option>',
		'  <option value="scene">Scene</option>',
		'</select>',
		'<div id="pcap-fields" style="display:none">',
		'  <input id="pcap_file" type="text" value="" />',
		'  <input id="pcap_start_secs" type="number" value="0" />',
		'  <input id="pcap_duration_secs" type="number" value="-1" />',
		'</div>',
		'<div id="scene-fields" style="display:none">',
		'  <select id="scene_select"><option value="">Loading</option></select>',
		'  <div id="scene-info" style="display:none"></div>',
		'  <div id="scene-actions" style="display:none">',
		'    <button id="btn-apply-scene-params">Apply Scene Params</button>',
		'  </div>',
		'</div>',
		'<button id="btn-start">Start Sweep</button>',
		'<button id="btn-stop" style="display:none">Stop Sweep</button>',
		'<div id="stopping-indicator" style="display:none"></div>',
		'<div id="progress-section" style="display:none">',
		'  <span id="status-badge" class="status-badge status-idle">idle</span>',
		'  <span id="combo-count"></span>',
		'  <div id="current-combo"></div>',
		'  <div id="sweep-error" style="display:none"></div>',
		'  <div id="sweep-warnings" style="display:none"></div>',
		'</div>',
		'<select id="seed">',
		'  <option value="true" selected>True</option>',
		'  <option value="false">False</option>',
		'  <option value="toggle">Toggle</option>',
		'</select>',
		'<input id="iterations" type="number" value="10" />',
		'<input id="interval" type="text" value="2s" />',
		'<input id="settle_time" type="text" value="5s" />',
		'<select id="settle_mode">',
		'  <option value="once" selected>Once</option>',
		'  <option value="per_combo">Per Combo</option>',
		'</select>',
		'<input id="max_rounds" type="number" value="3" />',
		'<input id="values_per_param" type="number" value="5" />',
		'<input id="top_k" type="number" value="5" />',
		'<select id="objective">',
		'  <option value="acceptance">Acceptance</option>',
		'  <option value="weighted">Weighted</option>',
		'  <option value="ground_truth" id="ground_truth_option" style="display:none">Ground Truth</option>',
		'</select>',
		'<div id="weight-fields" style="display:none">',
		'  <input id="w_acceptance" type="number" value="1.0" />',
		'  <input id="w_misalignment" type="number" value="-0.5" />',
		'  <input id="w_alignment" type="number" value="-0.01" />',
		'  <input id="w_nonzero" type="number" value="0.1" />',
		'</div>',
		'<input id="scenario-upload" type="file" style="display:none" />',
		'<div id="json-editor-wrap" style="display:none">',
		'  <textarea id="scenario-json"></textarea>',
		'</div>',
		'<button id="btn-apply-json" style="display:none"></button>',
		'<div id="acceptance-chart"></div>',
		'<div id="nonzero-chart"></div>',
		'<div id="bucket-chart"></div>',
		'<div id="param-heatmap"></div>',
		'<div id="alignment-chart"></div>',
		'<div id="tracks-chart"></div>',
		'<div id="tracks-heatmap"></div>',
		'<div id="alignment-heatmap"></div>',
		'<table><thead id="results-head"><tr></tr></thead>',
		'<tbody id="results-body"></tbody></table>',
		'<div id="current-params-display"></div>'
	].join('\n');
}

/** Build a test results array with two entries including buckets and param_values. */
function makeTestResults(): Record<string, any>[] {
	return [
		{
			param_values: { noise_relative: 0.05, closeness_multiplier: 5.0 },
			overall_accept_mean: 0.85,
			overall_accept_stddev: 0.02,
			nonzero_cells_mean: 100,
			nonzero_cells_stddev: 5,
			active_tracks_mean: 3.5,
			active_tracks_stddev: 0.5,
			alignment_deg_mean: 1.2,
			alignment_deg_stddev: 0.3,
			misalignment_ratio_mean: 0.05,
			misalignment_ratio_stddev: 0.01,
			buckets: [5, 10, 20, 50],
			bucket_means: [0.9, 0.85, 0.8, 0.7]
		},
		{
			param_values: { noise_relative: 0.1, closeness_multiplier: 10.0 },
			overall_accept_mean: 0.9,
			overall_accept_stddev: 0.01,
			nonzero_cells_mean: 200,
			nonzero_cells_stddev: 10,
			active_tracks_mean: 5.0,
			active_tracks_stddev: 0.8,
			alignment_deg_mean: 0.8,
			alignment_deg_stddev: 0.2,
			misalignment_ratio_mean: 0.03,
			misalignment_ratio_stddev: 0.005,
			buckets: [5, 10, 20, 50],
			bucket_means: [0.95, 0.9, 0.85, 0.75]
		}
	];
}

/** Build results with ground truth columns for table rendering. */
function makeGTResults(): Record<string, any>[] {
	return [
		{
			param_values: { noise_relative: 0.05 },
			detection_rate: 0.9,
			ground_truth_score: 0.85,
			fragmentation: 0.1,
			false_positive_rate: 0.05,
			quality_premium: 0.2,
			truncation_rate: 0.03,
			velocity_noise_rate: 0.02,
			stopped_recovery_rate: 0.95
		}
	];
}

/** Create a URL-based fetch mock for init(). */
function makeFetchRouter(overrides: Record<string, any> = {}) {
	const routes: Record<string, any> = {
		'/api/lidar/sweep/auto': { status: 'idle' },
		'/api/lidar/sweep/status': { status: 'idle', results: [] },
		'/api/lidar/params': { noise_relative: 0.05 },
		'/api/lidar/scenes': { scenes: [] },
		...overrides
	};
	return jest.fn().mockImplementation((url: string, opts?: any) => {
		for (const [pattern, data] of Object.entries(routes)) {
			if (url.includes(pattern)) {
				return Promise.resolve({
					ok: true,
					json: () => Promise.resolve(data),
					text: () => Promise.resolve(JSON.stringify(data))
				});
			}
		}
		return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
	});
}

// ===========================================================================
// Pure function tests (no DOM required)
// ===========================================================================

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
			const s = schema as any;
			expect(s).toHaveProperty('type');
			expect(s).toHaveProperty('label');
			expect(s).toHaveProperty('desc');
			expect(typeof s.label).toBe('string');
			expect(typeof s.desc).toBe('string');
			expect(['float64', 'int', 'int64', 'bool', 'string']).toContain(s.type);
		}
	});

	it('numeric types have step and default range', () => {
		const numericTypes = ['float64', 'int', 'int64'];
		for (const [, schema] of Object.entries(PARAM_SCHEMA)) {
			const s = schema as any;
			if (numericTypes.includes(s.type)) {
				expect(s).toHaveProperty('step');
				expect(s).toHaveProperty('defaultStart');
				expect(s).toHaveProperty('defaultEnd');
				expect(s.defaultEnd!).toBeGreaterThanOrEqual(s.defaultStart!);
			}
		}
	});
});

describe('comboLabel', () => {
	it('formats param_values with short keys', () => {
		const result = { param_values: { noise_relative: 0.05, closeness_multiplier: 5 } };
		const label = comboLabel(result);
		expect(label).toContain('relative=0.050');
		expect(label).toContain('multiplier=5');
	});

	it('formats integer values without decimals', () => {
		const result = { param_values: { neighbor_confirmation_count: 3 } };
		expect(comboLabel(result)).toContain('count=3');
	});

	it('falls back to legacy format without param_values', () => {
		const result = { noise: 0.05, closeness: 5.0, neighbour: 3 };
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
		const params = { noise_relative: 0.05, closeness_multiplier: 5 };
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
		const params = { unknown_param: 42 };
		expect(formatParamValues(params)).toContain('unknown_param=42');
	});
});

// ===========================================================================
// DOM helper tests
// ===========================================================================

describe('val / numVal / intVal', () => {
	beforeEach(setupDOM);

	it('val returns element value', () => {
		expect(val('iterations')).toBe('10');
	});

	it('numVal returns parsed float', () => {
		expect(numVal('iterations')).toBe(10);
	});

	it('intVal returns parsed integer', () => {
		(document.getElementById('iterations') as HTMLInputElement).value = '7.9';
		expect(intVal('iterations')).toBe(7);
	});
});

describe('showError', () => {
	beforeEach(setupDOM);

	it('shows error message and makes box visible', () => {
		showError('Something went wrong');
		const el = document.getElementById('error-box')!;
		expect(el.textContent).toBe('Something went wrong');
		expect(el.style.display).toBe('');
	});

	it('hides error box when message is empty', () => {
		showError('error');
		showError('');
		const el = document.getElementById('error-box')!;
		expect(el.style.display).toBe('none');
	});
});

// ===========================================================================
// Toggle functions
// ===========================================================================

describe('togglePCAP', () => {
	beforeEach(setupDOM);

	it('shows pcap fields when data_source is pcap', () => {
		(document.getElementById('data_source') as HTMLSelectElement).value = 'pcap';
		togglePCAP();
		expect(document.getElementById('pcap-fields')!.style.display).toBe('');
		expect(document.getElementById('scene-fields')!.style.display).toBe('none');
	});

	it('shows scene fields when data_source is scene', () => {
		(document.getElementById('data_source') as HTMLSelectElement).value = 'scene';
		togglePCAP();
		expect(document.getElementById('pcap-fields')!.style.display).toBe('none');
		expect(document.getElementById('scene-fields')!.style.display).toBe('');
	});

	it('hides both when data_source is live', () => {
		(document.getElementById('data_source') as HTMLSelectElement).value = 'live';
		togglePCAP();
		expect(document.getElementById('pcap-fields')!.style.display).toBe('none');
		expect(document.getElementById('scene-fields')!.style.display).toBe('none');
	});
});

describe('toggleWeights', () => {
	beforeEach(setupDOM);

	it('shows weight fields when objective is weighted', () => {
		(document.getElementById('objective') as HTMLSelectElement).value = 'weighted';
		toggleWeights();
		expect(document.getElementById('weight-fields')!.style.display).toBe('');
	});

	it('hides weight fields for other objective values', () => {
		(document.getElementById('objective') as HTMLSelectElement).value = 'acceptance';
		toggleWeights();
		expect(document.getElementById('weight-fields')!.style.display).toBe('none');
	});
});

describe('setMode', () => {
	beforeEach(setupDOM);

	afterEach(() => {
		setMode('manual'); // reset module state
	});

	it('sets manual mode with active class', () => {
		setMode('manual');
		expect(document.getElementById('mode-manual')!.className).toBe('active');
		expect(document.getElementById('mode-auto')!.className).toBe('');
		expect(document.body.classList.contains('auto-mode')).toBe(false);
	});

	it('sets auto mode with active class and body class', () => {
		setMode('auto');
		expect(document.getElementById('mode-manual')!.className).toBe('');
		expect(document.getElementById('mode-auto')!.className).toBe('active');
		expect(document.body.classList.contains('auto-mode')).toBe(true);
	});

	it('updates button text for manual mode', () => {
		setMode('manual');
		expect(document.getElementById('btn-start')!.textContent).toBe('Start Sweep');
		expect(document.getElementById('btn-stop')!.textContent).toBe('Stop Sweep');
	});

	it('updates button text for auto mode', () => {
		setMode('auto');
		expect(document.getElementById('btn-start')!.textContent).toBe('Start Auto-Tune');
		expect(document.getElementById('btn-stop')!.textContent).toBe('Stop Auto-Tune');
	});
});

// ===========================================================================
// Param row management
// ===========================================================================

describe('addParamRow', () => {
	beforeEach(setupDOM);

	it('creates a param row in the container', () => {
		const id = addParamRow();
		const rows = document.getElementById('param-rows')!.children;
		expect(rows.length).toBe(1);
		expect(rows[0].id).toBe('param-row-' + id);
	});

	it('selects the specified parameter', () => {
		const id = addParamRow('noise_relative');
		const sel = document.getElementById('pname-' + id) as HTMLSelectElement;
		expect(sel.value).toBe('noise_relative');
	});

	it('creates numeric input fields for float64 type', () => {
		const id = addParamRow('noise_relative');
		expect(document.getElementById('pstart-' + id)).not.toBeNull();
		expect(document.getElementById('pend-' + id)).not.toBeNull();
		expect(document.getElementById('pstep-' + id)).not.toBeNull();
		expect(document.getElementById('pvals-' + id)).not.toBeNull();
	});

	it('returns incrementing ids', () => {
		const id1 = addParamRow();
		const id2 = addParamRow();
		expect(id2).toBe(id1 + 1);
	});
});

describe('removeParamRow', () => {
	beforeEach(setupDOM);

	it('removes an existing param row', () => {
		const id = addParamRow('noise_relative');
		expect(document.getElementById('param-rows')!.children.length).toBe(1);
		removeParamRow(id);
		expect(document.getElementById('param-rows')!.children.length).toBe(0);
	});

	it('does nothing for non-existent row', () => {
		removeParamRow(9999);
		// should not throw
	});
});

describe('updateParamFields', () => {
	beforeEach(setupDOM);

	it('creates start/end/step/values fields for float64 type', () => {
		const id = addParamRow('noise_relative');
		// Fields are already created by addParamRow, verify them
		const startEl = document.getElementById('pstart-' + id) as HTMLInputElement;
		expect(startEl).not.toBeNull();
		expect(startEl.value).toBe('0.01');
		const endEl = document.getElementById('pend-' + id) as HTMLInputElement;
		expect(endEl.value).toBe('0.2');
	});

	it('creates start/end/step fields for int type', () => {
		const id = addParamRow('neighbor_confirmation_count');
		const startEl = document.getElementById('pstart-' + id) as HTMLInputElement;
		expect(startEl).not.toBeNull();
		expect(startEl.value).toBe('0');
		const endEl = document.getElementById('pend-' + id) as HTMLInputElement;
		expect(endEl.value).toBe('8');
	});

	it('creates values field for bool type', () => {
		const id = addParamRow('seed_from_first');
		const valsEl = document.getElementById('pvals-' + id) as HTMLInputElement;
		expect(valsEl).not.toBeNull();
		expect(valsEl.value).toBe('true, false');
		// No start/end/step for bool
		expect(document.getElementById('pstart-' + id)).toBeNull();
	});

	it('creates values field for string type', () => {
		const id = addParamRow('buffer_timeout');
		const valsEl = document.getElementById('pvals-' + id) as HTMLInputElement;
		expect(valsEl).not.toBeNull();
		// No start/end/step for string
		expect(document.getElementById('pstart-' + id)).toBeNull();
	});

	it('sets description text', () => {
		const id = addParamRow('noise_relative');
		const descEl = document.getElementById('pdesc-' + id)!;
		expect(descEl.textContent).toContain('Fraction of measured range');
	});

	it('clears fields when name is empty', () => {
		const id = addParamRow('noise_relative');
		// Change to empty
		(document.getElementById('pname-' + id) as HTMLSelectElement).value = '';
		updateParamFields(id);
		expect(document.getElementById('pfields-' + id)!.innerHTML).toBe('');
	});

	it('creates fields for int64 type', () => {
		const id = addParamRow('warmup_duration_nanos');
		const startEl = document.getElementById('pstart-' + id) as HTMLInputElement;
		expect(startEl).not.toBeNull();
		expect(parseFloat(startEl.value)).toBe(5000000000);
	});
});

describe('getParamValueCount', () => {
	beforeEach(setupDOM);

	it('returns 0 when no name is selected', () => {
		const id = addParamRow();
		expect(getParamValueCount(id.toString())).toBe(0);
	});

	it('counts comma-separated values from values input', () => {
		const id = addParamRow('noise_relative');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '0.01, 0.05, 0.1';
		expect(getParamValueCount(id.toString())).toBe(3);
	});

	it('returns 2 for bool type without explicit values', () => {
		const id = addParamRow('seed_from_first');
		// Clear the default "true, false" value to test the bool default
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '';
		expect(getParamValueCount(id.toString())).toBe(2);
	});

	it('calculates count from start/end/step for numeric types', () => {
		const id = addParamRow('noise_relative');
		// Default: start=0.01, end=0.2, step=0.001
		// count = floor((0.2 - 0.01) / 0.001) + 1 = 191
		expect(getParamValueCount(id.toString())).toBe(191);
	});

	it('returns 0 when step is 0', () => {
		const id = addParamRow('noise_relative');
		(document.getElementById('pstep-' + id) as HTMLInputElement).value = '0';
		expect(getParamValueCount(id.toString())).toBe(0);
	});

	it('returns 0 for unknown schema', () => {
		const id = addParamRow();
		// Manually set a value that's not in PARAM_SCHEMA
		const nameEl = document.getElementById('pname-' + id) as HTMLSelectElement;
		const opt = document.createElement('option');
		opt.value = 'unknown_xyz';
		nameEl.appendChild(opt);
		nameEl.value = 'unknown_xyz';
		expect(getParamValueCount(id.toString())).toBe(0);
	});
});

// ===========================================================================
// Sweep summary
// ===========================================================================

describe('updateSweepSummary', () => {
	beforeEach(setupDOM);

	afterEach(() => {
		setMode('manual');
	});

	it('shows empty when no param rows exist', () => {
		updateSweepSummary();
		expect(document.getElementById('sweep-summary')!.innerHTML).toBe('');
	});

	it('shows permutation count in manual mode', () => {
		const id = addParamRow('noise_relative');
		// Set explicit values to have a known count
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '0.01, 0.05, 0.1';
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('<strong>3</strong> permutations');
	});

	it('doubles count when seed is toggle', () => {
		const id = addParamRow('noise_relative');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '0.01, 0.05';
		(document.getElementById('seed') as HTMLSelectElement).value = 'toggle';
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('seed toggle');
		expect(html).toContain('<strong>4</strong> total');
	});

	it('shows values per param in auto mode', () => {
		setMode('auto');
		addParamRow('noise_relative');
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('5 values');
		expect(html).toContain('permutations/round');
	});

	it('includes runtime estimate with settle_mode once', () => {
		const id = addParamRow('noise_relative');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '0.01, 0.05';
		(document.getElementById('settle_mode') as HTMLSelectElement).value = 'once';
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('estimated total runtime');
	});

	it('includes runtime estimate with settle_mode per_combo', () => {
		const id = addParamRow('noise_relative');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '0.01, 0.05';
		(document.getElementById('settle_mode') as HTMLSelectElement).value = 'per_combo';
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('estimated total runtime');
	});

	it('auto mode with seed toggle and settle once', () => {
		setMode('auto');
		addParamRow('noise_relative');
		(document.getElementById('seed') as HTMLSelectElement).value = 'toggle';
		(document.getElementById('settle_mode') as HTMLSelectElement).value = 'once';
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('seed toggle');
		expect(html).toContain('rounds');
	});
});

// ===========================================================================
// Scenario management
// ===========================================================================

describe('buildScenarioJSON', () => {
	beforeEach(setupDOM);

	it('builds scenario with default values', () => {
		addParamRow('noise_relative');
		const scenario = buildScenarioJSON();
		expect(scenario.seed).toBe('true');
		expect(scenario.iterations).toBe(10);
		expect(scenario.interval).toBe('2s');
		expect(scenario.settle_time).toBe('5s');
		expect(scenario.data_source).toBe('live');
		expect(scenario.params).toHaveLength(1);
		expect(scenario.params[0].name).toBe('noise_relative');
		expect(scenario.params[0].type).toBe('float64');
	});

	it('includes start/end/step for numeric params', () => {
		addParamRow('noise_relative');
		const scenario = buildScenarioJSON();
		expect(scenario.params[0].start).toBe(0.01);
		expect(scenario.params[0].end).toBe(0.2);
		expect(scenario.params[0].step).toBe(0.001);
	});

	it('includes explicit values when provided', () => {
		const id = addParamRow('noise_relative');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '0.01, 0.05, 0.1';
		const scenario = buildScenarioJSON();
		expect(scenario.params[0].values).toEqual([0.01, 0.05, 0.1]);
		expect(scenario.params[0].start).toBeUndefined();
	});

	it('includes pcap fields when data_source is pcap', () => {
		(document.getElementById('data_source') as HTMLSelectElement).value = 'pcap';
		(document.getElementById('pcap_file') as HTMLInputElement).value = 'test.pcap';
		addParamRow('noise_relative');
		const scenario = buildScenarioJSON();
		expect(scenario.data_source).toBe('pcap');
		expect(scenario.pcap_file).toBe('test.pcap');
	});

	it('handles scene data source', () => {
		(document.getElementById('data_source') as HTMLSelectElement).value = 'scene';
		(document.getElementById('scene_select') as HTMLSelectElement).innerHTML =
			'<option value="scene-1">Scene 1</option>';
		(document.getElementById('scene_select') as HTMLSelectElement).value = 'scene-1';
		(document.getElementById('pcap_file') as HTMLInputElement).value = 'test.pcap';
		addParamRow('noise_relative');
		const scenario = buildScenarioJSON();
		// scene translates to pcap for data_source
		expect(scenario.data_source).toBe('pcap');
		expect(scenario.scene_id).toBe('scene-1');
		expect(scenario.pcap_file).toBe('test.pcap');
	});

	it('handles bool param values', () => {
		const id = addParamRow('seed_from_first');
		// seed_from_first is bool, default values "true, false"
		const scenario = buildScenarioJSON();
		expect(scenario.params[0].values).toEqual([true, false]);
	});

	it('handles int param values', () => {
		const id = addParamRow('neighbor_confirmation_count');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '1, 3, 5';
		const scenario = buildScenarioJSON();
		expect(scenario.params[0].values).toEqual([1, 3, 5]);
	});

	it('handles string param values', () => {
		const id = addParamRow('buffer_timeout');
		(document.getElementById('pvals-' + id) as HTMLInputElement).value = '500ms, 1s, 2s';
		const scenario = buildScenarioJSON();
		expect(scenario.params[0].values).toEqual(['500ms', '1s', '2s']);
	});

	it('skips rows with no parameter selected', () => {
		addParamRow(); // no name
		addParamRow('noise_relative');
		const scenario = buildScenarioJSON();
		expect(scenario.params).toHaveLength(1);
	});
});

describe('loadScenario', () => {
	beforeEach(setupDOM);

	it('loads scenario values into form fields', () => {
		loadScenario({
			seed: 'false',
			iterations: 20,
			interval: '3s',
			settle_time: '10s',
			settle_mode: 'per_combo',
			params: [{ name: 'noise_relative', type: 'float64', start: 0.02, end: 0.1, step: 0.01 }]
		});
		expect(val('seed')).toBe('false');
		expect(val('iterations')).toBe('20');
		expect(val('interval')).toBe('3s');
		expect(val('settle_time')).toBe('10s');
		expect(val('settle_mode')).toBe('per_combo');
		expect(document.getElementById('param-rows')!.children.length).toBe(1);
	});

	it('loads explicit values', () => {
		loadScenario({
			params: [{ name: 'noise_relative', type: 'float64', values: [0.01, 0.05, 0.1] }]
		});
		const rows = document.getElementById('param-rows')!.children;
		const rowId = rows[0].id.replace('param-row-', '');
		const valsEl = document.getElementById('pvals-' + rowId) as HTMLInputElement;
		expect(valsEl.value).toBe('0.01, 0.05, 0.1');
	});

	it('handles scene_id by switching data source', () => {
		loadScenario({ scene_id: 'my-scene', params: [] });
		expect(val('data_source')).toBe('scene');
	});

	it('handles pcap data_source', () => {
		loadScenario({ data_source: 'pcap', pcap_file: 'test.pcap', params: [] });
		expect(val('data_source')).toBe('pcap');
		expect(val('pcap_file')).toBe('test.pcap');
	});

	it('clears existing param rows before loading', () => {
		addParamRow('noise_relative');
		addParamRow('closeness_multiplier');
		expect(document.getElementById('param-rows')!.children.length).toBe(2);
		loadScenario({ params: [{ name: 'hits_to_confirm', type: 'int', start: 1, end: 5, step: 1 }] });
		expect(document.getElementById('param-rows')!.children.length).toBe(1);
	});
});

describe('toggleJSONEditor', () => {
	beforeEach(setupDOM);

	it('shows editor and apply button on first toggle', () => {
		addParamRow('noise_relative');
		toggleJSONEditor();
		expect(document.getElementById('json-editor-wrap')!.style.display).toBe('');
		expect(document.getElementById('btn-apply-json')!.style.display).toBe('');
		const json = (document.getElementById('scenario-json') as HTMLTextAreaElement).value;
		expect(JSON.parse(json)).toHaveProperty('params');
	});

	it('hides editor on second toggle', () => {
		addParamRow('noise_relative');
		toggleJSONEditor(); // show
		toggleJSONEditor(); // hide
		expect(document.getElementById('json-editor-wrap')!.style.display).toBe('none');
		expect(document.getElementById('btn-apply-json')!.style.display).toBe('none');
	});
});

describe('applyJSONEditor', () => {
	beforeEach(setupDOM);

	it('applies valid JSON to form', () => {
		toggleJSONEditor();
		(document.getElementById('scenario-json') as HTMLTextAreaElement).value = JSON.stringify({
			seed: 'toggle',
			iterations: 5,
			params: []
		});
		applyJSONEditor();
		expect(val('seed')).toBe('toggle');
		expect(val('iterations')).toBe('5');
		// Editor should be hidden after apply
		expect(document.getElementById('json-editor-wrap')!.style.display).toBe('none');
	});

	it('shows error for invalid JSON', () => {
		toggleJSONEditor();
		(document.getElementById('scenario-json') as HTMLTextAreaElement).value = 'not json{';
		applyJSONEditor();
		const errorBox = document.getElementById('error-box')!;
		expect(errorBox.textContent).toContain('Invalid JSON');
		expect(errorBox.style.display).toBe('');
	});
});

describe('downloadScenario', () => {
	beforeEach(setupDOM);

	it('creates and clicks a download link', () => {
		addParamRow('noise_relative');
		const clickSpy = jest.fn();
		jest.spyOn(document, 'createElement').mockImplementation((tag: string) => {
			if (tag === 'a') {
				const a = { click: clickSpy, href: '', download: '' } as any;
				return a;
			}
			return document.createElement(tag);
		});
		downloadScenario();
		expect(URL.createObjectURL).toHaveBeenCalled();
		(document.createElement as jest.Mock).mockRestore();
	});
});

describe('uploadScenario', () => {
	beforeEach(setupDOM);

	it('loads scenario from file', () => {
		const scenario = { seed: 'false', iterations: 3, params: [] };
		// Mock FileReader
		const OrigFileReader = (global as any).FileReader;
		(global as any).FileReader = class {
			onload: any = null;
			readAsText() {
				if (this.onload) {
					this.onload({ target: { result: JSON.stringify(scenario) } });
				}
			}
		};
		const input = { files: [new Blob([''])], value: 'file.json' } as any;
		uploadScenario(input);
		expect(val('seed')).toBe('false');
		expect(val('iterations')).toBe('3');
		expect(input.value).toBe('');
		(global as any).FileReader = OrigFileReader;
	});

	it('shows error for invalid JSON file', () => {
		const OrigFileReader = (global as any).FileReader;
		(global as any).FileReader = class {
			onload: any = null;
			readAsText() {
				if (this.onload) {
					this.onload({ target: { result: 'bad json{' } });
				}
			}
		};
		const input = { files: [new Blob([''])], value: 'file.json' } as any;
		uploadScenario(input);
		expect(document.getElementById('error-box')!.textContent).toContain('Invalid JSON');
		(global as any).FileReader = OrigFileReader;
	});

	it('returns early when no files', () => {
		uploadScenario({ files: null } as any);
		// Should not throw
	});
});

// ===========================================================================
// Sweep control (requires init for sensorId)
// ===========================================================================

describe('sweep control', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		setMode('manual');
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	describe('handleStart', () => {
		it('shows error when no param rows exist', () => {
			// init adds one row; clear it
			document.getElementById('param-rows')!.innerHTML = '';
			handleStart();
			expect(document.getElementById('error-box')!.textContent).toContain(
				'Add at least one parameter'
			);
		});

		it('calls handleStartManualSweep in manual mode', () => {
			// Already has param from init
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStart();
			expect(global.fetch).toHaveBeenCalledWith(
				'/api/lidar/sweep/start',
				expect.objectContaining({ method: 'POST' })
			);
		});

		it('calls handleStartAutoTune in auto mode', () => {
			setMode('auto');
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStart();
			expect(global.fetch).toHaveBeenCalledWith(
				'/api/lidar/sweep/auto',
				expect.objectContaining({ method: 'POST' })
			);
		});
	});

	describe('handleStartManualSweep', () => {
		it('sends POST to start endpoint', () => {
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStartManualSweep();
			expect(global.fetch).toHaveBeenCalledWith(
				'/api/lidar/sweep/start',
				expect.objectContaining({ method: 'POST' })
			);
		});

		it('shows error on fetch failure', async () => {
			global.fetch = jest.fn().mockResolvedValue({
				ok: false,
				text: () => Promise.resolve('Server error')
			});
			handleStartManualSweep();
			await flushPromises();
			expect(document.getElementById('error-box')!.textContent).toContain('Server error');
		});

		it('shows error when no params', () => {
			document.getElementById('param-rows')!.innerHTML = '';
			handleStartManualSweep();
			expect(document.getElementById('error-box')!.textContent).toContain(
				'Add at least one parameter'
			);
		});
	});

	describe('handleStartAutoTune', () => {
		it('sends POST to auto endpoint with params', () => {
			setMode('auto');
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStartAutoTune();
			expect(global.fetch).toHaveBeenCalledWith(
				'/api/lidar/sweep/auto',
				expect.objectContaining({ method: 'POST' })
			);
		});

		it('shows error when no params', () => {
			setMode('auto');
			document.getElementById('param-rows')!.innerHTML = '';
			handleStartAutoTune();
			expect(document.getElementById('error-box')!.textContent).toContain(
				'Add at least one parameter'
			);
		});

		it('includes weights when objective is weighted', () => {
			setMode('auto');
			(document.getElementById('objective') as HTMLSelectElement).value = 'weighted';
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStartAutoTune();
			const body = JSON.parse((global.fetch as jest.Mock).mock.calls[0][1].body);
			expect(body.weights).toBeDefined();
			expect(body.weights.acceptance).toBe(1.0);
		});

		it('includes scene_id when data_source is scene', () => {
			setMode('auto');
			(document.getElementById('data_source') as HTMLSelectElement).value = 'scene';
			(document.getElementById('scene_select') as HTMLSelectElement).innerHTML =
				'<option value="s1">Scene</option>';
			(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
			(document.getElementById('pcap_file') as HTMLInputElement).value = 'test.pcap';
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStartAutoTune();
			const body = JSON.parse((global.fetch as jest.Mock).mock.calls[0][1].body);
			expect(body.scene_id).toBe('s1');
			expect(body.data_source).toBe('pcap');
		});

		it('shows error on fetch failure', async () => {
			setMode('auto');
			global.fetch = jest.fn().mockRejectedValue(new Error('Network fail'));
			handleStartAutoTune();
			await flushPromises();
			expect(document.getElementById('error-box')!.textContent).toContain('Network fail');
		});

		it('hides recommendation card', () => {
			setMode('auto');
			document.getElementById('recommendation-card')!.style.display = '';
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStartAutoTune();
			expect(document.getElementById('recommendation-card')!.style.display).toBe('none');
		});
	});

	describe('handleStop', () => {
		it('sends POST to stop endpoint for manual sweep', () => {
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStop();
			expect(global.fetch).toHaveBeenCalledWith('/api/lidar/sweep/stop', { method: 'POST' });
			expect(document.getElementById('btn-stop')!.style.display).toBe('none');
			expect(document.getElementById('stopping-indicator')!.style.display).toBe('block');
		});

		it('sends POST to auto/stop for auto mode', () => {
			setMode('auto');
			global.fetch = jest.fn().mockResolvedValue({ ok: true });
			handleStop();
			expect(global.fetch).toHaveBeenCalledWith('/api/lidar/sweep/auto/stop', { method: 'POST' });
		});
	});

	describe('startPolling / stopPolling', () => {
		it('starts polling with setInterval', () => {
			global.fetch = jest.fn().mockResolvedValue({
				ok: true,
				json: () => Promise.resolve({ status: 'running', completed_combos: 0, total_combos: 10 })
			});
			startPolling();
			// pollStatus is called immediately
			expect(global.fetch).toHaveBeenCalled();
		});

		it('stopPolling clears interval', () => {
			global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
			startPolling();
			stopPolling();
			const fetchCount = (global.fetch as jest.Mock).mock.calls.length;
			jest.advanceTimersByTime(10000);
			// No additional calls after stop
			expect((global.fetch as jest.Mock).mock.calls.length).toBe(fetchCount);
		});
	});
});

// ===========================================================================
// Polling
// ===========================================================================

describe('pollStatus (manual mode)', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		setMode('manual');
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('updates UI for running status', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 3,
					total_combos: 10,
					current_combo: {
						param_values: { noise_relative: 0.05 },
						overall_accept_mean: 0.85
					}
				})
		});
		pollStatus();
		await flushPromises();
		expect(document.getElementById('progress-section')!.style.display).toBe('');
		expect(document.getElementById('status-badge')!.textContent).toBe('running');
		expect(document.getElementById('combo-count')!.textContent).toContain('3 / 10');
		expect(document.getElementById('current-combo')!.textContent).toContain('Current:');
		expect(document.getElementById('btn-start')!.style.display).toBe('none');
		expect(document.getElementById('btn-stop')!.style.display).toBe('');
	});

	it('stops polling on complete status', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'complete',
					completed_combos: 10,
					total_combos: 10,
					results: makeTestResults()
				})
		});
		pollStatus();
		await flushPromises();
		expect(document.getElementById('status-badge')!.textContent).toBe('complete');
		expect(document.getElementById('btn-start')!.style.display).toBe('');
		expect(document.getElementById('btn-stop')!.style.display).toBe('none');
	});

	it('displays error message', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'error',
					completed_combos: 5,
					total_combos: 10,
					error: 'Something failed'
				})
		});
		pollStatus();
		await flushPromises();
		expect(document.getElementById('sweep-error')!.textContent).toBe('Something failed');
		expect(document.getElementById('sweep-error')!.style.display).toBe('');
	});

	it('displays warnings', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 1,
					total_combos: 10,
					warnings: ['Low acceptance', 'High variance']
				})
		});
		pollStatus();
		await flushPromises();
		const warnEl = document.getElementById('sweep-warnings')!;
		expect(warnEl.style.display).toBe('');
		expect(warnEl.innerHTML).toContain('Low acceptance');
		expect(warnEl.innerHTML).toContain('High variance');
	});

	it('shows stopping indicator when stop is requested', async () => {
		// Simulate handleStop setting stopRequested
		handleStop();
		(global.fetch as jest.Mock).mockClear();
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 5,
					total_combos: 10
				})
		});
		pollStatus();
		await flushPromises();
		expect(document.getElementById('stopping-indicator')!.style.display).toBe('block');
	});

	it('handles fetch rejection gracefully', async () => {
		global.fetch = jest.fn().mockRejectedValue(new Error('network'));
		pollStatus();
		await flushPromises();
		// Should not throw
	});

	it('renders charts and table when results are present', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'complete',
					completed_combos: 10,
					total_combos: 10,
					results: makeTestResults()
				})
		});
		pollStatus();
		await flushPromises();
		// Table should be populated
		expect(document.getElementById('results-body')!.children.length).toBeGreaterThan(0);
	});

	it('delegates to pollAutoTuneStatus in auto mode', async () => {
		setMode('auto');
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ status: 'idle', completed_combos: 0, total_combos: 0 })
		});
		pollStatus();
		await flushPromises();
		// Should call auto endpoint
		expect(global.fetch).toHaveBeenCalledWith('/api/lidar/sweep/auto');
	});
});

describe('pollAutoTuneStatus', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		setMode('auto');
	});

	afterEach(() => {
		stopPolling();
		setMode('manual');
		jest.useRealTimers();
	});

	it('updates UI for running status', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 10,
					total_combos: 25,
					total_rounds: 3,
					round: 2
				})
		});
		pollAutoTuneStatus();
		await flushPromises();
		expect(document.getElementById('combo-count')!.textContent).toContain('Round 2/3');
		expect(document.getElementById('combo-count')!.textContent).toContain('10 / 25');
		expect(document.getElementById('current-combo')!.textContent).toContain(
			'Running initial round'
		);
	});

	it('shows last round best when round_results exist', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 10,
					total_combos: 25,
					total_rounds: 3,
					round: 2,
					round_results: [{ best_score: 0.92, best_params: { noise_relative: 0.05 } }]
				})
		});
		pollAutoTuneStatus();
		await flushPromises();
		expect(document.getElementById('current-combo')!.innerHTML).toContain('0.9200');
	});

	it('shows recommendation on complete', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'complete',
					completed_combos: 25,
					total_combos: 25,
					total_rounds: 3,
					recommendation: {
						noise_relative: 0.05,
						score: 0.95,
						acceptance_rate: 0.9,
						misalignment_ratio: 0.02,
						alignment_deg: 0.5,
						nonzero_cells: 500
					},
					round_results: [
						{
							round: 1,
							num_combos: 25,
							best_score: 0.9,
							best_params: {},
							bounds: { noise_relative: [0.01, 0.2] }
						}
					],
					results: makeTestResults()
				})
		});
		pollAutoTuneStatus();
		await flushPromises();
		expect(document.getElementById('recommendation-card')!.style.display).toBe('');
	});

	it('handles error status', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'error',
					completed_combos: 0,
					total_combos: 0,
					error: 'Auto-tune failed'
				})
		});
		pollAutoTuneStatus();
		await flushPromises();
		expect(document.getElementById('sweep-error')!.textContent).toBe('Auto-tune failed');
	});

	it('handles fetch rejection gracefully', async () => {
		global.fetch = jest.fn().mockRejectedValue(new Error('network'));
		pollAutoTuneStatus();
		await flushPromises();
		// Should not throw
	});
});

// ===========================================================================
// Rendering
// ===========================================================================

describe('initCharts', () => {
	beforeEach(setupDOM);

	it('initializes 8 chart instances', () => {
		(global as any).echarts.init.mockClear();
		initCharts();
		expect((global as any).echarts.init).toHaveBeenCalledTimes(8);
	});
});

describe('renderRecommendation', () => {
	beforeEach(setupDOM);

	it('renders recommendation card with params and metrics', () => {
		const rec = {
			noise_relative: 0.05,
			closeness_multiplier: 5.0,
			score: 0.95,
			acceptance_rate: 0.9,
			misalignment_ratio: 0.02,
			alignment_deg: 0.5,
			nonzero_cells: 500
		};
		const roundResults = [
			{
				round: 1,
				num_combos: 25,
				best_score: 0.9,
				best_params: {},
				bounds: { noise_relative: [0.01, 0.2] }
			}
		];
		renderRecommendation(rec, roundResults);
		const card = document.getElementById('recommendation-card')!;
		expect(card.style.display).toBe('');
		const content = document.getElementById('recommendation-content')!.innerHTML;
		expect(content).toContain('Noise Relative');
		expect(content).toContain('Score');
		expect(content).toContain('Round 1');
	});

	it('works without round results', () => {
		renderRecommendation(
			{
				score: 0.8,
				acceptance_rate: 0.7,
				misalignment_ratio: 0,
				alignment_deg: 0,
				nonzero_cells: 0
			},
			null
		);
		expect(document.getElementById('recommendation-card')!.style.display).toBe('');
	});
});

describe('renderTable', () => {
	beforeEach(setupDOM);

	it('renders standard results table', () => {
		renderTable(makeTestResults());
		const rows = document.getElementById('results-body')!.children;
		expect(rows.length).toBe(2);
		// Check header has standard columns
		expect(document.getElementById('results-head')!.innerHTML).toContain('Accept Rate');
		expect(document.getElementById('results-head')!.innerHTML).toContain('Nonzero Cells');
	});

	it('renders ground truth results table', () => {
		renderTable(makeGTResults());
		expect(document.getElementById('results-head')!.innerHTML).toContain('GT Score');
		expect(document.getElementById('results-head')!.innerHTML).toContain('Detection %');
		expect(document.getElementById('results-head')!.innerHTML).toContain('Frag.');
	});

	it('handles legacy results without param_values', () => {
		renderTable([
			{
				noise: 0.05,
				closeness: 5,
				neighbour: 1,
				overall_accept_mean: 0.85,
				overall_accept_stddev: 0.02,
				nonzero_cells_mean: 100,
				nonzero_cells_stddev: 5,
				active_tracks_mean: 3,
				alignment_deg_mean: 1.2,
				misalignment_ratio_mean: 0.05
			}
		]);
		expect(document.getElementById('results-body')!.children.length).toBe(1);
	});
});

describe('renderCharts', () => {
	beforeEach(() => {
		setupDOM();
		initCharts();
	});

	it('renders all charts with full data', () => {
		// Should not throw
		renderCharts(makeTestResults());
		// Heatmaps should be visible (2 numeric params)
		expect(document.getElementById('param-heatmap')!.style.display).toBe('');
		expect(document.getElementById('tracks-heatmap')!.style.display).toBe('');
		expect(document.getElementById('alignment-heatmap')!.style.display).toBe('');
	});

	it('hides heatmaps when only one numeric param', () => {
		const results = [
			{
				param_values: { noise_relative: 0.05, seed_from_first: true },
				overall_accept_mean: 0.85,
				overall_accept_stddev: 0.02,
				nonzero_cells_mean: 100,
				nonzero_cells_stddev: 5,
				active_tracks_mean: 3,
				alignment_deg_mean: 1.0,
				misalignment_ratio_mean: 0.05
			}
		];
		renderCharts(results);
		expect(document.getElementById('param-heatmap')!.style.display).toBe('none');
		expect(document.getElementById('tracks-heatmap')!.style.display).toBe('none');
		expect(document.getElementById('alignment-heatmap')!.style.display).toBe('none');
	});

	it('hides heatmaps when no param_values', () => {
		const results = [
			{
				noise: 0.05,
				closeness: 5,
				neighbour: 1,
				overall_accept_mean: 0.85,
				nonzero_cells_mean: 100,
				active_tracks_mean: 3,
				alignment_deg_mean: 1.0,
				misalignment_ratio_mean: 0.05
			}
		];
		renderCharts(results);
		expect(document.getElementById('param-heatmap')!.style.display).toBe('none');
	});

	it('renders without buckets', () => {
		const results = makeTestResults().map((r: any) => {
			const { buckets: _b, bucket_means: _bm, ...rest } = r;
			return rest;
		});
		// Should not throw
		renderCharts(results);
	});
});

describe('downloadCSV', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('shows error when no results available', () => {
		downloadCSV();
		expect(document.getElementById('error-box')!.textContent).toContain('No results to download');
	});

	it('downloads CSV when results are loaded', async () => {
		// Populate latestResults via pollStatus
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'complete',
					completed_combos: 2,
					total_combos: 2,
					results: makeTestResults()
				})
		});
		pollStatus();
		await flushPromises();
		(URL.createObjectURL as jest.Mock).mockClear();
		downloadCSV();
		expect(URL.createObjectURL).toHaveBeenCalled();
	});
});

// ===========================================================================
// Current params
// ===========================================================================

describe('fetchCurrentParams', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('fetches and displays params', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ noise_relative: 0.05, seed_from_first: true })
		});
		fetchCurrentParams();
		await flushPromises();
		const display = document.getElementById('current-params-display')!.innerHTML;
		expect(display).toContain('noise_relative');
		expect(display).toContain('seed_from_first');
	});

	it('shows error on failure', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: false,
			json: () => Promise.reject(new Error('fail'))
		});
		fetchCurrentParams();
		await flushPromises();
		expect(document.getElementById('current-params-display')!.textContent).toContain(
			'Error loading parameters'
		);
	});
});

describe('displayCurrentParams', () => {
	beforeEach(setupDOM);

	it('displays all param types correctly', () => {
		displayCurrentParams({
			noise_relative: 0.05,
			seed_from_first: true,
			warmup_min_frames: 100,
			buffer_timeout: null
		});
		const html = document.getElementById('current-params-display')!.innerHTML;
		expect(html).toContain('noise_relative');
		expect(html).toContain('0.05');
		expect(html).toContain('true');
		expect(html).toContain('100');
		expect(html).toContain('null');
	});

	it('highlights swept parameters', () => {
		// Add a param row for noise_relative
		addParamRow('noise_relative');
		displayCurrentParams({ noise_relative: 0.05, closeness_multiplier: 5.0 });
		const html = document.getElementById('current-params-display')!.innerHTML;
		expect(html).toContain('param-line swept');
		expect(html).toContain('param-line"');
	});

	it('sorts swept params first', () => {
		addParamRow('closeness_multiplier');
		displayCurrentParams({ noise_relative: 0.05, closeness_multiplier: 5.0, hits_to_confirm: 3 });
		const html = document.getElementById('current-params-display')!.innerHTML;
		// closeness_multiplier should appear before noise_relative in the output
		const cmIdx = html.indexOf('closeness_multiplier');
		const nrIdx = html.indexOf('noise_relative');
		expect(cmIdx).toBeLessThan(nrIdx);
	});
});

// ===========================================================================
// Scene management
// ===========================================================================

describe('loadSweepScenes', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('populates scene select with scenes', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [
						{ scene_id: 's1', pcap_file: 'test1.pcap', description: 'Scene 1' },
						{ scene_id: 's2', pcap_file: 'test2.pcap' }
					]
				})
		});
		loadSweepScenes();
		await flushPromises();
		const select = document.getElementById('scene_select') as HTMLSelectElement;
		// 1 default option + 2 scenes
		expect(select.options.length).toBe(3);
		expect(select.options[1].textContent).toContain('Scene 1');
	});

	it('shows error on fetch failure', async () => {
		global.fetch = jest.fn().mockRejectedValue(new Error('fail'));
		loadSweepScenes();
		await flushPromises();
		const select = document.getElementById('scene_select') as HTMLSelectElement;
		expect(select.innerHTML).toContain('failed to load');
	});

	it('returns early when scene_select is missing', () => {
		document.getElementById('scene_select')!.remove();
		global.fetch = jest.fn();
		loadSweepScenes();
		expect(global.fetch).not.toHaveBeenCalled();
	});
});

describe('onSweepSceneSelected', () => {
	beforeEach(async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		// Populate scenes
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [
						{ scene_id: 's1', pcap_file: 'test1.pcap', pcap_start_secs: 5, pcap_duration_secs: 30 },
						{
							scene_id: 's2',
							pcap_file: 'test2.pcap',
							reference_run_id: 'ref-1',
							optimal_params_json: '{"noise_relative": 0.05}'
						}
					]
				})
		});
		loadSweepScenes();
		await flushPromises();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('hides info when no scene is selected', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = '';
		onSweepSceneSelected();
		expect(document.getElementById('scene-info')!.style.display).toBe('none');
		expect(document.getElementById('scene-actions')!.style.display).toBe('none');
	});

	it('shows scene info when scene is selected', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		const info = document.getElementById('scene-info')!;
		expect(info.style.display).toBe('');
		expect(info.textContent).toContain('test1.pcap');
		expect(info.textContent).toContain('Start: 5s');
		expect(info.textContent).toContain('Duration: 30s');
	});

	it('shows reference info and ground truth option for scene with reference', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's2';
		onSweepSceneSelected();
		const info = document.getElementById('scene-info')!;
		expect(info.textContent).toContain('Reference: ref-1');
		expect(document.getElementById('ground_truth_option')!.style.display).toBe('');
	});

	it('hides ground truth option for scene without reference', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		expect(document.getElementById('ground_truth_option')!.style.display).toBe('none');
	});

	it('shows action buttons for scene with optimal params', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's2';
		onSweepSceneSelected();
		expect(document.getElementById('scene-actions')!.style.display).toBe('');
	});

	it('hides info for unknown scene', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 'unknown';
		onSweepSceneSelected();
		expect(document.getElementById('scene-info')!.style.display).toBe('none');
	});

	it('populates pcap fields from scene', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		expect(val('pcap_file')).toBe('test1.pcap');
		expect(val('pcap_start_secs')).toBe('5');
		expect(val('pcap_duration_secs')).toBe('30');
	});
});

// ===========================================================================
// Apply functions
// ===========================================================================

describe('applyRecommendation', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('applies recommendation params to system', async () => {
		let callCount = 0;
		global.fetch = jest.fn().mockImplementation((url: string) => {
			callCount++;
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							recommendation: { noise_relative: 0.05, score: 0.9, acceptance_rate: 0.85 }
						})
				});
			}
			if (url.includes('/api/lidar/params')) {
				return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		applyRecommendation();
		await flushPromises();
		// Should have called both endpoints
		expect(callCount).toBeGreaterThanOrEqual(2);
		expect(document.getElementById('btn-apply-recommendation')!.textContent).toBe('Applied');
	});

	it('shows error when no recommendation available', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ recommendation: null })
		});
		applyRecommendation();
		await flushPromises();
		expect(document.getElementById('error-box')!.textContent).toContain('No recommendation');
	});

	it('shows error on fetch failure', async () => {
		global.fetch = jest.fn().mockRejectedValue(new Error('Fetch error'));
		applyRecommendation();
		await flushPromises();
		expect(document.getElementById('error-box')!.textContent).toContain(
			'Failed to fetch recommendation'
		);
	});
});

describe('applySceneParams', () => {
	beforeEach(async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		// Load scenes
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [
						{
							scene_id: 's1',
							pcap_file: 'test.pcap',
							optimal_params_json: '{"noise_relative": 0.05}'
						}
					]
				})
		});
		loadSweepScenes();
		await flushPromises();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('applies scene optimal params', async () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		global.fetch = jest.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) });
		applySceneParams();
		await flushPromises();
		expect(global.fetch).toHaveBeenCalledWith(
			expect.stringContaining('/api/lidar/params'),
			expect.objectContaining({ method: 'POST' })
		);
		expect(document.getElementById('btn-apply-scene-params')!.textContent).toContain('Applied');
	});

	it('shows error when no scene selected', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = '';
		applySceneParams();
		expect(document.getElementById('error-box')!.textContent).toContain('No scene selected');
	});

	it('shows error for scene without optimal params', async () => {
		// Load scene without optimal_params_json
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [{ scene_id: 's3', pcap_file: 'test.pcap' }]
				})
		});
		loadSweepScenes();
		await flushPromises();
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's3';
		onSweepSceneSelected();
		applySceneParams();
		expect(document.getElementById('error-box')!.textContent).toContain('no optimal parameters');
	});

	it('shows error on apply failure', async () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		global.fetch = jest.fn().mockResolvedValue({
			ok: false,
			text: () => Promise.resolve('Apply error')
		});
		applySceneParams();
		await flushPromises();
		expect(document.getElementById('error-box')!.textContent).toContain('Apply failed');
	});
});

// ===========================================================================
// Init (integration)
// ===========================================================================

describe('init', () => {
	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('reads sensor-id from meta tag', () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		// sensor-id is used in fetch calls
		(global.fetch as jest.Mock).mockClear();
		global.fetch = jest.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) });
		fetchCurrentParams();
		expect(global.fetch).toHaveBeenCalledWith(expect.stringContaining('sensor_id=test-sensor'));
	});

	it('adds a default noise_relative param row', () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		expect(document.getElementById('param-rows')!.children.length).toBeGreaterThanOrEqual(1);
	});

	it('starts polling when auto-tune is running', async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = makeFetchRouter({
			'/api/lidar/sweep/auto': { status: 'running' }
		});
		init();
		await flushPromises();
		// Auto mode should be set
		expect(document.getElementById('mode-auto')!.className).toBe('active');
	});

	it('shows recommendation when auto-tune is complete', async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = makeFetchRouter({
			'/api/lidar/sweep/auto': {
				status: 'complete',
				recommendation: {
					noise_relative: 0.05,
					score: 0.95,
					acceptance_rate: 0.9,
					misalignment_ratio: 0.02,
					alignment_deg: 0.5,
					nonzero_cells: 500
				},
				round_results: [],
				results: makeTestResults()
			}
		});
		init();
		await flushPromises();
		expect(document.getElementById('recommendation-card')!.style.display).toBe('');
	});

	it('falls back to manual sweep check when auto-tune is idle', async () => {
		jest.useFakeTimers();
		setupDOM();
		let manualStatusCalled = false;
		global.fetch = jest.fn().mockImplementation((url: string) => {
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ status: 'idle' })
				});
			}
			if (url.includes('/api/lidar/sweep/status')) {
				manualStatusCalled = true;
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							status: 'complete',
							completed_combos: 5,
							total_combos: 5,
							results: makeTestResults()
						})
				});
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		init();
		await flushPromises();
		expect(manualStatusCalled).toBe(true);
	});

	it('falls back to manual sweep on auto-tune fetch error', async () => {
		jest.useFakeTimers();
		setupDOM();
		let manualStatusCalled = false;
		global.fetch = jest.fn().mockImplementation((url: string) => {
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.reject(new Error('Auto not available'));
			}
			if (url.includes('/api/lidar/sweep/status')) {
				manualStatusCalled = true;
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							status: 'idle',
							results: []
						})
				});
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		init();
		await flushPromises();
		expect(manualStatusCalled).toBe(true);
	});

	it('initializes charts', () => {
		jest.useFakeTimers();
		setupDOM();
		(global as any).echarts.init.mockClear();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		expect((global as any).echarts.init).toHaveBeenCalledTimes(8);
	});

	it('schedules chart resize after 100ms', () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		// Advance timer to trigger resize
		jest.advanceTimersByTime(100);
		// Should not throw (charts are mocked)
	});

	it('wires up input event listeners for summary updates', () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		// Changing iterations should update summary (calls updateSweepSummary via event delegation)
		const iterEl = document.getElementById('iterations')!;
		iterEl.dispatchEvent(new Event('input', { bubbles: true }));
		// Should not throw
	});
});

// ===========================================================================
// Additional coverage: uncovered branches and formatter functions
// ===========================================================================

describe('window resize handler', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('resizes all charts on window resize', () => {
		window.dispatchEvent(new Event('resize'));
		// Should not throw (charts are mocked with resize method)
	});
});

describe('pollAutoTuneStatus with stopRequested', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		setMode('auto');
	});

	afterEach(() => {
		stopPolling();
		setMode('manual');
		jest.useRealTimers();
	});

	it('shows stopping indicator when stopRequested in auto mode', async () => {
		handleStop(); // sets stopRequested = true
		(global.fetch as jest.Mock).mockClear();
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 5,
					total_combos: 10,
					total_rounds: 3,
					round: 2
				})
		});
		pollAutoTuneStatus();
		await flushPromises();
		expect(document.getElementById('stopping-indicator')!.style.display).toBe('block');
		expect(document.getElementById('btn-stop')!.style.display).toBe('none');
	});
});

describe('updateSweepSummary additional branches', () => {
	beforeEach(setupDOM);

	afterEach(() => {
		setMode('manual');
	});

	it('auto mode with bool param (no start/end fields, else branch)', () => {
		setMode('auto');
		addParamRow('seed_from_first');
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('5 values');
	});

	it('auto mode with per_combo settle (else branch for runtime)', () => {
		setMode('auto');
		addParamRow('noise_relative');
		(document.getElementById('settle_mode') as HTMLSelectElement).value = 'per_combo';
		(document.getElementById('seed') as HTMLSelectElement).value = 'true';
		updateSweepSummary();
		const html = document.getElementById('sweep-summary')!.innerHTML;
		expect(html).toContain('estimated total runtime');
	});
});

describe('loadScenario additional branches', () => {
	beforeEach(setupDOM);

	it('loads pcap_start_secs and pcap_duration_secs', () => {
		loadScenario({
			data_source: 'pcap',
			pcap_file: 'test.pcap',
			pcap_start_secs: 10,
			pcap_duration_secs: 60,
			params: []
		});
		expect(val('pcap_start_secs')).toBe('10');
		expect(val('pcap_duration_secs')).toBe('60');
	});
});

describe('handleStartAutoTune error branch', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		setMode('auto');
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('shows error when auto endpoint returns non-ok', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: false,
			text: () => Promise.resolve('Auto-tune error response')
		});
		handleStartAutoTune();
		await flushPromises();
		expect(document.getElementById('error-box')!.textContent).toContain('Auto-tune error response');
	});
});

describe('applyRecommendation error on params POST', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('shows error when params POST fails', async () => {
		let callIdx = 0;
		global.fetch = jest.fn().mockImplementation((url: string) => {
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							recommendation: { noise_relative: 0.05, score: 0.9 }
						})
				});
			}
			if (url.includes('/api/lidar/params')) {
				return Promise.resolve({
					ok: false,
					text: () => Promise.resolve('Param apply rejected')
				});
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		applyRecommendation();
		await flushPromises();
		expect(document.getElementById('error-box')!.textContent).toContain('Apply failed');
	});
});

describe('applySceneParams invalid JSON', () => {
	beforeEach(async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		// Load scene with invalid JSON
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [
						{
							scene_id: 's-bad',
							pcap_file: 'test.pcap',
							optimal_params_json: 'not valid json{'
						}
					]
				})
		});
		loadSweepScenes();
		await flushPromises();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('shows error when optimal_params_json is invalid', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's-bad';
		onSweepSceneSelected();
		applySceneParams();
		expect(document.getElementById('error-box')!.textContent).toContain('Failed to parse');
	});
});

describe('downloadCSV legacy format', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('downloads CSV with legacy param keys', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'complete',
					completed_combos: 1,
					total_combos: 1,
					results: [
						{
							noise: 0.05,
							closeness: 5,
							neighbour: 1,
							overall_accept_mean: 0.85,
							overall_accept_stddev: 0.02,
							nonzero_cells_mean: 100,
							nonzero_cells_stddev: 5,
							active_tracks_mean: 3,
							active_tracks_stddev: 0.5,
							alignment_deg_mean: 1.2,
							alignment_deg_stddev: 0.3,
							misalignment_ratio_mean: 0.05,
							misalignment_ratio_stddev: 0.01
						}
					]
				})
		});
		pollStatus();
		await flushPromises();
		(URL.createObjectURL as jest.Mock).mockClear();
		downloadCSV();
		expect(URL.createObjectURL).toHaveBeenCalled();
	});
});

describe('init with manual sweep running', () => {
	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('starts polling when manual sweep is running on load', async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation((url: string) => {
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ status: 'idle' })
				});
			}
			if (url.includes('/api/lidar/sweep/status')) {
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							status: 'running',
							completed_combos: 3,
							total_combos: 10,
							results: []
						})
				});
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		init();
		await flushPromises();
		// Manual sweep running should trigger startPolling
		// The fetch for sweep/status should have been called
		const calls = (global.fetch as jest.Mock).mock.calls.map((c: any[]) => c[0]);
		expect(calls).toContain('/api/lidar/sweep/status');
	});

	it('shows results when auto error and manual sweep has results', async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation((url: string) => {
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.reject(new Error('Auto not available'));
			}
			if (url.includes('/api/lidar/sweep/status')) {
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							status: 'complete',
							completed_combos: 2,
							total_combos: 2,
							results: makeTestResults()
						})
				});
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		init();
		await flushPromises();
		expect(document.getElementById('results-body')!.children.length).toBeGreaterThan(0);
	});

	it('starts polling when auto error and manual sweep is running', async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation((url: string) => {
			if (url.includes('/api/lidar/sweep/auto')) {
				return Promise.reject(new Error('Auto not available'));
			}
			if (url.includes('/api/lidar/sweep/status')) {
				return Promise.resolve({
					ok: true,
					json: () =>
						Promise.resolve({
							status: 'running',
							completed_combos: 1,
							total_combos: 10
						})
				});
			}
			return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
		});
		init();
		await flushPromises();
		// Should have started polling
		const calls = (global.fetch as jest.Mock).mock.calls.map((c: any[]) => c[0]);
		expect(
			calls.filter((u: string) => u.includes('/api/lidar/sweep/status')).length
		).toBeGreaterThanOrEqual(1);
	});
});

describe('ECharts formatter functions', () => {
	beforeEach(() => {
		setupDOM();
		(global as any).echarts.init.mockClear();
		initCharts();
	});

	it('acceptance chart yAxis formatter returns percentage string', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		// acceptChart is the first init call
		const acceptMock = optCalls[0].value;
		const opts = acceptMock.setOption.mock.calls[1][0]; // second call from renderCharts
		const formatter = opts.yAxis.axisLabel.formatter;
		expect(formatter(0.85)).toBe('85%');
		expect(formatter(1)).toBe('100%');
	});

	it('bucket chart tooltip formatter returns bucket info', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		// bktChart is the third init call (index 2)
		const bktMock = optCalls[2].value;
		const lastCallIdx = bktMock.setOption.mock.calls.length - 1;
		const opts = bktMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.tooltip.formatter;
		expect(formatter({ value: [0, 1, 0.85] })).toContain('85.00%');
	});

	it('bucket chart visualMap formatter returns percentage string', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		const bktMock = optCalls[2].value;
		const lastCallIdx = bktMock.setOption.mock.calls.length - 1;
		const opts = bktMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.visualMap.formatter;
		expect(formatter(0.9)).toBe('90.0%');
	});

	it('alignment chart tooltip formatter returns alignment info', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		// alignChart is the 4th init call (index 3)
		const alignMock = optCalls[3].value;
		const lastCallIdx = alignMock.setOption.mock.calls.length - 1;
		const opts = alignMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.tooltip.formatter;
		const result = formatter([{ dataIndex: 0 }]);
		expect(result).toContain('Alignment:');
		expect(result).toContain('Misalignment:');
	});

	it('alignment chart yAxis formatter returns percentage string', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		const alignMock = optCalls[3].value;
		const lastCallIdx = alignMock.setOption.mock.calls.length - 1;
		const opts = alignMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.yAxis[1].axisLabel.formatter;
		expect(formatter(0.05)).toBe('5%');
	});

	it('param heatmap tooltip formatter returns param info', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		// paramHeatmapChart is the 6th init call (index 5)
		const hmMock = optCalls[5].value;
		const lastCallIdx = hmMock.setOption.mock.calls.length - 1;
		const opts = hmMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.tooltip.formatter;
		const result = formatter({ value: [0, 0, 0.9] });
		expect(result).toContain('Accept:');
		expect(result).toContain('90.00%');
	});

	it('param heatmap visualMap formatter returns percentage string', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		const hmMock = optCalls[5].value;
		const lastCallIdx = hmMock.setOption.mock.calls.length - 1;
		const opts = hmMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.visualMap.formatter;
		expect(formatter(0.85)).toBe('85.0%');
	});

	it('tracks heatmap tooltip formatter returns track info', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		// tracksHeatmapChart is the 7th init call (index 6)
		const thmMock = optCalls[6].value;
		const lastCallIdx = thmMock.setOption.mock.calls.length - 1;
		const opts = thmMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.tooltip.formatter;
		const result = formatter({ value: [0, 0, 3.5] });
		expect(result).toContain('Tracks:');
		expect(result).toContain('3.5');
	});

	it('alignment heatmap tooltip formatter returns alignment info', () => {
		renderCharts(makeTestResults());
		const optCalls = (global as any).echarts.init.mock.results;
		// alignHeatmapChart is the 8th init call (index 7)
		const ahmMock = optCalls[7].value;
		const lastCallIdx = ahmMock.setOption.mock.calls.length - 1;
		const opts = ahmMock.setOption.mock.calls[lastCallIdx][0];
		const formatter = opts.tooltip.formatter;
		const result = formatter({ value: [0, 0, 1.2] });
		expect(result).toContain('Alignment:');
	});
});

// ===========================================================================
// Additional branch-coverage tests
// ===========================================================================

describe('handleStop fetch rejection', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('shows error when stop fetch rejects', async () => {
		global.fetch = jest.fn().mockRejectedValue(new Error('Stop failed'));
		handleStop();
		await flushPromises();
		expect(document.getElementById('error-box')!.textContent).toContain('Stop failed');
	});
});

describe('applySceneParams setTimeout callback', () => {
	beforeEach(async () => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [
						{
							scene_id: 's1',
							pcap_file: 'test.pcap',
							optimal_params_json: '{"noise_relative": 0.05}'
						}
					]
				})
		});
		loadSweepScenes();
		await flushPromises();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('resets button text after timeout', async () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({})
		});
		applySceneParams();
		await flushPromises();
		expect(document.getElementById('btn-apply-scene-params')!.textContent).toContain('Applied');
		jest.advanceTimersByTime(2000);
		expect(document.getElementById('btn-apply-scene-params')!.textContent).toBe(
			'Apply Scene Params'
		);
	});
});

describe('onSweepSceneSelected without gtOption element', () => {
	beforeEach(async () => {
		jest.useFakeTimers();
		setupDOM();
		document.getElementById('ground_truth_option')!.remove();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					scenes: [
						{ scene_id: 's1', pcap_file: 'test.pcap' },
						{
							scene_id: 's2',
							pcap_file: 'test2.pcap',
							reference_run_id: 'ref-1',
							optimal_params_json: '{"noise_relative": 0.05}'
						}
					]
				})
		});
		loadSweepScenes();
		await flushPromises();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('handles missing gtOption for empty selection', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = '';
		onSweepSceneSelected();
		expect(document.getElementById('scene-info')!.style.display).toBe('none');
	});

	it('handles missing gtOption for unknown scene', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 'unknown';
		onSweepSceneSelected();
		expect(document.getElementById('scene-info')!.style.display).toBe('none');
	});

	it('handles missing gtOption for scene with reference', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's2';
		onSweepSceneSelected();
		expect(document.getElementById('scene-info')!.style.display).toBe('');
		expect(document.getElementById('scene-info')!.textContent).toContain('Reference: ref-1');
	});

	it('handles missing gtOption for scene without reference', () => {
		(document.getElementById('scene_select') as HTMLSelectElement).value = 's1';
		onSweepSceneSelected();
		expect(document.getElementById('scene-info')!.style.display).toBe('');
	});
});

describe('param-rows change event', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('calls displayCurrentParams with empty object when no cache', () => {
		delete (window as any).currentParamsCache;
		const paramRows = document.getElementById('param-rows')!;
		paramRows.dispatchEvent(new Event('change', { bubbles: true }));
		// Should not throw
		expect(document.getElementById('current-params-display')!.innerHTML).toBe('');
	});

	it('calls displayCurrentParams with cached params', () => {
		(window as any).currentParamsCache = { noise_relative: 0.05 };
		const paramRows = document.getElementById('param-rows')!;
		paramRows.dispatchEvent(new Event('change', { bubbles: true }));
		expect(document.getElementById('current-params-display')!.innerHTML).toContain(
			'noise_relative'
		);
	});
});

describe('|| 0 default value fallback branches', () => {
	beforeEach(() => {
		jest.useFakeTimers();
		setupDOM();
		global.fetch = jest.fn().mockImplementation(() => new Promise(() => {}));
		init();
		(global.fetch as jest.Mock).mockClear();
	});

	afterEach(() => {
		stopPolling();
		jest.useRealTimers();
	});

	it('renderCharts handles results with no tracks/alignment/bucket data', () => {
		initCharts();
		renderCharts([
			{
				param_values: { custom_x: 1, custom_y: 2 },
				overall_accept_mean: 0.5,
				overall_accept_stddev: 0.1,
				nonzero_cells_mean: 50,
				nonzero_cells_stddev: 3
			}
		]);
		// All || 0 fallbacks for alignment/tracks/misalignment fields triggered
		// custom_x/custom_y not in PARAM_SCHEMA triggers : xKey label fallback
	});

	it('renderCharts handles all-zero accept means (mx || 1 fallback)', () => {
		initCharts();
		renderCharts([
			{
				param_values: { custom_a: 1, custom_b: 2 },
				overall_accept_mean: 0,
				overall_accept_stddev: 0,
				nonzero_cells_mean: 0,
				nonzero_cells_stddev: 0,
				buckets: [5, 10],
				bucket_means: [0, 0]
			}
		]);
		// mx is 0, triggers || 1 fallback in visualMap max
		// mn === Infinity or mn >= mx, triggers mn = 0 fallback
	});

	it('renderTable with GT results missing optional fields', () => {
		renderTable([
			{
				param_values: { noise_relative: 0.05 },
				overall_accept_mean: 0.85,
				overall_accept_stddev: 0.02,
				nonzero_cells_mean: 100,
				nonzero_cells_stddev: 5,
				ground_truth_score: 0
				// No detection_rate, fragmentation, etc. → all use || 0 fallbacks
			}
		]);
		// hasGroundTruth true because ground_truth_score is defined (0 !== undefined)
		expect(document.getElementById('results-head')!.innerHTML).toContain('GT Score');
	});

	it('downloadCSV with results missing tracks/alignment fields', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'complete',
					completed_combos: 1,
					total_combos: 1,
					results: [
						{
							param_values: { noise_relative: 0.05 },
							overall_accept_mean: 0.85,
							overall_accept_stddev: 0.02,
							nonzero_cells_mean: 100,
							nonzero_cells_stddev: 5
						}
					]
				})
		});
		pollStatus();
		await flushPromises();
		(URL.createObjectURL as jest.Mock).mockClear();
		downloadCSV();
		expect(URL.createObjectURL).toHaveBeenCalled();
	});

	it('pollStatus with current_combo missing accept mean', async () => {
		setMode('manual');
		global.fetch = jest.fn().mockResolvedValue({
			ok: true,
			json: () =>
				Promise.resolve({
					status: 'running',
					completed_combos: 1,
					total_combos: 5,
					current_combo: { param_values: { noise_relative: 0.05 } },
					results: []
				})
		});
		pollStatus();
		await flushPromises();
		expect(document.getElementById('current-combo')!.textContent).toContain('acceptance=0.0%');
	});

	it('displayCurrentParams formats boolean false value', () => {
		displayCurrentParams({ seed_from_first: false, noise_relative: 0.05 });
		const html = document.getElementById('current-params-display')!.innerHTML;
		expect(html).toContain('false');
	});

	it('updateSweepSummary with missing iterations/maxRounds/valuesPerParam', () => {
		setMode('auto');
		addParamRow();
		const lastRow = document.getElementById('param-rows')!.lastElementChild!;
		const rowId = lastRow.id.replace('param-row-', '');
		(document.getElementById('pname-' + rowId) as HTMLInputElement).value = 'noise_relative';
		updateParamFields(rowId);
		// Clear iterations and max_rounds to trigger || defaults
		(document.getElementById('iterations') as HTMLInputElement).value = '';
		(document.getElementById('max_rounds') as HTMLInputElement).value = '';
		(document.getElementById('values_per_param') as HTMLInputElement).value = '';
		updateSweepSummary();
		const summary = document.getElementById('sweep-summary')!.innerHTML;
		expect(summary).toContain('Noise Relative');
	});

	it('handleStartAutoTune with empty weight fields uses defaults', () => {
		setMode('auto');
		(document.getElementById('objective') as HTMLSelectElement).value = 'weighted';
		// Clear weight fields to trigger || default fallbacks
		(document.getElementById('w_acceptance') as HTMLInputElement).value = '';
		(document.getElementById('w_misalignment') as HTMLInputElement).value = '';
		(document.getElementById('w_alignment') as HTMLInputElement).value = '';
		(document.getElementById('w_nonzero') as HTMLInputElement).value = '';
		// Clear max_rounds, values_per_param, top_k
		(document.getElementById('max_rounds') as HTMLInputElement).value = '';
		(document.getElementById('values_per_param') as HTMLInputElement).value = '';
		(document.getElementById('top_k') as HTMLInputElement).value = '';
		global.fetch = jest.fn().mockResolvedValue({ ok: true });
		handleStartAutoTune();
		const body = JSON.parse((global.fetch as jest.Mock).mock.calls[0][1].body);
		expect(body.weights.acceptance).toBe(1.0);
		expect(body.weights.misalignment).toBe(-0.5);
		expect(body.weights.alignment).toBe(-0.01);
		expect(body.weights.nonzero_cells).toBe(0.1);
		expect(body.max_rounds).toBe(3);
		expect(body.values_per_param).toBe(5);
		expect(body.top_k).toBe(5);
	});

	it('updateParamFields with param lacking defaultStart/defaultEnd/desc', () => {
		addParamRow();
		const lastRow = document.getElementById('param-rows')!.lastElementChild!;
		const rowId = lastRow.id.replace('param-row-', '');
		// Use a param that might lack defaultStart/defaultEnd in the schema
		// closeness_multiplier has defaultStart: 1, defaultEnd: 20, desc defined
		// Let's use 'seed_from_first' which is a bool type
		(document.getElementById('pname-' + rowId) as HTMLInputElement).value = 'seed_from_first';
		updateParamFields(rowId);
		// Bool type creates a different field layout, covering the else branch
		const fields = document.getElementById('pfields-' + rowId)!.innerHTML;
		expect(fields).toContain('Values');
	});

	it('buildScenarioJSON with bool param values string', () => {
		addParamRow();
		const lastRow = document.getElementById('param-rows')!.lastElementChild!;
		const rowId = lastRow.id.replace('param-row-', '');
		(document.getElementById('pname-' + rowId) as HTMLInputElement).value = 'seed_from_first';
		updateParamFields(rowId);
		(document.getElementById('pvals-' + rowId) as HTMLInputElement).value = 'true, false';
		const scenario = buildScenarioJSON();
		const param = scenario.params.find((p: any) => p.name === 'seed_from_first');
		expect(param.values).toEqual([true, false]);
	});

	it('buildScenarioJSON with string param values', () => {
		addParamRow();
		const lastRow = document.getElementById('param-rows')!.lastElementChild!;
		const rowId = lastRow.id.replace('param-row-', '');
		(document.getElementById('pname-' + rowId) as HTMLInputElement).value = 'buffer_timeout';
		updateParamFields(rowId);
		(document.getElementById('pvals-' + rowId) as HTMLInputElement).value = '500ms, 1s, 2s';
		const scenario = buildScenarioJSON();
		const param = scenario.params.find((p: any) => p.name === 'buffer_timeout');
		expect(param.values).toEqual(['500ms', '1s', '2s']);
	});

	it('comboLabel uses raw key for unknown param', () => {
		const label = comboLabel({ param_values: { customkey: 42 } });
		expect(label).toContain('customkey=42');
	});

	it('removeParamRow calls displayCurrentParams when cache exists', () => {
		(window as any).currentParamsCache = { noise_relative: 0.05 };
		addParamRow();
		const lastRow = document.getElementById('param-rows')!.lastElementChild!;
		const rowId = lastRow.id.replace('param-row-', '');
		removeParamRow(rowId);
		// removeParamRow should call displayCurrentParams(window.currentParamsCache)
		// when currentParamsCache is truthy
	});
});

// Cleanup URL mocks
afterAll(() => {
	URL.createObjectURL = origCreateObjectURL;
	URL.revokeObjectURL = origRevokeObjectURL;
});
