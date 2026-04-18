// src/lib/api.ts
// Simple API client for /api/events and /api/radar_stats using fetch
// Add more endpoints as needed

export interface Event {
	speed: number;
	magnitude?: number;
	uptime?: number;
}

export interface RadarStats {
	classifier?: string;
	date: Date;
	count: number;
	p50: number;
	p85: number;
	p98: number;
	max: number;
}

export interface Config {
	units: string;
	timezone: string;
}

export interface TimeSeriesChartRequest {
	siteId: number;
	startDate: string;
	endDate: string;
	group?: string;
	units?: string;
	timezone?: string;
	source?: string;
	minSpeed?: number;
	boundaryThreshold?: number;
	paperSize?: 'a4' | 'letter';
	expandedChart?: boolean;
}

export interface HistogramChartRequest {
	siteId: number;
	startDate: string;
	endDate: string;
	units?: string;
	timezone?: string;
	source?: string;
	bucketSize?: number;
	max?: number;
	minSpeed?: number;
	boundaryThreshold?: number;
	paperSize?: 'a4' | 'letter';
}

export interface ComparisonChartRequest {
	siteId: number;
	startDate: string;
	endDate: string;
	compareStartDate: string;
	compareEndDate: string;
	units?: string;
	timezone?: string;
	source?: string;
	compareSource?: string;
	bucketSize?: number;
	max?: number;
	minSpeed?: number;
	boundaryThreshold?: number;
	paperSize?: 'a4' | 'letter';
}

// Raw shape returned from the server for a single metric row
type RawRadarStats = {
	classifier: string;
	start_time: string;
	count: number;
	p50_speed: number;
	p85_speed: number;
	p98_speed: number;
	max_speed: number;
};

// Histogram shape: server returns a map of bucket label -> count. Keys are strings
// (formatted numbers) and values are counts.
export type Histogram = Record<string, number>;

// Response from getRadarStats
export interface RadarStatsResponse {
	metrics: RadarStats[];
	histogram?: Histogram;
	cosineCorrection?: {
		angles: number[];
		applied: boolean;
	};
}

// LiDAR capability state as reported by /api/capabilities.
export interface LidarCapability {
	enabled: boolean;
	state: 'disabled' | 'starting' | 'ready' | 'error';
}

// Sensor capabilities reported by /api/capabilities.
export interface Capabilities {
	radar: boolean;
	lidar: LidarCapability;
	lidar_sweep: boolean;
}

const API_BASE = '/api';

function buildRelativeApiPath(
	path: string,
	params: Record<string, string | number | null | undefined>
): string {
	const searchParams = new URLSearchParams();
	for (const [key, value] of Object.entries(params)) {
		if (value === undefined || value === null || value === '') continue;
		searchParams.set(key, String(value));
	}
	const query = searchParams.toString();
	return query ? `${API_BASE}${path}?${query}` : `${API_BASE}${path}`;
}

/**
 * Build a human-readable API error with status-code context.
 * Used across all fetch wrappers so failure messages sound consistent and
 * give the reader a hint about what went wrong beyond the number.
 */
function apiError(action: string, status: number): Error {
	const hint =
		status === 401 || status === 403
			? ' — not authorised'
			: status === 404
				? ' — not found on server'
				: status >= 500
					? ' — server error, check the service is running'
					: '';
	return new Error(`${action} (HTTP ${status}${hint})`);
}

export async function getEvents(units?: string, timezone?: string): Promise<Event[]> {
	const url = new URL(`${API_BASE}/events`, window.location.origin);
	if (units) {
		url.searchParams.append('units', units);
	}
	if (timezone) {
		url.searchParams.append('timezone', timezone);
	}
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load events', res.status);
	return res.json();
}

// getRadarStats now requires start and end (unix seconds), and optional group, units, timezone
export async function getRadarStats(
	start: number,
	end: number,
	group?: string,
	units?: string,
	timezone?: string,
	source?: string,
	siteId?: number | null
): Promise<RadarStatsResponse> {
	const url = new URL(`${API_BASE}/radar_stats`, window.location.origin);
	url.searchParams.append('start', start.toString());
	url.searchParams.append('end', end.toString());
	if (group) url.searchParams.append('group', group);
	if (units) url.searchParams.append('units', units);
	if (timezone) url.searchParams.append('timezone', timezone);
	if (source) url.searchParams.append('source', source);
	if (siteId != null) url.searchParams.append('site_id', siteId.toString());
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load radar stats', res.status);
	// Expect the server to return the new root object: { metrics: [...], histogram: {...} }
	const payload = await res.json();
	const rows = Array.isArray(payload.metrics) ? (payload.metrics as RawRadarStats[]) : [];

	const metrics = rows.map((r) => ({
		classifier: r.classifier,
		date: new Date(r.start_time),
		count: r.count,
		p50: r.p50_speed,
		p85: r.p85_speed,
		p98: r.p98_speed,
		max: r.max_speed
	})) as RadarStats[];

	const histogram = payload && payload.histogram ? (payload.histogram as Histogram) : undefined;
	const cosineCorrection =
		payload && payload.cosine_correction ? payload.cosine_correction : undefined;
	return { metrics, histogram, cosineCorrection };
}

export function buildTimeSeriesChartPath(request: TimeSeriesChartRequest): string {
	return buildRelativeApiPath('/charts/timeseries', {
		site_id: request.siteId,
		start: request.startDate,
		end: request.endDate,
		group: request.group,
		units: request.units,
		tz: request.timezone,
		source: request.source,
		min_speed: request.minSpeed,
		boundary_threshold: request.boundaryThreshold,
		paper_size: request.paperSize,
		expanded_chart: request.expandedChart ? 'true' : undefined
	});
}

export function buildHistogramChartPath(request: HistogramChartRequest): string {
	return buildRelativeApiPath('/charts/histogram', {
		site_id: request.siteId,
		start: request.startDate,
		end: request.endDate,
		units: request.units,
		tz: request.timezone,
		source: request.source,
		bucket_size: request.bucketSize,
		max: request.max,
		min_speed: request.minSpeed,
		boundary_threshold: request.boundaryThreshold,
		paper_size: request.paperSize
	});
}

export function buildComparisonChartPath(request: ComparisonChartRequest): string {
	return buildRelativeApiPath('/charts/comparison', {
		site_id: request.siteId,
		start: request.startDate,
		end: request.endDate,
		compare_start: request.compareStartDate,
		compare_end: request.compareEndDate,
		units: request.units,
		tz: request.timezone,
		source: request.source,
		compare_source: request.compareSource,
		bucket_size: request.bucketSize,
		max: request.max,
		min_speed: request.minSpeed,
		boundary_threshold: request.boundaryThreshold,
		paper_size: request.paperSize
	});
}

export async function getConfig(): Promise<Config> {
	const res = await fetch(`${API_BASE}/config`);
	if (!res.ok) throw apiError('Could not load configuration', res.status);
	return res.json();
}

/**
 * Fetch sensor capabilities from /api/capabilities.
 * Returns which sensors are active and their runtime state.
 */
export async function getCapabilities(): Promise<Capabilities> {
	const res = await fetch(`${API_BASE}/capabilities`);
	if (!res.ok) throw apiError('Could not load capabilities', res.status);
	return res.json();
}

export interface ReportRequest {
	site_id?: number; // Optional: use site configuration
	start_date: string; // YYYY-MM-DD format
	end_date: string; // YYYY-MM-DD format
	compare_start_date?: string; // YYYY-MM-DD format
	compare_end_date?: string; // YYYY-MM-DD format
	timezone: string; // e.g., "US/Pacific"
	units: string; // "mph" or "kph"
	group?: string; // e.g., "1h", "4h"
	source?: string; // "radar_objects", "radar_data", or "radar_data_transits"
	compare_source?: string; // Optional: source for comparison period (defaults to source)
	min_speed?: number; // minimum speed filter
	boundary_threshold?: number; // filter boundary hours with < N samples (default: 5)
	histogram?: boolean; // whether to generate histogram
	hist_bucket_size?: number; // histogram bucket size
	hist_max?: number; // histogram max value
	// These can be overridden if site_id is not provided
	location?: string; // site location
	surveyor?: string; // surveyor name
	contact?: string; // contact info
	speed_limit?: number; // posted speed limit
	site_description?: string; // site description
	cosine_error_angle?: number; // radar mounting angle
	paper_size?: 'a4' | 'letter'; // PDF paper size
	expanded_chart?: boolean; // preserve linear timestamp spacing in time-series charts
}

export interface ReportResponse {
	success: boolean;
	message: string;
	output?: string;
	error?: string;
}

export interface GenerateReportResponse {
	success: boolean;
	report_id: number;
	message: string;
}

export async function generateReport(request: ReportRequest): Promise<GenerateReportResponse> {
	const res = await fetch(`${API_BASE}/generate_report`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => null);
		throw errorData?.error
			? new Error(errorData.error)
			: apiError('Could not generate report', res.status);
	}
	return res.json();
}

export interface SiteReport {
	id: number;
	site_id: number;
	start_date: string;
	end_date: string;
	filepath: string;
	filename: string;
	zip_filepath?: string | null;
	zip_filename?: string | null;
	run_id: string;
	timezone: string;
	units: string;
	source: string;
	created_at: string;
}

export interface DownloadResult {
	blob: Blob;
	filename: string;
}

export interface SiteConfigPeriod {
	id?: number;
	site_id: number;
	effective_start_unix: number;
	effective_end_unix?: number | null;
	is_active: boolean;
	notes?: string | null;
	cosine_error_angle: number;
	created_at?: string;
	updated_at?: string;
}

export interface TimelineResponse {
	site_id: number;
	data_range: {
		start_unix: number;
		end_unix: number;
	};
	config_periods: SiteConfigPeriod[];
	unconfigured_periods: Array<{ start_unix: number; end_unix: number }>;
}

export async function listSiteConfigPeriods(siteId: number): Promise<SiteConfigPeriod[]> {
	const url = new URL(`${API_BASE}/site_config_periods`, window.location.origin);
	url.searchParams.append('site_id', siteId.toString());
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load site configuration periods', res.status);
	return res.json();
}

export async function upsertSiteConfigPeriod(period: SiteConfigPeriod): Promise<SiteConfigPeriod> {
	const res = await fetch(`${API_BASE}/site_config_periods`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(period)
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Could not save site configuration period: ${res.status}`);
	}
	return res.json();
}

export async function getTimeline(siteId: number): Promise<TimelineResponse> {
	const url = new URL(`${API_BASE}/timeline`, window.location.origin);
	url.searchParams.append('site_id', siteId.toString());
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load timeline', res.status);
	return res.json();
}

export async function downloadReport(
	reportId: number,
	fileType: 'pdf' | 'zip' = 'pdf'
): Promise<DownloadResult> {
	const defaultFilename = `report.${fileType}`;
	const res = await fetch(`${API_BASE}/reports/${reportId}/download/${defaultFilename}`);
	if (!res.ok) {
		throw apiError('Could not download report', res.status);
	}

	// Extract filename from Content-Disposition header
	const contentDisposition = res.headers.get('Content-Disposition');
	let filename = defaultFilename;
	if (contentDisposition) {
		const match = contentDisposition.match(/filename="?([^";]+)"?/);
		if (match) {
			filename = match[1].trim();
		}
	}

	const blob = await res.blob();
	return { blob, filename };
}

export async function getRecentReports(): Promise<SiteReport[]> {
	const res = await fetch(`${API_BASE}/reports`);
	if (!res.ok) throw apiError('Could not load reports', res.status);
	return res.json();
}

export async function getReportsForSite(siteId: number): Promise<SiteReport[]> {
	const res = await fetch(`${API_BASE}/reports/site/${siteId}`);
	if (!res.ok) throw apiError('Could not load site reports', res.status);
	return res.json();
}

export async function getReport(reportId: number): Promise<SiteReport> {
	const res = await fetch(`${API_BASE}/reports/${reportId}`);
	if (!res.ok) throw apiError('Could not load report', res.status);
	return res.json();
}

export async function deleteReport(reportId: number): Promise<void> {
	const res = await fetch(`${API_BASE}/reports/${reportId}`, {
		method: 'DELETE'
	});
	if (!res.ok) throw apiError('Could not delete report', res.status);
}

// Site management interfaces and functions

export interface Site {
	id: number;
	name: string;
	location: string;
	description?: string | null;
	surveyor: string;
	contact: string;
	address?: string | null;
	latitude?: number | null;
	longitude?: number | null;
	map_angle?: number | null;
	include_map: boolean;
	site_description?: string | null;
	cosine_error_angle?: number | null;
	speed_limit?: number | null;
	speed_limit_note?: string | null;
	bbox_ne_lat?: number | null;
	bbox_ne_lng?: number | null;
	bbox_sw_lat?: number | null;
	bbox_sw_lng?: number | null;
	/**
	 * Base64-encoded SVG image data as a string.
	 * This must be a base64 string (matching the Go BLOB field), not raw SVG XML/text.
	 */
	map_svg_data?: string | null;
	radar_svg_x?: number | null;
	radar_svg_y?: number | null;
	created_at: string;
	updated_at: string;
}

export async function getSites(): Promise<Site[]> {
	const res = await fetch(`${API_BASE}/sites`);
	if (!res.ok) throw apiError('Could not load sites', res.status);
	return res.json();
}

export async function getSite(id: number): Promise<Site> {
	const res = await fetch(`${API_BASE}/sites/${id}`);
	if (!res.ok) throw apiError('Could not load site', res.status);
	return res.json();
}

export async function createSite(site: Partial<Site>): Promise<Site> {
	const res = await fetch(`${API_BASE}/sites`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(site)
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Could not create site: ${res.status}`);
	}
	return res.json();
}

export async function updateSite(id: number, site: Partial<Site>): Promise<Site> {
	const res = await fetch(`${API_BASE}/sites/${id}`, {
		method: 'PUT',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(site)
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Could not update site: ${res.status}`);
	}
	return res.json();
}

export async function deleteSite(id: number): Promise<void> {
	const res = await fetch(`${API_BASE}/sites/${id}`, {
		method: 'DELETE'
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Could not delete site: ${res.status}`);
	}
}

// Transit Worker API
export interface TransitRunInfo {
	trigger?: string;
	started_at: string;
	finished_at?: string;
	duration_ms?: number;
	error?: string;
}

export interface TransitWorkerState {
	enabled: boolean;
	last_run_at: string;
	last_run_error?: string;
	run_count: number;
	is_healthy: boolean;
	current_run?: TransitRunInfo | null;
	last_run?: TransitRunInfo | null;
}

export interface TransitWorkerUpdateRequest {
	enabled?: boolean;
	trigger?: boolean;
	trigger_full_history?: boolean;
}

export interface TransitWorkerUpdateResponse {
	enabled: boolean;
	last_run_at: string;
	last_run_error?: string;
	run_count: number;
	is_healthy: boolean;
	current_run?: TransitRunInfo | null;
	last_run?: TransitRunInfo | null;
}

export async function getTransitWorkerState(): Promise<TransitWorkerState> {
	const res = await fetch(`${API_BASE}/transit_worker`);
	if (!res.ok) throw apiError('Could not load transit worker state', res.status);
	return res.json();
}

export async function updateTransitWorker(
	request: TransitWorkerUpdateRequest
): Promise<TransitWorkerUpdateResponse> {
	const res = await fetch(`${API_BASE}/transit_worker`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(request)
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Could not update transit worker: ${res.status}`);
	}
	return res.json();
}

// Tailscale enrollment status reported by /api/tailscale/status.
export interface TailscaleStatus {
	daemon_running: boolean;
	backend_state: string;
	login_url?: string;
	login_in_progress: boolean;
	hostname?: string;
	magic_dns?: string;
	tailnet_name?: string;
	peer_count: number;
	// Per-step results from the most recent device-policy apply.
	// SSH and `tailscale serve` are reported independently so the
	// operator's next step is unambiguous when one half fails.
	ssh_enabled: boolean;
	ssh_error?: string;
	serve_published: boolean;
	serve_error?: string;
}

async function tailscaleResult(action: string, res: Response): Promise<TailscaleStatus> {
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `${action} (HTTP ${res.status})`);
	}
	return res.json();
}

export async function getTailscaleStatus(): Promise<TailscaleStatus> {
	const res = await fetch(`${API_BASE}/tailscale/status`);
	return tailscaleResult('Could not load Tailscale status', res);
}

export async function enableTailscale(): Promise<TailscaleStatus> {
	const res = await fetch(`${API_BASE}/tailscale/enable`, { method: 'POST' });
	return tailscaleResult('Could not enable Tailscale', res);
}

export async function disableTailscale(): Promise<TailscaleStatus> {
	const res = await fetch(`${API_BASE}/tailscale/disable`, { method: 'POST' });
	return tailscaleResult('Could not disable Tailscale', res);
}

// LiDAR Track API

import type {
	AnalysisRun,
	BackgroundGrid,
	LabellingProgress,
	LidarReplayCase,
	MissedRegion,
	ObservationListResponse,
	RunTrack,
	ScoreComponents,
	SweepRecord,
	SweepSummary,
	Track,
	TrackHistoryResponse,
	TrackListResponse,
	TrackObservation,
	TrackSummaryResponse
} from './types/lidar';

/**
 * Get active tracks from the LiDAR tracker
 * @param sensorId - Sensor identifier (e.g., "hesai-pandar40p")
 * @param state - Optional state filter: "tentative", "confirmed", "all"
 */
export async function getActiveTracks(
	sensorId: string,
	state?: 'tentative' | 'confirmed' | 'all'
): Promise<TrackListResponse> {
	const url = new URL(`${API_BASE}/lidar/tracks/active`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	if (state) {
		url.searchParams.append('state', state);
	}
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load active tracks', res.status);
	return res.json();
}

/**
 * Get details for a specific track
 * @param trackId - Track identifier
 */
export async function getTrackById(trackId: string): Promise<Track> {
	const res = await fetch(`${API_BASE}/lidar/tracks/${trackId}`);
	if (!res.ok) throw apiError('Could not load track', res.status);
	return res.json();
}

/**
 * Get trajectory observations for a track
 * @param trackId - Track identifier
 */
export async function getTrackObservations(trackId: string): Promise<TrackObservation[]> {
	const res = await fetch(`${API_BASE}/lidar/tracks/${trackId}/observations`);
	if (!res.ok) throw apiError('Could not load track observations', res.status);
	const data = await res.json();
	// Backend returns an envelope { track_id, observations, count, timestamp }.
	// Extract the inner observations array.
	return data.observations ?? [];
}

/**
 * Get historical tracks within a time range
 * @param sensorId - Sensor identifier
 * @param startTime - Start time in Unix nanoseconds
 * @param endTime - End time in Unix nanoseconds
 * @param limit - Maximum number of tracks to return (default 500, max 1000)
 */
export async function getTrackHistory(
	sensorId: string,
	startTime: number,
	endTime: number,
	limit = 500
): Promise<TrackHistoryResponse> {
	const url = new URL(`${API_BASE}/lidar/tracks/history`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	url.searchParams.append('start_time', startTime.toString());
	url.searchParams.append('end_time', endTime.toString());
	url.searchParams.append('limit', limit.toString());
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load track history', res.status);
	const data = await res.json();
	return {
		tracks: Array.isArray(data.tracks) ? data.tracks : [],
		observations: data.observations ?? {}
	};
}

/**
 * Get raw observations for a sensor within a time window (nanoseconds).
 */
export async function getTrackObservationsRange(
	sensorId: string,
	startTime: number,
	endTime: number,
	limit = 2000,
	trackId?: string
): Promise<ObservationListResponse> {
	const url = new URL(`${API_BASE}/lidar/observations`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	url.searchParams.append('start_time', startTime.toString());
	url.searchParams.append('end_time', endTime.toString());
	url.searchParams.append('limit', limit.toString());
	if (trackId) {
		url.searchParams.append('track_id', trackId);
	}
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load track observations for range', res.status);
	return res.json();
}

/**
 * Get track summary statistics
 * @param sensorId - Sensor identifier
 */
export async function getTrackSummary(sensorId: string): Promise<TrackSummaryResponse> {
	const url = new URL(`${API_BASE}/lidar/tracks/summary`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load track summary', res.status);
	return res.json();
}

/**
 * Get background grid for visualization
 * @param sensorId - Sensor identifier
 */
export async function getBackgroundGrid(sensorId: string): Promise<BackgroundGrid> {
	const url = new URL(`${API_BASE}/lidar/background/grid`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load background grid', res.status);
	return res.json();
}

// LiDAR Replay Case and Run Labelling API
// Uses API_BASE for consistency with other LiDAR endpoints.

export async function getLidarReplayCases(sensorId?: string): Promise<LidarReplayCase[]> {
	const params = new URLSearchParams();
	if (sensorId) params.set('sensor_id', sensorId);
	const url = `${API_BASE}/lidar/scenes${params.toString() ? '?' + params : ''}`;
	const res = await fetch(url);
	if (!res.ok) throw apiError('Could not load replay cases', res.status);
	const data = await res.json();
	return data.scenes || [];
}

export async function getLidarReplayCase(replayCaseId: string): Promise<LidarReplayCase> {
	const res = await fetch(`${API_BASE}/lidar/scenes/${replayCaseId}`);
	if (!res.ok) throw apiError('Could not load replay case', res.status);
	return res.json();
}

export async function createLidarReplayCase(scene: {
	sensor_id: string;
	pcap_file: string;
	pcap_start_secs?: number;
	pcap_duration_secs?: number;
	description?: string;
}): Promise<LidarReplayCase> {
	const res = await fetch(`${API_BASE}/lidar/scenes`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(scene)
	});
	if (!res.ok) throw apiError('Could not create replay case', res.status);
	return res.json();
}

export async function updateLidarReplayCase(
	replayCaseId: string,
	update: {
		description?: string;
		reference_run_id?: string;
		optimal_params_json?: Record<string, unknown> | null;
		pcap_start_secs?: number;
		pcap_duration_secs?: number;
	}
): Promise<LidarReplayCase> {
	// Omit null optimal_params_json to avoid persisting the JSON literal "null";
	// undefined keys are stripped by JSON.stringify, which the server treats as
	// "no change".
	const { optimal_params_json, ...rest } = update;
	const body = optimal_params_json != null ? { ...rest, optimal_params_json } : rest;
	const res = await fetch(`${API_BASE}/lidar/scenes/${replayCaseId}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) throw apiError('Could not update replay case', res.status);
	return res.json();
}

export async function deleteLidarReplayCase(replayCaseId: string): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/scenes/${replayCaseId}`, {
		method: 'DELETE'
	});
	if (!res.ok) throw apiError('Could not delete replay case', res.status);
}

// PCAP file scanning API

export interface PcapFileInfo {
	path: string;
	size_bytes: number;
	modified_at: string;
	in_use: boolean;
}

export interface PcapFilesResponse {
	pcap_dir: string;
	files: PcapFileInfo[];
	count: number;
}

export async function scanPcapFiles(): Promise<PcapFilesResponse> {
	const res = await fetch(`${API_BASE}/lidar/pcap/files`);
	if (!res.ok) throw new Error(`Could not scan PCAP files: ${res.status}`);
	return res.json();
}

export async function getLidarRuns(params?: {
	sensor_id?: string;
	status?: string;
	limit?: number;
}): Promise<AnalysisRun[]> {
	const searchParams = new URLSearchParams();
	if (params?.sensor_id) searchParams.set('sensor_id', params.sensor_id);
	if (params?.status) searchParams.set('status', params.status);
	if (params?.limit) searchParams.set('limit', String(params.limit));
	const url = `${API_BASE}/lidar/runs${searchParams.toString() ? '?' + searchParams : ''}`;
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Could not load runs: ${res.status}`);
	const data = await res.json();
	return data.runs || [];
}

export async function getRunTracks(runId: string): Promise<RunTrack[]> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/tracks`);
	if (!res.ok) throw new Error(`Could not load tracks: ${res.status}`);
	const data = await res.json();
	return data.tracks || [];
}

export async function updateTrackLabel(
	runId: string,
	trackId: string,
	label: {
		user_label?: string;
		quality_label?: string;
		label_confidence?: number;
		labeler_id?: string;
	}
): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/tracks/${trackId}/label`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(label)
	});
	if (!res.ok) throw new Error(`Could not update label: ${res.status}`);
}

export async function updateTrackFlags(
	runId: string,
	trackId: string,
	flags: {
		linked_track_ids?: string[];
		user_label?: string;
	}
): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/tracks/${trackId}/flags`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(flags)
	});
	if (!res.ok) throw new Error(`Could not update flags: ${res.status}`);
}

export async function getLabellingProgress(runId: string): Promise<LabellingProgress> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/labelling-progress`);
	if (!res.ok) throw new Error(`Could not load labelling progress: ${res.status}`);
	return res.json();
}

// LiDAR Missed Regions API

export async function getMissedRegions(runId: string): Promise<MissedRegion[]> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/missed-regions`);
	if (!res.ok) throw new Error(`Could not load missed regions: ${res.status}`);
	const data = await res.json();
	return data.regions || [];
}

export async function createMissedRegion(
	runId: string,
	region: {
		center_x: number;
		center_y: number;
		radius_m?: number;
		time_start_ns: number;
		time_end_ns: number;
		expected_label?: string;
		notes?: string;
	}
): Promise<MissedRegion> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/missed-regions`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(region)
	});
	if (!res.ok) throw new Error(`Could not create missed region: ${res.status}`);
	return res.json();
}

export async function deleteMissedRegion(runId: string, regionId: string): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/missed-regions/${regionId}`, {
		method: 'DELETE'
	});
	if (!res.ok) throw new Error(`Could not delete missed region: ${res.status}`);
}

/**
 * Delete all runs for a sensor
 * @param sensorId - Sensor identifier (optional)
 */
export async function deleteAllRuns(sensorId?: string): Promise<void> {
	const url = new URL(`${API_BASE}/lidar/runs/clear`, window.location.origin);
	if (sensorId) {
		url.searchParams.append('sensor_id', sensorId);
	}
	const res = await fetch(url, {
		method: 'POST'
	});
	if (!res.ok) throw new Error(`Could not delete runs: ${res.status}`);
}

/**
 * Delete a specific run
 * @param runId - Run identifier
 */
export async function deleteRun(runId: string): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}`, {
		method: 'DELETE'
	});
	if (!res.ok) throw new Error(`Could not delete run: ${res.status}`);
}

/**
 * Delete a specific run track
 * @param runId - Run identifier
 * @param trackId - Track identifier
 */
export async function deleteRunTrack(runId: string, trackId: string): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/tracks/${trackId}`, {
		method: 'DELETE'
	});
	if (!res.ok) throw new Error(`Could not delete track: ${res.status}`);
}

// ---- Sweep / Auto-Tune ----

/**
 * List recent sweeps for a sensor.
 * @param sensorId - Sensor identifier
 * @param limit - Maximum number of results (default 20)
 */
export async function listSweeps(sensorId: string, limit = 20): Promise<SweepSummary[]> {
	const url = new URL(`${API_BASE}/lidar/sweeps`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	if (limit !== 20) url.searchParams.append('limit', String(limit));
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Could not load sweeps: ${res.status}`);
	return res.json();
}

/**
 * Get a single sweep record with full results.
 * @param sweepId - Sweep identifier
 */
export async function getSweep(sweepId: string): Promise<SweepRecord> {
	const res = await fetch(`${API_BASE}/lidar/sweeps/${sweepId}`);
	if (!res.ok) throw new Error(`Could not load sweep: ${res.status}`);
	return res.json();
}

/** Score explanation response from the explain endpoint. */
export interface SweepExplanation {
	sweep_id: string;
	objective_name?: string;
	objective_version?: string;
	score_components?: ScoreComponents;
	recommendation_explanation?: Record<string, unknown>;
	label_provenance_summary?: Record<string, unknown>;
}

/**
 * Fetch score explanation for a sweep.
 * @param sweepId - Sweep identifier
 */
export async function getSweepExplanation(sweepId: string): Promise<SweepExplanation | null> {
	try {
		const resp = await fetch(`${API_BASE}/lidar/sweep/explain/${encodeURIComponent(sweepId)}`);
		if (!resp.ok) return null;
		return await resp.json();
	} catch {
		return null;
	}
}

/**
 * Get the current LiDAR tuning parameters.
 * @param sensorId - Sensor identifier
 */
export async function getLidarParams(sensorId: string): Promise<Record<string, unknown>> {
	const url = new URL(`${API_BASE}/lidar/params`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Could not load parameters: ${res.status}`);
	return res.json();
}

/**
 * Apply tuning parameters to the LiDAR system.
 * @param sensorId - Sensor identifier
 * @param params - Partial tuning parameter object
 */
export async function applyLidarParams(
	sensorId: string,
	params: Record<string, unknown>
): Promise<void> {
	const url = new URL(`${API_BASE}/lidar/params`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	const res = await fetch(url, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(params)
	});
	if (!res.ok) {
		const text = await res.text();
		throw new Error(`Could not apply parameters: ${text}`);
	}
}

/**
 * Get the current HINT sweep state.
 */
export async function getHINTState(): Promise<Record<string, unknown>> {
	const res = await fetch(`${API_BASE}/lidar/sweep/hint`);
	if (!res.ok) throw new Error(`Could not load HINT state: ${res.status}`);
	return res.json();
}

/**
 * Start an HINT sweep.
 * @param req - HINT sweep request parameters
 */
export async function startHINTSweep(req: Record<string, unknown>): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/sweep/hint`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(req)
	});
	if (!res.ok) {
		const text = await res.text();
		let msg = `Could not start HINT sweep: ${res.status}`;
		try {
			const body = JSON.parse(text);
			if (body.error) msg = body.error;
		} catch {
			if (text) msg = text;
		}
		throw new Error(msg);
	}
}

/**
 * Signal HINT tuner to continue from labelling to sweep phase.
 * @param nextDurationMins - Override for next sweep duration (0 = use default)
 * @param addRound - Whether to add an extra round
 */
export async function continueHINT(nextDurationMins = 0, addRound = false): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/sweep/hint/continue`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ next_sweep_duration_mins: nextDurationMins, add_round: addRound })
	});
	if (!res.ok) {
		const text = await res.text();
		let msg = `Could not continue HINT: ${res.status}`;
		try {
			const body = JSON.parse(text);
			if (body.error) msg = body.error;
		} catch {
			if (text) msg = text;
		}
		throw new Error(msg);
	}
}

/**
 * Stop a running HINT sweep.
 */
export async function stopHINT(): Promise<void> {
	const res = await fetch(`${API_BASE}/lidar/sweep/hint/stop`, { method: 'POST' });
	if (!res.ok) throw new Error(`Could not stop HINT: ${res.status}`);
}
