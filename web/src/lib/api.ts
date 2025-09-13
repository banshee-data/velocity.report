// src/lib/api.ts
// Simple API client for /api/events and /api/radar_stats using fetch
// Add more endpoints as needed

export interface Event {
	Speed: number;
	Magnitude?: number;
	Uptime?: number;
}

export interface RadarStats {
	Classifier: string;
	StartTime: string;
	Count: number;
	P50Speed: number;
	P85Speed: number;
	P98Speed: number;
	MaxSpeed: number;
}

export interface Config {
	units: string;
}

const API_BASE = '/api';

export async function getEvents(units?: string): Promise<Event[]> {
	const url = new URL(`${API_BASE}/events`, window.location.origin);
	if (units) {
		url.searchParams.append('units', units);
	}
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch events: ${res.status}`);
	return res.json();
}

export async function getRadarStats(units?: string, days = 14): Promise<RadarStats[]> {
	const url = new URL(`${API_BASE}/radar_stats`, window.location.origin);
	url.searchParams.append('days', days.toString());
	if (units) {
		url.searchParams.append('units', units);
	}
	const res = await fetch(url);
	if (!res.ok) throw new Error(`Failed to fetch radar stats: ${res.status}`);
	return res.json();
}

export async function getConfig(): Promise<Config> {
	const res = await fetch(`${API_BASE}/config`);
	if (!res.ok) throw new Error(`Failed to fetch config: ${res.status}`);
	return res.json();
}
