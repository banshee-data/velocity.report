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
	source?: string
): Promise<RadarStatsResponse> {
	const url = new URL(`${API_BASE}/radar_stats`, window.location.origin);
	url.searchParams.append('start', start.toString());
	url.searchParams.append('end', end.toString());
	if (group) url.searchParams.append('group', group);
	if (units) url.searchParams.append('units', units);
	if (timezone) url.searchParams.append('timezone', timezone);
	if (source) url.searchParams.append('source', source);
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
	return { metrics, histogram };
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
	timezone: string; // e.g., "US/Pacific"
	units: string; // "mph" or "kph"
	group?: string; // e.g., "1h", "4h"
	source?: string; // "radar_objects" or "radar_data_transits"
	min_speed?: number; // minimum speed filter
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
	cosine_error_angle: number;
	speed_limit: number;
	surveyor: string;
	contact: string;
	address?: string | null;
	latitude?: number | null;
	longitude?: number | null;
	map_angle?: number | null;
	include_map: boolean;
	site_description?: string | null;
	speed_limit_note?: string | null;
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
export interface TransitWorkerState {
	enabled: boolean;
	last_run_at: string;
	last_run_error?: string;
	run_count: number;
	is_healthy: boolean;
}

export interface TransitWorkerUpdateRequest {
	enabled?: boolean;
	trigger?: boolean;
}

export interface TransitWorkerUpdateResponse {
	enabled: boolean;
	last_run_at: string;
	last_run_error?: string;
	run_count: number;
	is_healthy: boolean;
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
 */
export async function getTrackHistory(
	sensorId: string,
	startTime: number,
	endTime: number
): Promise<TrackHistoryResponse> {
	const url = new URL(`${API_BASE}/lidar/tracks/history`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	url.searchParams.append('start_time', startTime.toString());
	url.searchParams.append('end_time', endTime.toString());
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

interface VCTrackRaw {
	track_id: string;
	sensor_id: string;
	state: string;
	x: number;
	y: number;
	vx: number;
	vy: number;
	speed_mps: number;
	object_class?: 'pedestrian' | 'car' | 'bird' | 'other';
	object_confidence?: number;
	observation_count: number;
}

/**
 * Get velocity-coherent tracks (6D clustering algorithm)
 * @param sensorId - Sensor identifier
 */
export async function getVCTracks(sensorId: string): Promise<Track[]> {
	const url = new URL(`${API_BASE}/lidar/tracking/vc/tracks`, window.location.origin);
	url.searchParams.append('sensor_id', sensorId);
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch VC tracks: ${res.status}`);
	const data = await res.json();
	// API returns { tracks: [...], track_count: N }
	const rawTracks: VCTrackRaw[] = data.tracks || [];
	// Convert VC track format to standard Track format
	return rawTracks.map((t) => ({
		track_id: t.track_id,
		sensor_id: t.sensor_id,
		state: t.state as 'tentative' | 'confirmed' | 'deleted',
		position: { x: t.x, y: t.y, z: 0 },
		velocity: { vx: t.vx, vy: t.vy },
		speed_mps: t.speed_mps,
		heading_rad: Math.atan2(t.vy, t.vx),
		object_class: t.object_class,
		object_confidence: t.object_confidence,
		observation_count: t.observation_count,
		age_seconds: 0, // Not available in VC track response
		avg_speed_mps: t.speed_mps,
		peak_speed_mps: t.speed_mps,
		bounding_box: { length_avg: 0, width_avg: 0, height_avg: 0 },
		first_seen: '',
		last_seen: ''
	}));
}
