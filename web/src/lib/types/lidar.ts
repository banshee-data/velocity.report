// src/lib/types/lidar.ts
// TypeScript type definitions for LiDAR tracking system

export interface Track {
	track_id: string;
	sensor_id: string;
	state: 'tentative' | 'confirmed' | 'deleted';
	position: { x: number; y: number; z: number };
	velocity: { vx: number; vy: number };
	speed_mps: number;
	heading_rad: number;
	object_class?: 'pedestrian' | 'car' | 'bird' | 'other';
	object_confidence?: number;
	observation_count: number;
	age_seconds: number;
	avg_speed_mps: number;
	peak_speed_mps: number;
	bounding_box: {
		length_avg: number;
		width_avg: number;
		height_avg: number;
	};
	first_seen: string; // ISO timestamp
	last_seen: string; // ISO timestamp
	history?: {
		x: number;
		y: number;
		timestamp: string;
	}[];
}

export interface TrackObservation {
	track_id: string;
	timestamp: string; // ISO timestamp
	position: { x: number; y: number; z: number };
	velocity: { vx: number; vy: number };
	speed_mps: number;
	heading_rad: number;
	bounding_box: {
		length: number;
		width: number;
		height: number;
	};
}

export interface BackgroundGrid {
	sensor_id: string;
	timestamp: string;
	rings: number;
	azimuth_bins: number;
	cells: BackgroundCell[];
}

export interface BackgroundCell {
	x: number;
	y: number;
	range_spread_meters: number;
	times_seen: number;
	// Legacy fields (optional or removed)
	ring?: number;
	azimuth_deg?: number;
	average_range_meters?: number;
}

export interface TrackListResponse {
	tracks: Track[];
	count: number;
	timestamp: string;
}

export interface TrackHistoryResponse {
	tracks: Track[];
	observations: Record<string, TrackObservation[]>;
}

export interface ClusterResponse {
	cluster_id: number;
	sensor_id: string;
	timestamp: string;
	centroid: { x: number; y: number; z: number };
	bounding_box: {
		length: number;
		width: number;
		height: number;
	};
	points_count: number;
	height_p95: number;
	intensity_mean: number;
}

export interface TrackSummaryResponse {
	by_class: Record<string, TrackClassSummary>;
	by_state: Record<string, TrackStateSummary>;
	total_tracks: number;
	timestamp: string;
}

export interface TrackClassSummary {
	count: number;
	avg_speed_mps: number;
	max_speed_mps: number;
	avg_duration_seconds: number;
}

export interface TrackStateSummary {
	count: number;
	avg_age_seconds: number;
}

// Color scheme for track visualization
export const TRACK_COLORS = {
	pedestrian: '#4CAF50', // Green
	car: '#2196F3', // Blue
	bird: '#FFC107', // Amber
	other: '#9E9E9E', // Grey
	tentative: '#FF9800', // Orange (unconfirmed)
	deleted: '#F44336' // Red (just deleted)
} as const;
