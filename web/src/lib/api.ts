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

const API_BASE = '/api';

export async function getEvents(): Promise<Event[]> {
	const res = await fetch(`${API_BASE}/events`);
	if (!res.ok) throw new Error(`Failed to fetch events: ${res.status}`);
	return res.json();
}

export async function getRadarStats(): Promise<RadarStats[]> {
	const res = await fetch(`${API_BASE}/radar_stats?days=14`);
	if (!res.ok) throw new Error(`Failed to fetch radar stats: ${res.status}`);
	return res.json();
}
