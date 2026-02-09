// src/lib/api.ts
// Simple API client for /api/events and /api/radar_stats using fetch
// Add more endpoints as needed

export interface Event {
	Speed: number;
	Magnitude?: number;
	Uptime?: number;
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

// Raw shape returned from the server for a single metric row
type RawRadarStats = {
	Classifier: string;
	StartTime: string;
	Count: number;
	P50Speed: number;
	P85Speed: number;
	P98Speed: number;
	MaxSpeed: number;
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

export interface Config {
	units: string;
	timezone: string;
}

const API_BASE = '/api';

export async function getEvents(units?: string, timezone?: string): Promise<Event[]> {
	const url = new URL(`${API_BASE}/events`, window.location.origin);
	if (units) {
		url.searchParams.append('units', units);
	}
	if (timezone) {
		url.searchParams.append('timezone', timezone);
	}
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch events: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to fetch radar stats: ${res.status}`);
	// Expect the server to return the new root object: { metrics: [...], histogram: {...} }
	const payload = await res.json();
	const rows = Array.isArray(payload.metrics) ? (payload.metrics as RawRadarStats[]) : [];

	const metrics = rows.map((r) => ({
		classifier: r.Classifier,
		date: new Date(r.StartTime),
		count: r.Count,
		p50: r.P50Speed,
		p85: r.P85Speed,
		p98: r.P98Speed,
		max: r.MaxSpeed
	})) as RadarStats[];

	const histogram = payload && payload.histogram ? (payload.histogram as Histogram) : undefined;
	const cosineCorrection =
		payload && payload.cosine_correction ? payload.cosine_correction : undefined;
	return { metrics, histogram, cosineCorrection };
}

export async function getConfig(): Promise<Config> {
	const res = await fetch(`${API_BASE}/config`);
	if (!res.ok) throw new Error(`Failed to fetch config: ${res.status}`);
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
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Failed to generate report: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to fetch site config periods: ${res.status}`);
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
		throw new Error(errorData.error || `Failed to save site config period: ${res.status}`);
	}
	return res.json();
}

export async function getTimeline(siteId: number): Promise<TimelineResponse> {
	const url = new URL(`${API_BASE}/timeline`, window.location.origin);
	url.searchParams.append('site_id', siteId.toString());
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch timeline: ${res.status}`);
	return res.json();
}

export async function downloadReport(
	reportId: number,
	fileType: 'pdf' | 'zip' = 'pdf'
): Promise<DownloadResult> {
	const url = new URL(`${API_BASE}/reports/${reportId}/download`, window.location.origin);
	url.searchParams.append('file_type', fileType);
	const res = await fetch(url);
	if (!res.ok) {
		throw new Error(`Failed to download report: ${res.status}`);
	}

	// Extract filename from Content-Disposition header
	const contentDisposition = res.headers.get('Content-Disposition');
	let filename = `report.${fileType}`; // fallback
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
	if (!res.ok) throw new Error(`Failed to fetch reports: ${res.status}`);
	return res.json();
}

export async function getReportsForSite(siteId: number): Promise<SiteReport[]> {
	const res = await fetch(`${API_BASE}/reports/site/${siteId}`);
	if (!res.ok) throw new Error(`Failed to fetch site reports: ${res.status}`);
	return res.json();
}

export async function getReport(reportId: number): Promise<SiteReport> {
	const res = await fetch(`${API_BASE}/reports/${reportId}`);
	if (!res.ok) throw new Error(`Failed to fetch report: ${res.status}`);
	return res.json();
}

export async function deleteReport(reportId: number): Promise<void> {
	const res = await fetch(`${API_BASE}/reports/${reportId}`, {
		method: 'DELETE'
	});
	if (!res.ok) throw new Error(`Failed to delete report: ${res.status}`);
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
	bbox_ne_lat?: number | null;
	bbox_ne_lng?: number | null;
	bbox_sw_lat?: number | null;
	bbox_sw_lng?: number | null;
	/**
	 * Base64-encoded SVG image data as a string.
	 * This must be a base64 string (matching the Go BLOB field), not raw SVG XML/text.
	 */
	map_svg_data?: string | null;
	created_at: string;
	updated_at: string;
}

export async function getSites(): Promise<Site[]> {
	const res = await fetch(`${API_BASE}/sites`);
	if (!res.ok) throw new Error(`Failed to fetch sites: ${res.status}`);
	return res.json();
}

export async function getSite(id: number): Promise<Site> {
	const res = await fetch(`${API_BASE}/sites/${id}`);
	if (!res.ok) throw new Error(`Failed to fetch site: ${res.status}`);
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
		throw new Error(errorData.error || `Failed to create site: ${res.status}`);
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
		throw new Error(errorData.error || `Failed to update site: ${res.status}`);
	}
	return res.json();
}

export async function deleteSite(id: number): Promise<void> {
	const res = await fetch(`${API_BASE}/sites/${id}`, {
		method: 'DELETE'
	});
	if (!res.ok) {
		const errorData = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
		throw new Error(errorData.error || `Failed to delete site: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to fetch transit worker state: ${res.status}`);
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
		throw new Error(errorData.error || `Failed to update transit worker: ${res.status}`);
	}
	return res.json();
}

// LiDAR Track API

import type {
	BackgroundGrid,
	ObservationListResponse,
	Track,
	TrackHistoryResponse,
	TrackListResponse,
	TrackObservation,
	TrackSummaryResponse,
	LidarScene,
	AnalysisRun,
	RunTrack,
	LabellingProgress,
	LidarTransit,
	LidarTransitSummary
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
	if (!res.ok) throw new Error(`Failed to fetch active tracks: ${res.status}`);
	return res.json();
}

/**
 * Get details for a specific track
 * @param trackId - Track identifier
 */
export async function getTrackById(trackId: string): Promise<Track> {
	const res = await fetch(`${API_BASE}/lidar/tracks/${trackId}`);
	if (!res.ok) throw new Error(`Failed to fetch track: ${res.status}`);
	return res.json();
}

/**
 * Get trajectory observations for a track
 * @param trackId - Track identifier
 */
export async function getTrackObservations(trackId: string): Promise<TrackObservation[]> {
	const res = await fetch(`${API_BASE}/lidar/tracks/${trackId}/observations`);
	if (!res.ok) throw new Error(`Failed to fetch track observations: ${res.status}`);
	return res.json();
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
	if (!res.ok) throw new Error(`Failed to fetch track history: ${res.status}`);
	return res.json();
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
	if (!res.ok) throw new Error(`Failed to fetch track observations range: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to fetch track summary: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to fetch background grid: ${res.status}`);
	return res.json();
}

// LiDAR Scene and Run Labelling API (Phase 3: track labelling UI)
// Uses API_BASE for consistency with other LiDAR endpoints.

export async function getLidarScenes(sensorId?: string): Promise<LidarScene[]> {
	const params = new URLSearchParams();
	if (sensorId) params.set('sensor_id', sensorId);
	const url = `${API_BASE}/lidar/scenes${params.toString() ? '?' + params : ''}`;
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch scenes: ${res.status}`);
	const data = await res.json();
	return data.scenes || [];
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
	if (!res.ok) throw new Error(`Failed to fetch runs: ${res.status}`);
	const data = await res.json();
	return data.runs || [];
}

export async function getRunTracks(runId: string): Promise<RunTrack[]> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/tracks`);
	if (!res.ok) throw new Error(`Failed to fetch tracks: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to update label: ${res.status}`);
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
	if (!res.ok) throw new Error(`Failed to update flags: ${res.status}`);
}

export async function getLabellingProgress(runId: string): Promise<LabellingProgress> {
	const res = await fetch(`${API_BASE}/lidar/runs/${runId}/labelling-progress`);
	if (!res.ok) throw new Error(`Failed to fetch progress: ${res.status}`);
	return res.json();
}

// LiDAR Transit API (Phase 6: polished transit data for dashboards and reports)

/**
 * Get list of LiDAR transits with optional filters
 * @param params - Query parameters for filtering transits
 */
export async function getLidarTransits(params: {
	sensor_id?: string;
	start?: number;
	end?: number;
	min_speed?: number;
	max_speed?: number;
	limit?: number;
}): Promise<LidarTransit[]> {
	const url = new URL(`${API_BASE}/lidar/transits`, window.location.origin);

	if (params.sensor_id) {
		url.searchParams.append('sensor_id', params.sensor_id);
	}
	if (params.start !== undefined) {
		url.searchParams.append('start', params.start.toString());
	}
	if (params.end !== undefined) {
		url.searchParams.append('end', params.end.toString());
	}
	if (params.min_speed !== undefined) {
		url.searchParams.append('min_speed', params.min_speed.toString());
	}
	if (params.max_speed !== undefined) {
		url.searchParams.append('max_speed', params.max_speed.toString());
	}
	if (params.limit !== undefined) {
		url.searchParams.append('limit', params.limit.toString());
	}

	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch LiDAR transits: ${res.status}`);
	return res.json();
}

/**
 * Get aggregate statistics for LiDAR transits in a time range
 * @param params - Query parameters for filtering summary
 */
export async function getLidarTransitSummary(params: {
	sensor_id?: string;
	start?: number;
	end?: number;
}): Promise<LidarTransitSummary> {
	const url = new URL(`${API_BASE}/lidar/transits/summary`, window.location.origin);

	if (params.sensor_id) {
		url.searchParams.append('sensor_id', params.sensor_id);
	}
	if (params.start !== undefined) {
		url.searchParams.append('start', params.start.toString());
	}
	if (params.end !== undefined) {
		url.searchParams.append('end', params.end.toString());
	}

	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch LiDAR transit summary: ${res.status}`);
	return res.json();
}
