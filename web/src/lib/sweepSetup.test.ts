import {
	applySweepSetup,
	buildAutoTuneRequest,
	buildHintRequest,
	buildManualRequest,
	buildSweepSetup,
	canonicalParamKey,
	defaultSweepForm,
	DEFAULT_HINT_BACKGROUND_PARAMS,
	DEFAULT_HINT_FOREGROUND_PARAMS,
	estimateSweep,
	formatSeconds,
	newParamRow,
	normaliseParamMap,
	paramLabel,
	parseDuration,
	replaySlotProblem,
	rowValueCount,
	validateSweepForm,
	type SweepForm
} from './sweepSetup';

const EPS = 'l4.dbscan_xy_v1.foreground_dbscan_eps';
const POINTS = 'l4.dbscan_xy_v1.foreground_min_cluster_points';
const SEED = 'l3.ema_baseline_v1.seed_from_first';
const TIMEOUT = 'pipeline.buffer_timeout';
const WARMUP = 'l3.ema_baseline_v1.warmup_duration_nanos';

function form(overrides: Partial<SweepForm> = {}): SweepForm {
	return { ...defaultSweepForm(), ...overrides };
}

describe('parameter keys', () => {
	it('resolves unique suffixes and leaves the rest alone', () => {
		expect(canonicalParamKey('foreground_dbscan_eps')).toBe(EPS);
		expect(canonicalParamKey(EPS)).toBe(EPS);
		expect(canonicalParamKey('unknown_key')).toBe('unknown_key');
		expect(canonicalParamKey('a.b')).toBe('a.b');
	});

	it('flattens nested parameter objects', () => {
		expect(
			normaliseParamMap({
				l4: { dbscan_xy_v1: { foreground_dbscan_eps: 0.3 } },
				measurement_noise: 1
			})
		).toEqual({ [EPS]: 0.3, 'l5.cv_kf_v1.measurement_noise': 1 });
		expect(normaliseParamMap(null)).toEqual({});
		expect(normaliseParamMap([1, 2])).toEqual({});
	});

	it('labels known and unknown parameters', () => {
		expect(paramLabel('foreground_dbscan_eps')).toBe('FG DBSCAN Eps');
		expect(paramLabel('mystery')).toBe('mystery');
	});
});

describe('durations', () => {
	it('parses Go durations', () => {
		expect(parseDuration('5s')).toBe(5);
		expect(parseDuration('500ms')).toBe(0.5);
		expect(parseDuration('1m30s')).toBe(90);
		expect(parseDuration('2h')).toBe(7200);
		expect(parseDuration('')).toBe(0);
		expect(parseDuration(undefined)).toBe(0);
	});

	it('formats seconds', () => {
		expect(formatSeconds(5)).toBe('5s');
		expect(formatSeconds(60)).toBe('1m');
		expect(formatSeconds(90)).toBe('1m 30s');
		expect(formatSeconds(3600)).toBe('1h');
		expect(formatSeconds(3900)).toBe('1h 5m');
	});
});

describe('parameter rows', () => {
	it('starts at the schema defaults', () => {
		expect(newParamRow(EPS)).toEqual({ name: EPS, start: 0, end: 2, step: 0.1, values: '' });
		expect(newParamRow(SEED).values).toBe('true, false');
		expect(newParamRow()).toEqual({ name: '', start: 0, end: 1, step: 1, values: '' });
	});

	it('counts values from a list, a range, or a bool', () => {
		expect(rowValueCount({ ...newParamRow(EPS), values: '0.1, 0.2,, 0.3' })).toBe(3);
		expect(rowValueCount({ ...newParamRow(EPS), start: 0, end: 1, step: 0.25 })).toBe(5);
		expect(rowValueCount({ ...newParamRow(SEED), values: '' })).toBe(2);
		expect(rowValueCount({ ...newParamRow(EPS), start: 2, end: 1 })).toBe(0);
		expect(rowValueCount({ ...newParamRow(TIMEOUT), values: '' })).toBe(0);
		expect(rowValueCount(newParamRow('nope'))).toBe(0);
	});
});

describe('manual sweep request', () => {
	it('sends ranges, typed value lists and the source', () => {
		const f = form({
			rows: [
				{ ...newParamRow(EPS), start: 0.1, end: 0.5, step: 0.2 },
				{ ...newParamRow(POINTS), values: '3, 5' },
				{ ...newParamRow(SEED), values: 'true, FALSE' },
				{ ...newParamRow(TIMEOUT), values: '500ms, 1s' },
				newParamRow('')
			],
			dataSource: 'replay_case',
			replayCaseId: 'clip-1',
			pcapFile: 'kirk0.pcapng',
			pcapStartSecs: 4,
			pcapDurationSecs: 30
		});
		const req = buildManualRequest(f);
		expect(req).toMatchObject({
			mode: 'params',
			seed: 'true',
			data_source: 'pcap',
			replay_case_id: 'clip-1',
			pcap_file: 'kirk0.pcapng',
			pcap_start_secs: 4,
			pcap_duration_secs: 30
		});
		expect(req.params).toEqual([
			{ name: EPS, type: 'float64', start: 0.1, end: 0.5, step: 0.2 },
			{ name: POINTS, type: 'int', values: [3, 5] },
			{ name: SEED, type: 'bool', values: [true, false] },
			{ name: TIMEOUT, type: 'string', values: ['500ms', '1s'] }
		]);
	});

	it('omits capture fields for live data', () => {
		const req = buildSweepSetup(form({ rows: [newParamRow(EPS)] }));
		expect(req.data_source).toBe('live');
		expect(req).not.toHaveProperty('pcap_file');
		expect(req).not.toHaveProperty('replay_case_id');
	});

	it('round-trips a setup through the JSON editor', () => {
		const original = form({
			rows: [
				{ ...newParamRow(EPS), start: 0.2, end: 0.4, step: 0.1 },
				{ ...newParamRow(POINTS), values: '2, 4' }
			],
			dataSource: 'pcap',
			pcapFile: 'a.pcap',
			seed: 'toggle',
			settleMode: 'per_combo'
		});
		const loaded = applySweepSetup(defaultSweepForm(), buildSweepSetup(original));
		expect(buildSweepSetup(loaded)).toEqual(buildSweepSetup(original));
		expect(applySweepSetup(defaultSweepForm(), { replay_case_id: 'c' })).toMatchObject({
			dataSource: 'replay_case',
			replayCaseId: 'c'
		});
		expect(applySweepSetup(defaultSweepForm(), { params: [{ name: SEED }] }).rows[0].values).toBe(
			'true, false'
		);
		expect(applySweepSetup(defaultSweepForm(), {})).toEqual(defaultSweepForm());
	});
});

describe('auto-tune request', () => {
	it('sends numeric ranges and the objective', () => {
		const req = buildAutoTuneRequest(
			form({
				mode: 'auto',
				rows: [newParamRow(EPS), newParamRow(SEED)],
				dataSource: 'pcap',
				pcapFile: 'a'
			})
		);
		expect(req.params).toEqual([{ name: EPS, type: 'float64', start: 0, end: 2 }]);
		expect(req).toMatchObject({
			objective: 'acceptance',
			max_rounds: 3,
			values_per_param: 5,
			top_k: 5
		});
		expect(req).not.toHaveProperty('weights');
	});

	it('adds weights and only the acceptance criteria given', () => {
		const f = form({ mode: 'auto', rows: [newParamRow(EPS)], objective: 'weighted' });
		expect(buildAutoTuneRequest(f).weights).toMatchObject({ acceptance: 1, misalignment: -0.5 });
		expect(buildAutoTuneRequest(f)).not.toHaveProperty('acceptance_criteria');
		f.acceptance = { maxFragmentation: '0.2', maxUnbounded: '', maxEmptyBoxes: '0.1' };
		expect(buildAutoTuneRequest(f).acceptance_criteria).toEqual({
			max_fragmentation_ratio: 0.2,
			max_empty_box_ratio: 0.1
		});
		f.acceptance = { maxFragmentation: '', maxUnbounded: '0.3', maxEmptyBoxes: '' };
		expect(buildAutoTuneRequest(f).acceptance_criteria).toEqual({ max_unbounded_point_ratio: 0.3 });
	});
});

describe('HINT request', () => {
	it('falls back to the foreground defaults, adding background when asked', () => {
		const names = (f: SweepForm) =>
			(buildHintRequest(f).params as Array<{ name: string }>).map((p) => p.name);
		expect(names(form({ mode: 'hint', replayCaseId: 'c' }))).toEqual(
			DEFAULT_HINT_FOREGROUND_PARAMS
		);
		expect(names(form({ mode: 'hint', replayCaseId: 'c', hintTuneBackground: true }))).toEqual([
			...DEFAULT_HINT_FOREGROUND_PARAMS,
			...DEFAULT_HINT_BACKGROUND_PARAMS
		]);
		expect(names(form({ mode: 'hint', replayCaseId: 'c', rows: [newParamRow(WARMUP)] }))).toEqual([
			WARMUP
		]);
	});

	it('sends the gates', () => {
		const req = buildHintRequest(
			form({
				mode: 'hint',
				replayCaseId: 'c',
				hintThresholdPct: 75,
				hintClassCoverage: '{"vehicle":3}',
				hintTemporalSpreadSecs: '20'
			})
		);
		expect(req).toMatchObject({
			replay_case_id: 'c',
			num_rounds: 3,
			min_label_threshold: 0.75,
			carry_over_labels: true,
			tune_background: false,
			min_class_coverage: { vehicle: 3 },
			min_temporal_spread_secs: 20
		});
		const bare = buildHintRequest(form({ replayCaseId: 'c', hintThresholdPct: 0, topK: 0 }));
		expect(bare.min_label_threshold).toBe(0.9);
		expect(bare.top_k).toBe(3);
		expect(bare).not.toHaveProperty('min_class_coverage');
		expect(bare).not.toHaveProperty('min_temporal_spread_secs');
	});

	it('refuses an unparseable class coverage', () => {
		expect(() =>
			buildHintRequest(form({ replayCaseId: 'c', hintClassCoverage: '{vehicle' }))
		).toThrow(/Class coverage is not valid JSON/);
	});
});

describe('validateSweepForm', () => {
	it('names the first problem', () => {
		expect(validateSweepForm(form({ mode: 'hint' }))).toMatch(/clip/);
		expect(validateSweepForm(form({ mode: 'hint', replayCaseId: 'c' }))).toBeNull();
		expect(validateSweepForm(form())).toMatch(/at least one parameter/);
		expect(validateSweepForm(form({ rows: [newParamRow(EPS)], dataSource: 'pcap' }))).toMatch(
			/capture file/
		);
		expect(validateSweepForm(form({ mode: 'auto', rows: [newParamRow(SEED)] }))).toMatch(/numeric/);
		expect(validateSweepForm(form({ mode: 'auto', rows: [newParamRow(EPS)] }))).toBeNull();
	});
});

describe('estimateSweep', () => {
	it('returns null with no parameters', () => {
		expect(estimateSweep(form())).toBeNull();
	});

	it('estimates a manual sweep that settles once', () => {
		const est = estimateSweep(
			form({
				rows: [{ ...newParamRow(POINTS), values: '1, 2, 3' }, newParamRow(TIMEOUT)],
				seed: 'toggle',
				iterations: 2,
				interval: '2s',
				settleTime: '10s'
			})
		);
		// The string row with no values counts nothing and is skipped.
		expect(est).toEqual({
			permutations: 3,
			withSeed: 6,
			rounds: 1,
			total: 6,
			measurementSecs: 4,
			runtimeSecs: 10 + 4 + 5 * (2 + 4)
		});
	});

	it('estimates auto-tune per combination and per round', () => {
		const est = estimateSweep(
			form({
				mode: 'auto',
				rows: [newParamRow(EPS), newParamRow(POINTS)],
				valuesPerParam: 3,
				maxRounds: 2,
				settleMode: 'per_combo',
				iterations: 1,
				interval: '1s',
				settleTime: '1s'
			})
		);
		expect(est).toMatchObject({
			permutations: 9,
			withSeed: 9,
			rounds: 2,
			total: 18,
			runtimeSecs: 36
		});
		expect(
			estimateSweep(
				form({
					mode: 'auto',
					rows: [newParamRow(EPS)],
					valuesPerParam: 0,
					maxRounds: 0,
					iterations: 0
				})
			)
		).toMatchObject({ permutations: 5, rounds: 3 });
	});
});

describe('replaySlotProblem', () => {
	const live = { data_source: 'live', pcap_in_progress: false };
	const held = { data_source: 'pcap_analysis', pcap_in_progress: false };
	const busy = { data_source: 'pcap', pcap_in_progress: true };

	it('allows a live sweep whatever the pipeline is doing', () => {
		expect(replaySlotProblem(form(), held)).toBeNull();
	});

	it('refuses a replaying sweep while the slot is held', () => {
		expect(replaySlotProblem(form({ dataSource: 'pcap' }), live)).toBeNull();
		expect(replaySlotProblem(form({ dataSource: 'pcap' }), busy)).toMatch(/replay is running/);
		expect(replaySlotProblem(form({ dataSource: 'replay_case' }), held)).toMatch(/pcap_analysis/);
		expect(replaySlotProblem(form({ mode: 'hint' }), held)).toMatch(/finished replay/);
	});
});
