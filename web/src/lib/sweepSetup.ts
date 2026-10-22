/**
 * The sweep form's model: the sweepable parameters, a parameter row, and the
 * three start requests the sweep, auto-tune and HINT endpoints take. Ported
 * from the :8081 sweep dashboard so the requests it sends are unchanged.
 */

export type ParamType = 'float64' | 'int' | 'int64' | 'bool' | 'string';

export interface ParamSchema {
	type: ParamType;
	label: string;
	desc: string;
	step?: number;
	defaultStart?: number;
	defaultEnd?: number;
}

/** The parameters a sweep can vary, keyed by their tuning-config path. */
export const PARAM_SCHEMA: Record<string, ParamSchema> = {
	'l3.ema_baseline_v1.noise_relative': {
		type: 'float64',
		label: 'Noise Relative',
		step: 0.001,
		defaultStart: 0.01,
		defaultEnd: 0.2,
		desc: 'Fraction of measured range treated as noise threshold (0–1). Higher = more tolerant of range variation.'
	},
	'l3.ema_baseline_v1.closeness_multiplier': {
		type: 'float64',
		label: 'Closeness Multiplier',
		step: 0.5,
		defaultStart: 1.0,
		defaultEnd: 20.0,
		desc: 'Multiplier for closeness threshold. Higher = wider band for background acceptance.'
	},
	'l3.ema_baseline_v1.neighbour_confirmation_count': {
		type: 'int',
		label: 'Neighbour Confirmation',
		step: 1,
		defaultStart: 0,
		defaultEnd: 8,
		desc: 'Number of neighbouring cells (0–8) that must agree before marking foreground.'
	},
	'l3.ema_baseline_v1.seed_from_first': {
		type: 'bool',
		label: 'Seed From First',
		desc: 'If true, initialise background model from the very first observation.'
	},
	'l3.ema_baseline_v1.warmup_duration_nanos': {
		type: 'int64',
		label: 'Warmup Duration (ns)',
		step: 1000000000,
		defaultStart: 5000000000,
		defaultEnd: 120000000000,
		desc: 'Duration in nanoseconds for the warmup phase before classification begins.'
	},
	'l3.ema_baseline_v1.warmup_min_frames': {
		type: 'int',
		label: 'Warmup Min Frames',
		step: 10,
		defaultStart: 10,
		defaultEnd: 500,
		desc: 'Minimum frames required before the warmup phase can complete.'
	},
	'l3.ema_baseline_v1.post_settle_update_fraction': {
		type: 'float64',
		label: 'Post-Settle Update Fraction',
		step: 0.01,
		defaultStart: 0,
		defaultEnd: 0.5,
		desc: 'Background update alpha after settling (0 = freeze background).'
	},
	'l3.ema_baseline_v1.background_update_fraction': {
		type: 'float64',
		label: 'Background Update Fraction',
		step: 0.005,
		defaultStart: 0.005,
		defaultEnd: 0.1,
		desc: 'EMA learning rate for background grid updates during settling (0–1).'
	},
	'l3.ema_baseline_v1.safety_margin_metres': {
		type: 'float64',
		label: 'Safety Margin (m)',
		step: 0.1,
		defaultStart: 0,
		defaultEnd: 2.0,
		desc: 'Additional additive margin (metres) on the closeness threshold for background classification.'
	},
	'l3.ema_baseline_v1.enable_diagnostics': {
		type: 'bool',
		label: 'Enable Diagnostics',
		desc: 'If true, enables verbose per-frame diagnostic logging.'
	},
	'l4.dbscan_xy_v1.foreground_min_cluster_points': {
		type: 'int',
		label: 'FG Min Cluster Points',
		step: 1,
		defaultStart: 0,
		defaultEnd: 20,
		desc: 'Minimum points for a foreground cluster to be reported. 0 = disabled.'
	},
	'l4.dbscan_xy_v1.foreground_dbscan_eps': {
		type: 'float64',
		label: 'FG DBSCAN Eps',
		step: 0.1,
		defaultStart: 0,
		defaultEnd: 2.0,
		desc: 'DBSCAN epsilon for foreground point clustering. 0 = disabled.'
	},
	'pipeline.buffer_timeout': {
		type: 'string',
		label: 'Buffer Timeout',
		desc: 'Max wait time for a complete frame (Go duration, e.g. "500ms").'
	},
	'pipeline.min_frame_points': {
		type: 'int',
		label: 'Min Frame Points',
		step: 100,
		defaultStart: 100,
		defaultEnd: 5000,
		desc: 'Minimum points in a frame before processing. Fewer points = frame dropped.'
	},
	'pipeline.flush_interval': {
		type: 'string',
		label: 'Flush Interval',
		desc: 'How often the background grid is flushed to disk (e.g. "60s").'
	},
	'pipeline.background_flush': {
		type: 'bool',
		label: 'Background Flush',
		desc: 'If true, enables periodic background grid flush to disk. Default: false (disabled).'
	},
	'l5.cv_kf_v1.gating_distance_squared': {
		type: 'float64',
		label: 'Gating Distance²',
		step: 1.0,
		defaultStart: 4.0,
		defaultEnd: 100.0,
		desc: 'Squared Mahalanobis distance threshold for track-to-cluster association.'
	},
	'l5.cv_kf_v1.process_noise_pos': {
		type: 'float64',
		label: 'Process Noise Pos',
		step: 0.01,
		defaultStart: 0.01,
		defaultEnd: 1.0,
		desc: 'Kalman process noise for position. Higher = more position uncertainty expected.'
	},
	'l5.cv_kf_v1.process_noise_vel': {
		type: 'float64',
		label: 'Process Noise Vel',
		step: 0.01,
		defaultStart: 0.05,
		defaultEnd: 2.0,
		desc: 'Kalman process noise for velocity. Higher = more velocity changes expected.'
	},
	'l5.cv_kf_v1.measurement_noise': {
		type: 'float64',
		label: 'Measurement Noise',
		step: 0.01,
		defaultStart: 0.01,
		defaultEnd: 2.0,
		desc: 'Kalman measurement noise. Higher = less trust in observations.'
	},
	occlusion_cov_inflation: {
		type: 'float64',
		label: 'Occlusion Cov Inflation',
		step: 0.1,
		defaultStart: 0.1,
		defaultEnd: 5.0,
		desc: 'Covariance inflation factor during occlusion (missed observations).'
	},
	'l5.cv_kf_v1.hits_to_confirm': {
		type: 'int',
		label: 'Hits to Confirm',
		step: 1,
		defaultStart: 1,
		defaultEnd: 10,
		desc: 'Consecutive successful associations needed to confirm a tentative track.'
	},
	'l5.cv_kf_v1.max_misses': {
		type: 'int',
		label: 'Max Misses',
		step: 1,
		defaultStart: 1,
		defaultEnd: 10,
		desc: 'Consecutive misses before a tentative track is deleted.'
	},
	'l5.cv_kf_v1.max_misses_confirmed': {
		type: 'int',
		label: 'Max Misses Confirmed',
		step: 1,
		defaultStart: 3,
		defaultEnd: 30,
		desc: 'Consecutive misses before a confirmed track is deleted.'
	},
	'l5.cv_kf_v1.max_tracks': {
		type: 'int',
		label: 'Max Tracks',
		step: 10,
		defaultStart: 10,
		defaultEnd: 500,
		desc: 'Maximum number of simultaneous tracks the tracker maintains.'
	},
	speed_ratio: {
		type: 'float64',
		label: 'Speed Ratio',
		step: 0.25,
		defaultStart: 0.25,
		defaultEnd: 1.5,
		desc: 'PCAP replay speed multiplier (e.g. 0.5 = half speed, 1.0 = realtime, 1.5 = 1.5x). Requires PCAP mode.'
	}
};

export const PARAM_NAMES = Object.keys(PARAM_SCHEMA);

/** HINT's default parameters when the form names none. */
export const DEFAULT_HINT_FOREGROUND_PARAMS = [
	'l4.dbscan_xy_v1.foreground_min_cluster_points',
	'l4.dbscan_xy_v1.foreground_dbscan_eps'
];
export const DEFAULT_HINT_BACKGROUND_PARAMS = [
	'l3.ema_baseline_v1.noise_relative',
	'l3.ema_baseline_v1.closeness_multiplier',
	'l3.ema_baseline_v1.background_update_fraction',
	'l3.ema_baseline_v1.safety_margin_metres'
];

// A short key (the last path segment) names a parameter when no other
// parameter shares it, as older results and recommendations use.
const SUFFIX_MAP: Record<string, string> = (() => {
	const counts: Record<string, number> = {};
	for (const key of PARAM_NAMES) {
		const suffix = key.split('.').pop() as string;
		counts[suffix] = (counts[suffix] ?? 0) + 1;
	}
	const map: Record<string, string> = {};
	for (const key of PARAM_NAMES) {
		const suffix = key.split('.').pop() as string;
		if (counts[suffix] === 1) map[suffix] = key;
	}
	return map;
})();

/** The schema key for a parameter named in full or by its unique suffix. */
export function canonicalParamKey(key: string): string {
	if (PARAM_SCHEMA[key]) return key;
	if (!key.includes('.') && SUFFIX_MAP[key]) return SUFFIX_MAP[key];
	return key;
}

function isObject(value: unknown): value is Record<string, unknown> {
	return !!value && typeof value === 'object' && !Array.isArray(value);
}

/** Flatten a nested parameter object to canonical dotted keys. */
export function normaliseParamMap(params: unknown): Record<string, unknown> {
	const flat: Record<string, unknown> = {};
	const walk = (prefix: string, value: unknown) => {
		if (isObject(value)) {
			for (const [k, v] of Object.entries(value)) walk(prefix ? `${prefix}.${k}` : k, v);
			return;
		}
		if (prefix) flat[canonicalParamKey(prefix)] = value;
	};
	if (isObject(params)) walk('', params);
	return flat;
}

/** The label a parameter is shown under. */
export function paramLabel(key: string): string {
	return PARAM_SCHEMA[canonicalParamKey(key)]?.label ?? key;
}

/** Parse a Go duration ("5s", "500ms", "1m30s") to seconds. */
export function parseDuration(s: string | undefined): number {
	if (!s) return 0;
	let total = 0;
	for (const m of s.matchAll(/(\d+(?:\.\d+)?)(ms|s|m|h)/g)) {
		const v = parseFloat(m[1]);
		total += m[2] === 'h' ? v * 3600 : m[2] === 'm' ? v * 60 : m[2] === 's' ? v : v / 1000;
	}
	return total;
}

/** Format seconds as "5s", "1m 30s" or "1h 5m". */
export function formatSeconds(secs: number): string {
	const total = Math.round(secs);
	if (total < 60) return `${total}s`;
	if (total < 3600) {
		const s = total % 60;
		return s > 0 ? `${Math.floor(total / 60)}m ${s}s` : `${Math.floor(total / 60)}m`;
	}
	const m = Math.floor((total % 3600) / 60);
	return m > 0 ? `${Math.floor(total / 3600)}h ${m}m` : `${Math.floor(total / 3600)}h`;
}

/** One parameter row of the form. Values, when given, override the range. */
export interface ParamRow {
	name: string;
	start: number;
	end: number;
	step: number;
	values: string;
}

/** A new row for a parameter, at the schema's default range. */
export function newParamRow(name = ''): ParamRow {
	const schema = PARAM_SCHEMA[name];
	return {
		name,
		start: schema?.defaultStart ?? 0,
		end: schema?.defaultEnd ?? 1,
		step: schema?.step ?? 1,
		values: schema?.type === 'bool' ? 'true, false' : ''
	};
}

function splitValues(values: string): string[] {
	return values
		.split(',')
		.map((v) => v.trim())
		.filter((v) => v !== '');
}

function isNumeric(type: ParamType | undefined): boolean {
	return type === 'float64' || type === 'int' || type === 'int64';
}

/** How many values a manual sweep tries for a row. */
export function rowValueCount(row: ParamRow): number {
	const schema = PARAM_SCHEMA[row.name];
	if (!schema) return 0;
	const listed = splitValues(row.values);
	if (listed.length > 0) return listed.length;
	if (schema.type === 'bool') return 2;
	if (isNumeric(schema.type) && row.step > 0 && row.end >= row.start) {
		return Math.floor((row.end - row.start) / row.step) + 1;
	}
	return 0;
}

export type SweepMode = 'manual' | 'auto' | 'hint';
export type DataSource = 'live' | 'pcap' | 'replay_case';

/** Every field of the sweep form. */
export interface SweepForm {
	mode: SweepMode;
	rows: ParamRow[];
	dataSource: DataSource;
	replayCaseId: string;
	pcapFile: string;
	pcapStartSecs: number;
	pcapDurationSecs: number;
	seed: 'true' | 'false' | 'toggle';
	iterations: number;
	interval: string;
	settleTime: string;
	settleMode: 'once' | 'per_combo';
	// Auto-tune
	maxRounds: number;
	valuesPerParam: number;
	topK: number;
	objective: 'acceptance' | 'weighted' | 'ground_truth';
	weights: Record<WeightKey, number>;
	acceptance: { maxFragmentation: string; maxUnbounded: string; maxEmptyBoxes: string };
	// HINT
	hintRounds: number;
	hintThresholdPct: number;
	hintCarryOver: boolean;
	hintTuneBackground: boolean;
	hintClassCoverage: string;
	hintTemporalSpreadSecs: string;
}

export type WeightKey =
	| 'acceptance'
	| 'misalignment'
	| 'alignment'
	| 'nonzero_cells'
	| 'active_tracks'
	| 'foreground_capture'
	| 'empty_boxes'
	| 'fragmentation'
	| 'heading_jitter';

export const WEIGHT_FIELDS: Array<{ key: WeightKey; label: string; step: number }> = [
	{ key: 'acceptance', label: 'Accept weight', step: 0.1 },
	{ key: 'misalignment', label: 'Misalign weight', step: 0.1 },
	{ key: 'alignment', label: 'Align weight', step: 0.01 },
	{ key: 'nonzero_cells', label: 'Cells weight', step: 0.1 },
	{ key: 'active_tracks', label: 'Tracks weight', step: 0.1 },
	{ key: 'foreground_capture', label: 'Capture weight', step: 0.1 },
	{ key: 'empty_boxes', label: 'Empty box weight', step: 0.1 },
	{ key: 'fragmentation', label: 'Fragment weight', step: 0.1 },
	{ key: 'heading_jitter', label: 'Jitter weight', step: 0.01 }
];

export function defaultSweepForm(): SweepForm {
	return {
		mode: 'manual',
		rows: [],
		dataSource: 'live',
		replayCaseId: '',
		pcapFile: '',
		pcapStartSecs: 0,
		pcapDurationSecs: -1,
		seed: 'true',
		iterations: 10,
		interval: '2s',
		settleTime: '5s',
		settleMode: 'once',
		maxRounds: 3,
		valuesPerParam: 5,
		topK: 5,
		objective: 'acceptance',
		weights: {
			acceptance: 1.0,
			misalignment: -0.5,
			alignment: -0.01,
			nonzero_cells: 0.1,
			active_tracks: 0.3,
			foreground_capture: 0,
			empty_boxes: 0,
			fragmentation: 0,
			heading_jitter: 0
		},
		acceptance: { maxFragmentation: '', maxUnbounded: '', maxEmptyBoxes: '' },
		hintRounds: 3,
		hintThresholdPct: 50,
		hintCarryOver: true,
		hintTuneBackground: false,
		hintClassCoverage: '',
		hintTemporalSpreadSecs: ''
	};
}

/** The data-source fields every request shares. */
function sourceFields(form: SweepForm): Record<string, unknown> {
	const out: Record<string, unknown> = {
		data_source: form.dataSource === 'replay_case' ? 'pcap' : form.dataSource
	};
	if (form.dataSource === 'pcap' || form.dataSource === 'replay_case') {
		out.pcap_file = form.pcapFile;
		out.pcap_start_secs = form.pcapStartSecs;
		out.pcap_duration_secs = form.pcapDurationSecs;
	}
	if (form.dataSource === 'replay_case' && form.replayCaseId) {
		out.replay_case_id = form.replayCaseId;
	}
	return out;
}

/** One parameter of a manual sweep: listed values, or a range. */
function manualParam(row: ParamRow): Record<string, unknown> {
	const schema = PARAM_SCHEMA[row.name];
	const p: Record<string, unknown> = { name: row.name, type: schema.type };
	const listed = splitValues(row.values);
	if (listed.length > 0) {
		if (schema.type === 'float64') p.values = listed.map((v) => parseFloat(v));
		else if (schema.type === 'int' || schema.type === 'int64')
			p.values = listed.map((v) => parseInt(v, 10));
		else if (schema.type === 'bool') p.values = listed.map((v) => v.toLowerCase() === 'true');
		else p.values = listed;
	} else if (isNumeric(schema.type)) {
		p.start = row.start;
		p.end = row.end;
		p.step = row.step;
	}
	return p;
}

/** The rows that name a known parameter. */
function namedRows(form: SweepForm): ParamRow[] {
	return form.rows.filter((r) => PARAM_SCHEMA[r.name]);
}

/** A ranged parameter for auto-tune and HINT: start and end only. */
function rangedParam(row: ParamRow): Record<string, unknown> {
	return { name: row.name, type: PARAM_SCHEMA[row.name].type, start: row.start, end: row.end };
}

/** The setup a manual sweep starts from; also what the JSON editor shows. */
export function buildSweepSetup(form: SweepForm): Record<string, unknown> {
	return {
		seed: form.seed,
		iterations: form.iterations,
		interval: form.interval,
		settle_time: form.settleTime,
		settle_mode: form.settleMode,
		...sourceFields(form),
		params: namedRows(form).map(manualParam)
	};
}

/** Load a setup (from the JSON editor or a file) back into the form. */
export function applySweepSetup(form: SweepForm, setup: Record<string, unknown>): SweepForm {
	const next: SweepForm = { ...form };
	if (typeof setup.seed === 'string') next.seed = setup.seed as SweepForm['seed'];
	if (typeof setup.iterations === 'number') next.iterations = setup.iterations;
	if (typeof setup.interval === 'string') next.interval = setup.interval;
	if (typeof setup.settle_time === 'string') next.settleTime = setup.settle_time;
	if (typeof setup.settle_mode === 'string')
		next.settleMode = setup.settle_mode as SweepForm['settleMode'];
	if (typeof setup.replay_case_id === 'string' && setup.replay_case_id) {
		next.dataSource = 'replay_case';
		next.replayCaseId = setup.replay_case_id;
	} else if (setup.data_source === 'live' || setup.data_source === 'pcap') {
		next.dataSource = setup.data_source;
	}
	if (typeof setup.pcap_file === 'string') next.pcapFile = setup.pcap_file;
	if (typeof setup.pcap_start_secs === 'number') next.pcapStartSecs = setup.pcap_start_secs;
	if (typeof setup.pcap_duration_secs === 'number')
		next.pcapDurationSecs = setup.pcap_duration_secs;
	if (Array.isArray(setup.params)) {
		next.rows = setup.params.filter(isObject).map((p) => {
			const row = newParamRow(String(p.name ?? ''));
			if (Array.isArray(p.values) && p.values.length > 0) row.values = p.values.join(', ');
			else {
				if (typeof p.start === 'number') row.start = p.start;
				if (typeof p.end === 'number') row.end = p.end;
				if (typeof p.step === 'number') row.step = p.step;
				if (PARAM_SCHEMA[row.name]?.type !== 'bool') row.values = '';
			}
			return row;
		});
	}
	return next;
}

/** POST /api/lidar/sweep/start. */
export function buildManualRequest(form: SweepForm): Record<string, unknown> {
	return { ...buildSweepSetup(form), mode: 'params' };
}

/** POST /api/lidar/sweep/auto. */
export function buildAutoTuneRequest(form: SweepForm): Record<string, unknown> {
	const req: Record<string, unknown> = {
		params: namedRows(form)
			.filter((r) => isNumeric(PARAM_SCHEMA[r.name].type))
			.map(rangedParam),
		max_rounds: form.maxRounds || 3,
		values_per_param: form.valuesPerParam || 5,
		top_k: form.topK || 5,
		objective: form.objective,
		iterations: form.iterations,
		interval: form.interval,
		settle_time: form.settleTime,
		seed: form.seed,
		settle_mode: form.settleMode,
		...sourceFields(form)
	};
	if (form.objective === 'weighted') {
		req.weights = { ...form.weights };
		const ac: Record<string, number> = {};
		const { maxFragmentation, maxUnbounded, maxEmptyBoxes } = form.acceptance;
		if (maxFragmentation !== '') ac.max_fragmentation_ratio = parseFloat(maxFragmentation);
		if (maxUnbounded !== '') ac.max_unbounded_point_ratio = parseFloat(maxUnbounded);
		if (maxEmptyBoxes !== '') ac.max_empty_box_ratio = parseFloat(maxEmptyBoxes);
		if (Object.keys(ac).length > 0) req.acceptance_criteria = ac;
	}
	return req;
}

/**
 * POST /api/lidar/sweep/hint. With no parameter rows it sweeps HINT's
 * defaults, foreground only unless background tuning is on. Throws on an
 * unparseable class-coverage gate.
 */
export function buildHintRequest(form: SweepForm): Record<string, unknown> {
	let params = namedRows(form)
		.filter((r) => isNumeric(PARAM_SCHEMA[r.name].type))
		.map(rangedParam);
	if (params.length === 0) {
		const defaults = form.hintTuneBackground
			? [...DEFAULT_HINT_FOREGROUND_PARAMS, ...DEFAULT_HINT_BACKGROUND_PARAMS]
			: DEFAULT_HINT_FOREGROUND_PARAMS;
		params = defaults.map((name) => rangedParam(newParamRow(name)));
	}
	const req: Record<string, unknown> = {
		replay_case_id: form.replayCaseId,
		num_rounds: form.hintRounds || 3,
		params,
		values_per_param: form.valuesPerParam || 5,
		top_k: form.topK || 3,
		min_label_threshold: (form.hintThresholdPct || 90) / 100,
		carry_over_labels: form.hintCarryOver,
		tune_background: form.hintTuneBackground
	};
	if (form.hintClassCoverage.trim()) {
		try {
			req.min_class_coverage = JSON.parse(form.hintClassCoverage);
		} catch (e) {
			throw new Error(`Class coverage is not valid JSON: ${e instanceof Error ? e.message : e}`, {
				cause: e
			});
		}
	}
	const spread = parseFloat(form.hintTemporalSpreadSecs);
	if (spread > 0) req.min_temporal_spread_secs = spread;
	return req;
}

/** The first reason the form cannot start, or null. */
export function validateSweepForm(form: SweepForm): string | null {
	if (form.mode === 'hint') {
		return form.replayCaseId ? null : 'Choose a clip before starting a HINT sweep.';
	}
	if (namedRows(form).length === 0) return 'Add at least one parameter.';
	if ((form.dataSource === 'pcap' || form.dataSource === 'replay_case') && !form.pcapFile.trim()) {
		return 'Choose a capture file to replay.';
	}
	if (form.mode === 'auto' && !namedRows(form).some((r) => isNumeric(PARAM_SCHEMA[r.name].type))) {
		return 'Auto-tune needs at least one numeric parameter.';
	}
	return null;
}

/** Combinations, rounds and an estimated runtime, as the form summarises them. */
export interface SweepEstimate {
	permutations: number;
	withSeed: number;
	rounds: number;
	total: number;
	measurementSecs: number;
	runtimeSecs: number;
}

// Restoring settled regions between combinations takes about two seconds.
const REGION_RESTORE_SECS = 2;

export function estimateSweep(form: SweepForm): SweepEstimate | null {
	const rows = namedRows(form);
	if (rows.length === 0) return null;
	let permutations = 1;
	for (const row of rows) {
		const count = form.mode === 'manual' ? rowValueCount(row) : form.valuesPerParam || 5;
		if (count === 0) continue;
		permutations *= count;
	}
	const withSeed = permutations * (form.seed === 'toggle' ? 2 : 1);
	const settle = parseDuration(form.settleTime);
	const measurementSecs = (form.iterations || 1) * parseDuration(form.interval);
	const perRound =
		form.settleMode === 'once' && withSeed > 1
			? settle + measurementSecs + (withSeed - 1) * (REGION_RESTORE_SECS + measurementSecs)
			: withSeed * (settle + measurementSecs);
	const rounds = form.mode === 'auto' ? form.maxRounds || 3 : 1;
	return {
		permutations,
		withSeed,
		rounds,
		total: withSeed * rounds,
		measurementSecs,
		runtimeSecs: perRound * rounds
	};
}

/**
 * Why a sweep that replays captures cannot start now, or null. Each
 * combination waits for the replay slot, and the slot is held both by a
 * running replay and by a finished analysis replay that kept its grid, until
 * the replay is stopped or live data resumed. The server waits minutes per
 * combination before skipping it, so the form refuses up front instead.
 */
export function replaySlotProblem(
	form: SweepForm,
	status: { data_source: string; pcap_in_progress: boolean }
): string | null {
	const replays = form.mode === 'hint' || form.dataSource !== 'live';
	if (!replays || status.data_source === 'live') return null;
	return status.pcap_in_progress
		? 'A replay is running. Stop it on the Status page before starting a sweep that replays a capture.'
		: `The pipeline is still holding a finished replay (${status.data_source}). Stop it or resume live data on the Status page first.`;
}
