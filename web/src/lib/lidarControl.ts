/**
 * Client for the LiDAR control and monitoring endpoints that the retired
 * :8081 dashboards used to call: replay and recording, background grid and
 * snapshot maintenance, the debug chart JSON, the background regions, and
 * sweep, auto-tune and HINT control. Every route is on the main :8080 server.
 */

import type { LidarReplayCase } from './types/lidar.js';

const API_BASE = '/api';

/** The sensor the single-sensor deployments run. */
export const DEFAULT_LIDAR_SENSOR_ID = 'hesai-pandar40p';

interface RequestOptions {
	method?: 'GET' | 'POST' | 'PUT';
	query?: Record<string, string | number | undefined>;
	body?: unknown;
	signal?: AbortSignal;
}

/** The message a failed response carries: its JSON `error`, else its text. */
async function errorMessage(res: Response, action: string): Promise<string> {
	const text = await res.text().catch(() => '');
	try {
		const parsed = JSON.parse(text);
		if (parsed && typeof parsed.error === 'string') return parsed.error;
	} catch {
		// Not JSON: fall through to the raw text.
	}
	return text.trim() || `Could not ${action}: ${res.status}`;
}

/** Build a relative API path with its query string. */
export function apiPath(path: string, query: RequestOptions['query'] = {}): string {
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(query)) {
		if (value !== undefined && value !== '') params.append(key, String(value));
	}
	const qs = params.toString();
	return `${API_BASE}${path}${qs ? `?${qs}` : ''}`;
}

async function request<T>(path: string, action: string, opts: RequestOptions = {}): Promise<T> {
	const init: RequestInit = { method: opts.method ?? 'GET', signal: opts.signal };
	if (opts.body !== undefined) {
		init.headers = { 'Content-Type': 'application/json' };
		init.body = JSON.stringify(opts.body);
	}
	const res = await fetch(apiPath(path, opts.query), init);
	if (!res.ok) throw new Error(await errorMessage(res, action));
	const text = await res.text();
	return (text ? JSON.parse(text) : {}) as T;
}

// ─── Status and replay ──────────────────────────────────────────────────────

/** Packet statistics. The Go struct has no JSON tags, so keys are Go names. */
export interface PacketStats {
	PacketsPerSec: number;
	MBPerSec: number;
	PointsPerSec: number;
	DroppedCount: number;
	Timestamp: string;
	ParseEnabled: boolean;
}

/** GET /api/lidar/status. */
export interface LidarStatus {
	status: string;
	sensor_id: string;
	udp_port: number;
	forwarding_enabled: boolean;
	forward_addr?: string;
	forward_port?: number;
	parsing_enabled: boolean;
	data_source: string;
	pcap_file?: string;
	pcap_in_progress: boolean;
	uptime: string;
	stats?: PacketStats;
	pcap_safe_dir?: string;
	tuning_config?: string;
	tuning_fingerprint?: string;
}

export function getLidarStatus(): Promise<LidarStatus> {
	return request('/lidar/status', 'load LiDAR status');
}

/** GET /api/lidar/playback/status. */
export interface PlaybackStatus {
	mode: string;
	paused: boolean;
	rate: number;
	seekable: boolean;
	current_frame: number;
	total_frames: number;
	timestamp_ns: number;
	log_start_ns: number;
	log_end_ns: number;
	vrlog_path?: string;
	replay_active: boolean;
	replay_pass: string;
	replay_total_passes: number;
	grid_preserved: boolean;
	replay_epoch: number;
	recording: boolean;
	recording_path?: string;
	recording_run_id?: string;
	recording_frames: number;
}

export function getPlaybackStatus(): Promise<PlaybackStatus> {
	return request('/lidar/playback/status', 'load playback status');
}

/** One file under the server's tuning config directory. */
export interface TuningConfigInfo {
	path: string;
	kind: string;
	error?: string;
	fingerprint?: string;
	profile?: string;
	engines?: string;
	solid_body?: string;
	applicable: boolean;
	reason?: string;
	active: boolean;
}

export interface TuningConfigList {
	config_dir: string;
	configs: TuningConfigInfo[];
	count: number;
}

export function getTuningConfigs(): Promise<TuningConfigList> {
	return request('/lidar/configs', 'list tuning configs');
}

/** The label the replay form's dropdown shows for a config file. */
export function tuningConfigLabel(c: TuningConfigInfo): string {
	let label = c.path;
	if (c.kind === 'tuning') {
		if (c.solid_body) label += ` [${c.solid_body}]`;
		if (c.fingerprint) label += ` ${c.fingerprint}`;
	}
	if (c.active) label += ' (active)';
	if (!c.applicable) label += ` — ${c.reason || 'cannot be applied'}`;
	return label;
}

export type ReplaySpeedMode = 'analysis' | 'realtime' | 'scaled';

/** POST /api/lidar/pcap/start body. */
export interface ReplayStartRequest {
	pcap_file: string;
	pcap_files?: string[];
	analysis_mode: boolean;
	settle_before_recording: boolean;
	speed_mode: ReplaySpeedMode;
	speed_ratio?: number;
	start_seconds: number;
	duration_seconds: number;
	debug_ring_min?: number;
	debug_ring_max?: number;
	debug_az_min?: number;
	debug_az_max?: number;
	enable_debug?: boolean;
	enable_plots?: boolean;
	benchmark_mode?: boolean;
	tuning_config?: string;
}

export interface ReplayStartResponse {
	status: string;
	sensor_id: string;
	current_source: string;
	pcap_file: string;
	analysis_mode: boolean;
	settle_before_recording: boolean;
	tuning_config?: string;
	tuning_fingerprint?: string;
}

/**
 * The replay form's starting values. Settling before recording is ticked:
 * it settles the grid on a first pass, restores the snapshot, then replays
 * and records, which is the flow that produces a recording worth replaying.
 * Analysis is ticked with it because the server requires the pair, and a
 * default that needed the operator to fix it would be a trap.
 */
export function defaultReplayRequest(): ReplayStartRequest {
	return {
		pcap_file: '',
		analysis_mode: true,
		settle_before_recording: true,
		speed_mode: 'analysis',
		speed_ratio: 1,
		start_seconds: 0,
		duration_seconds: -1,
		debug_ring_min: 0,
		debug_ring_max: 0,
		debug_az_min: 0,
		debug_az_max: 0,
		enable_debug: false,
		enable_plots: false,
		benchmark_mode: false,
		tuning_config: ''
	};
}

/**
 * Choosing to settle before recording turns analysis on, so the form cannot
 * build a request the server refuses. Turning settle off leaves it alone.
 */
export function withSettleRule(req: ReplayStartRequest): ReplayStartRequest {
	return req.settle_before_recording ? { ...req, analysis_mode: true } : req;
}

/**
 * Check a replay request before it is sent, returning the first problem or
 * null. The server enforces the same rules; this only saves a round trip.
 */
export function validateReplayRequest(req: ReplayStartRequest): string | null {
	if (!req.pcap_file.trim()) return 'Choose a capture file to replay.';
	if (req.settle_before_recording && !req.analysis_mode) {
		return 'Settling before recording needs analysis and recording on.';
	}
	if (req.speed_mode === 'scaled' && !(Number(req.speed_ratio) > 0)) {
		return 'A scaled replay needs a speed ratio above zero.';
	}
	if (req.start_seconds < 0) return 'Start must be zero or later.';
	if (req.duration_seconds === 0 || req.duration_seconds < -1) {
		return 'Duration must be positive, or -1 for the whole capture.';
	}
	return null;
}

/**
 * The form fields a clip fills: its first capture, its trim, and the ordered
 * captures when it spans more than one. A single-case fetch carries the file
 * list; a listing does not, so a multi-file clip read from a listing only
 * names its first file.
 */
export function replayFieldsForCase(c: LidarReplayCase): {
	pcap_file: string;
	pcap_files?: string[];
	start_seconds: number;
	duration_seconds: number;
} {
	const files = [...(c.files ?? [])].sort((a, b) => a.ordinal - b.ordinal).map((f) => f.pcap_file);
	return {
		pcap_file: files[0] ?? c.pcap_file,
		pcap_files: files.length > 1 ? files : undefined,
		start_seconds: c.pcap_start_secs ?? 0,
		duration_seconds:
			c.pcap_duration_secs != null && c.pcap_duration_secs > 0 ? c.pcap_duration_secs : -1
	};
}

export function startReplay(
	sensorId: string,
	req: ReplayStartRequest
): Promise<ReplayStartResponse> {
	const body: ReplayStartRequest = { ...req };
	if (body.speed_mode !== 'scaled') delete body.speed_ratio;
	if (!body.pcap_files?.length) delete body.pcap_files;
	if (!body.tuning_config) delete body.tuning_config;
	return request('/lidar/pcap/start', 'start the replay', {
		method: 'POST',
		query: { sensor_id: sensorId },
		body
	});
}

export function stopReplay(sensorId: string): Promise<unknown> {
	return request('/lidar/replay/stop', 'stop the replay', {
		method: 'POST',
		query: { sensor_id: sensorId }
	});
}

export function resumeLive(sensorId: string): Promise<unknown> {
	return request('/lidar/pcap/resume_live', 'resume live data', {
		method: 'POST',
		query: { sensor_id: sensorId }
	});
}

// ─── Background grid, snapshots and tracks ──────────────────────────────────

/** The maintenance actions the status page offers, each a POST. */
export const MAINTENANCE_ACTIONS = {
	resetGrid: { path: '/lidar/grid_reset', action: 'reset the background grid' },
	resetAcceptance: { path: '/lidar/acceptance/reset', action: 'reset acceptance metrics' },
	persistBackground: { path: '/lidar/persist', action: 'save a background snapshot' },
	cleanupSnapshots: { path: '/lidar/snapshots/cleanup', action: 'clean up snapshots' },
	clearTracks: { path: '/lidar/tracks/clear', action: 'clear tracks' }
} as const;

export type MaintenanceAction = keyof typeof MAINTENANCE_ACTIONS;

export function runMaintenance(sensorId: string, which: MaintenanceAction): Promise<unknown> {
	const { path, action } = MAINTENANCE_ACTIONS[which];
	return request(path, action, { method: 'POST', query: { sensor_id: sensorId } });
}

export function getGridStatus(sensorId: string): Promise<Record<string, unknown>> {
	return request('/lidar/grid_status', 'load grid status', { query: { sensor_id: sensorId } });
}

export function getAcceptanceMetrics(sensorId: string): Promise<Record<string, unknown>> {
	return request('/lidar/acceptance', 'load acceptance metrics', {
		query: { sensor_id: sensorId }
	});
}

/** The settling metrics the Go grid chart colours by. */
export const GRID_METRICS = [
	{ value: 'settle_rate', label: 'Settle rate' },
	{ value: 'fill_rate', label: 'Fill rate' },
	{ value: 'unsettled_ratio', label: 'Unsettled ratio' },
	{ value: 'frozen_ratio', label: 'Frozen ratio' },
	{ value: 'mean_times_seen', label: 'Mean times seen' }
] as const;

export type GridMetric = (typeof GRID_METRICS)[number]['value'];

/** The Go-served SVG of the background grid, for InlineSvgChart. */
export function gridChartPath(sensorId: string, metric: GridMetric, azimuthBucketDeg = 3): string {
	return apiPath('/charts/grid', {
		sensor_id: sensorId,
		metric,
		azimuth_bucket_deg: azimuthBucketDeg
	});
}

// ─── Debug chart JSON ───────────────────────────────────────────────────────

export interface ScatterPointJSON {
	x: number;
	y: number;
	value: number;
}

/** GET /api/lidar/chart/polar: background cells in sensor XY. */
export interface PolarChartData {
	points: ScatterPointJSON[];
	max_abs: number;
	max_value: number;
	sensor_id: string;
	stride: number;
	num_points: number;
}

/** GET /api/lidar/chart/heatmap: coarse buckets at their mean range. */
export interface HeatmapBucketData {
	points: ScatterPointJSON[];
	max_value: number;
	max_abs: number;
	sensor_id: string;
	num_buckets: number;
}

/** GET /api/lidar/chart/foreground: the latest foreground snapshot. */
export interface ForegroundChartData {
	foreground_points: ScatterPointJSON[];
	background_points: ScatterPointJSON[];
	max_abs: number;
	foreground_count: number;
	background_count: number;
	total_points: number;
	foreground_percent: number;
	timestamp: string;
	sensor_id: string;
}

/** GET /api/lidar/chart/clusters: recent cluster centroids. */
export interface RecentClustersData {
	clusters: Array<{ x: number; y: number; points_count: number }>;
	max_abs: number;
	max_points: number;
	num_clusters: number;
	sensor_id: string;
}

/** GET /api/lidar/chart/traffic. */
export interface TrafficMetrics {
	packets_per_sec: number;
	mb_per_sec: number;
	points_per_sec: number;
	dropped_count: number;
	timestamp: string;
}

/**
 * Debug chart reads answer 404 while the pipeline has nothing to show yet,
 * which is a state, not a failure: these return null for it.
 */
async function optionalChart<T>(path: string, action: string, query: RequestOptions['query']) {
	const res = await fetch(apiPath(path, query));
	if (res.status === 404) return null;
	if (!res.ok) throw new Error(await errorMessage(res, action));
	return (await res.json()) as T;
}

export function getPolarChart(sensorId: string, maxPoints = 8000) {
	return optionalChart<PolarChartData>('/lidar/chart/polar', 'load the background points', {
		sensor_id: sensorId,
		max_points: maxPoints
	});
}

export function getHeatmapChart(sensorId: string, azimuthBucketDeg = 3, settledThreshold = 5) {
	return optionalChart<HeatmapBucketData>('/lidar/chart/heatmap', 'load the background buckets', {
		sensor_id: sensorId,
		azimuth_bucket_deg: azimuthBucketDeg,
		settled_threshold: settledThreshold
	});
}

export function getForegroundChart(sensorId: string) {
	return optionalChart<ForegroundChartData>(
		'/lidar/chart/foreground',
		'load the foreground frame',
		{
			sensor_id: sensorId
		}
	);
}

export function getClustersChart(sensorId: string, limit = 500) {
	return optionalChart<RecentClustersData>('/lidar/chart/clusters', 'load recent clusters', {
		sensor_id: sensorId,
		limit
	});
}

export function getTrafficChart() {
	return optionalChart<TrafficMetrics>('/lidar/chart/traffic', 'load traffic metrics', {});
}

// ─── Background regions ─────────────────────────────────────────────────────

/** The parameters a region overrides. */
export interface RegionParams {
	noise_relative_fraction: number;
	neighbor_confirmation_count: number;
	settle_update_fraction: number;
}

/** One region of the background grid with its own parameters. */
export interface BackgroundRegion {
	id: number;
	cell_count: number;
	mean_variance: number;
	params: RegionParams;
	cells?: Array<{ ring: number; azimuth_deg: number }>;
}

/** GET /api/lidar/background/regions. */
export interface BackgroundRegions {
	sensor_id: string;
	timestamp: string;
	identification_complete: boolean;
	identification_time?: string;
	frames_sampled: number;
	region_count: number;
	regions: BackgroundRegion[];
	/** Region id for each grid cell, ring-major; -1 for a cell in none. */
	grid_mapping: number[];
	rings: number;
	azimuth_bins: number;
}

/** Null while the sensor has no background grid yet. */
export function getBackgroundRegions(sensorId: string) {
	return optionalChart<BackgroundRegions>('/lidar/background/regions', 'load background regions', {
		sensor_id: sensorId
	});
}

// ─── Sweeps, auto-tune and HINT ─────────────────────────────────────────────

/** GET /api/lidar/sweep/status and the auto-tune status share these fields. */
export interface SweepRunStatus {
	status: string;
	completed_combos?: number;
	total_combos?: number;
	current_combo?: Record<string, unknown>;
	results?: Record<string, unknown>[];
	error?: string;
	warnings?: string[];
	// Auto-tune only.
	round?: number;
	total_rounds?: number;
	round_results?: Array<Record<string, unknown>>;
	recommendation?: Record<string, unknown>;
	started_at?: string;
	cumulative_completed?: number;
	cumulative_total?: number;
}

export function startManualSweep(req: Record<string, unknown>): Promise<unknown> {
	return request('/lidar/sweep/start', 'start the sweep', { method: 'POST', body: req });
}

export function getManualSweepStatus(): Promise<SweepRunStatus> {
	return request('/lidar/sweep/status', 'load sweep status');
}

export function stopManualSweep(): Promise<unknown> {
	return request('/lidar/sweep/stop', 'stop the sweep', { method: 'POST' });
}

export function startAutoTune(req: Record<string, unknown>): Promise<unknown> {
	return request('/lidar/sweep/auto', 'start auto-tune', { method: 'POST', body: req });
}

export function getAutoTuneStatus(): Promise<SweepRunStatus> {
	return request('/lidar/sweep/auto', 'load auto-tune status');
}

export function stopAutoTune(): Promise<unknown> {
	return request('/lidar/sweep/auto/stop', 'stop auto-tune', { method: 'POST' });
}

export function suspendAutoTune(): Promise<unknown> {
	return request('/lidar/sweep/auto/suspend', 'suspend auto-tune', { method: 'POST' });
}

/** Resume the in-memory suspended run, or a checkpoint by sweep id. */
export function resumeAutoTune(sweepId?: string): Promise<unknown> {
	return request('/lidar/sweep/auto/resume', 'resume auto-tune', {
		method: 'POST',
		body: sweepId ? { sweep_id: sweepId } : {}
	});
}

/** GET /api/lidar/sweep/auto/suspended: a checkpoint left by a restart. */
export interface SuspendedSweep {
	found: boolean;
	sweep_id?: string;
	sensor_id?: string;
	started_at?: string;
	checkpoint_round?: number;
}

export function getSuspendedSweep(): Promise<SuspendedSweep> {
	return request('/lidar/sweep/auto/suspended', 'check for a suspended sweep');
}

/**
 * Block until the HINT state leaves `status`, the server's long poll. The
 * caller aborts it with `signal` when the page goes.
 */
export async function waitForHINTChange(status: string, signal: AbortSignal): Promise<void> {
	await request('/lidar/sweep/hint', 'wait for the HINT state', {
		query: { wait_for_change: status },
		signal
	});
}
