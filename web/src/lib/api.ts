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

// Serial Configuration Types and API
export interface SerialConfig {
	id: number;
	name: string;
	port_path: string;
	baud_rate: number;
	data_bits: number;
	stop_bits: number;
	parity: string;
	enabled: boolean;
	description: string;
	sensor_model: string;
	created_at: number;
	updated_at: number;
}

export interface SerialConfigRequest {
	name: string;
	port_path: string;
	baud_rate: number;
	data_bits: number;
	stop_bits: number;
	parity: string;
	enabled: boolean;
	description: string;
	sensor_model: string;
}

export interface SensorModel {
	slug: string;
	display_name: string;
	has_doppler: boolean;
	has_fmcw: boolean;
	has_distance: boolean;
	default_baud_rate: number;
	init_commands: string[];
	description: string;
}

export interface SerialDevice {
	port_path: string;
	friendly_name: string;
	vendor_id?: string;
	product_id?: string;
	last_seen: number;
}

export interface SerialTestRequest {
	port_path: string;
	baud_rate: number;
	data_bits: number;
	stop_bits: number;
	parity: string;
	timeout_seconds: number;
	auto_correct_baud: boolean;
}

export interface SerialCommandResult {
	command: string;
	response: string;
	is_json: boolean;
}

export interface SerialTestResponse {
	success: boolean;
	port_path: string;
	baud_rate: number;
	test_duration_ms: number;
	bytes_received?: number;
	sample_data?: string;
	raw_responses?: SerialCommandResult[];
	error?: string;
	message: string;
	suggestion?: string;
}

// Get all serial configurations
export async function getSerialConfigs(): Promise<SerialConfig[]> {
	const res = await fetch(`${API_BASE}/serial/configs`);
	if (!res.ok) throw new Error(`Failed to fetch serial configs: ${res.status}`);
	return res.json();
}

// Get a single serial configuration
export async function getSerialConfig(id: number): Promise<SerialConfig> {
	const res = await fetch(`${API_BASE}/serial/configs/${id}`);
	if (!res.ok) throw new Error(`Failed to fetch serial config: ${res.status}`);
	return res.json();
}

// Create a new serial configuration
export async function createSerialConfig(config: SerialConfigRequest): Promise<SerialConfig> {
	const res = await fetch(`${API_BASE}/serial/configs`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(config)
	});
	if (!res.ok) {
		const error = await res.text();
		throw new Error(`Failed to create serial config: ${error}`);
	}
	return res.json();
}

// Update an existing serial configuration
export async function updateSerialConfig(
	id: number,
	config: SerialConfigRequest
): Promise<SerialConfig> {
	const res = await fetch(`${API_BASE}/serial/configs/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(config)
	});
	if (!res.ok) {
		const error = await res.text();
		throw new Error(`Failed to update serial config: ${error}`);
	}
	return res.json();
}

// Delete a serial configuration
export async function deleteSerialConfig(id: number): Promise<void> {
	const res = await fetch(`${API_BASE}/serial/configs/${id}`, {
		method: 'DELETE'
	});
	if (!res.ok) {
		const error = await res.text();
		throw new Error(`Failed to delete serial config: ${error}`);
	}
}

// Get all sensor models
export async function getSensorModels(): Promise<SensorModel[]> {
	const res = await fetch(`${API_BASE}/serial/models`);
	if (!res.ok) throw new Error(`Failed to fetch sensor models: ${res.status}`);
	return res.json();
}

// Get available serial devices (excluding already configured ones)
export async function getSerialDevices(): Promise<SerialDevice[]> {
	const res = await fetch(`${API_BASE}/serial/devices`);
	if (!res.ok) throw new Error(`Failed to fetch serial devices: ${res.status}`);
	return res.json();
}

// Test a serial port configuration
export async function testSerialPort(request: SerialTestRequest): Promise<SerialTestResponse> {
	const res = await fetch(`${API_BASE}/serial/test`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(request)
	});
	if (!res.ok) {
		const error = await res.text();
		throw new Error(`Failed to test serial port: ${error}`);
	}
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
