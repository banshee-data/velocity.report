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
	/** PCA-derived oriented bounding box heading (radians) */
	obb_heading_rad: number;
	/**
	 * Source of the current heading estimate (for debug rendering).
	 * 0=PCA (raw), 1=velocity-disambiguated, 2=displacement-disambiguated, 3=locked
	 */
	heading_source?: number;
	/** Bounding box dimensions (meters) */
	bounding_box: {
		/** Average length (meters) */
		length_avg: number;
		/** Average width (meters) */
		width_avg: number;
		/** Average height (meters) */
		height_avg: number;
		/** Per-frame length (meters) */
		length: number;
		/** Per-frame width (meters) */
		width: number;
		/** Per-frame height (meters) */
		height: number;
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

/**
 * Deterministic per-track colour variation within the class palette (task 6.2).
 * Hashes the track_id to shift the base class hue by ±25° so individual
 * tracks of the same class are visually distinguishable on the map.
 */
export function trackColour(trackId: string, objectClass?: string, state?: string): string {
	// State colours are fixed — no per-track variation
	if (state === 'tentative') return TRACK_COLORS.tentative;
	if (state === 'deleted') return TRACK_COLORS.deleted;

	const base =
		objectClass && objectClass in TRACK_COLORS
			? TRACK_COLORS[objectClass as keyof typeof TRACK_COLORS]
			: TRACK_COLORS.other;

	// Simple string hash → deterministic hue offset
	let hash = 0;
	for (let i = 0; i < trackId.length; i++) {
		hash = (hash * 31 + trackId.charCodeAt(i)) | 0;
	}
	const hueShift = (hash % 51) - 25; // −25 to +25 degrees

	// Parse hex → HSL → shift hue → return hex
	const r = parseInt(base.slice(1, 3), 16) / 255;
	const g = parseInt(base.slice(3, 5), 16) / 255;
	const b = parseInt(base.slice(5, 7), 16) / 255;
	const max = Math.max(r, g, b),
		min = Math.min(r, g, b);
	const l = (max + min) / 2;
	let h = 0,
		s = 0;
	if (max !== min) {
		const d = max - min;
		s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
		if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) * 60;
		else if (max === g) h = ((b - r) / d + 2) * 60;
		else h = ((r - g) / d + 4) * 60;
	}
	h = (h + hueShift + 360) % 360;

	// HSL → hex
	const hue2rgb = (p: number, q: number, t: number) => {
		if (t < 0) t += 1;
		if (t > 1) t -= 1;
		if (t < 1 / 6) return p + (q - p) * 6 * t;
		if (t < 1 / 2) return q;
		if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6;
		return p;
	};
	const q2 = l < 0.5 ? l * (1 + s) : l + s - l * s;
	const p2 = 2 * l - q2;
	const rr = Math.round(hue2rgb(p2, q2, h / 360 + 1 / 3) * 255);
	const gg = Math.round(hue2rgb(p2, q2, h / 360) * 255);
	const bb = Math.round(hue2rgb(p2, q2, h / 360 - 1 / 3) * 255);
	return `#${((1 << 24) | (rr << 16) | (gg << 8) | bb).toString(16).slice(1)}`;
}

/** Classification labels for track identity (single-select: what is the object?) */
export type DetectionLabel = 'car' | 'ped' | 'noise' | 'impossible';

/** Quality flags for track attributes (multi-select: properties of the track) */
export type QualityLabel =
	| 'good'
	| 'noisy'
	| 'jitter_velocity'
	| 'merge'
	| 'split'
	| 'truncated'
	| 'disconnected';

/** Scene represents a PCAP-based evaluation environment */
export interface LidarScene {
	scene_id: string;
	sensor_id: string;
	pcap_file: string;
	pcap_start_secs?: number;
	pcap_duration_secs?: number;
	description?: string;
	reference_run_id?: string;
	optimal_params_json?: string;
	created_at_ns: number;
	updated_at_ns?: number;
}

/** Analysis run with track snapshots */
export interface AnalysisRun {
	run_id: string;
	created_at: string;
	source_type: string;
	source_path?: string;
	sensor_id: string;
	params_json?: Record<string, unknown>;
	duration_secs?: number;
	total_frames?: number;
	total_clusters?: number;
	total_tracks: number;
	confirmed_tracks: number;
	processing_time_ms?: number;
	status: string;
	error_message?: string;
	parent_run_id?: string;
	notes?: string;
}

/** Run track with labelling fields */
export interface RunTrack {
	run_id: string;
	track_id: string;
	sensor_id: string;
	track_state: string;
	start_unix_nanos: number;
	end_unix_nanos: number;
	observation_count: number;
	avg_speed_mps: number;
	peak_speed_mps: number;
	p50_speed_mps: number;
	p85_speed_mps: number;
	p95_speed_mps: number;
	bounding_box_length_avg: number;
	bounding_box_width_avg: number;
	bounding_box_height_avg: number;
	object_class?: string;
	object_confidence?: number;
	user_label?: string;
	quality_label?: string;
	label_confidence?: number;
	labeler_id?: string;
	labeled_at?: number;
	is_split_candidate?: boolean;
	is_merge_candidate?: boolean;
	linked_track_ids?: string[];
}

/** Labelling progress statistics */
export interface LabellingProgress {
	total: number;
	labelled: number;
	progress_pct: number;
	by_class: Record<string, number>;
}

/** Missed region: area where an object should have been tracked but wasn't */
export interface MissedRegion {
	region_id: string;
	run_id: string;
	center_x: number;
	center_y: number;
	radius_m: number;
	time_start_ns: number;
	time_end_ns: number;
	expected_label: string;
	labeler_id?: string;
	labeled_at?: number;
	notes?: string;
}

/** Lightweight sweep record for list views (no large JSON blobs). */
export interface SweepSummary {
	id: number;
	sweep_id: string;
	sensor_id: string;
	mode: string;
	status: string;
	error?: string;
	started_at: string;
	completed_at?: string;
}

/** Full sweep record including results, recommendation, charts, and round results. */
export interface SweepRecord {
	id: number;
	sweep_id: string;
	sensor_id: string;
	mode: string;
	status: string;
	request?: Record<string, unknown>;
	results?: Record<string, unknown>[];
	charts?: Record<string, unknown>[];
	recommendation?: Record<string, unknown>;
	round_results?: Record<string, unknown>[];
	error?: string;
	started_at: string;
	completed_at?: string;
	score_components?: ScoreComponents | string;
}

/** Score components for sweep scoring explanation */
export interface ScoreComponents {
	detection_rate: number;
	fragmentation: number;
	false_positives: number;
	velocity_coverage: number;
	quality_premium: number;
	truncation_rate: number;
	velocity_noise_rate: number;
	stopped_recovery: number;
	composite_score: number;
	weights_used: Record<string, number>;
	top_contributors?: string[];
	label_coverage_confidence?: number;
}
