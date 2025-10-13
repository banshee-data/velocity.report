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
}

// Histogram shape: server returns a map of bucket label -> count. Keys are strings
// (formatted numbers) and values are counts.
export type Histogram = Record<string, number>;

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
): Promise<{ metrics: RadarStats[]; histogram?: Histogram }> {
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
		max: r.MaxSpeed,
	})) as RadarStats[];

	const histogram = payload && payload.histogram ? (payload.histogram as Histogram) : undefined;
	return { metrics, histogram };
}

export async function getConfig(): Promise<Config> {
	const res = await fetch(`${API_BASE}/config`);
	if (!res.ok) throw new Error(`Failed to fetch config: ${res.status}`);
	return res.json();
}
