// src/lib/types/lidar.ts
// TypeScript type definitions for LiDAR tracking system

/**
 * Track represents a tracked object in the LiDAR system.
 */
export interface Track {
	/** Unique identifier for the track */
	track_id: string;
	/** Sensor that detected the track */
	sensor_id: string;
	/** Current track state: tentative (unconfirmed), confirmed, or deleted */
	state: 'tentative' | 'confirmed' | 'deleted';
	/** Current position in world frame (meters) */
	position: { x: number; y: number; z: number };
	/** Current velocity (meters/second) */
	velocity: { vx: number; vy: number };
	/** Current speed (meters/second) */
	speed_mps: number;
	/** Current heading (radians, world frame) */
	heading_rad: number;
	/** Classified object type (optional) */
	object_class?: 'pedestrian' | 'car' | 'bird' | 'other';
	/** Confidence in object classification (0-1, optional) */
	object_confidence?: number;
	/** Number of observations for this track */
	observation_count: number;
	/**
	 * Age of track in seconds (computed as last_seen - first_seen on the backend).
	 * This represents the duration the track has existed.
	 */
	age_seconds: number;
	/** Average speed over track lifetime (meters/second) */
	avg_speed_mps: number;
	/** Peak speed observed (meters/second) */
	peak_speed_mps: number;
	/** Average bounding box dimensions (meters) */
	bounding_box: {
		/** Average length (meters) */
		length_avg: number;
		/** Average width (meters) */
		width_avg: number;
		/** Average height (meters) */
		height_avg: number;
	};
	/** ISO 8601 timestamp when track was first seen */
	first_seen: string;
	/** ISO 8601 timestamp when track was last seen */
	last_seen: string;
	/** Historical positions for trail visualization (optional) */
	history?: {
		x: number;
		y: number;
		timestamp: string;
	}[];
}

/**
 * TrackObservation represents a single observation of a tracked object.
 */
export interface TrackObservation {
	/** Track identifier this observation belongs to */
	track_id: string;
	/** ISO 8601 timestamp of this observation */
	timestamp: string;
	/** Position in world frame at this observation (meters) */
	position: { x: number; y: number; z: number };
	/** Velocity at this observation (meters/second) */
	velocity: { vx: number; vy: number };
	/** Speed at this observation (meters/second) */
	speed_mps: number;
	/** Heading at this observation (radians, world frame) */
	heading_rad: number;
	/** Bounding box dimensions at this observation (meters) */
	bounding_box: {
		length: number;
		width: number;
		height: number;
	};
}

/**
 * BackgroundGrid represents the learned background model from the LiDAR sensor.
 * Used for visualizing the static environment structure.
 */
export interface BackgroundGrid {
	/** Sensor identifier */
	sensor_id: string;
	/** ISO 8601 timestamp of grid snapshot */
	timestamp: string;
	/** Number of elevation rings in the sensor */
	rings: number;
	/** Number of azimuth bins in the grid */
	azimuth_bins: number;
	/** Background cell data */
	cells: BackgroundCell[];
}

/**
 * BackgroundCell represents a single cell in the background grid.
 * Contains learned statistics about the static environment.
 */
export interface BackgroundCell {
	/** Cartesian X position (meters) */
	x: number;
	/** Cartesian Y position (meters) */
	y: number;
	/** Variability in range measurements (meters) - stability indicator */
	range_spread_meters: number;
	/** Number of times this cell has been observed */
	times_seen: number;
	// Legacy fields (optional, for backward compatibility)
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

export interface ObservationListResponse {
	observations: TrackObservation[];
	count: number;
	timestamp: string;
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
